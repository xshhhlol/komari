package messageSender

import (
	"strings"
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils/messageSender/factory"
)

func Test(t *testing.T) {
	senders := factory.GetAllMessageSenders()
	if len(senders) == 0 {
		t.Error("No message senders found")
		return
	}
	cfg := factory.GetSenderConfigs()
	if len(cfg) == 0 {
		t.Error("No sender configs found")
		return
	}
	LoadProvider("email", `{"host":"smtp.example.com","port":587,"username":"user","password":"pass"}`)
	cp := CurrentProvider
	if cp() == nil {
		t.Error("Current provider is nil")
		return
	}
}

func TestParseTemplateEscapesValuesForHTMLChannels(t *testing.T) {
	event := models.EventMessage{
		Event:   "CnBlocked",
		Clients: []models.Client{{UUID: "u1", Name: "A<B & C"}},
		Time:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message: "  IPv4 <code>1.2.3.4</code>",
		Emoji:   "🚧",
	}
	tpl := "{{emoji}} {{event}}\n{{client}}\n{{message}}"

	plain := parseTemplate(tpl, event, false)
	if !strings.Contains(plain, "A<B & C") {
		t.Fatalf("plain channels must keep the raw client name, got %q", plain)
	}

	asHTML := parseTemplate(tpl, event, true)
	if !strings.Contains(asHTML, "A&lt;B &amp; C") {
		t.Fatalf("html channels must escape the client name, got %q", asHTML)
	}
	// {{message}} 由通知器自行排版，其中的标签必须原样保留。
	if !strings.Contains(asHTML, "<code>1.2.3.4</code>") {
		t.Fatalf("notifier-authored markup must survive, got %q", asHTML)
	}
}
