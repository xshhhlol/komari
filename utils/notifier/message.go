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

// formatClientIPMessage 排版单节点事件（上线 / 下线）的通知正文：只有 IP 行。
// 节点名已经在模板的 Clients 行里，这里不再重复。
// 节点从未上报过 IP 时返回空串，正文照旧为空。
//
// 下线时用的是库里最后一次上报的 IP——节点已经断开，拿不到更新的了。
func formatClientIPMessage(client models.Client, asHTML bool) string {
	return strings.Join(appendIPLines(nil, client, asHTML), "\n")
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
