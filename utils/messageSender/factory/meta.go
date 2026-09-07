package factory

import "github.com/komari-monitor/komari/database/models"

type IMessageSender interface {
	GetName() string
	// 请务必返回 &Configuration{} 的指针
	GetConfiguration() Configuration
	SendTextMessage(message, title string) error
	Init() error
	Destroy() error
}

// IEventMessageSender 是可选接口,如果实现则可以接收结构化的事件消息
type IEventMessageSender interface {
	SendEvent(event models.EventMessage) error
}

// IHTMLMessageSender 是可选接口，实现则表示该渠道的消息体按 HTML 解析，
// 通知内容可以使用 <b>、<code> 等标签排版。
// 注意：邮件渠道仅在正文含 <html>/<!doctype>/<div> 时才按 HTML 发送，
// 因此不属于这一类，零散的标签在那里会被原样显示。
type IHTMLMessageSender interface {
	SupportsHTMLMessage() bool
}

type Configuration interface{}

type MessageSenderConstructor func() IMessageSender
