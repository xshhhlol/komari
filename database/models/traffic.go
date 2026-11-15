package models

// TrafficDaily 每个节点每天的流量用量（字节）。
// Day 为应用时区（TZ 环境变量）下的自然日，格式 YYYY-MM-DD，字符串比较即日期比较。
type TrafficDaily struct {
	Client string `json:"client" gorm:"type:varchar(36);primaryKey"`
	Day    string `json:"day" gorm:"type:varchar(10);primaryKey;index"`
	Up     int64  `json:"up" gorm:"type:bigint;not null;default:0"`
	Down   int64  `json:"down" gorm:"type:bigint;not null;default:0"`
}

func (TrafficDaily) TableName() string { return "traffic_daily" }

// TrafficCursor 记录每个节点上一次参与流量统计的上报样本。
// 面板重启后以它为基线继续计算增量，停机期间的流量不会丢失。
type TrafficCursor struct {
	Client    string    `json:"client" gorm:"type:varchar(36);primaryKey"`
	Time      LocalTime `json:"time"`
	TotalUp   int64     `json:"total_up" gorm:"type:bigint"`
	TotalDown int64     `json:"total_down" gorm:"type:bigint"`
	Uptime    int64     `json:"uptime" gorm:"type:bigint"` // 系统开机秒数；0 表示未知
	SpeedUp   int64     `json:"speed_up" gorm:"type:bigint"`
	SpeedDown int64     `json:"speed_down" gorm:"type:bigint"`
}

func (TrafficCursor) TableName() string { return "traffic_cursors" }
