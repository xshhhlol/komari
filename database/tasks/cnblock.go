package tasks

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

// CnBlockState 表示一个节点的"被墙"判定结果。
type CnBlockState int

const (
	// CnBlockUnknown 暂无定论：刚加入、刚上线、记录不足，或结果刚变化还没满 cnBlockStreak 轮。
	CnBlockUnknown CnBlockState = iota
	// CnBlockNormal 至少有一个国内参照目标连续 cnBlockStreak 轮能 ping 通，判为未被墙。
	CnBlockNormal
	// CnBlockBlocked 所有国内参照目标都连续 cnBlockStreak 轮超时，判为被墙。
	CnBlockBlocked
)

// cnBlockStreak 为去抖轮数：单个参照目标需连续这么多轮 ping 结果一致，才算稳定超时 / 稳定可达，
// 否则沿用上一次确认的判定。用于过滤"某一轮所有目标同时超时、下一轮又恢复"这类抖动。
const cnBlockStreak = 2

// CnBlockChanges 为一轮刷新中页面"被墙"标记（在线且被墙）发生变化的节点。
type CnBlockChanges struct {
	Blocked   []string // 在线节点新判定为被墙（含掉线后重新上线仍被墙）
	Recovered []string // 被墙的在线节点恢复正常
	Offline   []string // 被墙节点掉线
}

// Empty 报告本轮是否没有任何变化。
func (c CnBlockChanges) Empty() bool {
	return len(c.Blocked) == 0 && len(c.Recovered) == 0 && len(c.Offline) == 0
}

// cnBlockNode 为单个节点跟踪中的状态。
type cnBlockNode struct {
	state  CnBlockState // 已确认的判定；本轮无定论时沿用
	online bool         // 上一轮是否在线
	// staleBefore 为观测到掉线时该节点最新一条 ping 记录的时间（agent 时钟）。
	// 重新上线后只认比它新的记录，免得拿掉线前的旧结果（例如换 IP 之前的超时）判定。
	staleBefore time.Time
}

type cnBlockKey struct {
	client string
	task   uint
}

var (
	// cnBlockRefreshMu 串行化整轮刷新，避免较早的结果覆盖较新的。
	cnBlockRefreshMu sync.Mutex
	cnBlockMu        sync.Mutex
	// cnBlockNodes 为各节点跟踪中的状态，页面标注与通知都以它为准。
	// 每轮整体替换、不原地修改；nil 表示服务启动后尚未完成首轮刷新。
	cnBlockNodes map[string]cnBlockNode
)

// RefreshCnBlockStates 按近期 ping 记录和在线状态推进各节点的"被墙"状态，返回本轮的变化。
// online 为当前在线的节点，应与页面使用同一来源。由定时任务每分钟调用；节点列表接口
// 读取的也是这里的状态，所以页面上被墙数量的增减与通知一一对应。
//
// 查询失败时保持现有状态不变。
func RefreshCnBlockStates(online map[string]bool) CnBlockChanges {
	cnBlockRefreshMu.Lock()
	defer cnBlockRefreshMu.Unlock()

	blockTasks, recs, err := loadCnBlockRecords(time.Now())
	if err != nil {
		log.Printf("Failed to refresh cn-blocked states: %v", err)
		return CnBlockChanges{}
	}

	cnBlockMu.Lock()
	defer cnBlockMu.Unlock()
	changes, next := advanceCnBlockStates(cnBlockNodes, blockTasks, recs, online)
	cnBlockNodes = next
	return changes
}

// ComputeCnBlockedMap 返回节点 UUID -> 是否被墙，供节点列表接口标注（前端再按在线状态过滤）。
// 读取 RefreshCnBlockStates 跟踪的状态，与通知保持一致；服务刚启动、首轮刷新尚未完成时，
// 直接按当前 ping 记录判定。
func ComputeCnBlockedMap() map[string]bool {
	cnBlockMu.Lock()
	nodes := cnBlockNodes
	cnBlockMu.Unlock()

	result := map[string]bool{}
	if nodes != nil {
		for uuid, n := range nodes {
			if n.state == CnBlockBlocked {
				result[uuid] = true
			}
		}
		return result
	}

	blockTasks, recs, err := loadCnBlockRecords(time.Now())
	if err != nil {
		return result
	}
	for uuid, state := range judgeCnBlockStates(blockTasks, recs, nil) {
		if state == CnBlockBlocked {
			result[uuid] = true
		}
	}
	return result
}

// advanceCnBlockStates 以本轮数据推进各节点状态（prev 为 nil 表示服务启动后的首轮），
// 返回页面"被墙"标记（在线且被墙）的变化与新状态：
//   - 在线节点：判定变为被墙即 Blocked，由被墙变为正常即 Recovered；无定论时沿用已确认状态。
//   - 在线 → 掉线：原先被墙即 Offline。同时清空判定，重新上线后等拿到新记录再定，
//     届时仍被墙会再发一次 Blocked。
//   - 首轮只记录基线、不通知。重启时 agent 大多还没连上，这些节点既不算"被墙后掉线"，
//     重连后也按重启前的记录继续跟踪，不会重复推送。
//   - 不在判定范围内的节点（任务被删除、节点被移除等）被清理。
func advanceCnBlockStates(prev map[string]cnBlockNode, blockTasks []models.PingTask, recs []models.PingRecord, online map[string]bool) (changes CnBlockChanges, next map[string]cnBlockNode) {
	firstRun := prev == nil
	observed := judgeCnBlockStates(blockTasks, recs, prev)

	next = make(map[string]cnBlockNode, len(observed))
	for uuid, state := range observed {
		old := prev[uuid]
		cur := old
		cur.online = online[uuid]
		switch {
		case firstRun:
			cur.state = state
		case !cur.online:
			if !old.online {
				break // 持续离线，或服务启动后还没上线过：保持不变
			}
			if old.state == CnBlockBlocked {
				changes.Offline = append(changes.Offline, uuid)
			}
			cur.state = CnBlockUnknown
			cur.staleBefore = latestCnBlockTime(recs, uuid, old.staleBefore)
		case state != CnBlockUnknown:
			cur.state = state
			cur.staleBefore = time.Time{}
			if state == old.state {
				break
			}
			if state == CnBlockBlocked {
				changes.Blocked = append(changes.Blocked, uuid)
			} else if old.state == CnBlockBlocked {
				changes.Recovered = append(changes.Recovered, uuid)
			}
		}
		next[uuid] = cur
	}
	sort.Strings(changes.Blocked)
	sort.Strings(changes.Recovered)
	sort.Strings(changes.Offline)
	return changes, next
}

// loadCnBlockRecords 取所有 block_check=true 的 ping 任务（国内参照目标）及其近期记录（按时间倒序）。
func loadCnBlockRecords(now time.Time) ([]models.PingTask, []models.PingRecord, error) {
	allTasks, err := GetAllPingTasks()
	if err != nil {
		return nil, nil, err
	}
	var blockTasks []models.PingTask
	var taskIDs []uint
	maxInterval := 60
	for _, t := range allTasks {
		if !t.BlockCheck {
			continue
		}
		blockTasks = append(blockTasks, t)
		taskIDs = append(taskIDs, t.Id)
		maxInterval = max(maxInterval, t.Interval)
	}
	if len(blockTasks) == 0 {
		return nil, nil, nil
	}
	// 窗口取最大间隔的 3 倍（足够凑齐去抖所需的轮数），至少 10 分钟：容忍上报抖动/丢点，
	// 面板升级重启的几分钟空档也不会让重启前的判定失效。
	lookback := max(time.Duration(maxInterval)*3*time.Second, 10*time.Minute)
	recs, err := GetRecentPingRecords(taskIDs, now.Add(-lookback))
	if err != nil {
		return nil, nil, err
	}
	return blockTasks, recs, nil
}

// judgeCnBlockStates 按每个参照目标最新一段连续一致的 ping 结果判定各节点：
//   - 任一目标连续 cnBlockStreak 轮能 ping 通 → CnBlockNormal
//   - 所有适用目标都连续 cnBlockStreak 轮超时 → CnBlockBlocked
//   - 其余（记录不足、结果刚变化还没满轮数）  → CnBlockUnknown，沿用上一次确认的状态
//
// recs 须按时间倒序；nodes 中带掉线标记的节点只认比 staleBefore 新的记录。
func judgeCnBlockStates(blockTasks []models.PingTask, recs []models.PingRecord, nodes map[string]cnBlockNode) map[string]CnBlockState {
	runs := latestCnBlockRuns(recs, nodes)
	type tally struct {
		reachable bool // 有目标稳定可达
		unsettled bool // 有目标尚无稳定结果
	}
	tallies := map[string]*tally{}
	for _, t := range blockTasks {
		for _, client := range t.Clients {
			tl, ok := tallies[client]
			if !ok {
				tl = &tally{}
				tallies[client] = tl
			}
			run, ok := runs[cnBlockKey{client, t.Id}]
			switch {
			case !ok || run.count < cnBlockStreak:
				tl.unsettled = true
			case !run.timeout:
				tl.reachable = true
			}
		}
	}

	result := make(map[string]CnBlockState, len(tallies))
	for client, tl := range tallies {
		switch {
		case tl.reachable:
			result[client] = CnBlockNormal
		case tl.unsettled:
			result[client] = CnBlockUnknown
		default:
			result[client] = CnBlockBlocked
		}
	}
	return result
}

// cnBlockRun 为节点在单个任务上、从最新一条往前连续一致的一段结果。
type cnBlockRun struct {
	timeout bool // 这段结果是否为超时
	count   int
	ended   bool // 已遇到不一致的结果
}

// latestCnBlockRuns 统计每个 (节点, 任务) 最新一段连续一致的结果；recs 须按时间倒序。
// nodes 中带掉线标记的节点只统计比 staleBefore 新的记录。
func latestCnBlockRuns(recs []models.PingRecord, nodes map[string]cnBlockNode) map[cnBlockKey]*cnBlockRun {
	runs := map[cnBlockKey]*cnBlockRun{}
	for _, r := range recs {
		if !r.Time.ToTime().After(nodes[r.Client].staleBefore) {
			continue
		}
		key := cnBlockKey{r.Client, r.TaskId}
		timeout := r.Value < 0
		run, ok := runs[key]
		switch {
		case !ok:
			runs[key] = &cnBlockRun{timeout: timeout, count: 1}
		case run.ended:
		case run.timeout == timeout:
			run.count++
		default:
			run.ended = true
		}
	}
	return runs
}

// latestCnBlockTime 返回节点最新一条记录的时间（recs 须按时间倒序），与 floor 取较晚者。
func latestCnBlockTime(recs []models.PingRecord, uuid string, floor time.Time) time.Time {
	for _, r := range recs {
		if r.Client != uuid {
			continue
		}
		if t := r.Time.ToTime(); t.After(floor) {
			return t
		}
		break
	}
	return floor
}
