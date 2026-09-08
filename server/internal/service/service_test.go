package service

import (
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/google/uuid"
)

func TestRetryLimitReached(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		retryCount int
		want       bool
	}{
		{name: "unlimited", maxRetries: -1, retryCount: 100, want: false},
		{name: "below limit", maxRetries: 3, retryCount: 2, want: false},
		{name: "at limit", maxRetries: 3, retryCount: 3, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retryLimitReached(entity.Outbox{
				MaxRetries: tt.maxRetries,
				RetryCount: tt.retryCount,
			})
			if got != tt.want {
				t.Fatalf("retryLimitReached() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryAgeExceeded(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	created := now.Add(-2 * time.Hour)

	if !retryAgeExceeded(
		entity.Outbox{CreatedAt: &created},
		time.Hour,
		now,
	) {
		t.Fatal("expected old message to exceed retry age")
	}

	if retryAgeExceeded(
		entity.Outbox{CreatedAt: &created},
		0,
		now,
	) {
		t.Fatal("disabled retry age should not expire messages")
	}

	fresh := now.Add(-30 * time.Minute)
	if retryAgeExceeded(
		entity.Outbox{CreatedAt: &fresh},
		time.Hour,
		now,
	) {
		t.Fatal("fresh message should not exceed retry age")
	}
}

func TestNormalizeNegativeMaxRetries(t *testing.T) {
	for _, value := range []int{-1, -2, -100} {
		if normalizeMaxRetries(value) != -1 {
			t.Fatalf("normalized retries = %d, want -1", value)
		}
	}
}

func TestOutboxStatusUsesID(t *testing.T) {
	id := uuid.New()
	outbox := &entity.Outbox{ID: id}
	status := outbox.Status()
	if status.ID != id {
		t.Fatalf("status ID = %s, want %s", status.ID, id)
	}
}

func TestConfigNormalizesNegativeRetryAge(t *testing.T) {
	cfg := (&Config{MessageRetryMaxAge: -time.Hour}).OrDefault()
	if cfg.MessageRetryMaxAge != 0 {
		t.Fatalf("retry max age = %s, want 0", cfg.MessageRetryMaxAge)
	}
}
