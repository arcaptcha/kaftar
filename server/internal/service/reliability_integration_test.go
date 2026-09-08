package service

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
	outboxrepo "github.com/arcaptcha/kaftar/server/internal/repository/outbox_repo"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func postgresBrokerFixture(test *testing.T) (*gorm.DB, repository.DurableOutbox, MQConnectionGetter) {
	test.Helper()
	dsn, brokerURL := os.Getenv("KAFTAR_INTEGRATION_POSTGRES"), os.Getenv("KAFTAR_RELIABILITY_AMQP")
	if dsn == "" || brokerURL == "" {
		test.Skip("requires disposable PostgreSQL and a dedicated kaftar-hardening RabbitMQ vhost")
	}
	parsed, err := url.Parse(brokerURL)
	require.NoError(test, err)
	if parsed.Hostname() != "127.0.0.1" || parsed.Path != "/kaftar-hardening" {
		test.Fatal("reliability tests require the dedicated local kaftar-hardening vhost")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(test, err)
	adminPool, err := admin.DB()
	require.NoError(test, err)
	test.Cleanup(func() { _ = adminPool.Close() })
	schema := "kaftar_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(test, admin.Exec("CREATE SCHEMA "+schema).Error)
	test.Cleanup(func() { require.NoError(test, admin.Exec("DROP SCHEMA "+schema+" CASCADE").Error) })
	database, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Discard})
	require.NoError(test, err)
	pool, err := database.DB()
	require.NoError(test, err)
	test.Cleanup(func() { _ = pool.Close() })
	require.NoError(test, outboxrepo.Migrate(database, 0))
	getter := func() (*amqp.Connection, error) {
		return amqp.DialConfig(brokerURL, amqp.Config{Dial: amqp.DefaultDial(3 * time.Second)})
	}
	connection, err := getter()
	require.NoError(test, err)
	channel, err := connection.Channel()
	require.NoError(test, err)
	require.NoError(test, declareTopology(channel, entity.ChannelHTTP))
	_, err = channel.QueuePurge("http", false)
	require.NoError(test, err)
	require.NoError(test, channel.Close())
	require.NoError(test, connection.Close())
	return database, outboxrepo.New(database), getter
}

func startReliabilityService(test *testing.T, store repository.DurableOutbox, getter MQConnectionGetter, sender entity.Sender) *service {
	test.Helper()
	backend, err := New(context.Background(), zap.NewNop(), store, getter, nil, sender)
	require.NoError(test, err)
	test.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(test, backend.Shutdown(ctx))
	})
	return backend.(*service)
}

func waitForSent(test *testing.T, store repository.Outbox, id string) {
	test.Helper()
	require.Eventually(test, func() bool {
		rows, err := store.Get(context.Background(), &entity.OutboxFilter{ID: id})
		return err == nil && rows[0].State == entity.OutboxStateSent
	}, 20*time.Second, 20*time.Millisecond)
}

func TestPostgresConcurrentIdentityAndClaims(test *testing.T) {
	_, store, _ := postgresBrokerFixture(test)
	sender := &deliverySender{}
	svc := &service{repo: store, cfg: &Config{}, senders: map[entity.Channel]entity.Sender{entity.ChannelHTTP: sender}}
	var workers sync.WaitGroup
	ids, failures := make(chan string, 24), make(chan error, 24)
	for index := 0; index < 24; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			id, err := svc.Send(context.Background(), &entity.HTTPMessage{URL: "https://example.invalid"}, &SendOptions{IdempotencyKey: "concurrent"})
			ids <- id
			failures <- err
		}()
	}
	workers.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		require.NoError(test, err)
	}
	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		}
		require.Equal(test, expected, id)
	}
	claims := make(chan *entity.Outbox, 24)
	failures = make(chan error, 24)
	for index := 0; index < 24; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			claim, err := store.Claim(context.Background(), expected, entity.ChannelHTTP, time.Now(), time.Minute)
			claims <- claim
			failures <- err
		}()
	}
	workers.Wait()
	close(claims)
	close(failures)
	for err := range failures {
		if !errors.Is(err, repository.ErrRecordNotFound) {
			require.NoError(test, err)
		}
	}
	count := 0
	for claim := range claims {
		if claim != nil {
			count++
		}
	}
	require.Equal(test, 1, count)
}

func TestPostgresCommitFailureNeverAccepts(test *testing.T) {
	database, store, _ := postgresBrokerFixture(test)
	require.NoError(test, database.Exec(`CREATE FUNCTION reject_outbox_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected commit failure'; END; $$`).Error)
	require.NoError(test, database.Exec(`CREATE CONSTRAINT TRIGGER reject_outbox_commit AFTER INSERT ON outboxes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_outbox_commit()`).Error)
	sender := &deliverySender{}
	svc := &service{repo: store, cfg: &Config{}, senders: map[entity.Channel]entity.Sender{entity.ChannelHTTP: sender}}
	id, err := svc.Send(context.Background(), &entity.HTTPMessage{URL: "https://example.invalid"}, nil)
	require.Error(test, err)
	require.Empty(test, id)
	var count int64
	require.NoError(test, database.Model(&model.Outbox{}).Count(&count).Error)
	require.Zero(test, count)
}

func TestPostgresCommitBeforePublishAndReconnect(test *testing.T) {
	_, store, getter := postgresBrokerFixture(test)
	sender := &deliverySender{}
	svc := &service{repo: store, cfg: &Config{}, senders: map[entity.Channel]entity.Sender{entity.ChannelHTTP: sender}, mqConnGetter: func() (*amqp.Connection, error) { return nil, errors.New("injected outage") }}
	id, err := svc.Send(context.Background(), &entity.HTTPMessage{URL: "https://example.invalid"}, nil)
	require.NoError(test, err)
	rows, err := store.ReserveDispatch(context.Background(), entity.ChannelHTTP, time.Now(), time.Millisecond, 10)
	require.NoError(test, err)
	require.Len(test, rows, 1)
	require.Error(test, svc.publish(context.Background(), &rows[0]))
	restarted := startReliabilityService(test, store, getter, sender)
	waitForSent(test, store, id)
	require.EqualValues(test, 1, sender.calls.Load())
	restarted.connectionMu.Lock()
	connection := restarted.mqConn
	restarted.connectionMu.Unlock()
	require.NoError(test, connection.Close())
	id, err = restarted.Send(context.Background(), &entity.HTTPMessage{URL: "https://example.invalid"}, nil)
	require.NoError(test, err)
	waitForSent(test, store, id)
	require.EqualValues(test, 2, sender.calls.Load())
}

type failedCompletion struct {
	repository.DurableOutbox
	attempted chan struct{}
	once      sync.Once
}

func (store *failedCompletion) Finish(context.Context, *entity.Outbox, time.Time) error {
	store.once.Do(func() { close(store.attempted) })
	return errors.New("injected commit failure")
}

func TestPostgresAmbiguousAttemptRecovery(test *testing.T) {
	database, store, getter := postgresBrokerFixture(test)
	sender := &deliverySender{}
	broken := &failedCompletion{DurableOutbox: store, attempted: make(chan struct{})}
	first := startReliabilityService(test, broken, getter, sender)
	id, err := first.Send(context.Background(), &entity.HTTPMessage{URL: "https://example.invalid"}, &SendOptions{MaxRetries: 1})
	require.NoError(test, err)
	select {
	case <-broken.attempted:
	case <-time.After(15 * time.Second):
		test.Fatal("provider attempt never reached failed commit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(test, first.Shutdown(ctx))
	require.NoError(test, database.Model(&model.Outbox{}).Where("id = ?", id).Update("lease_until", time.Now().Add(-time.Second)).Error)
	startReliabilityService(test, store, getter, sender)
	waitForSent(test, store, id)
	require.EqualValues(test, 2, sender.calls.Load())
	rows, err := store.Get(context.Background(), &entity.OutboxFilter{ID: id})
	require.NoError(test, err)
	require.Equal(test, 1, rows[0].RetryCount)
	require.Empty(test, rows[0].LastError)
	require.Nil(test, rows[0].NextRetryAt)
}

func TestRabbitMQUnroutableConfirmation(test *testing.T) {
	_, _, getter := postgresBrokerFixture(test)
	connection, err := getter()
	require.NoError(test, err)
	defer connection.Close()
	channel, err := connection.Channel()
	require.NoError(test, err)
	defer channel.Close()
	require.NoError(test, channel.Confirm(false))
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	closed := channel.NotifyClose(make(chan *amqp.Error, 1))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(test, channel.PublishWithContext(ctx, "", "missing-"+uuid.NewString(), true, false, amqp.Publishing{DeliveryMode: amqp.Persistent, Body: []byte(`{}`)}))
	require.ErrorContains(test, awaitConfirmation(ctx, returns, confirmations, closed), "return")
}

func TestPostgresScheduleOrderAndGracefulDrain(test *testing.T) {
	_, store, getter := postgresBrokerFixture(test)
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	sender := &deliverySender{send: func(ctx context.Context, message entity.Message) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	backend := &service{repo: store, cfg: &Config{}, senders: map[entity.Channel]entity.Sender{entity.ChannelHTTP: sender}}
	later := time.Now().Add(time.Hour)
	future, err := backend.Send(context.Background(), &entity.HTTPMessage{URL: "https://future.invalid"}, &SendOptions{SendAt: &later})
	require.NoError(test, err)
	immediate, err := backend.Send(context.Background(), &entity.HTTPMessage{URL: "https://immediate.invalid"}, nil)
	require.NoError(test, err)
	active := startReliabilityService(test, store, getter, sender)
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		test.Fatal("later schedule blocked immediate delivery")
	}
	active.BeginShutdown()
	_, err = active.Send(context.Background(), &entity.HTTPMessage{URL: "https://rejected.invalid"}, nil)
	require.ErrorIs(test, err, ErrShuttingDown)
	finished := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		finished <- active.Shutdown(ctx)
	}()
	release <- struct{}{}
	require.NoError(test, <-finished)
	waitForSent(test, store, immediate)
	rows, err := store.Get(context.Background(), &entity.OutboxFilter{ID: future})
	require.NoError(test, err)
	require.Equal(test, entity.OutboxStatePending, rows[0].State)
	require.Zero(test, rows[0].Attempts)
}
