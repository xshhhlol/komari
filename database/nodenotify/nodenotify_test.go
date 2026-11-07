package nodenotify

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/pkg/config"
)

// newDB 每个测试一个独立的内存库（命名 + shared cache，事务换连接也看得到同一份数据）。
func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.OfflineNotification{}))
	return db
}

func rowOf(t *testing.T, db *gorm.DB, client string) models.OfflineNotification {
	t.Helper()
	var row models.OfflineNotification
	require.NoError(t, db.Where("client = ?", client).First(&row).Error)
	return row
}

// 一套和内置默认处处相反的默认值，好看出某列到底取的是不是"新节点默认"。
var flipped = Defaults{OfflineEnabled: true, GracePeriod: 300, CnBlockedEnabled: false}

// 加列前就存在的行，迁移后照常发被墙通知——与加开关前"所有节点都发"一致。
func TestCnBlockedMutedColumnDefaultsOffForExistingRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:migrate?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE offline_notifications (
		client varchar(36) NOT NULL UNIQUE,
		enable boolean DEFAULT false,
		grace_period int NOT NULL DEFAULT 180,
		last_notified datetime
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO offline_notifications (client, enable) VALUES ('old', true)`).Error)

	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.OfflineNotification{}))
	assert.False(t, rowOf(t, db, "old").CnBlockedMuted)
}

func TestEnsureCreatesFromDefaultsOnce(t *testing.T) {
	db := newDB(t)

	row, err := ensure(db, flipped, "a")
	require.NoError(t, err)
	assert.True(t, row.Enable)
	assert.Equal(t, 300, row.GracePeriod)
	assert.True(t, row.CnBlockedMuted)

	// 已有行不受之后默认值变化影响。
	row, err = ensure(db, BuiltinDefaults, "a")
	require.NoError(t, err)
	assert.True(t, row.Enable)
	assert.Equal(t, 300, row.GracePeriod)
	assert.True(t, row.CnBlockedMuted)

	row, err = ensure(db, BuiltinDefaults, "b")
	require.NoError(t, err)
	assert.False(t, row.Enable)
	assert.Equal(t, 180, row.GracePeriod)
	assert.False(t, row.CnBlockedMuted)
}

func TestSetFlagNewRowsTakeDefaultsExistingRowsChangeOneColumn(t *testing.T) {
	db := newDB(t)

	// 新行：被墙开关按这次的改动，其余列取默认（离线通知开、宽限期 300）。
	require.NoError(t, setFlag(db, flipped, []string{"a"}, "cn_blocked_muted",
		func(n *models.OfflineNotification) { n.CnBlockedMuted = false }))
	a := rowOf(t, db, "a")
	assert.False(t, a.CnBlockedMuted)
	assert.True(t, a.Enable)
	assert.Equal(t, 300, a.GracePeriod)

	// 已有行：只改这一列，即使这次带的默认值不同。
	require.NoError(t, setFlag(db, BuiltinDefaults, []string{"a"}, "enable",
		func(n *models.OfflineNotification) { n.Enable = false }))
	a = rowOf(t, db, "a")
	assert.False(t, a.Enable)
	assert.False(t, a.CnBlockedMuted)
	assert.Equal(t, 300, a.GracePeriod)

	// 静音 / 取消静音都要真的写进去。
	require.NoError(t, setFlag(db, BuiltinDefaults, []string{"a", "b"}, "cn_blocked_muted",
		func(n *models.OfflineNotification) { n.CnBlockedMuted = true }))
	assert.True(t, rowOf(t, db, "a").CnBlockedMuted)
	assert.True(t, rowOf(t, db, "b").CnBlockedMuted)
	require.NoError(t, setFlag(db, BuiltinDefaults, []string{"a"}, "cn_blocked_muted",
		func(n *models.OfflineNotification) { n.CnBlockedMuted = false }))
	assert.False(t, rowOf(t, db, "a").CnBlockedMuted)

	assert.Error(t, setFlag(db, BuiltinDefaults, nil, "enable", func(*models.OfflineNotification) {}))
	assert.Error(t, setFlag(db, BuiltinDefaults, []string{""}, "enable", func(*models.OfflineNotification) {}))
}

func TestApplyEditsCnBlockedField(t *testing.T) {
	db := newDB(t)
	yes, no := true, false

	// 新行且没带 cn_blocked_muted：取默认。
	require.NoError(t, applyEdits(db, flipped, []Edit{{Client: "a", Enable: false, GracePeriod: 60}}))
	a := rowOf(t, db, "a")
	assert.True(t, a.CnBlockedMuted)
	assert.False(t, a.Enable)
	assert.Equal(t, 60, a.GracePeriod)

	require.NoError(t, applyEdits(db, BuiltinDefaults, []Edit{
		{Client: "a", Enable: true, GracePeriod: 300, CnBlockedMuted: &no},
		{Client: "b", Enable: false, GracePeriod: 60, CnBlockedMuted: &yes},
	}))
	assert.False(t, rowOf(t, db, "a").CnBlockedMuted)
	assert.True(t, rowOf(t, db, "b").CnBlockedMuted)

	// 不带该字段的编辑（老调用方）不能顺手改动被墙通知开关。
	require.NoError(t, applyEdits(db, flipped, []Edit{{Client: "b", Enable: true, GracePeriod: 120}}))
	b := rowOf(t, db, "b")
	assert.True(t, b.CnBlockedMuted)
	assert.True(t, b.Enable)
	assert.Equal(t, 120, b.GracePeriod)

	assert.Error(t, applyEdits(db, BuiltinDefaults, nil))
	assert.Error(t, applyEdits(db, BuiltinDefaults, []Edit{{Client: "", GracePeriod: 60}}))
	assert.Error(t, applyEdits(db, BuiltinDefaults, []Edit{{Client: "a", GracePeriod: 0}}))
}

func TestCnBlockedSwitchesFallBackToDefaultForNodesWithoutRow(t *testing.T) {
	db := newDB(t)
	require.NoError(t, db.Create(&models.OfflineNotification{Client: "on"}).Error)
	require.NoError(t, db.Create(&models.OfflineNotification{Client: "off", CnBlockedMuted: true}).Error)

	s, err := loadCnBlockedSwitches(db, BuiltinDefaults)
	require.NoError(t, err)
	assert.False(t, s.Muted("on"))
	assert.True(t, s.Muted("off"))
	assert.False(t, s.Muted("no-row"))

	s, err = loadCnBlockedSwitches(db, flipped)
	require.NoError(t, err)
	assert.False(t, s.Muted("on"), "a node's own setting wins over the default")
	assert.True(t, s.Muted("no-row"))

	assert.False(t, CnBlockedSwitches{}.Muted("x"), "zero value notifies everyone")
}

func TestLoadDefaultsReadsSettings(t *testing.T) {
	db := newDB(t)
	config.SetDb(db)

	assert.Equal(t, BuiltinDefaults, LoadDefaults(), "missing keys fall back to built-in defaults")

	require.NoError(t, config.SetMany(map[string]any{
		config.OfflineNotificationDefaultEnabledKey:     true,
		config.OfflineNotificationDefaultGracePeriodKey: 240, // 存成 JSON 后读回是 float64
		config.CnBlockedNotificationDefaultEnabledKey:   false,
	}))
	assert.Equal(t, Defaults{OfflineEnabled: true, GracePeriod: 240, CnBlockedEnabled: false}, LoadDefaults())

	// 非法宽限期退回内置值。
	require.NoError(t, config.Set(config.OfflineNotificationDefaultGracePeriodKey, 0))
	assert.Equal(t, BuiltinDefaults.GracePeriod, LoadDefaults().GracePeriod)
}
