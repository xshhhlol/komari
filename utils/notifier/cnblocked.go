package notifier

import (
	"html"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/models"
	messageevent "github.com/komari-monitor/komari/database/models/messageEvent"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/utils/messageSender"
)

// cnBlockStableDuration 为状态确认时长：新状态需要连续维持这么久才会发出通知，
// 避免国内线路瞬时抖动导致的"被墙 / 恢复"误报。
const cnBlockStableDuration = 3 * time.Minute

// cnBlockNotifyState 记录单个节点的被墙通知状态。
type cnBlockNotifyState struct {
	confirmed      tasks.CnBlockState // 已确认并已通知过的状态
	candidate      tasks.CnBlockState // 待确认的新状态
	candidateSince time.Time          // 待确认状态的首次观测时间
}

var (
	cnBlockMu     sync.Mutex
	cnBlockStates = map[string]*cnBlockNotifyState{}
)

// CheckCnBlockedScheduledWork 供定时任务调用。
func CheckCnBlockedScheduledWork() {
	CheckCnBlocked()
}

// CheckCnBlocked 检查各节点的"被墙"状态变化，并在确认发生转换后发送通知。
//
// 仅在状态确定时才推进：ping 数据缺失（例如节点离线、agent 未上报）只会得到
// CnBlockUnknown，此时保持既有状态不变，不会误报"已恢复"。
// 首次观测到某节点时只记录基线，不发通知，避免服务重启后集中补发。
func CheckCnBlocked() {
	cfg, err := config.GetMany(map[string]any{
		config.NotificationEnabledKey:          false,
		config.CnBlockedNotificationEnabledKey: true,
	})
	if err != nil {
		return
	}
	enabled, _ := cfg[config.NotificationEnabledKey].(bool)
	cnEnabled, _ := cfg[config.CnBlockedNotificationEnabledKey].(bool)
	if !enabled || !cnEnabled {
		// 关闭期间不积累状态，重新开启时以当时的状态为基线。
		resetCnBlockStates()
		return
	}

	states, ok := tasks.ComputeCnBlockStates()
	if !ok {
		// 没有配置国内参照（block_check）任务，或查询失败：不做判定。
		resetCnBlockStates()
		return
	}

	blockedUUIDs, recoveredUUIDs := advanceCnBlockStates(states, time.Now())

	if len(blockedUUIDs) == 0 && len(recoveredUUIDs) == 0 {
		return
	}

	all, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return
	}
	byUUID := make(map[string]models.Client, len(all))
	for _, c := range all {
		byUUID[c.UUID] = c
	}

	sendCnBlockEvent(messageevent.CnBlocked, "🚧", blockedUUIDs, byUUID)
	sendCnBlockEvent(messageevent.CnUnblocked, "✅", recoveredUUIDs, byUUID)
}

// advanceCnBlockStates 推进状态机，返回本轮确认发生转换的节点：
// blocked 为新判定被墙的节点，recovered 为从被墙恢复的节点。
func advanceCnBlockStates(states map[string]tasks.CnBlockState, now time.Time) (blocked, recovered []string) {
	cnBlockMu.Lock()
	defer cnBlockMu.Unlock()

	for uuid, observed := range states {
		if observed == tasks.CnBlockUnknown {
			// 数据不足：撤销待确认的状态，保持已确认状态不变。
			if st, exists := cnBlockStates[uuid]; exists {
				st.candidate = tasks.CnBlockUnknown
			}
			continue
		}

		st, exists := cnBlockStates[uuid]
		if !exists {
			// 首次观测：只记录基线，不通知。
			cnBlockStates[uuid] = &cnBlockNotifyState{confirmed: observed}
			continue
		}
		if st.confirmed == tasks.CnBlockUnknown {
			st.confirmed = observed
			st.candidate = tasks.CnBlockUnknown
			continue
		}
		if observed == st.confirmed {
			st.candidate = tasks.CnBlockUnknown
			continue
		}
		if st.candidate != observed {
			st.candidate = observed
			st.candidateSince = now
			continue
		}
		if now.Sub(st.candidateSince) < cnBlockStableDuration {
			continue
		}
		st.confirmed = observed
		st.candidate = tasks.CnBlockUnknown
		if observed == tasks.CnBlockBlocked {
			blocked = append(blocked, uuid)
		} else {
			recovered = append(recovered, uuid)
		}
	}
	// 清理已不在判定范围内的节点（任务被删除、节点被移除等）。
	for uuid := range cnBlockStates {
		if _, exists := states[uuid]; !exists {
			delete(cnBlockStates, uuid)
		}
	}
	return blocked, recovered
}

// sendCnBlockEvent 组装并发送一条聚合通知（同一轮内的多个节点合并为一条消息）。
//
// IP 是这类告警里最需要被读到、被拿去用的信息，所以单独成行突出显示；
// 在按 HTML 解析的渠道（Telegram）上用 <code> 包裹，渲染为等宽块，
// 点一下即可复制。其它渠道退回纯文本，不会看到裸标签。
func sendCnBlockEvent(event, emoji string, uuids []string, byUUID map[string]models.Client) {
	if len(uuids) == 0 {
		return
	}
	asHTML := messageSender.SupportsHTML()
	involved := make([]models.Client, 0, len(uuids))
	lines := make([]string, 0, len(uuids)*3)
	for _, uuid := range uuids {
		client, ok := byUUID[uuid]
		if !ok {
			continue
		}
		involved = append(involved, client)
		name := client.Name
		if strings.TrimSpace(name) == "" {
			name = client.UUID
		}
		lines = append(lines, "• "+plainText(name, asHTML))
		if client.IPv4 != "" {
			lines = append(lines, "  IPv4 "+copyableText(client.IPv4, asHTML))
		}
		if client.IPv6 != "" {
			lines = append(lines, "  IPv6 "+copyableText(client.IPv6, asHTML))
		}
	}
	if len(involved) == 0 {
		return
	}
	if err := messageSender.SendEvent(models.EventMessage{
		Event:   event,
		Clients: involved,
		Time:    time.Now(),
		Message: strings.Join(lines, "\n"),
		Emoji:   emoji,
	}); err != nil {
		log.Printf("Failed to send %s notification: %v", event, err)
	}
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

func resetCnBlockStates() {
	cnBlockMu.Lock()
	defer cnBlockMu.Unlock()
	if len(cnBlockStates) > 0 {
		cnBlockStates = map[string]*cnBlockNotifyState{}
	}
}
