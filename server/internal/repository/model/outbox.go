package model

import (
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Outbox struct {
	TimeModel
	ID            string `gorm:"primarykey"`
	MessageID     string // idempotency key
	Channel       string
	Payload       []byte
	Fingerprint   string
	EligibleAt    time.Time
	RetryDeadline *time.Time
	Attempts      int
	LeaseToken    string
	LeaseUntil    *time.Time
	DispatchAfter *time.Time

	// Delivery state
	State      int
	RetryCount int
	MaxRetries int

	// Retry control
	NextRetryAt *time.Time

	// Error tracking
	LastError string

	SentAt *time.Time
}

func (o *Outbox) BeforeCreate(_ *gorm.DB) error {
	if o.ID != "" && o.ID != uuid.Nil.String() {
		return nil
	}
	id, err := uuid.NewV7()
	o.ID = id.String()
	return err
}

func OutboxEntityToModel(o *entity.Outbox) *Outbox {
	return &Outbox{
		ID:            o.ID.String(),
		MessageID:     o.MessageID,
		Channel:       o.Channel.String(),
		Payload:       o.Payload,
		State:         int(o.State),
		RetryCount:    o.RetryCount,
		MaxRetries:    o.MaxRetries,
		NextRetryAt:   o.NextRetryAt,
		LastError:     o.LastError,
		SentAt:        o.SentAt,
		Fingerprint:   o.Fingerprint,
		EligibleAt:    o.EligibleAt,
		RetryDeadline: o.RetryDeadline,
		Attempts:      o.Attempts,
		LeaseToken:    o.LeaseToken,
		LeaseUntil:    o.LeaseUntil,
		DispatchAfter: o.DispatchAfter,
	}
}

func OutboxModelToEntity(o *Outbox) *entity.Outbox {
	id, _ := uuid.Parse(o.ID)
	return &entity.Outbox{
		ID:            id,
		MessageID:     o.MessageID,
		Channel:       entity.Channel(o.Channel),
		Payload:       o.Payload,
		State:         entity.OutboxState(o.State),
		RetryCount:    o.RetryCount,
		MaxRetries:    o.MaxRetries,
		NextRetryAt:   o.NextRetryAt,
		LastError:     o.LastError,
		CreatedAt:     &o.CreatedAt,
		UpdatedAt:     &o.UpdatedAt,
		SentAt:        o.SentAt,
		Fingerprint:   o.Fingerprint,
		EligibleAt:    o.EligibleAt,
		RetryDeadline: o.RetryDeadline,
		Attempts:      o.Attempts,
		LeaseToken:    o.LeaseToken,
		LeaseUntil:    o.LeaseUntil,
		DispatchAfter: o.DispatchAfter,
	}
}

type OutboxFilter struct {
	entity.OutboxFilter
}

func (f *OutboxFilter) Query(q *gorm.DB) *gorm.DB {
	q = q.Model(&Outbox{})
	if f.ID != "" {
		q = q.Where("id = ?", f.ID)
	}
	if f.State.IsValid() {
		q = q.Where("state = ?", int(f.State))
	}
	if f.Channel.IsValid() {
		q = q.Where("channel = ?", f.Channel.String())
	}
	return q
}

func OutboxFilterEntityToModel(f *entity.OutboxFilter) *OutboxFilter {
	return &OutboxFilter{
		OutboxFilter: *f,
	}
}
