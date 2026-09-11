package outboxrepo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	require.NoError(t, err)

	err = db.AutoMigrate(&model.Outbox{})
	require.NoError(t, err)

	return db
}

func TestRepo_CreateAndGet(t *testing.T) {
	db := setupDB(t)
	repo := New(db)

	ctx := context.Background()

	outbox := &entity.Outbox{
		Channel:    entity.ChannelSMS,
		Payload:    []byte(`{"msg":"hello"}`),
		State:      entity.OutboxStatePending,
		MaxRetries: 3,
	}

	modelOutbox, err := repo.Create(ctx, outbox)
	require.NoError(t, err)
	require.NotEmpty(t, modelOutbox.ID)

	result, err := repo.Get(ctx, &entity.OutboxFilter{
		ID: modelOutbox.ID.String(),
	})

	require.NoError(t, err)
	require.Len(t, result, 1)

	got := result[0]
	require.Equal(t, outbox.Channel, got.Channel)
	require.Equal(t, outbox.State, got.State)
}

func TestRepo_Get_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := New(db)

	ctx := context.Background()

	_, err := repo.Get(ctx, &entity.OutboxFilter{
		ID: uuid.New().String(),
	})

	require.Error(t, err)
	require.ErrorIs(t, err, repository.ErrRecordNotFound)
}

func TestRepo_Delete(t *testing.T) {
	db := setupDB(t)
	repo := New(db)

	ctx := context.Background()

	outbox := &entity.Outbox{
		ID:      uuid.New(),
		Channel: entity.ChannelSMS,
		Payload: []byte(`{}`),
		State:   entity.OutboxStatePending,
	}

	mOutbox, err := repo.Create(ctx, outbox)
	require.NoError(t, err)

	err = repo.Delete(ctx, mOutbox.ID.String())
	require.NoError(t, err)

	_, err = repo.Get(ctx, &entity.OutboxFilter{ID: mOutbox.ID.String()})
	require.ErrorIs(t, err, repository.ErrRecordNotFound)
}

func TestRepo_UpdateState(t *testing.T) {
	db := setupDB(t)
	repo := New(db)

	ctx := context.Background()

	// Create an outbox entry with full fields
	outbox := &entity.Outbox{
		Channel:    entity.ChannelSMS,
		Payload:    []byte(`{"msg":"hello"}`),
		State:      entity.OutboxStatePending,
		MaxRetries: 3,
	}

	outbox, err := repo.Create(ctx, outbox)
	require.NoError(t, err)

	// Update state
	outbox.State = entity.OutboxStateSent
	err = repo.Update(ctx, outbox)
	require.NoError(t, err)

	// Fetch and assert
	result, err := repo.Get(ctx, &entity.OutboxFilter{ID: outbox.ID.String()})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, entity.OutboxStateSent, result[0].State)
	require.Equal(t, 3, result[0].MaxRetries) // make sure default preserved
}

func TestRepoStatsGroupsStatesAndOldestPending(t *testing.T) {
	db := setupDB(t)
	repo := New(db)
	now := time.Now().UTC()
	for _, outbox := range []*entity.Outbox{
		{
			Channel:    entity.ChannelHTTP,
			Payload:    []byte(`{"url":"https://example.com"}`),
			State:      entity.OutboxStatePending,
			EligibleAt: now.Add(-time.Minute),
		},
		{
			Channel:    entity.ChannelHTTP,
			Payload:    []byte(`{"url":"https://example.com"}`),
			State:      entity.OutboxStateFailed,
			EligibleAt: now,
		},
		{
			Channel:    entity.ChannelSMS,
			Payload:    []byte(`{"to":["1"],"text":"test"}`),
			State:      entity.OutboxStateSent,
			EligibleAt: now,
		},
	} {
		_, err := repo.Create(context.Background(), outbox)
		require.NoError(t, err)
	}

	stats, err := repo.Stats(context.Background())
	require.NoError(t, err)
	require.Len(t, stats.Counts, 3)
	require.Len(t, stats.OldestPending, 1)
	require.Equal(t, entity.ChannelHTTP, stats.OldestPending[0].Channel)
	require.WithinDuration(
		t,
		now.Add(-time.Minute),
		stats.OldestPending[0].EligibleAt,
		time.Millisecond,
	)
}
