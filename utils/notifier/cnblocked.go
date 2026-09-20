package notifier

import (
	"log"
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
		Message: formatClientsMessage(involved, messageSender.SupportsHTML()),
		Emoji:   emoji,
	}); err != nil {
		log.Printf("Failed to send %s notification: %v", event, err)
	}
}
