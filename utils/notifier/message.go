package notifier

import (
	"html"
	"strings"

	"github.com/komari-monitor/komari/database/models"
)

// appendIPLines 把节点已知的 IPv4 / IPv6 各追加为一行。
//
// IP 是这类告警里最需要被读到、被拿去用的信息，所以单独成行；在按 HTML 解析的
// 渠道（Telegram）上用 <code> 包裹，渲染为等宽块，点一下即可复制。其它渠道退回纯文本。
// IP 行不缩进：Telegram 会原样保留行首空格，看上去像凭空多了个空格。
func appendIPLines(lines []string, client models.Client, asHTML bool) []string {
	if client.IPv4 != "" {
		lines = append(lines, "IPv4 "+copyableText(client.IPv4, asHTML))
	}
	if client.IPv6 != "" {
		lines = append(lines, "IPv6 "+copyableText(client.IPv6, asHTML))
	}
	return lines
}

// formatClientsMessage 排版通知正文：每个节点一行名称，其后每个 IP 各占一行。
// 所有事件（被墙 / 上线 / 下线）共用这一套排版。
//
// 第一行必须是节点名而不是 IP：模板里正文接在 "Message: " 后面，正文的第一行
// 会和它挤在同一行——让节点名去占这个位置，IP 才能干干净净地独占一行。
// 名称和 Clients 行重复，但那是让 IP 单独成行的代价，划算。
//
// 下线通知用的是库里最后一次上报的 IP——节点已经断开，拿不到更新的了。
func formatClientsMessage(involved []models.Client, asHTML bool) string {
	lines := make([]string, 0, len(involved)*3)
	for _, client := range involved {
		name := client.Name
		if strings.TrimSpace(name) == "" {
			name = client.UUID
		}
		lines = append(lines, "• "+plainText(name, asHTML))
		lines = appendIPLines(lines, client, asHTML)
	}
	return strings.Join(lines, "\n")
}

// copyableText 把一段文本渲染为可点击复制的等宽块（HTML 渠道），
// 非 HTML 渠道原样返回。
func copyableText(text string, asHTML bool) string {
	if !asHTML {
		return text
	}
	return "<code>" + html.EscapeString(text) + "</code>"
}

// plainText 在 HTML 渠道下转义文本中的 < > &，避免节点名里的特殊字符
// 破坏整条消息的解析。
func plainText(text string, asHTML bool) string {
	if !asHTML {
		return text
	}
	return html.EscapeString(text)
}
