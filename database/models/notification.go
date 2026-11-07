package models

// Notification 定义了通知相关的数据库模型
type OfflineNotification struct {
	Client     string `json:"client" gorm:"type:varchar(36);not null;index;unique;constraint:OnDelete:CASCADE,OnUpdate:CASCADE;foreignKey:client;references:UUID"`
	ClientInfo Client `json:"client_info,omitempty" gorm:"foreignKey:Client;references:UUID"`
	Enable     bool   `json:"enable" gorm:"type:boolean;default:false"`
	//Cooldown     int       `json:"cooldown" gorm:"type:int;not null;default:1800"`                // 冷却时间（秒），默认 30 分钟
	GracePeriod  int       `json:"grace_period" gorm:"type:int;not null;default:180"` // 宽限期（秒），默认 3 分钟
	LastNotified LocalTime `json:"last_notified"`                                     // 上次通知时间
	// CnBlockedMuted 为 true 时不发该节点的被墙 / 恢复 / 被墙期间掉线通知，与上面的离线通知开关相互独立。
	// 默认 false（照常通知），与加开关之前"所有节点都发"一致；状态经常抖动的特殊节点可单独静音。
	// 全局开关（cn_blocked_notification_enabled）仍是总闸。
	//
	// 用"静音"而不是 default:true 的"启用"：gorm 建行时会把 default:true 字段上的 false 改写成 true
	//（即使 Select 了这一列），upsert 关不掉；零值即默认就没有这个坑。
	CnBlockedMuted bool `json:"cn_blocked_muted" gorm:"type:boolean;not null;default:false"`
}

// LoadNotification 定义了基于资源占用达标时间比的负载通知规则
type LoadNotification struct {
	Id           uint        `json:"id,omitempty" gorm:"primaryKey;autoIncrement"`
	Name         string      `json:"name" gorm:"type:varchar(255)"`
	Clients      StringArray `json:"clients" gorm:"type:longtext"`
	Metric       string      `json:"metric" gorm:"type:varchar(50);not null;default:'cpu'"`     // 监控指标，如 cpu, ram, load
	Threshold    float32     `json:"threshold" gorm:"type:decimal(5,2);not null;default:80.00"` // 阈值百分比
	Ratio        float32     `json:"ratio" gorm:"type:decimal(5,2);not null;default:0.80"`      // 达标时间比
	Interval     int         `json:"interval" gorm:"type:int;not null;default:15"`              // 监测间隔（分钟）
	LastNotified LocalTime   `json:"last_notified"`                                             // 上次通知时间
}

// TrafficReportNotification 定义了流量定时报告的数据库模型
type TrafficReportNotification struct {
	Client     string `json:"client" gorm:"type:varchar(36);not null;index;unique;constraint:OnDelete:CASCADE,OnUpdate:CASCADE;foreignKey:client;references:UUID"`
	ClientInfo Client `json:"client_info,omitempty" gorm:"foreignKey:Client;references:UUID"`
	Enable     bool   `json:"enable" gorm:"type:boolean;default:false"`
	Daily      bool   `json:"daily" gorm:"type:boolean;default:false"`  // 日报
	Weekly     bool   `json:"weekly" gorm:"type:boolean;default:false"` // 周报
	Monthly    bool   `json:"monthly" gorm:"type:boolean;default:false"` // 月报
}
