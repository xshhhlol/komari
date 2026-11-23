package models

import "time"

type EventNode struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
}

type HistoryEvent struct {
	ID         uint64     `gorm:"primaryKey" json:"id,string"`
	Type       string     `json:"type"`
	OccurredAt time.Time  `json:"occurred_at"`
	Node       *EventNode `gorm:"embedded;embeddedPrefix:node_" json:"node"`
	Message    string     `json:"message,omitempty"`
}
