package traffic

import (
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestFlushAndResume(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.TrafficDaily{}, &models.TrafficCursor{}); err != nil {
		t.Fatal(err)
	}
	makeAcc := func() *Accumulator {
		return NewAccumulator(func() *gorm.DB { return db }, func() *time.Location { return time.UTC })
	}
	a := makeAcc()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	a.Observe("node", Sample{Time: now, TotalUp: 100, Uptime: 1000})
	a.Observe("node", Sample{Time: now.Add(time.Second), TotalUp: 150, Uptime: 1001})
	if err = a.Flush(); err != nil {
		t.Fatal(err)
	}
	a = makeAcc()
	a.Observe("node", Sample{Time: now.Add(2 * time.Second), TotalUp: 175, Uptime: 1002})
	u, err := getClientUsage(db, a, "node", "2026-11-15", "2026-11-15")
	if err != nil || u.Up != 75 {
		t.Fatalf("usage %+v, error %v", u, err)
	}
	if err = a.Flush(); err != nil {
		t.Fatal(err)
	}
	u, err = getClientUsage(db, a, "node", "2026-11-15", "2026-11-15")
	if err != nil || u.Up != 75 {
		t.Fatalf("usage after flush %+v, error %v", u, err)
	}
	if err = a.Forget("node"); err != nil {
		t.Fatal(err)
	}
	u, err = getClientUsage(db, a, "node", "2026-11-15", "2026-11-15")
	if err != nil || u.Up != 0 {
		t.Fatalf("deleted usage %+v, error %v", u, err)
	}
}
