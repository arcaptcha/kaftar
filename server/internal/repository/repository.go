package repository

import (
	"context"
	"errors"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

var (
	ErrRecordNotFound      = errors.New("record not found")
	ErrIdempotencyConflict = errors.New("idempotency key already used for different content")
	ErrLeaseLost           = errors.New("delivery lease lost")
)

type Outbox interface {
	Create(ctx context.Context, o *entity.Outbox) (*entity.Outbox, error)
	Get(ctx context.Context, f *entity.OutboxFilter) ([]entity.Outbox, error)
	Update(ctx context.Context, o *entity.Outbox) error
	Delete(ctx context.Context, id string) error
}

type DurableOutbox interface {
	Outbox
	ReserveDispatch(context.Context, entity.Channel, time.Time, time.Duration, int) ([]entity.Outbox, error)
	Claim(context.Context, string, entity.Channel, time.Time, time.Duration) (*entity.Outbox, error)
	Finish(context.Context, *entity.Outbox, time.Time) error
}
