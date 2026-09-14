package notifier

import (
	"html"
	"log"
	"strings"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/models"
	messageevent "github.com/komari-monitor/komari/database/models/messageEvent"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/utils/messageSender"
	agent_runtime "github.com/komari-monitor/komari/web/agent"
)

// CheckCnBlockedScheduledWork 供定时任务调用。
func CheckCnBlockedScheduledWork() {
	CheckCnBlocked()
}

// CheckCnBlocked 推进各节点的"被墙"状态，并在页面"被墙"标记（在线且被墙）每次变化时发送通知：
// 新被墙、恢复、被墙期间掉线。在线状态与页面同源。
//
// 状态推进不受通知开关影响：节点列表接口读取的也是这份状态。
// 通知关闭期间发生的变化直接吸收，重新开启后不会补发。
func CheckCnBlocked() {
	onlineUUIDs := agent_runtime.GetAllOnlineUUIDs()
	online := make(map[string]bool, len(onlineUUIDs))
	for _, uuid := range onlineUUIDs {
		online[uuid] = true
	}
	changes := tasks.RefreshCnBlockStates(online)
	if changes.Empty() {
		return
	}

	cfg, err := config.GetMany(map[string]any{
		config.NotificationEnabledKey:          false,
		config.CnBlockedNotificationEnabledKey: true,
	})
	if err != nil {
		log.Printf("Failed to load cn-blocked notification settings: %v", err)
		return
	}
	enabled, _ := cfg[config.NotificationEnabledKey].(bool)
	cnEnabled, _ := cfg[config.CnBlockedNotificationEnabledKey].(bool)
	if !enabled || !cnEnabled {
		return
	}

	all, err := clients.GetAllClientBasicInfo()
	if err != nil {
		log.Printf("Failed to load clients for cn-blocked notification: %v", err)
		return
	}
	byUUID := make(map[string]models.Client, len(all))
	for _, c := range all {
		byUUID[c.UUID] = c
	}

	sendCnBlockEvent(messageevent.CnBlocked, "🚧", changes.Blocked, byUUID)
	sendCnBlockEvent(messageevent.CnUnblocked, "✅", changes.Recovered, byUUID)
	sendCnBlockEvent(messageevent.CnBlockedOffline, "📴", changes.Offline, byUUID)
}

// sendCnBlockEvent 组装并发送一条聚合通知（同一轮内的多个节点合并为一条消息）。
func sendCnBlockEvent(event, emoji string, uuids []string, byUUID map[string]models.Client) {
	involved := make([]models.Client, 0, len(uuids))
	for _, uuid := range uuids {
		if client, ok := byUUID[uuid]; ok {
			involved = append(involved, client)
		}
	}
	if len(involved) == 0 {
		return
	}
	if err := messageSender.SendEvent(models.EventMessage{
		Event:   event,
		Clients: involved,
		Time:    time.Now(),
		Message: formatCnBlockMessage(involved, messageSender.SupportsHTML()),
		Emoji:   emoji,
	}); err != nil {
		log.Printf("Failed to send %s notification: %v", event, err)
	}
}

// formatCnBlockMessage 排版通知正文：每个节点一行名称，其后每个 IP 各占一行。
//
// IP 是这类告警里最需要被读到、被拿去用的信息，所以单独成行；在按 HTML 解析的
// 渠道（Telegram）上用 <code> 包裹，渲染为等宽块，点一下即可复制。其它渠道退回纯文本。
// IP 行不缩进：Telegram 会原样保留行首空格，看上去像凭空多了个空格。
func formatCnBlockMessage(involved []models.Client, asHTML bool) string {
	lines := make([]string, 0, len(involved)*3)
	for _, client := range involved {
		name := client.Name
		if strings.TrimSpace(name) == "" {
			name = client.UUID
		}
		lines = append(lines, "• "+plainText(name, asHTML))
		if client.IPv4 != "" {
			lines = append(lines, "IPv4 "+copyableText(client.IPv4, asHTML))
		}
		if client.IPv6 != "" {
			lines = append(lines, "IPv6 "+copyableText(client.IPv6, asHTML))
		}
	}
	return strings.Join(lines, "\n")
}

// copyableText 把一段文本渲染为可点击复制的等宽块（HTML 渠道），
// 非 HTML 渠道原样返回。
func copyableText(text string, asHTML bool) string {
	if !asHTML {
		return text
	}
	return "<code>" + html.EscapeString(text) + "</code>"
}

// plainText 在 HTML 渠道下转义文本中的 < > &，避免节点名里的特殊字符
// 破坏整条消息的解析。
func plainText(text string, asHTML bool) string {
	if !asHTML {
		return text
	}
	return html.EscapeString(text)
}
