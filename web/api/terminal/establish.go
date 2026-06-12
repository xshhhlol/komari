package terminal

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/web/api"
)

func EstablishConnection(c *gin.Context) {
	session_id := c.Query("id")

	TerminalSessionsMutex.Lock()
	session, exists := TerminalSessions[session_id]
	TerminalSessionsMutex.Unlock()

	if !exists || session == nil || session.Browser == nil {
		c.JSON(404, gin.H{"status": "error", "error": "Session not found"})
		return
	}
	// Upgrade the connection to WebSocket
	if !api.IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Require WebSocket upgrade"})
		return
	}
	conn, err := api.UpgradeWebSocket(c, api.WithBufferSize(32*1024, 32*1024))
	if err != nil {
		TerminalSessionsMutex.Lock()
		if session.Browser != nil {
			session.Browser.Close()
		}
		delete(TerminalSessions, session_id)
		TerminalSessionsMutex.Unlock()
		return
	}

	// 在锁内绑定 Agent，并二次确认会话仍存在（升级期间可能已被等待超时清理），
	// 与 RequestTerminal 中的 30s 超时定时器消除竞态。
	TerminalSessionsMutex.Lock()
	if cur, ok := TerminalSessions[session_id]; !ok || cur != session {
		TerminalSessionsMutex.Unlock()
		conn.Close()
		if session.Browser != nil {
			session.Browser.Close()
		}
		return
	}
	session.Agent = conn
	TerminalSessionsMutex.Unlock()

	conn.SetCloseHandler(func(code int, text string) error {
		// 收到 Agent 关闭帧时同步关闭 Browser；会话的最终清理统一由 ForwardTerminal 收敛。
		if session.Browser != nil {
			session.Browser.Close()
		}
		return nil
	})
	go ForwardTerminal(session_id)
}
