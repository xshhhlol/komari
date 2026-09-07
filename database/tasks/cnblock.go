package tasks

import (
	"time"

	"github.com/komari-monitor/komari/database/models"
)

// CnBlockState 表示一个节点的"被墙"判定结果。
type CnBlockState int

const (
	// CnBlockUnknown 数据不足，无法判定（节点离线、刚加入、ping 记录尚未上报等）。
	CnBlockUnknown CnBlockState = iota
	// CnBlockNormal 至少有一个国内参照目标能 ping 通，判为未被墙。
	CnBlockNormal
	// CnBlockBlocked 所有国内参照目标的最新结果均为超时，判为被墙。
	CnBlockBlocked
)

// ComputeCnBlockStates 计算每个节点的"被墙"状态。
//
// 取所有 block_check=true 的 ping 任务（国内参照目标），对每个节点：
//   - 任一任务的最新记录能 ping 通            → CnBlockNormal
//   - 所有适用任务都有最新记录且全部为 -1（超时） → CnBlockBlocked
//   - 其余情况（缺少最新数据）                → CnBlockUnknown
//
// 第二个返回值表示"被墙判定"功能是否可用（存在 block_check 任务且查询成功）。
// 通知逻辑依赖三态：节点离线导致 ping 记录断流时只会得到 Unknown，
// 不会被误判成"已恢复"。
func ComputeCnBlockStates() (map[string]CnBlockState, bool) {
	result := map[string]CnBlockState{}
	allTasks, err := GetAllPingTasks()
	if err != nil {
		return result, false
	}
	blockTaskIDs := make([]uint, 0)
	taskClients := map[uint]models.StringArray{}
	maxInterval := 60
	for _, t := range allTasks {
		if !t.BlockCheck {
			continue
		}
		blockTaskIDs = append(blockTaskIDs, t.Id)
		taskClients[t.Id] = t.Clients
		if t.Interval > maxInterval {
			maxInterval = t.Interval
		}
	}
	if len(blockTaskIDs) == 0 {
		return result, false
	}

	// 窗口取最大间隔的 3 倍，至少 5 分钟，容忍上报抖动/丢点。
	lookback := time.Duration(maxInterval) * 3 * time.Second
	if lookback < 5*time.Minute {
		lookback = 5 * time.Minute
	}
	recs, err := GetRecentPingRecords(blockTaskIDs, time.Now().Add(-lookback))
	if err != nil {
		return result, false
	}

	// recs 已按 time DESC，每个 (client, task) 首次出现即为最新一条。
	type clientTask struct {
		client string
		task   uint
	}
	latest := map[clientTask]int{}
	for _, r := range recs {
		key := clientTask{r.Client, r.TaskId}
		if _, ok := latest[key]; !ok {
			latest[key] = r.Value
		}
	}

	// client -> 适用的 block 任务列表
	applicable := map[string][]uint{}
	for taskID, cls := range taskClients {
		for _, c := range cls {
			applicable[c] = append(applicable[c], taskID)
		}
	}
	for client, taskIDs := range applicable {
		if len(taskIDs) == 0 {
			continue
		}
		reachable := false
		missing := false
		for _, tid := range taskIDs {
			v, ok := latest[clientTask{client, tid}]
			if !ok {
				missing = true
				continue
			}
			if v != -1 {
				reachable = true
				break
			}
		}
		switch {
		case reachable:
			result[client] = CnBlockNormal
		case missing:
			result[client] = CnBlockUnknown
		default:
			result[client] = CnBlockBlocked
		}
	}
	return result, true
}

// ComputeCnBlockedMap 返回节点 UUID -> 是否被墙，供节点列表接口标注。
// 数据不足一律按未被墙处理，尽量避免误报。
func ComputeCnBlockedMap() map[string]bool {
	states, _ := ComputeCnBlockStates()
	result := make(map[string]bool, len(states))
	for client, state := range states {
		if state == CnBlockBlocked {
			result[client] = true
		}
	}
	return result
}
