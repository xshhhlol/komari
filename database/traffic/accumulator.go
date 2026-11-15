package traffic

import (
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
)

type dayKey struct {
	client string
	day    string
}

type amount struct {
	up   int64
	down int64
}

// Accumulator 在内存里按「节点 × 自然日」累加增量，定期合并进 traffic_daily。
// 游标（每个节点上一次的样本）与增量在同一个事务里落库：面板崩溃时，
// 未落库的增量会在重启后由「旧游标 → 新样本」的差值补回来。
type Accumulator struct {
	db  func() *gorm.DB
	loc func() *time.Location

	mu      sync.Mutex
	cursors map[string]Sample
	dirty   map[string]struct{}
	pending map[dayKey]amount

	loadOnce sync.Once
	// flushMu 串行化落库与删除节点，避免删除后又被一次进行中的落库写回。
	flushMu sync.Mutex
}

func NewAccumulator(db func() *gorm.DB, loc func() *time.Location) *Accumulator {
	return &Accumulator{
		db:      db,
		loc:     loc,
		cursors: map[string]Sample{},
		dirty:   map[string]struct{}{},
		pending: map[dayKey]amount{},
	}
}

// ensureLoaded 首次使用时从数据库载入游标。
func (a *Accumulator) ensureLoaded() {
	a.loadOnce.Do(func() {
		var rows []models.TrafficCursor
		if err := a.db().Find(&rows).Error; err != nil {
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, r := range rows {
			if _, seen := a.cursors[r.Client]; seen {
				continue
			}
			a.cursors[r.Client] = Sample{
				Time:      r.Time.ToTime(),
				TotalUp:   r.TotalUp,
				TotalDown: r.TotalDown,
				Uptime:    r.Uptime,
				SpeedUp:   r.SpeedUp,
				SpeedDown: r.SpeedDown,
			}
		}
	})
}

// Observe 记录一次上报。节点的第一次上报只建立基线，不计流量。
func (a *Accumulator) Observe(client string, s Sample) {
	if client == "" || s.TotalUp < 0 || s.TotalDown < 0 {
		return
	}
	a.ensureLoaded()
	loc := a.loc()

	a.mu.Lock()
	defer a.mu.Unlock()
	prev, ok := a.cursors[client]
	if ok && !s.Time.After(prev.Time) {
		return
	}
	a.cursors[client] = s
	a.dirty[client] = struct{}{}
	if !ok {
		return
	}
	for _, part := range SplitByDay(ComputeDelta(prev, s), loc) {
		k := dayKey{client: client, day: part.Day}
		cur := a.pending[k]
		cur.up += part.Up
		cur.down += part.Down
		a.pending[k] = cur
	}
}

// Flush 把内存中的增量与游标写入数据库。失败时数据留在内存，下次重试。
func (a *Accumulator) Flush() error {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()

	a.mu.Lock()
	pending := a.pending
	a.pending = map[dayKey]amount{}
	cursors := make(map[string]Sample, len(a.dirty))
	for client := range a.dirty {
		cursors[client] = a.cursors[client]
	}
	a.dirty = map[string]struct{}{}
	a.mu.Unlock()

	if len(pending) == 0 && len(cursors) == 0 {
		return nil
	}

	err := a.db().Transaction(func(tx *gorm.DB) error {
		for k, v := range pending {
			if err := addDaily(tx, k.client, k.day, v.up, v.down); err != nil {
				return err
			}
		}
		for client, s := range cursors {
			if err := saveCursor(tx, client, s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.mu.Lock()
		for k, v := range pending {
			cur := a.pending[k]
			cur.up += v.up
			cur.down += v.down
			a.pending[k] = cur
		}
		for client := range cursors {
			if _, ok := a.cursors[client]; ok {
				a.dirty[client] = struct{}{}
			}
		}
		a.mu.Unlock()
	}
	return err
}

// Forget 删除节点的全部流量统计（内存与数据库）。
func (a *Accumulator) Forget(client string) error {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()

	a.mu.Lock()
	delete(a.cursors, client)
	delete(a.dirty, client)
	for k := range a.pending {
		if k.client == client {
			delete(a.pending, k)
		}
	}
	a.mu.Unlock()

	return a.db().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client = ?", client).Delete(&models.TrafficDaily{}).Error; err != nil {
			return err
		}
		return tx.Where("client = ?", client).Delete(&models.TrafficCursor{}).Error
	})
}

// pendingInRange 返回尚未落库、日期落在 [startDay, endDay] 内的增量。
// client 为空时返回所有节点。
func (a *Accumulator) pendingInRange(client, startDay, endDay string) map[dayKey]amount {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[dayKey]amount{}
	for k, v := range a.pending {
		if client != "" && k.client != client {
			continue
		}
		if k.day < startDay || k.day > endDay {
			continue
		}
		out[k] = v
	}
	return out
}

// addDaily 把增量加到某一天的行上，行不存在时插入。
func addDaily(tx *gorm.DB, client, day string, up, down int64) error {
	res := tx.Model(&models.TrafficDaily{}).
		Where("client = ? AND day = ?", client, day).
		Updates(map[string]any{
			"up":   gorm.Expr("up + ?", up),
			"down": gorm.Expr("down + ?", down),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	return tx.Create(&models.TrafficDaily{Client: client, Day: day, Up: up, Down: down}).Error
}

func saveCursor(tx *gorm.DB, client string, s Sample) error {
	return tx.Save(&models.TrafficCursor{
		Client:    client,
		Time:      models.FromTime(s.Time),
		TotalUp:   s.TotalUp,
		TotalDown: s.TotalDown,
		Uptime:    s.Uptime,
		SpeedUp:   s.SpeedUp,
		SpeedDown: s.SpeedDown,
	}).Error
}
