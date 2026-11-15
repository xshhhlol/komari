package traffic

import (
	"log"
	"sort"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	v1 "github.com/komari-monitor/komari/protocol/v1"
	"gorm.io/gorm"
)

var defaultAccumulator = NewAccumulator(dbcore.GetDBInstance, models.GetAppLocation)

// Init 在开始接收上报前调用：首次部署时用历史负载记录回填，然后载入游标。
func Init() {
	if err := backfillIfEmpty(dbcore.GetDBInstance(), models.GetAppLocation()); err != nil {
		log.Printf("traffic: backfill from records failed: %v", err)
	}
	defaultAccumulator.ensureLoaded()
}

// ObserveReport 记录一次 agent 上报，统计时间取服务端收到的时间。
func ObserveReport(client string, report v1.Report) {
	defaultAccumulator.Observe(client, Sample{
		Time:      time.Now(),
		TotalUp:   report.Network.TotalUp,
		TotalDown: report.Network.TotalDown,
		Uptime:    report.Uptime,
		SpeedUp:   report.Network.Up,
		SpeedDown: report.Network.Down,
	})
}

// Flush 把内存中的增量写入数据库，由每分钟的定时任务与关机流程调用。
func Flush() {
	if err := defaultAccumulator.Flush(); err != nil {
		log.Printf("traffic: flush failed: %v", err)
	}
}

// Forget 删除节点时清理它的流量统计。
func Forget(client string) error {
	return defaultAccumulator.Forget(client)
}

// Today 返回应用时区下的今天。
func Today() string {
	return DayKey(time.Now(), models.GetAppLocation())
}

// Usage 是一段时间内的流量用量。
type Usage struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// DailyUsage 是某一天的流量用量。
type DailyUsage struct {
	Day  string `json:"day"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// GetDaily 返回节点在 [startDay, endDay] 内每天的用量（含尚未落库的部分）。
// 从节点第一天有数据开始逐日补零，之前没有数据的日子不返回。
func GetDaily(client, startDay, endDay string) ([]DailyUsage, error) {
	return getDaily(dbcore.GetDBInstance(), defaultAccumulator, client, startDay, endDay)
}

func getDaily(db *gorm.DB, acc *Accumulator, client, startDay, endDay string) ([]DailyUsage, error) {
	acc.flushMu.Lock()
	defer acc.flushMu.Unlock()
	var rows []models.TrafficDaily
	if err := db.Where("client = ? AND day >= ? AND day <= ?", client, startDay, endDay).
		Order("day ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	byDay := make(map[string]Usage, len(rows))
	for _, r := range rows {
		byDay[r.Day] = Usage{Up: r.Up, Down: r.Down}
	}
	for k, v := range acc.pendingInRange(client, startDay, endDay) {
		u := byDay[k.day]
		u.Up += v.up
		u.Down += v.down
		byDay[k.day] = u
	}

	// 从节点最早有数据的那天开始补零；更早的日子节点还没接入，不返回
	var earliest struct{ Day *string }
	if err := db.Model(&models.TrafficDaily{}).Select("MIN(day) AS day").
		Where("client = ?", client).Scan(&earliest).Error; err != nil {
		return nil, err
	}
	first := ""
	if earliest.Day != nil {
		first = *earliest.Day
	}
	for day := range byDay {
		if first == "" || day < first {
			first = day
		}
	}
	if first == "" {
		return []DailyUsage{}, nil
	}
	if first < startDay {
		first = startDay
	}

	start, err := time.Parse(time.DateOnly, first)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse(time.DateOnly, endDay)
	if err != nil {
		return nil, err
	}
	out := make([]DailyUsage, 0)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format(time.DateOnly)
		u := byDay[key]
		out = append(out, DailyUsage{Day: key, Up: u.Up, Down: u.Down})
	}
	return out, nil
}

// GetUsageByClient 返回所有节点在 [startDay, endDay] 内的总用量。
func GetUsageByClient(startDay, endDay string) (map[string]Usage, error) {
	return getUsageByClient(dbcore.GetDBInstance(), defaultAccumulator, startDay, endDay)
}

func getUsageByClient(db *gorm.DB, acc *Accumulator, startDay, endDay string) (map[string]Usage, error) {
	acc.flushMu.Lock()
	defer acc.flushMu.Unlock()
	type row struct {
		Client string
		Up     int64
		Down   int64
	}
	var rows []row
	if err := db.Model(&models.TrafficDaily{}).
		Select("client, SUM(up) AS up, SUM(down) AS down").
		Where("day >= ? AND day <= ?", startDay, endDay).
		Group("client").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]Usage, len(rows))
	for _, r := range rows {
		out[r.Client] = Usage{Up: r.Up, Down: r.Down}
	}
	for k, v := range acc.pendingInRange("", startDay, endDay) {
		u := out[k.client]
		u.Up += v.up
		u.Down += v.down
		out[k.client] = u
	}
	return out, nil
}

// GetClientUsage 返回单个节点在 [startDay, endDay] 内的总用量。
func GetClientUsage(client, startDay, endDay string) (Usage, error) {
	return getClientUsage(dbcore.GetDBInstance(), defaultAccumulator, client, startDay, endDay)
}

func getClientUsage(db *gorm.DB, acc *Accumulator, client, startDay, endDay string) (Usage, error) {
	acc.flushMu.Lock()
	defer acc.flushMu.Unlock()
	var u Usage
	if err := db.Model(&models.TrafficDaily{}).
		Select("COALESCE(SUM(up), 0) AS up, COALESCE(SUM(down), 0) AS down").
		Where("client = ? AND day >= ? AND day <= ?", client, startDay, endDay).
		Scan(&u).Error; err != nil {
		return Usage{}, err
	}
	for _, v := range acc.pendingInRange(client, startDay, endDay) {
		u.Up += v.up
		u.Down += v.down
	}
	return u, nil
}

// Period 是排行榜查询的日期范围。
type Period struct {
	Granularity string `json:"granularity"` // day | week | month
	Offset      int    `json:"offset"`      // 0 = 当前周期，-1 = 上一个周期
	Start       string `json:"start"`
	End         string `json:"end"`
}

// ResolvePeriod 计算 now 所在周期向前/后偏移 offset 个周期后的日期范围。
// 周从周一开始，与流量周报一致。offset 大于 0 时按 0 处理。
func ResolvePeriod(granularity string, offset int, now time.Time, loc *time.Location) Period {
	if offset > 0 {
		offset = 0
	}
	today := startOfDay(now, loc)
	var start, end time.Time
	switch granularity {
	case "week":
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = today.AddDate(0, 0, -(weekday-1)+7*offset)
		end = start.AddDate(0, 0, 6)
	case "month":
		start = time.Date(today.Year(), today.Month()+time.Month(offset), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, -1)
	default:
		granularity = "day"
		start = today.AddDate(0, 0, offset)
		end = start
	}
	return Period{
		Granularity: granularity,
		Offset:      offset,
		Start:       start.Format(time.DateOnly),
		End:         end.Format(time.DateOnly),
	}
}

// backfillIfEmpty 首次部署时，用负载记录里保存的累计流量回填每日用量。
// 历史记录没有 uptime，重启判断退化为按速率估算上限，精度略低于实时统计。
func backfillIfEmpty(db *gorm.DB, loc *time.Location) error {
	var count int64
	if err := db.Model(&models.TrafficDaily{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	if err := db.Model(&models.TrafficCursor{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}

	type recordRow struct {
		Client       string
		Time         models.LocalTime
		NetTotalUp   int64
		NetTotalDown int64
		NetIn        int64
		NetOut       int64
	}
	const columns = "client, time, net_total_up, net_total_down, net_in, net_out"
	var longTerm, recent []recordRow
	if err := db.Table("records_long_term").Select(columns).Order("time ASC").Scan(&longTerm).Error; err != nil {
		return err
	}
	if err := db.Table("records").Select(columns).Order("time ASC").Scan(&recent).Error; err != nil {
		return err
	}
	if len(longTerm) == 0 && len(recent) == 0 {
		return nil
	}

	samples := map[string][]Sample{}
	firstRecent := map[string]time.Time{}
	for _, r := range recent {
		if _, ok := firstRecent[r.Client]; !ok {
			firstRecent[r.Client] = r.Time.ToTime()
		}
	}
	toSample := func(r recordRow, t time.Time) Sample {
		return Sample{Time: t, TotalUp: r.NetTotalUp, TotalDown: r.NetTotalDown, SpeedUp: r.NetOut, SpeedDown: r.NetIn}
	}
	for _, r := range longTerm {
		// 长期表一行代表一个 15 分钟桶，时间是桶起点，累计值取自桶内最后一条
		t := r.Time.ToTime().Add(14 * time.Minute)
		if first, ok := firstRecent[r.Client]; ok && !t.Before(first) {
			continue
		}
		samples[r.Client] = append(samples[r.Client], toSample(r, t))
	}
	for _, r := range recent {
		samples[r.Client] = append(samples[r.Client], toSample(r, r.Time.ToTime()))
	}

	daily := map[dayKey]amount{}
	cursors := map[string]Sample{}
	for client, list := range samples {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Time.Before(list[j].Time) })
		for i := 1; i < len(list); i++ {
			for _, part := range SplitByDay(ComputeDelta(list[i-1], list[i]), loc) {
				k := dayKey{client: client, day: part.Day}
				v := daily[k]
				v.up += part.Up
				v.down += part.Down
				daily[k] = v
			}
		}
		cursors[client] = list[len(list)-1]
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for k, v := range daily {
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
	if err == nil {
		log.Printf("traffic: backfilled %d day rows for %d clients from load records", len(daily), len(cursors))
	}
	return err
}
