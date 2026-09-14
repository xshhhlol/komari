package tasks

import (
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

// CnBlockState 表示一个节点的"被墙"判定结果。
type CnBlockState int

const (
	// CnBlockUnknown 暂无定论：节点离线、刚加入、记录不足，或最近几轮结果时好时坏。
	CnBlockUnknown CnBlockState = iota
	// CnBlockNormal 至少有一个国内参照目标连续多轮能 ping 通，判为未被墙。
	CnBlockNormal
	// CnBlockBlocked 所有国内参照目标都连续多轮超时，判为被墙。
	CnBlockBlocked
)

const (
	// cnBlockMinStreak 单个参照目标至少要连续这么多轮结果一致，才算稳定超时 / 稳定可达。
	// agent 每轮只发一个探测包，只看最新一条的话，线路偶发丢包就会让判定来回跳。
	cnBlockMinStreak = 3
	// cnBlockMinStreakSpan 这段连续结果至少要覆盖的时长；间隔很短的任务据此折算成更多轮，
	// 避免十几秒的抖动就改变判定。
	cnBlockMinStreakSpan = 2 * time.Minute
	// cnBlockMinLookback 查询 ping 记录的最短回看时长。留得宽一些，
	// 面板升级重启的几分钟空档不会让重启前已确认的状态变成"无定论"。
	cnBlockMinLookback = 10 * time.Minute
)

var (
	// cnBlockRefreshMu 串行化整轮刷新，避免较早的判定结果覆盖较新的。
	cnBlockRefreshMu sync.Mutex
	cnBlockMu        sync.Mutex
	// cnBlockConfirmed 为各节点已确认的状态，页面标注与通知都以它为准。
	// 每轮整体替换、不原地修改；nil 表示服务启动后尚未完成首轮刷新。
	cnBlockConfirmed map[string]CnBlockState
)

// RefreshCnBlockStates 用最新 ping 记录推进各节点的"被墙"状态，返回本轮新被墙、从被墙恢复的节点。
// 由定时任务每分钟调用。节点列表接口读取的也是这里确认后的状态，
// 所以页面上"被墙"标记的变化与通知一一对应。
//
// 查询失败时保持现有状态不变，不会因为一次数据库错误丢掉已确认的状态。
func RefreshCnBlockStates() (blocked, recovered []string) {
	cnBlockRefreshMu.Lock()
	defer cnBlockRefreshMu.Unlock()

	observed, err := observeCnBlockStates(time.Now())
	if err != nil {
		log.Printf("Failed to refresh cn-blocked states: %v", err)
		return nil, nil
	}

	cnBlockMu.Lock()
	defer cnBlockMu.Unlock()
	blocked, recovered, cnBlockConfirmed = advanceCnBlockStates(cnBlockConfirmed, observed)
	return blocked, recovered
}

// ComputeCnBlockedMap 返回节点 UUID -> 是否被墙，供节点列表接口标注。
// 读取 RefreshCnBlockStates 确认后的状态，与被墙 / 恢复通知保持一致；
// 服务刚启动、首轮刷新尚未完成时，直接按当前 ping 记录判定。
func ComputeCnBlockedMap() map[string]bool {
	cnBlockMu.Lock()
	states := cnBlockConfirmed
	cnBlockMu.Unlock()
	if states == nil {
		observed, err := observeCnBlockStates(time.Now())
		if err != nil {
			return map[string]bool{}
		}
		states = observed
	}

	result := make(map[string]bool, len(states))
	for client, state := range states {
		if state == CnBlockBlocked {
			result[client] = true
		}
	}
	return result
}

// advanceCnBlockStates 以本轮判定推进已确认状态（prev 为 nil 表示服务启动后的首轮），
// 返回新被墙 / 已恢复的节点和新的确认状态：
//   - 本轮判定为 Unknown：沿用已确认的状态，离线、记录断流既不算恢复也不算被墙。
//   - 首轮：直接以判定结果为基线、不发通知。判定来自持久化的 ping 历史，
//     重启前已经通知过的被墙节点不会被重复推送。
//   - 此后状态一旦变化即通知。此前没有定论的节点（新节点、启动时结果尚不稳定的节点）
//     确定为被墙时同样通知，确定为正常则不发"恢复"。
//   - 不在本轮判定范围内的节点（任务被删除、节点被移除等）被清理。
func advanceCnBlockStates(prev, observed map[string]CnBlockState) (blocked, recovered []string, next map[string]CnBlockState) {
	firstRun := prev == nil
	next = make(map[string]CnBlockState, len(observed))
	for uuid, state := range observed {
		old := prev[uuid]
		if state == CnBlockUnknown {
			next[uuid] = old
			continue
		}
		next[uuid] = state
		if firstRun || state == old {
			continue
		}
		if state == CnBlockBlocked {
			blocked = append(blocked, uuid)
		} else if old == CnBlockBlocked {
			recovered = append(recovered, uuid)
		}
	}
	sort.Strings(blocked)
	sort.Strings(recovered)
	return blocked, recovered, next
}

// observeCnBlockStates 取所有 block_check=true 的 ping 任务（国内参照目标）的近期记录，
// 给出每个节点本轮的判定。没有这类任务时返回空结果。
func observeCnBlockStates(now time.Time) (map[string]CnBlockState, error) {
	allTasks, err := GetAllPingTasks()
	if err != nil {
		return nil, err
	}
	blockTasks := make([]models.PingTask, 0)
	recs := map[uint][]models.PingRecord{}
	for _, t := range allTasks {
		if !t.BlockCheck {
			continue
		}
		taskRecs, err := GetRecentPingRecords([]uint{t.Id}, now.Add(-cnBlockLookback(t.Interval)))
		if err != nil {
			return nil, err
		}
		blockTasks = append(blockTasks, t)
		recs[t.Id] = taskRecs
	}
	return judgeCnBlockStates(blockTasks, recs), nil
}

// judgeCnBlockStates 根据各参照任务的近期记录（须按时间倒序）判定每个节点：
//   - 任一参照目标最近连续多轮都能 ping 通 → CnBlockNormal
//   - 所有参照目标最近连续多轮都超时       → CnBlockBlocked
//   - 其余（记录不足、结果时好时坏）       → CnBlockUnknown，沿用上一次确认的状态
//
// "多轮"的条数见 cnBlockStreakLen。
func judgeCnBlockStates(blockTasks []models.PingTask, recs map[uint][]models.PingRecord) map[string]CnBlockState {
	type tally struct {
		reachable bool // 有参照目标稳定可达
		unsettled bool // 有参照目标尚无稳定结果
	}
	tallies := map[string]*tally{}
	for _, t := range blockTasks {
		need := cnBlockStreakLen(t.Interval)
		streaks := latestCnBlockStreaks(recs[t.Id])
		for _, client := range t.Clients {
			tl, ok := tallies[client]
			if !ok {
				tl = &tally{}
				tallies[client] = tl
			}
			s, ok := streaks[client]
			switch {
			case !ok || s.count < need:
				tl.unsettled = true
			case !s.timeout:
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

// cnBlockStreak 为节点在单个任务上、从最新一条往前连续一致的一段结果。
type cnBlockStreak struct {
	timeout bool // 这段结果是否为超时
	count   int
	ended   bool // 已遇到不一致的结果
}

// latestCnBlockStreaks 统计每个节点在单个任务上的最新一段连续结果；recs 须按时间倒序。
func latestCnBlockStreaks(recs []models.PingRecord) map[string]*cnBlockStreak {
	streaks := map[string]*cnBlockStreak{}
	for _, r := range recs {
		timeout := r.Value < 0
		s, ok := streaks[r.Client]
		switch {
		case !ok:
			streaks[r.Client] = &cnBlockStreak{timeout: timeout, count: 1}
		case s.ended:
		case s.timeout == timeout:
			s.count++
		default:
			s.ended = true
		}
	}
	return streaks
}

// cnBlockStreakLen 返回间隔为 interval 秒的任务判定所需的连续结果条数：
// 至少 cnBlockMinStreak 条，且首尾跨度不短于 cnBlockMinStreakSpan。
func cnBlockStreakLen(interval int) int {
	interval = normalizeCnBlockInterval(interval)
	n := int(math.Ceil(cnBlockMinStreakSpan.Seconds()/float64(interval))) + 1
	return max(n, cnBlockMinStreak)
}

// cnBlockLookback 返回查询该任务记录的回看时长：多留一轮余量，容忍上报抖动 / 丢点。
func cnBlockLookback(interval int) time.Duration {
	interval = normalizeCnBlockInterval(interval)
	return max(time.Duration((cnBlockStreakLen(interval)+1)*interval)*time.Second, cnBlockMinLookback)
}

func normalizeCnBlockInterval(interval int) int {
	if interval <= 0 {
		return 60
	}
	return interval
}
