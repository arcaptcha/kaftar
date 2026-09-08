package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	outboxrepo "github.com/arcaptcha/kaftar/server/internal/repository/outbox_repo"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type deliverySender struct {
	calls atomic.Int32
	send  func(context.Context, entity.Message) error
}

func (sender *deliverySender) Channel() entity.Channel { return entity.ChannelHTTP }
func (sender *deliverySender) Send(ctx context.Context, message entity.Message) error {
	sender.calls.Add(1)
	if sender.send != nil {
		return sender.send(ctx, message)
	}
	return nil
}

type acknowledgment struct{ acked, nacked, requeued bool }

func (settlement *acknowledgment) Ack(uint64, bool) error { settlement.acked = true; return nil }
func (settlement *acknowledgment) Nack(_ uint64, _ bool, requeue bool) error {
	settlement.nacked, settlement.requeued = true, requeue
	return nil
}
func (settlement *acknowledgment) Reject(_ uint64, requeue bool) error {
	settlement.nacked, settlement.requeued = true, requeue
	return nil
}

type failingRepository struct {
	repository.DurableOutbox
	createErr, finishErr error
}

func (store failingRepository) Create(ctx context.Context, outbox *entity.Outbox) (*entity.Outbox, error) {
	if store.createErr != nil {
		return nil, store.createErr
	}
	return store.DurableOutbox.Create(ctx, outbox)
}
func (store failingRepository) Finish(ctx context.Context, outbox *entity.Outbox, now time.Time) error {
	if store.finishErr != nil {
		return store.finishErr
	}
	return store.DurableOutbox.Finish(ctx, outbox, now)
}

func deliveryFixture(test *testing.T) (*service, *deliverySender) {
	test.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	require.NoError(test, err)
	pool, err := database.DB()
	require.NoError(test, err)
	pool.SetMaxOpenConns(1)
	test.Cleanup(func() { _ = pool.Close() })
	require.NoError(test, outboxrepo.Migrate(database, 0))
	sender := &deliverySender{}
	return &service{repo: outboxrepo.New(database), logger: zap.NewNop(), cfg: &Config{}, senders: map[entity.Channel]entity.Sender{entity.ChannelHTTP: sender}, mqConnGetter: func() (*amqp.Connection, error) {
		test.Error("acceptance must not publish")
		return nil, errors.New("broker unavailable")
	}}, sender
}

func makeDelivery(test *testing.T, envelope entity.Outbox) (amqp.Delivery, *acknowledgment) {
	test.Helper()
	body, err := json.Marshal(envelope)
	require.NoError(test, err)
	settlement := &acknowledgment{}
	return amqp.Delivery{Body: body, DeliveryTag: 1, Acknowledger: settlement}, settlement
}

func TestSubmissionIdentityAndBrokerIndependence(test *testing.T) {
	svc, _ := deliveryFixture(test)
	ctx := context.Background()
	scheduled := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	svc.cfg.MessageRetryMaxAge = time.Hour
	message := &entity.HTTPMessage{URL: "https://example.invalid", Headers: map[string]string{"b": "2", "a": "1"}}
	options := &SendOptions{IdempotencyKey: "same-request", MaxRetries: -2, SendAt: &scheduled}
	original, err := svc.Send(ctx, message, options)
	require.NoError(test, err)
	message.Method = "post"
	options.MaxRetries = -1
	sameInstant := scheduled.In(time.FixedZone("other", 3600))
	options.SendAt = &sameInstant
	duplicate, err := svc.Send(ctx, message, options)
	require.NoError(test, err)
	require.Equal(test, original, duplicate)
	message.Body = "changed"
	_, err = svc.Send(ctx, message, options)
	require.ErrorIs(test, err, repository.ErrIdempotencyConflict)
	saved, err := svc.repo.Get(ctx, &entity.OutboxFilter{ID: original})
	require.NoError(test, err)
	require.True(test, saved[0].RetryDeadline.Equal(scheduled.Add(time.Hour)))
	require.Equal(test, "post", message.Method)
	require.Equal(test, -1, options.MaxRetries)
}

func TestFailedPersistenceIsNotAcknowledged(test *testing.T) {
	for _, failure := range []string{"create", "finish"} {
		test.Run(failure, func(test *testing.T) {
			svc, sender := deliveryFixture(test)
			ctx := context.Background()
			payload := []byte(`{"url":"https://example.invalid","method":"POST"}`)
			envelope := entity.Outbox{Channel: entity.ChannelHTTP, Payload: payload}
			store := failingRepository{DurableOutbox: svc.repo}
			if failure == "create" {
				store.createErr = errors.New("database unavailable")
			} else {
				id, err := svc.Send(ctx, &entity.HTTPMessage{URL: "https://example.invalid"}, nil)
				require.NoError(test, err)
				envelope.ID = uuid.MustParse(id)
				store.finishErr = errors.New("database unavailable")
			}
			svc.repo = store
			delivery, settlement := makeDelivery(test, envelope)
			svc.handleDelivery(ctx, sender, delivery)
			require.False(test, settlement.acked)
			require.True(test, settlement.nacked && settlement.requeued)
		})
	}
}

func TestDurablePayloadAndTerminalDeduplication(test *testing.T) {
	svc, sender := deliveryFixture(test)
	ctx := context.Background()
	id, err := svc.Send(ctx, &entity.HTTPMessage{URL: "https://example.invalid", Body: "original"}, nil)
	require.NoError(test, err)
	sender.send = func(_ context.Context, message entity.Message) error {
		require.Equal(test, "original", message.(*entity.HTTPMessage).Body)
		return nil
	}
	envelope := entity.Outbox{ID: uuid.MustParse(id), Channel: entity.ChannelSMS, Payload: []byte(`{"url":"https://forged.invalid"}`), State: entity.OutboxStateDead}
	for attempt := 0; attempt < 2; attempt++ {
		delivery, settlement := makeDelivery(test, envelope)
		svc.handleDelivery(ctx, sender, delivery)
		require.True(test, settlement.acked)
	}
	require.EqualValues(test, 1, sender.calls.Load())
	status, err := svc.Status(ctx, id)
	require.NoError(test, err)
	require.Equal(test, entity.OutboxStateSent, status.State)
	require.NotNil(test, status.SentAt)
}

func TestForeignIdempotencySurvivesRedelivery(test *testing.T) {
	svc, sender := deliveryFixture(test)
	envelope := entity.Outbox{Channel: entity.ChannelHTTP, MessageID: "producer-request", Payload: []byte(`{"url":"https://example.invalid"}`)}
	for attempt := 0; attempt < 2; attempt++ {
		delivery, settlement := makeDelivery(test, envelope)
		svc.handleDelivery(context.Background(), sender, delivery)
		require.True(test, settlement.acked)
	}
	rows, err := svc.repo.Get(context.Background(), &entity.OutboxFilter{})
	require.NoError(test, err)
	require.Len(test, rows, 1)
	envelope.Payload = []byte(`{"url":"https://changed.invalid"}`)
	delivery, settlement := makeDelivery(test, envelope)
	svc.handleDelivery(context.Background(), sender, delivery)
	require.True(test, settlement.nacked)
	require.False(test, settlement.requeued)
}

func TestRejectedDeliveryLogsDoNotIncludePayload(test *testing.T) {
	svc, sender := deliveryFixture(test)
	core, observed := observer.New(zap.WarnLevel)
	svc.logger = zap.New(core)
	settlement := &acknowledgment{}
	delivery := amqp.Delivery{Body: []byte(`{"secret":"credential-value"`), DeliveryTag: 1, Acknowledger: settlement}
	svc.handleDelivery(context.Background(), sender, delivery)
	require.True(test, settlement.nacked)
	require.False(test, settlement.requeued)
	require.Len(test, observed.All(), 1)
	require.NotContains(test, observed.All()[0].Message, "credential-value")
	require.Empty(test, observed.All()[0].Context)
}

func TestShutdownStopsAdmissionAndHonorsDeadline(test *testing.T) {
	svc, _ := deliveryFixture(test)
	pollingCtx, stop := context.WithCancel(context.Background())
	workCtx, cancelWork := context.WithCancel(context.Background())
	svc.stopPolling, svc.cancelWork, svc.finished = stop, cancelWork, make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(test, svc.Shutdown(ctx), context.Canceled)
	require.ErrorIs(test, pollingCtx.Err(), context.Canceled)
	require.ErrorIs(test, workCtx.Err(), context.Canceled)
	_, err := svc.Send(context.Background(), &entity.HTTPMessage{URL: "https://example.invalid"}, nil)
	require.ErrorIs(test, err, ErrShuttingDown)
}
