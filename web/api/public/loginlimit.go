package public

import (
	"time"

	"github.com/patrickmn/go-cache"
)

// loginlimit.go
// 基于来源 IP 的登录失败限流 / 锁定。进程内（patrickmn/go-cache）实现：
// 单实例 + SQLite 部署足够；多副本部署计数不共享，如有需要再换共享存储。

const (
	loginMaxFailures   = 5                // 窗口内允许的连续失败次数，超过即锁定
	loginFailureWindow = 15 * time.Minute // 计数有效期 / 锁定时长（从最后一次失败起算）
)

// loginFailures 以来源 IP 为 key 记录失败次数；超时自动清零。
var loginFailures = cache.New(loginFailureWindow, 30*time.Minute)

func loginFailKey(ip string) string { return "login_fail:" + ip }

// isLoginLocked 判断该来源 IP 是否因失败过多而处于锁定状态。
func isLoginLocked(ip string) bool {
	if ip == "" {
		return false
	}
	v, found := loginFailures.Get(loginFailKey(ip))
	if !found {
		return false
	}
	n, ok := v.(int)
	return ok && n >= loginMaxFailures
}

// recordLoginFailure 记录一次失败，并把锁定窗口刷新到从本次失败起算。
func recordLoginFailure(ip string) {
	if ip == "" {
		return
	}
	key := loginFailKey(ip)
	n := 0
	if v, found := loginFailures.Get(key); found {
		if cur, ok := v.(int); ok {
			n = cur
		}
	}
	loginFailures.Set(key, n+1, loginFailureWindow)
}

// resetLoginFailures 登录成功后清除该 IP 的失败计数。
func resetLoginFailures(ip string) {
	if ip == "" {
		return
	}
	loginFailures.Delete(loginFailKey(ip))
}
