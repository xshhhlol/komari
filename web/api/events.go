package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
)

const EventFeedPath = "/api/events"
const eventKeyDigest = "e7a707a50544e553fb3b4dbcddc514cd32125b0ee18df29638018430160f3982"

func ReadEvents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	// Releases are public. Each installation explicitly opts in to this fixed credential.
	if os.Getenv("KOMARI_EVENT_FEED_ENABLED") != "true" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	digest := sha256.Sum256([]byte(token))
	if !ok || subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(eventKeyDigest)) != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	after, err := strconv.ParseUint(c.DefaultQuery("after_id", "0"), 10, 63)
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limitErr != nil || limit < 1 || limit > 500 {
		RespondError(c, http.StatusBadRequest, "Invalid after_id or limit (1-500)")
		return
	}
	rows := []models.HistoryEvent{}
	if err := dbcore.GetDBInstance().WithContext(c.Request.Context()).Where("id > ?", after).Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Event history unavailable")
		return
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	if len(rows) > 0 {
		after = rows[len(rows)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{"events": rows, "next_after_id": strconv.FormatUint(after, 10), "has_more": more})
}
