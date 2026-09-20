package notifier

import (
	"testing"

	"github.com/komari-monitor/komari/database/models"
)

func TestFormatClientsMessage(t *testing.T) {
	cases := []struct {
		name     string
		involved []models.Client
		asHTML   bool
		want     string
	}{
		{
			name:     "单节点：名称占住 Message 行，IP 各自独占一行且可点击复制",
			involved: []models.Client{{Name: "Yunyoo-美国", IPv4: "64.83.25.32", IPv6: "2001:db8::1"}},
			asHTML:   true,
			want:     "• Yunyoo-美国\nIPv4 <code>64.83.25.32</code>\nIPv6 <code>2001:db8::1</code>",
		},
		{
			name:     "纯文本渠道不带标签",
			involved: []models.Client{{Name: "Yunyoo-美国", IPv4: "64.83.25.32"}},
			asHTML:   false,
			want:     "• Yunyoo-美国\nIPv4 64.83.25.32",
		},
		{
			name:     "多节点：节点名转义、无名回退 UUID、行首无空白",
			involved: []models.Client{{UUID: "u1", Name: "HK <1>", IPv4: "1.2.3.4"}, {UUID: "u2", IPv4: "5.6.7.8"}},
			asHTML:   true,
			want:     "• HK &lt;1&gt;\nIPv4 <code>1.2.3.4</code>\n• u2\nIPv4 <code>5.6.7.8</code>",
		},
		{
			name:     "从未上报过 IP 时只有名称行，不留空行",
			involved: []models.Client{{Name: "HK01"}},
			asHTML:   true,
			want:     "• HK01",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatClientsMessage(c.involved, c.asHTML); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
