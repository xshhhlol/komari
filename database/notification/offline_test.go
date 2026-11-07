package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/komari-monitor/komari/database/models"
)

func newOfflineNotificationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.OfflineNotification{}))
	return db
}

func mutedOf(t *testing.T, db *gorm.DB, client string) bool {
	t.Helper()
	var row models.OfflineNotification
	require.NoError(t, db.Where("client = ?", client).First(&row).Error)
	return row.CnBlockedMuted
}

// 加列前就存在的行，迁移后照常发被墙通知——与加开关前"所有节点都发"一致。
func TestCnBlockedMutedColumnDefaultsOffForExistingRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE offline_notifications (
		client varchar(36) NOT NULL UNIQUE,
		enable boolean DEFAULT false,
		grace_period int NOT NULL DEFAULT 180,
		last_notified datetime
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO offline_notifications (client, enable) VALUES ('old', true)`).Error)

	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.OfflineNotification{}))
	assert.False(t, mutedOf(t, db, "old"))
}

func TestSetCnBlockedMutedTogglesOnlyThatColumn(t *testing.T) {
	db := newOfflineNotificationDB(t)
	require.NoError(t, upsertFlag(db, []string{"a"}, "enable", func(n *models.OfflineNotification) { n.Enable = true }))

	// 已有行：只改被墙开关；没有行：新建。
	require.NoError(t, setCnBlockedMuted(db, []string{"a", "b"}, true))
	assert.True(t, mutedOf(t, db, "a"))
	assert.True(t, mutedOf(t, db, "b"))

	var a models.OfflineNotification
	require.NoError(t, db.Where("client = ?", "a").First(&a).Error)
	assert.True(t, a.Enable, "offline switch must be untouched")
	assert.Equal(t, 180, a.GracePeriod)

	// 取消静音必须真的写进去（default:true 的"启用"字段在 gorm 下就关不掉，这里防回归）。
	require.NoError(t, setCnBlockedMuted(db, []string{"a"}, false))
	assert.False(t, mutedOf(t, db, "a"))
	assert.True(t, mutedOf(t, db, "b"))

	// 只开关离线通知时，新建行照常发被墙通知。
	require.NoError(t, upsertFlag(db, []string{"c"}, "enable", func(n *models.OfflineNotification) { n.Enable = true }))
	assert.False(t, mutedOf(t, db, "c"))
}

func TestEditOfflineNotificationsCnBlockedField(t *testing.T) {
	db := newOfflineNotificationDB(t)
	yes, no := true, false

	// 新建行且没带 cn_blocked_muted：照常通知。
	require.NoError(t, editOfflineNotifications(db, []OfflineNotificationEdit{{Client: "a", Enable: true, GracePeriod: 300}}))
	assert.False(t, mutedOf(t, db, "a"))

	require.NoError(t, editOfflineNotifications(db, []OfflineNotificationEdit{
		{Client: "a", Enable: true, GracePeriod: 300, CnBlockedMuted: &yes},
		{Client: "b", Enable: false, GracePeriod: 60, CnBlockedMuted: &yes},
	}))
	assert.True(t, mutedOf(t, db, "a"))
	assert.True(t, mutedOf(t, db, "b"))

	// 不带该字段的编辑（老调用方）不能顺手改动被墙通知开关。
	require.NoError(t, editOfflineNotifications(db, []OfflineNotificationEdit{{Client: "a", Enable: false, GracePeriod: 120}}))
	var a models.OfflineNotification
	require.NoError(t, db.Where("client = ?", "a").First(&a).Error)
	assert.True(t, a.CnBlockedMuted)
	assert.False(t, a.Enable)
	assert.Equal(t, 120, a.GracePeriod)

	require.NoError(t, editOfflineNotifications(db, []OfflineNotificationEdit{{Client: "b", Enable: false, GracePeriod: 60, CnBlockedMuted: &no}}))
	assert.False(t, mutedOf(t, db, "b"))

	assert.Error(t, editOfflineNotifications(db, nil))
	assert.Error(t, editOfflineNotifications(db, []OfflineNotificationEdit{{Client: "", GracePeriod: 60}}))
	assert.Error(t, editOfflineNotifications(db, []OfflineNotificationEdit{{Client: "a", GracePeriod: 0}}))
}
