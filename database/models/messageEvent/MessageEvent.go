package messageevent

const (
	Offline = "Offline"
	Online  = "Online"
	Expire  = "Expire"
	Renew   = "Renew"
	Login   = "Login"
	Alert   = "Alert"
	Traffic = "Traffic"
	// CnBlocked / CnUnblocked / CnBlockedOffline：节点 IP 被墙 / 从被墙恢复 / 被墙期间掉线
	CnBlocked        = "CnBlocked"
	CnUnblocked      = "CnUnblocked"
	CnBlockedOffline = "CnBlockedOffline"
	DReport          = "DReport" // 日报
	WReport          = "WReport" // 周报
	MReport          = "MReport" // 月报
)
