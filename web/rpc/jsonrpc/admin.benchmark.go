package jsonrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/pkg/rpc"
)

const benchmarkSettingsKey = "nodequality_benchmark_settings"

type benchmarkSettings struct {
	Hardware  string `json:"hardware"`
	IP        bool   `json:"ip"`
	Network   string `json:"network"`
	Backroute bool   `json:"backroute"`
}

func defaultBenchmarkSettings() benchmarkSettings {
	return benchmarkSettings{Hardware: "f", IP: true, Network: "y", Backroute: false}
}
func validateBenchmarkSettings(s benchmarkSettings) error {
	if s.Hardware != "y" && s.Hardware != "f" && s.Hardware != "v" && s.Hardware != "n" {
		return fmt.Errorf("invalid hardware mode")
	}
	if s.Network != "y" && s.Network != "l" && s.Network != "n" {
		return fmt.Errorf("invalid network mode")
	}
	if s.Hardware == "n" && !s.IP && s.Network == "n" && !s.Backroute {
		return fmt.Errorf("select at least one test")
	}
	return nil
}
func init() {
	reg("getBenchmarkSettings", adminGetBenchmarkSettings, "Get benchmark settings")
	reg("setBenchmarkSettings", adminSetBenchmarkSettings, "Save benchmark settings")
	reg("getBenchmarkCommand", adminGetBenchmarkCommand, "Build benchmark command")
}
func adminGetBenchmarkSettings(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	settings, err := config.GetAs[benchmarkSettings](benchmarkSettingsKey, defaultBenchmarkSettings())
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to load benchmark settings", err.Error())
	}
	if err := validateBenchmarkSettings(settings); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Invalid benchmark settings", err.Error())
	}
	return settings, nil
}
func adminSetBenchmarkSettings(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var settings benchmarkSettings
	if err := req.BindParams(&settings); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid benchmark settings", err.Error())
	}
	if err := validateBenchmarkSettings(settings); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid benchmark settings", err.Error())
	}
	if err := config.Set(benchmarkSettingsKey, settings); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to save benchmark settings", err.Error())
	}
	return settings, nil
}

func adminGetBenchmarkCommand(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	value, rpcErr := adminGetBenchmarkSettings(ctx, req)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return map[string]string{"command": buildBenchmarkCommand(value.(benchmarkSettings))}, nil
}

func buildBenchmarkCommand(s benchmarkSettings) string {
	ip, backroute := "n", "n"
	files := []string{"header_info.log"}
	if s.Hardware != "n" {
		files = append(files, "hardware_quality.log")
	}
	if s.IP {
		ip = "y"
		files = append(files, "ip_quality.log")
	}
	if s.Network != "n" {
		files = append(files, "net_quality.log")
	}
	if s.Backroute {
		backroute = "y"
		files = append(files, "backroute_trace.log")
	}
	replacements := strings.NewReplacer("__FILES__", strings.Join(files, " "), "__HARDWARE__", s.Hardware, "__IP__", ip, "__NETWORK__", s.Network, "__BACKROUTE__", backroute)
	return replacements.Replace(benchmarkCommand)
}

const benchmarkCommand = `# komari-nodequality-report-v1
set -eu
[ "$(uname -s)" = Linux ] || { echo '仅支持 Linux'; exit 1; }
[ "$(id -u)" = 0 ] || { echo '需要 root 权限'; exit 1; }
command -v flock >/dev/null && command -v timeout >/dev/null || { echo '需要 flock 和 timeout'; exit 1; }
exec 9>/tmp/komari-nodequality.lock
flock -n 9 || { echo '此节点已有测评正在运行'; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsSL --connect-timeout 30 --max-time 120 https://raw.githubusercontent.com/LloydAsp/NodeQuality/a92fca6c0067df29ddd03fdc2fee6f3000f64545/NodeQuality.sh -o "$work/source"
[ "$(tail -n 1 "$work/source")" = main ] || { echo 'NodeQuality 入口不匹配'; exit 1; }
sed '$d' "$work/source" > "$work/run"
cat >> "$work/run" <<'KOMARI_SCRIPT'
eval "$(declare -f upload_result | sed '1s/upload_result/original_upload_result/')"
function upload_result() {
  echo '--- KOMARI NODEQUALITY REPORT BEGIN ---'
  for file in __FILES__; do
    printf '\n--- %s ---\n' "$file"
    if [ -s "$result_directory/$file" ]; then cat "$result_directory/$file"; else echo '[结果文件缺失或为空]'; fi
  done
  echo '--- KOMARI NODEQUALITY REPORT END ---'
  echo '--- NODEQUALITY UPLOAD BEGIN ---'
  original_upload_result
  echo '--- NODEQUALITY UPLOAD END ---'
}
main
KOMARI_SCRIPT
printf '__HARDWARE__\n__IP__\n__NETWORK__\n__BACKROUTE__\n' | timeout -s TERM -k 120 7000 bash "$work/run" -d /tmp
`
