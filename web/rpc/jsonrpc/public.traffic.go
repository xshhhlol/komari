package jsonrpc

import (
	"context"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/traffic"
	"github.com/komari-monitor/komari/pkg/rpc"
	"sort"
	"strconv"
	"time"
)

func init() {
	regPublic("getTrafficDaily", publicTrafficDaily, "Daily traffic usage")
	regPublic("getTrafficRanking", publicTrafficRanking, "Traffic ranking")
}
func publicTrafficDaily(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var p struct {
		UUID string `json:"uuid"`
		Days string `json:"days"`
	}
	req.BindParams(&p)
	days := 30
	if p.Days != "" {
		n, e := strconv.Atoi(p.Days)
		if e != nil || n < 1 || n > 366 {
			return nil, rpc.MakeError(rpc.InvalidParams, "Invalid days", nil)
		}
		days = n
	}
	if p.UUID == "" || (!isLoginFromCtx(ctx) && isHiddenClient(p.UUID)) {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid UUID", nil)
	}
	now := time.Now().In(models.GetAppLocation())
	rows, err := traffic.GetDaily(p.UUID, now.AddDate(0, 0, 1-days).Format(time.DateOnly), now.Format(time.DateOnly))
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Traffic query failed", nil)
	}
	return map[string]any{"records": rows}, nil
}
func publicTrafficRanking(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var p struct {
		Granularity string `json:"granularity"`
		Offset      string `json:"offset"`
	}
	req.BindParams(&p)
	offset := 0
	if p.Offset != "" {
		n, e := strconv.Atoi(p.Offset)
		if e != nil || n > 0 || n < -366 {
			return nil, rpc.MakeError(rpc.InvalidParams, "Invalid offset", nil)
		}
		offset = n
	}
	if p.Granularity != "" && p.Granularity != "day" && p.Granularity != "week" && p.Granularity != "month" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid granularity", nil)
	}
	period := traffic.ResolvePeriod(p.Granularity, offset, time.Now(), models.GetAppLocation())
	usage, err := traffic.GetUsageByClient(period.Start, period.End)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Traffic query failed", nil)
	}
	var nodes []models.Client
	if err := dbcore.GetDBInstance().Find(&nodes).Error; err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Node query failed", nil)
	}
	type entry struct {
		IPv4 string `json:"ipv4,omitempty"`
		IPv6 string `json:"ipv6,omitempty"`
		UUID string `json:"uuid"`
		Name string `json:"name"`
		Up   int64  `json:"up"`
		Down int64  `json:"down"`
	}
	rows := make([]entry, 0, len(nodes))
	for _, n := range nodes {
		if !isLoginFromCtx(ctx) && isHiddenClient(n.UUID) {
			continue
		}
		u := usage[n.UUID]
		item := entry{UUID: n.UUID, Name: n.Name, Up: u.Up, Down: u.Down}
		if isLoginFromCtx(ctx) {
			item.IPv4, item.IPv6 = n.IPv4, n.IPv6
		}
		rows = append(rows, item)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Up+a.Down == b.Up+b.Down {
			return a.UUID < b.UUID
		}
		return a.Up+a.Down > b.Up+b.Down
	})
	return map[string]any{"period": period, "records": rows}, nil
}
