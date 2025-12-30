package domain

import (
	"encoding/json"
	"time"
)

type EventSinkKind string

const (
	EventSinkWebhook  EventSinkKind = "webhook"
	EventSinkKafka    EventSinkKind = "kafka"
	EventSinkNATS     EventSinkKind = "nats"
	EventSinkRabbitMQ EventSinkKind = "rabbitmq"
)

type EventSinkStatus string

const (
	EventSinkActive   EventSinkStatus = "active"
	EventSinkDisabled EventSinkStatus = "disabled"
)

type EventSink struct {
	SinkID     string
	MerchantID string
	Kind       EventSinkKind
	Status     EventSinkStatus
	Config     json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type EventDeliveryStatus string

const (
	EventDeliveryPending   EventDeliveryStatus = "pending"
	EventDeliveryDelivered EventDeliveryStatus = "delivered"
	EventDeliveryFailed    EventDeliveryStatus = "failed"
)

type EventDelivery struct {
	DeliveryID  string
	SinkID      string
	EventID     string
	Status      EventDeliveryStatus
	Attempt     int32
	NextRetryAt *time.Time
	LastError   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CloudEvent struct {
	SpecVersion     string          `json:"specversion"`
	ID              string          `json:"id"`
	Source          string          `json:"source"`
	Type            string          `json:"type"`
	Subject         string          `json:"subject,omitempty"`
	Time            time.Time       `json:"time"`
	DataContentType string          `json:"datacontenttype"`
	Data            json.RawMessage `json:"data"`
}

