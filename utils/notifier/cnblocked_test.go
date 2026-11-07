package notifier

import (
	"testing"

	"github.com/komari-monitor/komari/database/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCnBlockedMutedClientsOnlyListsSwitchedOff(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.OfflineNotification{}))

	require.NoError(t, db.Create(&models.OfflineNotification{Client: "on"}).Error)
	require.NoError(t, db.Create(&models.OfflineNotification{Client: "off", CnBlockedMuted: true}).Error)

	muted, err := cnBlockedMutedClients(db)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"off": true}, muted)
	assert.False(t, muted["no-row"], "nodes without a config row keep notifying")
}
