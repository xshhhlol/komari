package traffic

import (
	"testing"
	"time"
)

func TestCounterTransitions(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	prev := Sample{Time: now, TotalUp: 1000, TotalDown: 2000, Uptime: 10000}
	cases := []struct {
		name     string
		cur      Sample
		up, down int64
	}{
		{"normal", Sample{Time: now.Add(time.Second), TotalUp: 1100, TotalDown: 2200, Uptime: 10001}, 100, 200},
		{"reboot", Sample{Time: now.Add(time.Minute), TotalUp: 20, TotalDown: 30, Uptime: 10}, 20, 30},
		{"counter reset without reboot", Sample{Time: now.Add(time.Second), TotalUp: 20, TotalDown: 30, Uptime: 10001}, 0, 0},
		{"source jump", Sample{Time: now.Add(time.Second), TotalUp: 1 << 50, TotalDown: 1 << 50, Uptime: 10001}, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := ComputeDelta(prev, c.cur)
			if d.Up != c.up || d.Down != c.down {
				t.Fatalf("got %d/%d want %d/%d", d.Up, d.Down, c.up, c.down)
			}
		})
	}
}
func TestSplitConservesTotalsAcrossMidnight(t *testing.T) {
	from := time.Date(2026, 11, 14, 23, 59, 0, 0, time.UTC)
	parts := SplitByDay(Delta{From: from, To: from.Add(2 * time.Minute), Up: 101, Down: 203}, time.UTC)
	if len(parts) != 2 || parts[0].Day != "2026-11-14" || parts[1].Day != "2026-11-15" {
		t.Fatalf("unexpected parts: %+v", parts)
	}
	if parts[0].Up+parts[1].Up != 101 || parts[0].Down+parts[1].Down != 203 {
		t.Fatal("lost bytes")
	}
}
