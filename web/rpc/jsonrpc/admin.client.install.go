package jsonrpc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/pkg/rpc"
	"golang.org/x/crypto/ssh"
)

// admin.client.install.go
// 通过 SSH 自动安装 agent（v1：仅密码认证、默认 root、22 端口、仅 Linux、同步返回完整日志）。
// 面板主动连到目标机执行与「生成命令」相同的安装脚本。密码仅在内存中使用，绝不入库/写日志。

// agent 安装脚本地址（与前端「生成命令」保持一致，指向 xshhhlol 的 fork）。
const agentInstallScriptURL = "https://raw.githubusercontent.com/xshhhlol/komari-agent/refs/heads/main/install.sh"

const (
	sshDialTimeout = 15 * time.Second
	sshExecTimeout = 180 * time.Second
)

func adminInstallClientViaSSH(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID     string `json:"uuid"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		Endpoint string `json:"endpoint"` // 前端传入的面板地址（= window.location.origin），仅在未配置脚本域名时作为回退
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid params", nil)
	}

	params.UUID = strings.TrimSpace(params.UUID)
	params.Host = strings.TrimSpace(params.Host)
	if params.UUID == "" || params.Host == "" || params.Password == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "uuid, host and password are required", nil)
	}
	if params.Port <= 0 {
		params.Port = 22
	}
	if strings.TrimSpace(params.Username) == "" {
		params.Username = "root"
	}

	token, err := clients.GetClientTokenByUUID(params.UUID)
	if err != nil || token == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Client not found", nil)
	}

	endpoint := resolvePanelEndpoint(params.Endpoint)
	if endpoint == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "panel endpoint is empty; set script domain in settings or pass endpoint", nil)
	}

	installCmd := buildAgentInstallCommand(endpoint, token)
	logOut, exitCode, sshErr := runSSHCommand(
		fmt.Sprintf("%s:%d", params.Host, params.Port),
		params.Username, params.Password, installCmd,
	)

	// 审计：只记录目标主机与结果，绝不记录密码。
	actor, ip := auditActor(ctx)
	status := "info"
	if sshErr != nil || exitCode != 0 {
		status = "warn"
	}
	auditlog.Log(ip, actor,
		fmt.Sprintf("ssh-install client:%s host:%s exit:%d", params.UUID, params.Host, exitCode), status)

	resp := map[string]any{
		"success":  sshErr == nil && exitCode == 0,
		"exitCode": exitCode,
		"log":      logOut,
	}
	if sshErr != nil {
		resp["error"] = sshErr.Error()
	}
	return resp, nil
}

// resolvePanelEndpoint 决定 agent 上报用的面板地址：优先后台配置的脚本域名，否则用前端传入的 origin。
func resolvePanelEndpoint(fromClient string) string {
	normalize := func(s string) string {
		s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "/"))
		if s == "" {
			return ""
		}
		if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			s = "http://" + s
		}
		return s
	}
	if domain, _ := config.GetAs[string](config.ScriptDomainKey, ""); strings.TrimSpace(domain) != "" {
		return normalize(domain)
	}
	return normalize(fromClient)
}

// buildAgentInstallCommand 组装安装命令：curl 优先、wget 兜底；root 直接 bash（无需 sudo）。
// endpoint/token 以单引号包裹并转义，避免命令注入。
func buildAgentInstallCommand(endpoint, token string) string {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	url := q(agentInstallScriptURL)
	return fmt.Sprintf(
		"if command -v curl >/dev/null 2>&1; then curl -fsSL %s; else wget -qO- %s; fi | bash -s -- -e %s -t %s",
		url, url, q(endpoint), q(token),
	)
}

// lockedBuffer 是并发安全的写缓冲：ssh 库会用独立 goroutine 同时写 stdout/stderr。
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// runSSHCommand 用密码认证连到目标机执行命令，返回合并输出、退出码与错误。
func runSSHCommand(addr, user, password, cmd string) (string, int, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // v1：首连无法验证主机指纹，盲信
		Timeout:         sshDialTimeout,
	}
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return "", -1, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	out := &lockedBuffer{}
	session.Stdout = out
	session.Stderr = out

	if err := session.Start(cmd); err != nil {
		return "", -1, fmt.Errorf("启动安装命令失败: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	select {
	case werr := <-done:
		exitCode := 0
		if werr != nil {
			if ee, ok := werr.(*ssh.ExitError); ok {
				exitCode = ee.ExitStatus()
			} else {
				exitCode = -1
			}
		}
		return out.String(), exitCode, nil
	case <-time.After(sshExecTimeout):
		_ = session.Signal(ssh.SIGKILL)
		return out.String() + "\n[timeout] 安装超时，已中断", -1, fmt.Errorf("安装超时（%s）", sshExecTimeout)
	}
}
