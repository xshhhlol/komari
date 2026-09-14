package tasks

import (
	"reflect"
	"testing"

	"github.com/komari-monitor/komari/database/models"
)

// pingResults 生成节点在某任务上的记录，values 从最新到最旧排列，-1 表示超时。
func pingResults(client string, taskID uint, values ...int) []models.PingRecord {
	recs := make([]models.PingRecord, 0, len(values))
	for _, v := range values {
		recs = append(recs, models.PingRecord{Client: client, TaskId: taskID, Value: v})
	}
	return recs
}

func TestCnBlockStreakLen(t *testing.T) {
	cases := map[int]int{60: 3, 300: 3, 30: 5, 10: 13, 0: 3}
	for interval, want := range cases {
		if got := cnBlockStreakLen(interval); got != want {
			t.Errorf("interval %ds: got %d, want %d", interval, got, want)
		}
	}
}

func TestJudgeCnBlockStates(t *testing.T) {
	clients := models.StringArray{"a", "b", "c", "d", "e"}
	telecom := models.PingTask{Id: 1, Interval: 60, Clients: clients}
	unicom := models.PingTask{Id: 2, Interval: 60, Clients: clients}
	recs := map[uint][]models.PingRecord{}
	add := func(client string, taskID uint, values ...int) {
		recs[taskID] = append(recs[taskID], pingResults(client, taskID, values...)...)
	}
	// a：两个目标都连续 3 轮超时
	add("a", 1, -1, -1, -1)
	add("a", 2, -1, -1, -1, 30)
	// b：刚开始超时，其中一个目标只有 2 轮
	add("b", 1, -1, -1, 30)
	add("b", 2, -1, -1, -1)
	// c：一个目标偶发丢包，另一个稳定可达
	add("c", 1, -1, 30, -1)
	add("c", 2, 30, 31, 32)
	// d：被墙期间偶有一个包通过，不足以判恢复
	add("d", 1, 30, -1, -1, -1)
	add("d", 2, -1, -1, -1)
	// e：没有任何记录（离线）

	got := judgeCnBlockStates([]models.PingTask{telecom, unicom}, recs)
	want := map[string]CnBlockState{
		"a": CnBlockBlocked,
		"b": CnBlockUnknown,
		"c": CnBlockNormal,
		"d": CnBlockUnknown,
		"e": CnBlockUnknown,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestJudgeCnBlockStatesShortInterval(t *testing.T) {
	task := models.PingTask{Id: 1, Interval: 10, Clients: models.StringArray{"a"}}
	for n, want := range map[int]CnBlockState{12: CnBlockUnknown, 13: CnBlockBlocked} {
		values := make([]int, n)
		for i := range values {
			values[i] = -1
		}
		got := judgeCnBlockStates([]models.PingTask{task}, map[uint][]models.PingRecord{1: pingResults("a", 1, values...)})
		if got["a"] != want {
			t.Errorf("%d timeouts at 10s interval: got %v, want %v", n, got["a"], want)
		}
	}
}

func TestAdvanceCnBlockStates(t *testing.T) {
	t.Run("首轮以判定结果为基线，不通知", func(t *testing.T) {
		observed := map[string]CnBlockState{"a": CnBlockBlocked, "b": CnBlockNormal, "c": CnBlockUnknown}
		blocked, recovered, next := advanceCnBlockStates(nil, observed)
		if len(blocked) != 0 || len(recovered) != 0 {
			t.Fatalf("first run should be silent, got blocked=%v recovered=%v", blocked, recovered)
		}
		if !reflect.DeepEqual(next, observed) {
			t.Fatalf("baseline: got %v, want %v", next, observed)
		}
	})

	t.Run("状态变化即通知，同一变化只通知一次", func(t *testing.T) {
		prev := map[string]CnBlockState{"a": CnBlockNormal, "b": CnBlockBlocked}
		observed := map[string]CnBlockState{"a": CnBlockBlocked, "b": CnBlockNormal}
		blocked, recovered, next := advanceCnBlockStates(prev, observed)
		if !reflect.DeepEqual(blocked, []string{"a"}) || !reflect.DeepEqual(recovered, []string{"b"}) {
			t.Fatalf("got blocked=%v recovered=%v", blocked, recovered)
		}
		blocked, recovered, _ = advanceCnBlockStates(next, observed)
		if len(blocked) != 0 || len(recovered) != 0 {
			t.Fatalf("unchanged state should not notify again, got blocked=%v recovered=%v", blocked, recovered)
		}
	})

	t.Run("判定不确定时沿用已确认状态", func(t *testing.T) {
		prev := map[string]CnBlockState{"a": CnBlockBlocked, "b": CnBlockNormal}
		blocked, recovered, next := advanceCnBlockStates(prev, map[string]CnBlockState{"a": CnBlockUnknown, "b": CnBlockUnknown})
		if len(blocked) != 0 || len(recovered) != 0 {
			t.Fatalf("unknown must not notify, got blocked=%v recovered=%v", blocked, recovered)
		}
		if !reflect.DeepEqual(next, prev) {
			t.Fatalf("got %v, want %v", next, prev)
		}
	})

	t.Run("此前没有定论的节点：确定被墙要通知，确定正常不发恢复", func(t *testing.T) {
		prev := map[string]CnBlockState{"pending": CnBlockUnknown}
		blocked, recovered, _ := advanceCnBlockStates(prev, map[string]CnBlockState{
			"pending":     CnBlockBlocked,
			"new-blocked": CnBlockBlocked,
			"new-normal":  CnBlockNormal,
		})
		if !reflect.DeepEqual(blocked, []string{"new-blocked", "pending"}) || len(recovered) != 0 {
			t.Fatalf("got blocked=%v recovered=%v", blocked, recovered)
		}
	})

	t.Run("移出判定范围的节点被清理", func(t *testing.T) {
		_, _, next := advanceCnBlockStates(map[string]CnBlockState{"a": CnBlockBlocked}, map[string]CnBlockState{})
		if len(next) != 0 {
			t.Fatalf("state of a client no longer under block-check should be dropped, got %v", next)
		}
	})
}
