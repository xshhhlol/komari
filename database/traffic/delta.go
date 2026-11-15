// Package traffic 按天统计每个节点的流量用量。
//
// agent 上报的 TotalUp/TotalDown 是累计计数器：默认模式下是开机以来的网卡计数，
// 机器重启会清零；--month-rotate 模式下是 vnstat 的本月累计，按月重置。
// 这里对相邻两次上报求增量并按自然日累加，重启只损失「最后一次上报 → 关机」
// 这一个上报间隔内的流量。
package traffic

import (
	"time"
)

// Sample 是一次上报中参与流量统计的字段。
type Sample struct {
	Time      time.Time
	TotalUp   int64
	TotalDown int64
	Uptime    int64 // 系统开机秒数；0 表示未知（历史记录、未上报 uptime 的客户端）
	SpeedUp   int64 // 当前速率 B/s，仅在无法判断是否重启时用来估算增量上限
	SpeedDown int64
}

const (
	// maxRate 单方向速率上限（40 Gbps）。超过它的增量视为计数器跳变而非真实流量，
	// 例如 agent 从 vnstat 回退到网卡计数器时数值突然变大。
	maxRate int64 = 5_000_000_000
	// minCapWindow 速率上限的最小时间窗口，避免上报在网络里积压后集中到达时被误判。
	minCapWindow = 60 * time.Second
	// bootTolerance 判断「开机时间前移」的容差，吸收时钟与上报延迟带来的抖动。
	bootTolerance = 120 * time.Second
	// unknownBootSlack 无法判断重启时，按速率估算上限之外再放宽的字节数。
	unknownBootSlack int64 = 64 << 20
)

// Delta 是两次样本之间的流量增量，发生在 [From, To] 区间内。
type Delta struct {
	Up   int64
	Down int64
	From time.Time
	To   time.Time
}

func (d Delta) IsZero() bool { return d.Up == 0 && d.Down == 0 }

// ComputeDelta 计算 prev → cur 之间的流量增量。
//
//   - 计数器单调不减：正常求差（含 vnstat 模式下的重启，计数不清零）。
//   - 计数器回退且开机时间前移：机器重启，开机以来的计数全部是新增流量。
//   - 计数器回退但没有重启：网卡消失、vnstat 月重置、统计口径切换，重新定基线记 0。
//   - 无 uptime 可判断：按重启处理，但用观测到的速率给增量设上限。
func ComputeDelta(prev, cur Sample) Delta {
	from := prev.Time
	if from.After(cur.Time) {
		from = cur.Time
	}
	dt := cur.Time.Sub(from)
	zero := Delta{From: from, To: cur.Time}

	if cur.TotalUp >= prev.TotalUp && cur.TotalDown >= prev.TotalDown {
		up, down := cur.TotalUp-prev.TotalUp, cur.TotalDown-prev.TotalDown
		if exceedsRate(up, dt) || exceedsRate(down, dt) {
			return zero
		}
		return Delta{Up: up, Down: down, From: from, To: cur.Time}
	}

	if prev.Uptime > 0 && cur.Uptime > 0 {
		prevBoot := prev.Time.Add(-time.Duration(prev.Uptime) * time.Second)
		curBoot := cur.Time.Add(-time.Duration(cur.Uptime) * time.Second)
		if curBoot.Sub(prevBoot) <= bootTolerance {
			return zero
		}
		sinceBoot := time.Duration(cur.Uptime) * time.Second
		if exceedsRate(cur.TotalUp, sinceBoot) || exceedsRate(cur.TotalDown, sinceBoot) {
			return zero
		}
		if curBoot.After(from) {
			from = curBoot
		}
		return Delta{Up: cur.TotalUp, Down: cur.TotalDown, From: from, To: cur.Time}
	}

	return Delta{
		Up:   min(cur.TotalUp, speedBound(prev.SpeedUp, cur.SpeedUp, dt)),
		Down: min(cur.TotalDown, speedBound(prev.SpeedDown, cur.SpeedDown, dt)),
		From: from,
		To:   cur.Time,
	}
}

func exceedsRate(bytes int64, window time.Duration) bool {
	if window < minCapWindow {
		window = minCapWindow
	}
	return float64(bytes) > float64(maxRate)*window.Seconds()
}

func speedBound(a, b int64, dt time.Duration) int64 {
	speed := max(a, b, 0)
	if dt < time.Second {
		dt = time.Second
	}
	return int64(float64(speed)*dt.Seconds()*2) + unknownBootSlack
}

// DayKey 返回 t 在 loc 时区下的自然日。
func DayKey(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(time.DateOnly)
}

// maxSplitDays 跨天拆分的最大天数，掉线更久的增量只拆到最近这些天。
const maxSplitDays = 400

// DayAmount 是某一天分到的流量。
type DayAmount struct {
	Day  string
	Up   int64
	Down int64
}

// SplitByDay 把 [d.From, d.To] 内的增量按时长比例拆到经过的每个自然日。
// 节点掉线跨天后恢复时，流量不会全部堆在恢复的那一天。
func SplitByDay(d Delta, loc *time.Location) []DayAmount {
	if d.IsZero() {
		return nil
	}
	from, to := d.From.In(loc), d.To.In(loc)
	if earliest := startOfDay(to, loc).AddDate(0, 0, -(maxSplitDays - 1)); from.Before(earliest) {
		from = earliest
	}
	if !from.Before(to) || DayKey(from, loc) == DayKey(to, loc) {
		return []DayAmount{{Day: DayKey(to, loc), Up: d.Up, Down: d.Down}}
	}

	total := to.Sub(from).Seconds()
	var parts []DayAmount
	var usedUp, usedDown int64
	for cursor := from; cursor.Before(to); {
		next := startOfDay(cursor, loc).AddDate(0, 0, 1)
		if next.After(to) {
			next = to
		}
		frac := next.Sub(cursor).Seconds() / total
		up, down := int64(float64(d.Up)*frac), int64(float64(d.Down)*frac)
		parts = append(parts, DayAmount{Day: DayKey(cursor, loc), Up: up, Down: down})
		usedUp += up
		usedDown += down
		cursor = next
	}
	// 取整误差归到最后一天，保证总量不变
	last := &parts[len(parts)-1]
	last.Up += d.Up - usedUp
	last.Down += d.Down - usedDown
	return parts
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
