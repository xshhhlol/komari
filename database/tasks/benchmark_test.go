package tasks

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBenchmarkReportsSurviveTaskCleanup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Task{}, &models.TaskResult{}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []models.Task{{TaskId: "report", Command: BenchmarkCommandPrefix + "test"}, {TaskId: "ordinary", Command: "echo test"}} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.TaskResult{TaskId: task.TaskId, Client: "node", CreatedAt: models.FromTime(time.Now().Add(-48 * time.Hour))}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := clearTaskResultsByTimeBefore(db, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var remaining []models.TaskResult
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].TaskId != "report" {
		t.Fatalf("unexpected remaining results: %+v", remaining)
	}
}
