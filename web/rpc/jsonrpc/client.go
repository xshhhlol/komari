package jsonrpc

import (
	"context"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// client.go
// 客户端（agent）面向的 RPC2 方法（client 命名空间，client/admin 可调用）。
// 这些方法依赖 meta.ClientUUID 标识调用方客户端。

func init() {
	regClient("getPingTasks", clientGetPingTasks, "Get ping tasks assigned to the calling client")
	regClient("uploadPingResult", clientUploadPingResult, "Upload a ping result")
	regClient("taskResult", clientTaskResult, "Upload an exec task result")
}

func regClient(name string, h rpc.Handler, summary string) {
	RegisterWithGroupAndMeta(name, rpc.RoleClient, h, &rpc.MethodMeta{Name: "client:" + name, Summary: summary})
}

// callingClientUUID 返回发起调用的客户端 UUID。
func callingClientUUID(ctx context.Context) string {
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		return meta.ClientUUID
	}
	return ""
}

func clientGetPingTasks(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	uuid := callingClientUUID(ctx)
	if uuid == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "client_uuid not found", nil)
	}
	return tasks.GetPingTasksByClient(uuid), nil
}

func clientUploadPingResult(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	uuid := callingClientUUID(ctx)
	if uuid == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "client_uuid not found", nil)
	}
	var params struct {
		TaskID     uint      `json:"task_id"`
		Value      int       `json:"value"`
		PingType   string    `json:"ping_type"`
		FinishedAt time.Time `json:"finished_at"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request: "+err.Error(), nil)
	}
	record := models.PingRecord{
		Client: uuid,
		TaskId: params.TaskID,
		Value:  params.Value,
		Time:   models.FromTime(params.FinishedAt),
	}
	if err := tasks.SavePingRecord(record); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to save ping result: "+err.Error(), nil)
	}
	return map[string]any{"status": "success"}, nil
}

func clientTaskResult(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	uuid := callingClientUUID(ctx)
	if uuid == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing token", nil)
	}
	var params struct {
		TaskId     string     `json:"task_id"`
		Result     string     `json:"result"`
		ExitCode   *int       `json:"exit_code"`
		FinishedAt *time.Time `json:"finished_at"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request", nil)
	}
	// finished_at 为空 = 进行中的增量输出：只更新 result，保持「未完成」状态，
	// 前端轮询即可看到输出滚动增长（近实时）。
	if params.FinishedAt == nil {
		if err := tasks.UpdateTaskResultProgress(params.TaskId, uuid, params.Result); err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to update task progress: "+err.Error(), nil)
		}
		return map[string]any{"status": "success", "message": "Task progress updated"}, nil
	}
	exitCode := 0
	if params.ExitCode != nil {
		exitCode = *params.ExitCode
	}
	if err := tasks.SaveTaskResult(params.TaskId, uuid, params.Result, exitCode, models.FromTime(*params.FinishedAt)); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to update task result: "+err.Error(), nil)
	}
	return map[string]any{"status": "success", "message": "Task result updated successfully"}, nil
}
