package outboxrepo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
	"github.com/stretchr/testify/require"
)

func TestDurableTransitions(test *testing.T) {
	database := setupDB(test)
	require.NoError(test, Migrate(database, time.Hour))
	store := New(database)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	outbox, err := store.Create(ctx, &entity.Outbox{Channel: entity.ChannelHTTP, Payload: []byte(`{}`), State: entity.OutboxStatePending, MaxRetries: 2, LastError: "old failure"})
	require.NoError(test, err)
	rows, err := store.ReserveDispatch(ctx, entity.ChannelHTTP, now, time.Second, 10)
	require.NoError(test, err)
	require.Len(test, rows, 1)
	rows, err = store.ReserveDispatch(ctx, entity.ChannelHTTP, now, time.Second, 10)
	require.NoError(test, err)
	require.Empty(test, rows)
	claimed, err := store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, now, time.Minute)
	require.NoError(test, err)
	require.Equal(test, entity.OutboxStateSending, claimed.State)
	require.Equal(test, 1, claimed.Attempts)
	duplicate, err := store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, now, time.Minute)
	require.NoError(test, err)
	require.Nil(test, duplicate)
	next := now.Add(time.Second)
	claimed.State, claimed.LastError, claimed.NextRetryAt = entity.OutboxStateFailed, "failed", &next
	require.NoError(test, store.Finish(ctx, claimed, now))
	duplicate, err = store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, now, time.Minute)
	require.NoError(test, err)
	require.Nil(test, duplicate)
	claimed, err = store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, next, time.Minute)
	require.NoError(test, err)
	require.Equal(test, 1, claimed.RetryCount)
	claimed.State = entity.OutboxStateSent
	require.NoError(test, store.Finish(ctx, claimed, next))
	saved, err := store.Get(ctx, &entity.OutboxFilter{ID: outbox.ID.String()})
	require.NoError(test, err)
	require.NotNil(test, saved[0].SentAt)
	require.Empty(test, saved[0].LastError)
	require.Nil(test, saved[0].NextRetryAt)
	duplicate, err = store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, next, time.Minute)
	require.NoError(test, err)
	require.Nil(test, duplicate)
}

func TestLeaseRecoveryAndFencing(test *testing.T) {
	store := New(setupDB(test))
	ctx := context.Background()
	now := time.Now().UTC()
	outbox, err := store.Create(ctx, &entity.Outbox{Channel: entity.ChannelHTTP, State: entity.OutboxStatePending, MaxRetries: 1})
	require.NoError(test, err)
	stale, err := store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, now, time.Second)
	require.NoError(test, err)
	later := now.Add(2 * time.Second)
	rows, err := store.ReserveDispatch(ctx, entity.ChannelHTTP, later, time.Second, 10)
	require.NoError(test, err)
	require.Len(test, rows, 1)
	current, err := store.Claim(ctx, outbox.ID.String(), entity.ChannelHTTP, later, time.Second)
	require.NoError(test, err)
	require.NotEqual(test, stale.LeaseToken, current.LeaseToken)
	stale.State = entity.OutboxStateSent
	require.ErrorIs(test, store.Finish(ctx, stale, later), repository.ErrLeaseLost)
	rows, err = store.ReserveDispatch(ctx, entity.ChannelHTTP, later.Add(2*time.Second), time.Second, 10)
	require.NoError(test, err)
	require.Empty(test, rows)
	saved, err := store.Get(ctx, &entity.OutboxFilter{ID: outbox.ID.String()})
	require.NoError(test, err)
	require.Equal(test, entity.OutboxStateDead, saved[0].State)
	require.Equal(test, 1, saved[0].RetryCount)
}

func TestSchedulingAndAgeBoundary(test *testing.T) {
	store := New(setupDB(test))
	ctx := context.Background()
	now := time.Now().UTC()
	later := now.Add(time.Hour)
	_, err := store.Create(ctx, &entity.Outbox{Channel: entity.ChannelHTTP, State: entity.OutboxStatePending, NextRetryAt: &later})
	require.NoError(test, err)
	due, err := store.Create(ctx, &entity.Outbox{Channel: entity.ChannelHTTP, State: entity.OutboxStatePending})
	require.NoError(test, err)
	expired, err := store.Create(ctx, &entity.Outbox{Channel: entity.ChannelHTTP, State: entity.OutboxStatePending, RetryDeadline: &now})
	require.NoError(test, err)
	rows, err := store.ReserveDispatch(ctx, entity.ChannelHTTP, now, time.Second, 10)
	require.NoError(test, err)
	require.Len(test, rows, 1)
	require.Equal(test, due.ID, rows[0].ID)
	saved, err := store.Get(ctx, &entity.OutboxFilter{ID: expired.ID.String()})
	require.NoError(test, err)
	require.Equal(test, entity.OutboxStateDead, saved[0].State)
	require.Zero(test, saved[0].Attempts)
}

func TestAtomicIdempotency(test *testing.T) {
	database := setupDB(test)
	require.NoError(test, Migrate(database, 0))
	store := New(database)
	ctx := context.Background()
	message := &entity.Outbox{MessageID: "request", Fingerprint: strings.Repeat("a", 64), Channel: entity.ChannelHTTP, State: entity.OutboxStatePending}
	original, err := store.Create(ctx, message)
	require.NoError(test, err)
	duplicate, err := store.Create(ctx, message)
	require.NoError(test, err)
	require.Equal(test, original.ID, duplicate.ID)
	message.Fingerprint = strings.Repeat("b", 64)
	_, err = store.Create(ctx, message)
	require.ErrorIs(test, err, repository.ErrIdempotencyConflict)
	message.Channel = entity.ChannelSMS
	otherChannel, err := store.Create(ctx, message)
	require.NoError(test, err)
	require.NotEqual(test, original.ID, otherChannel.ID)
	message.MessageID = ""
	first, err := store.Create(ctx, message)
	require.NoError(test, err)
	second, err := store.Create(ctx, message)
	require.NoError(test, err)
	require.NotEqual(test, first.ID, second.ID)
}

func TestMigrationPreservesDataAndRejectsDuplicates(test *testing.T) {
	database := setupDB(test)
	for index := 0; index < 2; index++ {
		require.NoError(test, database.Create(&model.Outbox{MessageID: "legacy", Channel: "http", State: 1}).Error)
	}
	err := Migrate(database, time.Hour)
	require.ErrorContains(test, err, "duplicate")
	var count int64
	require.NoError(test, database.Model(&model.Outbox{}).Count(&count).Error)
	require.EqualValues(test, 2, count)
}

func TestMigrationBackfillIsStable(test *testing.T) {
	database := setupDB(test)
	now := time.Now().UTC().Truncate(time.Microsecond)
	later := now.Add(time.Hour)
	legacy := model.Outbox{Channel: "http", State: int(entity.OutboxStateSending), RetryCount: 2, MaxRetries: 4, NextRetryAt: &later}
	require.NoError(test, database.Create(&legacy).Error)
	require.NoError(test, Migrate(database, time.Hour))
	var migrated model.Outbox
	require.NoError(test, database.First(&migrated, "id = ?", legacy.ID).Error)
	require.Equal(test, int(entity.OutboxStateSending), migrated.State)
	require.Equal(test, 3, migrated.Attempts)
	require.True(test, migrated.EligibleAt.Equal(later))
	require.True(test, migrated.RetryDeadline.Equal(later.Add(time.Hour)))
	require.True(test, migrated.LeaseUntil.Before(now))
	require.NoError(test, Migrate(database, 24*time.Hour))
	require.NoError(test, database.First(&migrated, "id = ?", legacy.ID).Error)
	require.True(test, migrated.RetryDeadline.Equal(later.Add(time.Hour)))
}
