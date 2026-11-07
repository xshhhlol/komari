package notifier

import (
	"log"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	messageevent "github.com/komari-monitor/komari/database/models/messageEvent"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/utils/messageSender"
	agent_runtime "github.com/komari-monitor/komari/web/agent"
	"gorm.io/gorm"
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
// 开关有两级：全局总闸，以及后台「节点通知」里逐节点的被墙通知开关（关掉的节点不进通知）。
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
	muted, err := cnBlockedMutedClients(dbcore.GetDBInstance())
	if err != nil {
		// 读不到逐节点开关时照常全发：宁可多一条，也别把真被墙漏掉。
		log.Printf("Failed to load per-node cn-blocked notification switches: %v", err)
	}
	byUUID := make(map[string]models.Client, len(all))
	for _, c := range all {
		if !muted[c.UUID] {
			byUUID[c.UUID] = c
		}
	}

	sendCnBlockEvent(messageevent.CnBlocked, "🚧", changes.Blocked, byUUID)
	sendCnBlockEvent(messageevent.CnUnblocked, "✅", changes.Recovered, byUUID)
	sendCnBlockEvent(messageevent.CnBlockedOffline, "📴", changes.Offline, byUUID)
}

// cnBlockedMutedClients 返回关闭了被墙通知的节点集合。没有配置行的节点按默认（照常通知）处理，不在其中。
func cnBlockedMutedClients(db *gorm.DB) (map[string]bool, error) {
	var uuids []string
	if err := db.Model(&models.OfflineNotification{}).
		Where("cn_blocked_muted = ?", true).
		Pluck("client", &uuids).Error; err != nil {
		return nil, err
	}
	muted := make(map[string]bool, len(uuids))
	for _, uuid := range uuids {
		muted[uuid] = true
	}
	return muted, nil
}

// sendCnBlockEvent 组装并发送一条聚合通知（同一轮内的多个节点合并为一条消息）。
// byUUID 里只有该通知的节点；不在其中的（已删除、或关了被墙通知）直接跳过。
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
