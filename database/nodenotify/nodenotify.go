// Package nodenotify 管理每个节点的通知配置（offline_notifications 表，每个节点一行）：
// 离线 / 上线通知开关、宽限期、被墙通知开关，以及"新节点默认"。
//
// 还没有配置行的节点（新加的、或从没被设置过的）一律按"新节点默认"处理；配置行在第一次
// 需要时才落库（上线时、或后台改了它的任一开关时），落库时没指定的列也取默认值。
// 所以改默认只影响还没有配置行的节点，已经设置过的节点保持原样。
//
// 单独成包是为了让通知发送方（utils/notifier）和后台接口共用：database/notification
// 依赖 notifier，notifier 不能反过来依赖它。
package nodenotify

import (
	"errors"
	"fmt"
	"log"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/pkg/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Defaults 是新节点的通知配置。
type Defaults struct {
	OfflineEnabled   bool // 离线 / 上线通知
	GracePeriod      int  // 宽限期（秒）
	CnBlockedEnabled bool // 被墙通知
}

// BuiltinDefaults 与加"新节点默认"设置之前的行为一致：离线通知关、宽限期 180 秒、被墙通知开。
var BuiltinDefaults = Defaults{OfflineEnabled: false, GracePeriod: 180, CnBlockedEnabled: true}

// Row 按默认值构造一个节点的配置行。
func (d Defaults) Row(client string) models.OfflineNotification {
	grace := d.GracePeriod
	if grace <= 0 {
		grace = BuiltinDefaults.GracePeriod
	}
	return models.OfflineNotification{
		Client:         client,
		Enable:         d.OfflineEnabled,
		GracePeriod:    grace,
		CnBlockedMuted: !d.CnBlockedEnabled,
	}
}

// LoadDefaults 读取后台设置的新节点默认；读取失败时退回内置默认。
func LoadDefaults() Defaults {
	cfg, err := config.GetMany(map[string]any{
		config.OfflineNotificationDefaultEnabledKey:     BuiltinDefaults.OfflineEnabled,
		config.OfflineNotificationDefaultGracePeriodKey: BuiltinDefaults.GracePeriod,
		config.CnBlockedNotificationDefaultEnabledKey:   BuiltinDefaults.CnBlockedEnabled,
	})
	if err != nil {
		log.Printf("Failed to load node notification defaults, using built-in ones: %v", err)
		return BuiltinDefaults
	}
	d := BuiltinDefaults
	if v, ok := cfg[config.OfflineNotificationDefaultEnabledKey].(bool); ok {
		d.OfflineEnabled = v
	}
	if v, ok := toInt(cfg[config.OfflineNotificationDefaultGracePeriodKey]); ok && v > 0 {
		d.GracePeriod = v
	}
	if v, ok := cfg[config.CnBlockedNotificationDefaultEnabledKey].(bool); ok {
		d.CnBlockedEnabled = v
	}
	return d
}

// toInt 兼容配置里的数字：库里读出来是 float64，刚写入的默认值是 int。
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// Ensure 返回节点的配置行；没有则按默认值新建。
func Ensure(client string) (models.OfflineNotification, error) {
	return ensure(dbcore.GetDBInstance(), LoadDefaults(), client)
}

func ensure(db *gorm.DB, d Defaults, client string) (models.OfflineNotification, error) {
	var row models.OfflineNotification
	err := db.Where("client = ?", client).First(&row).Error
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return row, err
	}
	row = d.Row(client)
	// DoNothing：并发时别人先建了就用别人的，再读一次拿到库里真实的那行。
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Select("*").Create(&row).Error; err != nil {
		return row, err
	}
	err = db.Where("client = ?", client).First(&row).Error
	return row, err
}

// CnBlockedSwitches 是一次性读出的各节点被墙通知开关。
type CnBlockedSwitches struct {
	muted        map[string]bool // 有配置行的节点
	defaultMuted bool            // 没有配置行的节点
}

// Muted 报告该节点是否关闭了被墙通知。零值 CnBlockedSwitches 视为全部照常通知。
func (s CnBlockedSwitches) Muted(client string) bool {
	if m, ok := s.muted[client]; ok {
		return m
	}
	return s.defaultMuted
}

// LoadCnBlockedSwitches 读取所有节点的被墙通知开关。
func LoadCnBlockedSwitches() (CnBlockedSwitches, error) {
	return loadCnBlockedSwitches(dbcore.GetDBInstance(), LoadDefaults())
}

func loadCnBlockedSwitches(db *gorm.DB, d Defaults) (CnBlockedSwitches, error) {
	var rows []models.OfflineNotification
	if err := db.Model(&models.OfflineNotification{}).Select("client", "cn_blocked_muted").Find(&rows).Error; err != nil {
		return CnBlockedSwitches{}, err
	}
	s := CnBlockedSwitches{muted: make(map[string]bool, len(rows)), defaultMuted: !d.CnBlockedEnabled}
	for _, r := range rows {
		s.muted[r.Client] = r.CnBlockedMuted
	}
	return s, nil
}

// Edit 是后台"编辑"提交的一行。CnBlockedMuted 为指针：
// 不带这个字段的调用方（老前端、脚本）不会顺手改动被墙通知开关。
type Edit struct {
	Client         string `json:"client"`
	Enable         bool   `json:"enable"`
	GracePeriod    int    `json:"grace_period"`
	CnBlockedMuted *bool  `json:"cn_blocked_muted"`
}

// ValidateEdits 校验"编辑"提交：至少一行，每行有节点 UUID 且宽限期为正。
func ValidateEdits(edits []Edit) error {
	if len(edits) == 0 {
		return fmt.Errorf("at least one notification is required")
	}
	for _, e := range edits {
		if e.Client == "" {
			return fmt.Errorf("client UUID cannot be empty")
		}
		if e.GracePeriod <= 0 {
			return fmt.Errorf("grace_period must be a positive integer")
		}
	}
	return nil
}

// ApplyEdits 批量写入离线通知开关与宽限期；带了 cn_blocked_muted 的行一并更新被墙通知开关。
func ApplyEdits(edits []Edit) error {
	return applyEdits(dbcore.GetDBInstance(), LoadDefaults(), edits)
}

func applyEdits(db *gorm.DB, d Defaults, edits []Edit) error {
	if err := ValidateEdits(edits); err != nil {
		return err
	}
	rows := make([]models.OfflineNotification, 0, len(edits))
	var muted, unmuted []string
	for _, e := range edits {
		row := d.Row(e.Client)
		row.Enable = e.Enable
		row.GracePeriod = e.GracePeriod
		if e.CnBlockedMuted != nil {
			row.CnBlockedMuted = *e.CnBlockedMuted
			if *e.CnBlockedMuted {
				muted = append(muted, e.Client)
			} else {
				unmuted = append(unmuted, e.Client)
			}
		}
		rows = append(rows, row)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 新行整行写入（没带的列取默认）；已有行只更新这两列，没带 cn_blocked_muted 的保留原值。
		if err := tx.Model(&models.OfflineNotification{}).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "client"}},
				DoUpdates: clause.AssignmentColumns([]string{"enable", "grace_period"}),
			}).
			Select("*").
			Create(&rows).Error; err != nil {
			return err
		}
		if err := setColumn(tx, muted, "cn_blocked_muted", true); err != nil {
			return err
		}
		return setColumn(tx, unmuted, "cn_blocked_muted", false)
	})
}

func setColumn(db *gorm.DB, uuids []string, column string, value bool) error {
	if len(uuids) == 0 {
		return nil
	}
	return db.Model(&models.OfflineNotification{}).Where("client IN ?", uuids).Update(column, value).Error
}

// SetOfflineEnable 批量开关节点的离线 / 上线通知。
func SetOfflineEnable(uuids []string, enable bool) error {
	return setFlag(dbcore.GetDBInstance(), LoadDefaults(), uuids, "enable",
		func(n *models.OfflineNotification) { n.Enable = enable })
}

// SetCnBlockedEnable 批量开关节点的被墙通知。
func SetCnBlockedEnable(uuids []string, enable bool) error {
	return setFlag(dbcore.GetDBInstance(), LoadDefaults(), uuids, "cn_blocked_muted",
		func(n *models.OfflineNotification) { n.CnBlockedMuted = !enable })
}

// setFlag 只改 column 这一列：已有的行原地更新；没有的行按默认值新建，再套上这次的改动。
func setFlag(db *gorm.DB, d Defaults, uuids []string, column string, set func(*models.OfflineNotification)) error {
	if len(uuids) == 0 {
		return fmt.Errorf("at least one client UUID is required")
	}
	rows := make([]models.OfflineNotification, 0, len(uuids))
	for _, uuid := range uuids {
		if uuid == "" {
			return fmt.Errorf("client UUID cannot be empty")
		}
		row := d.Row(uuid)
		set(&row)
		rows = append(rows, row)
	}
	return db.Model(&models.OfflineNotification{}).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "client"}},
			DoUpdates: clause.AssignmentColumns([]string{column}),
		}).
		Select("*").
		Create(&rows).Error
}
