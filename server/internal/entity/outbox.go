package entity

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidOutboxStatus = errors.New("invalid outbox status")
)

type Outbox struct {
	ID            uuid.UUID  `json:"id,omitempty"`
	Fingerprint   string     `json:"-"`
	EligibleAt    time.Time  `json:"-"`
	RetryDeadline *time.Time `json:"-"`
	Attempts      int        `json:"-"`
	LeaseToken    string     `json:"-"`
	LeaseUntil    *time.Time `json:"-"`
	DispatchAfter *time.Time `json:"-"`

	// Message identity
	MessageID string  `json:"message_id,omitempty"` // idempotency key
	Channel   Channel `json:"channel"`
	Payload   []byte  `json:"payload"`

	// Delivery state
	State      OutboxState `json:"state"`
	RetryCount int         `json:"retry_count,omitempty"`
	MaxRetries int         `json:"max_retries"`

	// Retry control
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`

	// Error tracking
	LastError string `json:"last_error,omitempty"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
}

type OutboxState int

const (
	_ OutboxState = iota
	OutboxStatePending
	OutboxStateSending
	OutboxStateFailed // retryable
	OutboxStateSent
	OutboxStateDead // no more retries
)

var outboxStatusMap = map[OutboxState]string{
	OutboxStatePending: "pending",
	OutboxStateSending: "sending",
	OutboxStateSent:    "sent",
	OutboxStateFailed:  "failed",
	OutboxStateDead:    "dead",
}

func (s OutboxState) IsValid() bool {
	_, ok := outboxStatusMap[s]
	return ok
}

func (s OutboxState) Validate() error {
	if !s.IsValid() {
		return fmt.Errorf("%w: %s (%d)", ErrInvalidOutboxStatus, s.String(), s)
	}
	return nil
}

func (s OutboxState) String() string {
	return outboxStatusMap[s]
}

type OutboxFilter struct {
	ID      string
	State   OutboxState
	Channel Channel
}

type OutboxStatus struct {
	ID      uuid.UUID `json:"id"`
	Channel Channel   `json:"channel"`

	// Delivery state
	State      OutboxState `json:"state"`
	RetryCount int         `json:"retry_count"`
	MaxRetries int         `json:"max_retries"`

	// Retry control
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`

	// Error tracking
	LastError string `json:"last_error,omitempty"`

	SentAt    *time.Time `json:"sent_at,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func (o *Outbox) Status() OutboxStatus {
	return OutboxStatus{
		ID:          o.ID,
		Channel:     o.Channel,
		State:       o.State,
		RetryCount:  o.RetryCount,
		MaxRetries:  o.MaxRetries,
		NextRetryAt: o.NextRetryAt,
		LastError:   o.LastError,
		SentAt:      o.SentAt,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}
