package notifier

import (
	"testing"

	"github.com/komari-monitor/komari/database/models"
)

func TestFormatCnBlockMessage(t *testing.T) {
	involved := []models.Client{
		{UUID: "u1", Name: "HK <1>", IPv4: "1.2.3.4", IPv6: "2001:db8::1"},
		{UUID: "u2", IPv4: "5.6.7.8"},
	}

	t.Run("HTML 渠道：IP 可点击复制、节点名转义、行首无空白", func(t *testing.T) {
		got := formatCnBlockMessage(involved, true)
		want := "• HK &lt;1&gt;\nIPv4 <code>1.2.3.4</code>\nIPv6 <code>2001:db8::1</code>\n• u2\nIPv4 <code>5.6.7.8</code>"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("纯文本渠道不带标签", func(t *testing.T) {
		got := formatCnBlockMessage(involved, false)
		want := "• HK <1>\nIPv4 1.2.3.4\nIPv6 2001:db8::1\n• u2\nIPv4 5.6.7.8"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
