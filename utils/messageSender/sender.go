package messageSender

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/pkg/config"
	"github.com/komari-monitor/komari/utils/messageSender/factory"
)

var (
	currentProvider factory.IMessageSender
	mu              = sync.Mutex{}
	once            = sync.Once{}
)

func CurrentProvider() factory.IMessageSender {
	mu.Lock()
	defer mu.Unlock()
	return currentProvider
}

func Initialize() {
	go func() {
		once.Do(func() {
			all := factory.GetAllMessageSenders()
			for _, provider := range all {
				if _, err := database.GetMessageSenderConfigByName(provider.GetName()); err == nil {
					continue
				}
				// 如果数据库中没有该提供者的配置，则保存默认配置
				config := provider.GetConfiguration()
				configBytes, err := json.Marshal(config)
				if err != nil {
					log.Printf("Failed to marshal config for provider %s: %v", provider.GetName(), err)
					return
				}
				if err := database.SaveMessageSenderConfig(&models.MessageSenderProvider{
					Name:     provider.GetName(),
					Addition: string(configBytes),
				}); err != nil {
					log.Printf("Failed to save default config for provider %s: %v", provider.GetName(), err)
					return
				}
			}
		})
	}()
	NotificationMethod, _ := config.GetAs[string](config.NotificationMethodKey, "none")

	if NotificationMethod == "" || NotificationMethod == "none" {
		LoadProvider("empty", "{}")
		return
	}

	// 尝试从数据库加载配置
	senderConfig, err := database.GetMessageSenderConfigByName(NotificationMethod)
	if err != nil {
		// 如果没有找到配置，使用empty provider
		LoadProvider("empty", "{}")
		return
	}
	LoadProvider(NotificationMethod, senderConfig.Addition)
}

// SupportsHTML 报告当前通知渠道是否按 HTML 解析消息体。
// 通知内容据此决定用 <code> 之类的标签排版，还是退回纯文本。
func SupportsHTML() bool {
	provider := CurrentProvider()
	if provider == nil {
		return false
	}
	htmlSender, ok := provider.(factory.IHTMLMessageSender)
	return ok && htmlSender.SupportsHTMLMessage()
}

func SendTextMessage(message string, title string) error {
	if CurrentProvider() == nil {
		return fmt.Errorf("message sender provider is not initialized")
	}
	var err error
	NotificationEnabled, err := config.GetAs[bool](config.NotificationEnabledKey, false)
	if err != nil {
		return err
	}
	if !NotificationEnabled {
		return nil
	}
	for i := 0; i < 3; i++ {
		err = CurrentProvider().SendTextMessage(message, title)
		if err == nil {
			auditlog.Log("", "", "Message sent: "+title, "info")
			return nil
		}
	}
	auditlog.Log("", "", "Failed to send message after 3 attempts: "+err.Error()+","+title, "error")
	return err
}
func SendEvent(event models.EventMessage) error {
	if CurrentProvider() == nil {
		return fmt.Errorf("message sender provider is not initialized")
	}
	var err error
	cfg, err := config.GetMany(map[string]any{
		config.NotificationEnabledKey:  false,
		config.NotificationTemplateKey: "{{emoji}}{{emoji}}{{emoji}}\nEvent: {{event}}\nClients: {{client}}\nMessage: {{message}}\nTime: {{time}}",
	})
	if err != nil {
		return err
	}
	if !cfg[config.NotificationEnabledKey].(bool) {
		return nil
	}

	// 检查提供者是否实现了 IEventMessageSender 接口
	if eventSender, ok := CurrentProvider().(factory.IEventMessageSender); ok {
		// 如果实现了,直接调用 SendEvent
		for i := 0; i < 3; i++ {
			err = eventSender.SendEvent(event)
			if err == nil || err.Error() == "short response: \x00\x00\x00\x1a\x00\x00\x00" {
				auditlog.Log("", "", "Event message sent: "+event.Event, "info")
				return nil
			}
		}
		auditlog.Log("", "", "Failed to send event message after 3 attempts: "+err.Error()+","+event.Event, "error")
		return err
	}

	// 如果没有实现,使用模板格式化为文本消息
	messageTemplate := cfg[config.NotificationTemplateKey].(string)

	messageTemplate = parseTemplate(messageTemplate, event, SupportsHTML())

	for i := 0; i < 3; i++ {
		err = CurrentProvider().SendTextMessage(messageTemplate, event.Event)
		if err == nil || err.Error() == "short response: \x00\x00\x00\x1a\x00\x00\x00" { // QQ 会返回这个错误，但实际上消息是发送成功的
			auditlog.Log("", "", "Event message sent: "+event.Event, "info")
			return nil
		}
	}
	auditlog.Log("", "", "Failed to send event message after 3 attempts: "+err.Error()+","+event.Event, "error")
	return err
}

// parseTemplate 把事件填进用户配置的通知模板。
// asHTML 为真时（渠道按 HTML 解析消息体），转义各占位符填入的值——
// 节点名里的 < & 会让 Telegram 整条消息解析失败。模板本身不转义，
// 用户仍可在模板里写 HTML；{{message}} 也不转义，那是各通知器自己
// 排好的正文（例如被墙通知里点击可复制的 <code> IP）。
func parseTemplate(messageTemplate string, event models.EventMessage, asHTML bool) string {
	// Aggregate client names. If Name is empty, fall back to UUID.
	clientNames := make([]string, 0, len(event.Clients))
	for _, c := range event.Clients {
		name := c.Name
		if strings.TrimSpace(name) == "" {
			// fallback to UUID when name is not set
			name = c.UUID
		}
		clientNames = append(clientNames, name)
	}
	joinedClients := strings.Join(clientNames, ", ")

	escape := func(s string) string {
		if !asHTML {
			return s
		}
		return html.EscapeString(s)
	}

	replaceMap := map[string]string{
		"{{event}}":   escape(event.Event),
		"{{client}}":  escape(joinedClients),
		"{{time}}":    escape(event.Time.Format(time.RFC3339)),
		"{{message}}": event.Message,
		"{{emoji}}":   event.Emoji,
	}
	result := messageTemplate
	for placeholder, value := range replaceMap {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
