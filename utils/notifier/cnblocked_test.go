package notifier

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/tasks"
)

func TestAdvanceCnBlockStates(t *testing.T) {
	const uuid = "node-1"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("首次观测只记录基线不通知", func(t *testing.T) {
		resetCnBlockStates()
		blocked, recovered := advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockBlocked}, base)
		if len(blocked) != 0 || len(recovered) != 0 {
			t.Fatalf("first observation should be silent, got blocked=%v recovered=%v", blocked, recovered)
		}
	})

	t.Run("确认时长未满不通知，满后通知一次", func(t *testing.T) {
		resetCnBlockStates()
		advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockNormal}, base)

		states := map[string]tasks.CnBlockState{uuid: tasks.CnBlockBlocked}
		if blocked, _ := advanceCnBlockStates(states, base.Add(time.Minute)); len(blocked) != 0 {
			t.Fatalf("should not notify before stable duration, got %v", blocked)
		}
		blocked, _ := advanceCnBlockStates(states, base.Add(time.Minute+cnBlockStableDuration))
		if len(blocked) != 1 || blocked[0] != uuid {
			t.Fatalf("expected blocked notification for %s, got %v", uuid, blocked)
		}
		// 状态已确认，继续观测到同一状态不应重复通知。
		if blocked, _ := advanceCnBlockStates(states, base.Add(time.Hour)); len(blocked) != 0 {
			t.Fatalf("blocked state should notify only once, got %v", blocked)
		}
	})

	t.Run("确认期内抖动回原状态则取消通知", func(t *testing.T) {
		resetCnBlockStates()
		advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockNormal}, base)
		advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockBlocked}, base.Add(time.Minute))
		advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockNormal}, base.Add(2*time.Minute))

		blocked, _ := advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockBlocked}, base.Add(3*time.Minute))
		if len(blocked) != 0 {
			t.Fatalf("flapping should restart the confirmation window, got %v", blocked)
		}
	})

	t.Run("数据不足不会误报恢复", func(t *testing.T) {
		resetCnBlockStates()
		advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockNormal}, base)
		states := map[string]tasks.CnBlockState{uuid: tasks.CnBlockBlocked}
		advanceCnBlockStates(states, base.Add(time.Minute))
		if blocked, _ := advanceCnBlockStates(states, base.Add(time.Minute+cnBlockStableDuration)); len(blocked) != 1 {
			t.Fatalf("expected blocked notification, got %v", blocked)
		}

		// 节点离线，ping 记录断流：只应保持被墙状态，不能报恢复。
		unknown := map[string]tasks.CnBlockState{uuid: tasks.CnBlockUnknown}
		for i := 1; i <= 30; i++ {
			if _, recovered := advanceCnBlockStates(unknown, base.Add(time.Duration(i)*10*time.Minute)); len(recovered) != 0 {
				t.Fatalf("missing data must not report recovery, got %v", recovered)
			}
		}

		// 恢复后重新 ping 通，确认时长满足才通知。
		normal := map[string]tasks.CnBlockState{uuid: tasks.CnBlockNormal}
		start := base.Add(6 * time.Hour)
		advanceCnBlockStates(normal, start)
		_, recovered := advanceCnBlockStates(normal, start.Add(cnBlockStableDuration))
		if len(recovered) != 1 || recovered[0] != uuid {
			t.Fatalf("expected recovery notification for %s, got %v", uuid, recovered)
		}
	})

	t.Run("移出判定范围的节点状态被清理", func(t *testing.T) {
		resetCnBlockStates()
		advanceCnBlockStates(map[string]tasks.CnBlockState{uuid: tasks.CnBlockNormal}, base)
		advanceCnBlockStates(map[string]tasks.CnBlockState{}, base.Add(time.Minute))

		cnBlockMu.Lock()
		_, exists := cnBlockStates[uuid]
		cnBlockMu.Unlock()
		if exists {
			t.Fatal("state of a client no longer under block-check should be dropped")
		}
	})
}
