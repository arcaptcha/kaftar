package dto

import (
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

type MessageStatus struct {
	ID          string     `json:"id"`
	Channel     string     `json:"channel"`
	Status      int        `json:"status"`
	StatusText  string     `json:"status_text"`
	RetryCount  int        `json:"retry_count"`
	MaxRetries  int        `json:"max_retries"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func NewMessageStatus(status entity.OutboxStatus) MessageStatus {
	return MessageStatus{
		ID:          status.ID.String(),
		Channel:     status.Channel.String(),
		Status:      int(status.State),
		StatusText:  status.State.String(),
		RetryCount:  status.RetryCount,
		MaxRetries:  status.MaxRetries,
		NextRetryAt: status.NextRetryAt,
		LastError:   status.LastError,
		SentAt:      status.SentAt,
		CreatedAt:   status.CreatedAt,
		UpdatedAt:   status.UpdatedAt,
	}
}

type StatusSMSResult = MessageStatus
type StatusEmailResult = MessageStatus
type StatusMattermostResult = MessageStatus
type StatusBaleResult = MessageStatus
type StatusHTTPResult = MessageStatus

// status aliases are intentionally additive to preserve endpoint-specific names.
