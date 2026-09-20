package notifier

import (
	"testing"

	"github.com/komari-monitor/komari/database/models"
)

func TestFormatClientIPMessage(t *testing.T) {
	cases := []struct {
		name   string
		client models.Client
		asHTML bool
		want   string
	}{
		{
			name:   "HTML 渠道：双栈各占一行、点击可复制",
			client: models.Client{Name: "HK01", IPv4: "1.2.3.4", IPv6: "2001:db8::1"},
			asHTML: true,
			want:   "IPv4 <code>1.2.3.4</code>\nIPv6 <code>2001:db8::1</code>",
		},
		{
			name:   "纯文本渠道不带标签",
			client: models.Client{Name: "HK01", IPv4: "1.2.3.4", IPv6: "2001:db8::1"},
			asHTML: false,
			want:   "IPv4 1.2.3.4\nIPv6 2001:db8::1",
		},
		{
			name:   "只有 IPv4 时不留空行",
			client: models.Client{Name: "HK01", IPv4: "1.2.3.4"},
			asHTML: true,
			want:   "IPv4 <code>1.2.3.4</code>",
		},
		{
			name:   "从未上报过 IP 时正文为空",
			client: models.Client{Name: "HK01"},
			asHTML: true,
			want:   "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatClientIPMessage(c.client, c.asHTML); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
