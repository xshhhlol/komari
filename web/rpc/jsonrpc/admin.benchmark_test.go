package jsonrpc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/komari-monitor/komari/pkg/rpc"
)

func TestBenchmarkSettingsAndCommand(t *testing.T) {
	s := benchmarkSettings{Hardware: "n", IP: true, Network: "l", Backroute: true}
	if err := validateBenchmarkSettings(s); err != nil {
		t.Fatal(err)
	}
	command := buildBenchmarkCommand(s)
	for _, expected := range []string{"ip_quality.log", "net_quality.log", "backroute_trace.log", `printf 'n\ny\nl\ny\n'`, "original_upload_result"} {
		if !strings.Contains(command, expected) {
			t.Errorf("command missing %q", expected)
		}
	}
	if strings.Contains(command, "hardware_quality.log") {
		t.Error("disabled hardware included in report")
	}
	for _, bad := range []benchmarkSettings{{Hardware: "n", Network: "n"}, {Hardware: "f;id", IP: true, Network: "y"}, {Hardware: "f", IP: true, Network: "evil"}} {
		if validateBenchmarkSettings(bad) == nil {
			t.Errorf("accepted invalid settings: %+v", bad)
		}
	}
}

func TestBenchmarkMethodsRequireAdmin(t *testing.T) {
	for _, method := range []string{"admin:getBenchmarkSettings", "admin:setBenchmarkSettings", "admin:getBenchmarkCommand", "admin:getBenchmarkTasks"} {
		if rpc.CheckPermission(rpc.RoleGuest, method) {
			t.Errorf("guest can call %s", method)
		}
		if !rpc.CheckPermission(rpc.RoleAdmin, method) {
			t.Errorf("admin cannot call %s", method)
		}
	}
}

func TestBenchmarkWrapperUploadsAndCapturesReport(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"uname":   "#!/bin/sh\necho Linux\n",
		"id":      "#!/bin/sh\necho 0\n",
		"flock":   "#!/bin/sh\nexit 0\n",
		"timeout": "#!/bin/sh\nshift 5\nexec \"$@\"\n",
		"curl": `#!/bin/sh
while [ "$1" != '-o' ]; do shift; done
cat > "$2" <<'SCRIPT'
result_directory="$TMPDIR/results"
mkdir -p "$result_directory"
function upload_result() { echo 'https://nodequality.com/r/Test123'; }
function main() {
 read h; read i; read n; read b
 echo "choices=$h,$i,$n,$b"
 for f in header_info.log ip_quality.log net_quality.log backroute_trace.log; do echo "content-$f" > "$result_directory/$f"; done
 upload_result
 exit 1
}
main
SCRIPT
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", "-c", buildBenchmarkCommand(benchmarkSettings{Hardware: "n", IP: true, Network: "l", Backroute: true}))
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+dir)
	output, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("expected upstream exit 1, got %v: %s", err, output)
	}
	for _, want := range []string{"choices=n,y,l,y", "content-backroute_trace.log", "KOMARI NODEQUALITY REPORT END", "https://nodequality.com/r/Test123"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("missing %q in %s", want, output)
		}
	}
}
