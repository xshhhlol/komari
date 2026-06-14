package models

type PingRecord struct {
	Client     string    `json:"client" gorm:"type:varchar(36);not null;index"`
	ClientInfo Client    `json:"client_info" gorm:"foreignKey:Client;references:UUID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	TaskId     uint      `json:"task_id" gorm:"not null;index"`
	Task       PingTask  `json:"task" gorm:"foreignKey:TaskId;references:Id;constraint:OnDelete:CASCADE,OnUpdate:CASCADE;"`
	Time       LocalTime `json:"time" gorm:"index;not null"`
	Value      int       `json:"value" gorm:"type:int;not null"` // Ping 值，单位毫秒
}

// PingTask 表示一次延迟监测任务配置。
type PingTask struct {
	Id        uint        `json:"id,omitempty" gorm:"primaryKey;autoIncrement"`
	Weight    int         `json:"weight" gorm:"type:int;not null;default:0;index"`
	Name      string      `json:"name" gorm:"type:varchar(255);not null;index"`
	Clients   StringArray `json:"clients" gorm:"type:longtext"`
	DefaultOn bool        `json:"default_on" gorm:"column:all_clients;not null;default:false"` // 新加入的服务器是否自动开启此监测；现有服务器不受此字段影响
	Type      string      `json:"type" gorm:"type:varchar(12);not null;default:'icmp'"`        // icmp tcp http
	Target    string      `json:"target" gorm:"type:varchar(255);not null"`                    // Ping 目标地址
	Interval  int         `json:"interval" gorm:"type:int;not null;default:60"`                // 间隔时间
	BlockCheck bool       `json:"block_check" gorm:"column:block_check;not null;default:false"` // 是否作为"被墙"判定的国内参照目标：纳入的任务若对某节点全部超时，则该节点判为被墙
}

// AppliesToClient 判断当前 PingTask 是否适用于指定服务器。
func (task PingTask) AppliesToClient(uuid string) bool {
	if uuid == "" {
		return false
	}
	for _, client := range task.Clients {
		if client == uuid {
			return true
		}
	}
	return false
}
