package terminal

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/komari-monitor/komari/database/auditlog"
)

const (
	// 心跳与超时参数。pingPeriod 必须显著小于 pongWait：
	// 浏览器和 gorilla 客户端都会自动回应 Ping，因此只要对端存活，
	// 读截止时间就会被持续续期，活动连接永远不会被误断；
	// 只有真正失联的连接才会在 pongWait 之后被清理。
	pongWait   = 100 * time.Second
	pingPeriod = 30 * time.Second
	writeWait  = 30 * time.Second
)

// tConn 封装一条 websocket 连接并串行化所有写操作。
// gorilla/websocket 不允许并发写，无论是转发数据还是心跳 Ping，
// 都必须经由这里加锁写入，避免并发写导致的 panic / 数据错乱。
type tConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *tConn) write(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(messageType, data)
}

func (c *tConn) ping() error {
	return c.write(websocket.PingMessage, nil)
}

// configureRead 设置读截止时间，并在收到 Pong 时续期，实现失联探测。
func configureRead(c *websocket.Conn) {
	_ = c.SetReadDeadline(time.Now().Add(pongWait))
	c.SetPongHandler(func(string) error {
		return c.SetReadDeadline(time.Now().Add(pongWait))
	})
}

func recoverPump(name, id string) {
	if r := recover(); r != nil {
		log.Printf("terminal forward panic (%s, id=%s): %v", name, id, r)
	}
}

func ForwardTerminal(id string) {
	TerminalSessionsMutex.Lock()
	session, exists := TerminalSessions[id]
	TerminalSessionsMutex.Unlock()

	if !exists || session == nil || session.Agent == nil || session.Browser == nil {
		// 会话已被清理（例如等待 Agent 超时），关闭可能残留的连接以免泄漏。
		if session != nil {
			if session.Agent != nil {
				session.Agent.Close()
			}
			if session.Browser != nil {
				session.Browser.Close()
			}
		}
		return
	}

	browser := &tConn{conn: session.Browser}
	agent := &tConn{conn: session.Agent}

	auditlog.Log(session.RequesterIp, session.UserUUID, "established, terminal id:"+id, "terminal")
	establishedTime := time.Now()

	var closeOnce sync.Once
	closeAll := func() {
		closeOnce.Do(func() {
			browser.conn.Close()
			agent.conn.Close()
		})
	}

	configureRead(browser.conn)
	configureRead(agent.conn)

	var wg sync.WaitGroup
	wg.Add(2)

	// 浏览器 -> 被控端
	go func() {
		defer wg.Done()
		defer closeAll()
		defer recoverPump("browser->agent", id)
		for {
			messageType, data, err := browser.conn.ReadMessage()
			if err != nil {
				return
			}
			_ = browser.conn.SetReadDeadline(time.Now().Add(pongWait))
			// 兼容既有约定：以 '{' 开头的文本帧视为 JSON 控制消息（如 resize），
			// 原样按文本帧转发；其余一律按二进制透传。
			// 注意 len(data) > 0 守卫，避免空文本帧触发越界 panic。
			target := websocket.BinaryMessage
			if messageType == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
				target = websocket.TextMessage
			}
			if err := agent.write(target, data); err != nil {
				return
			}
		}
	}()

	// 被控端 -> 浏览器
	go func() {
		defer wg.Done()
		defer closeAll()
		defer recoverPump("agent->browser", id)
		for {
			_, data, err := agent.conn.ReadMessage()
			if err != nil {
				return
			}
			_ = agent.conn.SetReadDeadline(time.Now().Add(pongWait))
			if err := browser.write(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}()

	// 心跳：周期性向两端发送 Ping，既维持链路（防止反向代理 / NAT
	// 因空闲超时主动断开），又能探测掉线。任一端写失败即收敛关闭。
	stopPing := make(chan struct{})
	go func() {
		defer recoverPump("ping", id)
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				if err := browser.ping(); err != nil {
					closeAll()
					return
				}
				if err := agent.ping(); err != nil {
					closeAll()
					return
				}
			}
		}
	}()

	wg.Wait()
	close(stopPing)
	closeAll()

	auditlog.Log(session.RequesterIp, session.UserUUID,
		"disconnected, terminal id:"+id+", duration:"+time.Since(establishedTime).String(), "terminal")
	TerminalSessionsMutex.Lock()
	delete(TerminalSessions, id)
	TerminalSessionsMutex.Unlock()
}
