package eventlog

import (
	"log"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
)

// Serialize inserts so a reader cannot advance its cursor past an uncommitted lower ID.
var writeMu sync.Mutex

func Record(event models.EventMessage) {
	rows := make([]models.HistoryEvent, 0, len(event.Clients))
	base := models.HistoryEvent{Type: event.Event, OccurredAt: event.Time.UTC(), Message: event.Message}
	for _, client := range event.Clients {
		row := base
		row.Node = &models.EventNode{UUID: client.UUID, Name: client.Name, IPv4: client.IPv4, IPv6: client.IPv6}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		rows = append(rows, base)
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := dbcore.GetDBInstance().CreateInBatches(&rows, 50).Error; err != nil {
		log.Printf("Failed to record %s event: %v", event.Event, err)
	}
}

func NodeChange(kind string, client models.Client) {
	Record(models.EventMessage{Event: kind, Time: time.Now(), Clients: []models.Client{client}})
}
