package notification

import (
	"fmt"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 节点通知配置（offline_notifications 表，每个节点一行）：离线 / 上线通知 + 被墙通知。

// OfflineNotificationEdit 是后台"编辑"提交的一行。CnBlockedMuted 为指针：
// 不带这个字段的调用方（老前端、脚本）不会顺手改动被墙通知开关。
type OfflineNotificationEdit struct {
	Client         string `json:"client"`
	Enable         bool   `json:"enable"`
	GracePeriod    int    `json:"grace_period"`
	CnBlockedMuted *bool  `json:"cn_blocked_muted"`
}

// EditOfflineNotifications 批量写入离线通知开关与宽限期；带了 cn_blocked_muted 的行一并更新被墙通知开关。
func EditOfflineNotifications(edits []OfflineNotificationEdit) error {
	return editOfflineNotifications(dbcore.GetDBInstance(), edits)
}

// ValidateOfflineNotificationEdits 校验"编辑"提交：至少一行，每行有节点 UUID 且宽限期为正。
func ValidateOfflineNotificationEdits(edits []OfflineNotificationEdit) error {
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

func editOfflineNotifications(db *gorm.DB, edits []OfflineNotificationEdit) error {
	if err := ValidateOfflineNotificationEdits(edits); err != nil {
		return err
	}
	rows := make([]models.OfflineNotification, 0, len(edits))
	var muted, unmuted []string
	for _, e := range edits {
		rows = append(rows, models.OfflineNotification{Client: e.Client, Enable: e.Enable, GracePeriod: e.GracePeriod})
		if e.CnBlockedMuted != nil {
			if *e.CnBlockedMuted {
				muted = append(muted, e.Client)
			} else {
				unmuted = append(unmuted, e.Client)
			}
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 冲突时只更新这两列：没带 cn_blocked_muted 的行保留原有的被墙通知开关。
		if err := tx.Model(&models.OfflineNotification{}).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "client"}},
				DoUpdates: clause.AssignmentColumns([]string{"enable", "grace_period"}),
			}).
			Select("*").
			Create(&rows).Error; err != nil {
			return err
		}
		if err := setCnBlockedMuted(tx, muted, true); err != nil {
			return err
		}
		return setCnBlockedMuted(tx, unmuted, false)
	})
}

// SetOfflineNotificationEnable 批量开关节点的离线 / 上线通知；没有配置行的节点新建一行（其余列取默认值）。
func SetOfflineNotificationEnable(uuids []string, enable bool) error {
	if err := validateUUIDs(uuids); err != nil {
		return err
	}
	return upsertFlag(dbcore.GetDBInstance(), uuids, "enable", func(n *models.OfflineNotification) { n.Enable = enable })
}

// SetCnBlockedNotificationEnable 批量开关节点的被墙通知；没有配置行的节点新建一行（其余列取默认值）。
func SetCnBlockedNotificationEnable(uuids []string, enable bool) error {
	if err := validateUUIDs(uuids); err != nil {
		return err
	}
	return setCnBlockedMuted(dbcore.GetDBInstance(), uuids, !enable)
}

func setCnBlockedMuted(db *gorm.DB, uuids []string, muted bool) error {
	if len(uuids) == 0 {
		return nil
	}
	return upsertFlag(db, uuids, "cn_blocked_muted", func(n *models.OfflineNotification) { n.CnBlockedMuted = muted })
}

// upsertFlag 只写 column 这一列：已有的行原地更新，没有的行新建（其余列取数据库默认值）。
func upsertFlag(db *gorm.DB, uuids []string, column string, set func(*models.OfflineNotification)) error {
	rows := make([]models.OfflineNotification, 0, len(uuids))
	for _, uuid := range uuids {
		row := models.OfflineNotification{Client: uuid}
		set(&row)
		rows = append(rows, row)
	}
	return db.Model(&models.OfflineNotification{}).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "client"}},
			DoUpdates: clause.AssignmentColumns([]string{column}),
		}).
		Select("client", column).
		Create(&rows).Error
}

func validateUUIDs(uuids []string) error {
	if len(uuids) == 0 {
		return fmt.Errorf("at least one client UUID is required")
	}
	for _, uuid := range uuids {
		if uuid == "" {
			return fmt.Errorf("client UUID cannot be empty")
		}
	}
	return nil
}
