package models

import (
	"time"

	"github.com/google/uuid"
)

// SupportPolicy enables the consolidated workflow for an organization.
type SupportPolicy struct {
	BaseModel
	OrganizationID  uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"organization_id"`
	Enabled         bool      `json:"enabled"`
	Paused          bool      `json:"paused"`
	EnabledAt       time.Time `json:"enabled_at"`
	FirstRunAt      time.Time `json:"first_run_at"`
	BillingTimezone string    `gorm:"default:Asia/Karachi" json:"billing_timezone"`
}

// SupportJob is durable work, not a Redis cache. Revision prevents an older AI
// answer from overwriting a newer incoming message.
type SupportJob struct {
	BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;uniqueIndex:support_conversation;not null" json:"organization_id"`
	Account        string     `gorm:"uniqueIndex:support_conversation;not null" json:"account"`
	ContactID      uuid.UUID  `gorm:"type:uuid;uniqueIndex:support_conversation;not null" json:"contact_id"`
	PendingFrom    time.Time  `json:"pending_from"`
	LastInboundAt  time.Time  `json:"last_inbound_at"`
	LastMessageID  uuid.UUID  `gorm:"type:uuid" json:"last_message_id"`
	Revision       int64      `json:"revision"`
	State          string     `gorm:"index" json:"state"`
	DueAt          time.Time  `gorm:"index" json:"due_at"`
	RetryAt        time.Time  `gorm:"index" json:"retry_at"`
	LeaseUntil     *time.Time `json:"lease_until"`
	Attempts       int        `json:"attempts"` // AI preparation failures only.
	SendAttempts   int        `gorm:"not null;default:0" json:"send_attempts"`
	Decision       string     `gorm:"type:text" json:"decision"`
	Answer         string     `gorm:"type:text" json:"answer"`
	Reason         string     `json:"reason"`
	Version        string     `json:"version"`
	Contact        *Contact   `gorm:"foreignKey:ContactID" json:"contact,omitempty"`
}

// SupportSend remains reserved on ambiguous network failures. It is linked to
// the message created in the same transaction, so quota cannot be double spent.
type SupportSend struct {
	BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;not null" json:"organization_id"`
	PhoneID        string     `gorm:"index;not null" json:"phone_id"`
	ContactID      uuid.UUID  `gorm:"type:uuid;index;not null" json:"contact_id"`
	MessageID      uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null" json:"message_id"`
	JobID          *uuid.UUID `gorm:"type:uuid" json:"job_id"`
	CustomerMonth  string     `gorm:"index" json:"customer_month"`
	Automatic      bool       `json:"automatic"`
	State          string     `gorm:"index" json:"state"`
	WAMID          string     `gorm:"column:wamid" json:"wamid"`
}
