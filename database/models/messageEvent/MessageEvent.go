package messageevent

const (
	Offline = "Offline"
	Online  = "Online"
	Expire  = "Expire"
	Renew   = "Renew"
	Login   = "Login"
	Alert   = "Alert"
	Traffic = "Traffic"
	// CnBlocked / CnUnblocked：节点 IP 被墙 / 从被墙恢复
	CnBlocked   = "CnBlocked"
	CnUnblocked = "CnUnblocked"
	DReport     = "DReport" // 日报
	WReport     = "WReport" // 周报
	MReport     = "MReport" // 月报
)
