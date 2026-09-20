package tasks

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

var cnTestBase = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func cnTestTime(minute int) models.LocalTime {
	return models.FromTime(cnTestBase.Add(time.Duration(minute) * time.Minute))
}

// cnTestTasks 返回两个国内参照任务（如电信、联通），适用于给定节点。
func cnTestTasks(clients ...string) []models.PingTask {
	return []models.PingTask{
		{Id: 1, Interval: 60, BlockCheck: true, Clients: models.StringArray(clients)},
		{Id: 2, Interval: 60, BlockCheck: true, Clients: models.StringArray(clients)},
	}
}

// cnBlockSim 按轮次驱动状态跟踪，模拟定时任务每分钟一次的刷新。
type cnBlockSim struct {
	t     *testing.T
	tasks []models.PingTask
	recs  []models.PingRecord // 按时间倒序
	nodes map[string]cnBlockNode
}

// probe 记录节点在第 minute 分钟两个参照任务的结果，-1 表示超时。
func (s *cnBlockSim) probe(client string, minute int, telecom, unicom int) {
	at := cnTestTime(minute)
	s.recs = append(s.recs,
		models.PingRecord{Client: client, TaskId: 1, Value: telecom, Time: at},
		models.PingRecord{Client: client, TaskId: 2, Value: unicom, Time: at},
	)
	sort.SliceStable(s.recs, func(i, j int) bool {
		return s.recs[i].Time.ToTime().After(s.recs[j].Time.ToTime())
	})
}

// tick 执行一轮刷新并校验本轮变化；online 为此时在线的节点。
func (s *cnBlockSim) tick(want CnBlockChanges, online ...string) {
	s.t.Helper()
	set := map[string]bool{}
	for _, uuid := range online {
		set[uuid] = true
	}
	got, next := advanceCnBlockStates(s.nodes, s.tasks, s.recs, set)
	s.nodes = next
	if !reflect.DeepEqual(got, want) {
		s.t.Fatalf("changes: got %+v, want %+v", got, want)
	}
}

func TestJudgeCnBlockStates(t *testing.T) {
	rec := func(client string, task uint, minute, value int) models.PingRecord {
		return models.PingRecord{Client: client, TaskId: task, Value: value, Time: cnTestTime(minute)}
	}
	// 每个 (节点, 任务) 内按时间倒序
	recs := []models.PingRecord{
		// a：两个目标都连续 3 轮超时（更早能通不影响）
		rec("a", 1, 3, -1), rec("a", 1, 2, -1), rec("a", 1, 1, -1), rec("a", 1, 0, 30),
		rec("a", 2, 3, -1), rec("a", 2, 2, -1), rec("a", 2, 1, -1),
		// b：一个目标连续超时，另一个连续能通
		rec("b", 1, 3, -1), rec("b", 1, 2, -1), rec("b", 1, 1, -1),
		rec("b", 2, 3, 25), rec("b", 2, 2, 25), rec("b", 2, 1, 26),
		// c：其中一个目标只超时了 2 轮，差一轮
		rec("c", 1, 3, -1), rec("c", 1, 2, -1), rec("c", 1, 1, -1),
		rec("c", 2, 3, -1), rec("c", 2, 2, -1), rec("c", 2, 1, 30),
		// d：掉线标记之后只有 2 轮新记录
		rec("d", 1, 3, -1), rec("d", 1, 2, -1), rec("d", 1, 1, -1),
		rec("d", 2, 3, -1), rec("d", 2, 2, -1), rec("d", 2, 1, -1),
		// e：只有一个目标有记录
		rec("e", 1, 3, -1), rec("e", 1, 2, -1), rec("e", 1, 1, -1),
	}
	nodes := map[string]cnBlockNode{"d": {staleBefore: cnTestTime(1).ToTime()}}

	got := judgeCnBlockStates(cnTestTasks("a", "b", "c", "d", "e"), recs, nodes)
	want := map[string]CnBlockState{
		"a": CnBlockBlocked,
		"b": CnBlockNormal,
		"c": CnBlockUnknown,
		"d": CnBlockUnknown,
		"e": CnBlockUnknown,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAdvanceCnBlockStates(t *testing.T) {
	t.Run("连着两轮全部超时随即恢复（抖动）不通知", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, 30, 40)
		s.probe("a", 1, 30, 40)
		s.probe("a", 2, 30, 40)
		s.tick(CnBlockChanges{}, "a") // 首轮只记基线
		s.probe("a", 3, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 4, -1, -1)
		s.tick(CnBlockChanges{}, "a") // 差一轮，还不算被墙
		s.probe("a", 5, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		if s.nodes["a"].state != CnBlockNormal {
			t.Fatalf("two bad rounds should keep the node normal, got %v", s.nodes["a"].state)
		}
	})

	t.Run("连续 3 轮才判被墙 / 恢复，状态不变不重复", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, 30, 40)
		s.probe("a", 1, 30, 40)
		s.probe("a", 2, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 3, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 4, -1, -1)
		s.tick(CnBlockChanges{}, "a") // 只超时 2 轮
		s.probe("a", 5, -1, -1)
		s.tick(CnBlockChanges{Blocked: []string{"a"}}, "a")
		s.probe("a", 6, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 7, 30, -1)
		s.tick(CnBlockChanges{}, "a") // 只通了 1 轮
		s.probe("a", 8, 30, -1)
		s.tick(CnBlockChanges{}, "a") // 只通了 2 轮
		s.probe("a", 9, 30, -1)
		s.tick(CnBlockChanges{Recovered: []string{"a"}}, "a")
	})

	t.Run("只有一个目标超时不算被墙", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, 30, 40)
		s.probe("a", 1, 30, 40)
		s.probe("a", 2, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 3, -1, 40)
		s.probe("a", 4, -1, 40)
		s.probe("a", 5, -1, 40)
		s.tick(CnBlockChanges{}, "a")
	})

	t.Run("被墙节点掉线要通知；重新上线只认新记录，连续 3 轮仍被墙再通知", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, -1, -1)
		s.probe("a", 1, -1, -1)
		s.probe("a", 2, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		s.tick(CnBlockChanges{Offline: []string{"a"}}) // 掉线
		s.tick(CnBlockChanges{})                       // 持续离线不重复
		s.tick(CnBlockChanges{}, "a")                  // 刚上线，只有掉线前的旧记录
		if s.nodes["a"].state != CnBlockUnknown {
			t.Fatalf("stale records before going offline must not count, got state %v", s.nodes["a"].state)
		}
		s.probe("a", 3, -1, -1)
		s.tick(CnBlockChanges{}, "a") // 新记录只有 1 轮
		s.probe("a", 4, -1, -1)
		s.tick(CnBlockChanges{}, "a") // 新记录只有 2 轮
		s.probe("a", 5, -1, -1)
		s.tick(CnBlockChanges{Blocked: []string{"a"}}, "a")
	})

	t.Run("掉线期间换了 IP，重新上线后正常则不发通知", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, -1, -1)
		s.probe("a", 1, -1, -1)
		s.probe("a", 2, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		s.tick(CnBlockChanges{Offline: []string{"a"}})
		s.probe("a", 3, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 4, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 5, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		if s.nodes["a"].state != CnBlockNormal {
			t.Fatalf("expected normal after reconnecting with a clean IP, got %v", s.nodes["a"].state)
		}
	})

	t.Run("正常节点掉线不通知", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, 30, 40)
		s.probe("a", 1, 30, 40)
		s.probe("a", 2, 30, 40)
		s.tick(CnBlockChanges{}, "a")
		s.tick(CnBlockChanges{})
	})

	t.Run("服务重启：agent 重连前后都不补发", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, -1, -1) // 重启前已被墙（已通知过）
		s.probe("a", 1, -1, -1)
		s.probe("a", 2, -1, -1)
		s.tick(CnBlockChanges{})      // 首轮：agent 尚未重连
		s.tick(CnBlockChanges{})      // 仍未重连，不算"被墙后掉线"
		s.tick(CnBlockChanges{}, "a") // 重连后仍被墙
		s.probe("a", 3, 30, -1)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 4, 30, -1)
		s.tick(CnBlockChanges{}, "a")
		s.probe("a", 5, 30, -1)
		s.tick(CnBlockChanges{Recovered: []string{"a"}}, "a")
	})

	t.Run("此前没有定论的节点确定被墙即通知，确定正常不发恢复", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a", "b")}
		s.tick(CnBlockChanges{}, "a", "b") // 首轮都没有记录
		s.probe("a", 1, -1, -1)
		s.probe("b", 1, 30, -1)
		s.tick(CnBlockChanges{}, "a", "b")
		s.probe("a", 2, -1, -1)
		s.probe("b", 2, 30, -1)
		s.tick(CnBlockChanges{}, "a", "b")
		s.probe("a", 3, -1, -1)
		s.probe("b", 3, 30, -1)
		s.tick(CnBlockChanges{Blocked: []string{"a"}}, "a", "b")
	})

	t.Run("数据不足时沿用已确认状态", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, -1, -1)
		s.probe("a", 1, -1, -1)
		s.probe("a", 2, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		// 新增一个参照任务，节点还没有它的记录
		s.tasks = append(s.tasks, models.PingTask{Id: 3, Interval: 60, BlockCheck: true, Clients: models.StringArray{"a"}})
		s.tick(CnBlockChanges{}, "a")
		if s.nodes["a"].state != CnBlockBlocked {
			t.Fatalf("missing data should keep the confirmed state, got %v", s.nodes["a"].state)
		}
	})

	t.Run("移出判定范围的节点被清理", func(t *testing.T) {
		s := &cnBlockSim{t: t, tasks: cnTestTasks("a")}
		s.probe("a", 0, -1, -1)
		s.probe("a", 1, -1, -1)
		s.probe("a", 2, -1, -1)
		s.tick(CnBlockChanges{}, "a")
		s.tasks = nil
		s.tick(CnBlockChanges{}, "a")
		if len(s.nodes) != 0 {
			t.Fatalf("state of a client no longer under block-check should be dropped, got %v", s.nodes)
		}
	})
}
