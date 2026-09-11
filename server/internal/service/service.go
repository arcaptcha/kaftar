package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Service interface {
	BeginShutdown()
	Health() Health
	Send(context.Context, entity.Message, *SendOptions) (string, error)
	Status(context.Context, string) (entity.OutboxStatus, error)
	Shutdown(context.Context) error
}

type MQConnectionGetter func() (*amqp.Connection, error)

type Observer interface {
	ObserveBrokerError(operation string, channel entity.Channel)
	ObserveDeliveryAttempt(channel entity.Channel)
	ObserveDeliveryResult(channel entity.Channel, outcome string, duration time.Duration)
	ObserveSubmission(channel entity.Channel, outcome string)
	SetWorkerReady(worker string, channel entity.Channel, ready bool)
}

type Config struct {
	MessageRetryMaxAge time.Duration
	Observer           Observer
}

type WorkerHealth struct {
	Running   bool
	Connected bool
}

type Health struct {
	ShuttingDown bool
	RelayRunning bool
	Consumers    map[entity.Channel]WorkerHealth
	Providers    []entity.Channel
}

func (config *Config) OrDefault() *Config {
	if config == nil {
		return &Config{}
	}
	copyConfig := *config
	if copyConfig.MessageRetryMaxAge < 0 {
		copyConfig.MessageRetryMaxAge = 0
	}
	return &copyConfig
}

type SendOptions struct {
	MaxRetries     int
	SendAt         *time.Time
	IdempotencyKey string
}

var (
	ErrSenderUnavailable = errors.New("sender unavailable for channel")
	ErrInvalidSubmission = errors.New("invalid submission")
	ErrShuttingDown      = errors.New("service is shutting down")
)

type service struct {
	logger       *zap.Logger
	repo         repository.DurableOutbox
	senders      map[entity.Channel]entity.Sender
	mqConnGetter MQConnectionGetter
	mqConn       *amqp.Connection
	connectionMu sync.Mutex
	cfg          *Config
	stopPolling  context.CancelFunc
	cancelWork   context.CancelFunc
	finished     chan struct{}
	admissionMu  sync.Mutex
	stopping     bool
	healthMu     sync.RWMutex
	relayRunning bool
	consumers    map[entity.Channel]WorkerHealth
	observer     Observer
}

func New(ctx context.Context, logger *zap.Logger, repo repository.DurableOutbox, getter MQConnectionGetter, cfg *Config, senders ...entity.Sender) (Service, error) {
	if logger == nil || repo == nil || getter == nil {
		return nil, errors.New("service dependencies are required")
	}
	pollingCtx, stopPolling := context.WithCancel(ctx)
	workCtx, cancelWork := context.WithCancel(ctx)
	normalizedConfig := cfg.OrDefault()
	svc := &service{
		logger:       logger,
		repo:         repo,
		cfg:          normalizedConfig,
		mqConnGetter: getter,
		senders:      make(map[entity.Channel]entity.Sender),
		stopPolling:  stopPolling,
		cancelWork:   cancelWork,
		finished:     make(chan struct{}),
		consumers:    make(map[entity.Channel]WorkerHealth),
		observer:     normalizedConfig.Observer,
	}
	for _, sender := range senders {
		if sender == nil || !sender.Channel().IsValid() {
			stopPolling()
			cancelWork()
			return nil, errors.New("invalid sender")
		}
		svc.senders[sender.Channel()] = sender
		svc.consumers[sender.Channel()] = WorkerHealth{}
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() { defer workers.Done(); svc.relay(pollingCtx) }()
	for channel, sender := range svc.senders {
		workers.Add(1)
		go func() { defer workers.Done(); svc.consume(pollingCtx, workCtx, channel, sender) }()
	}
	go func() { workers.Wait(); close(svc.finished) }()
	return svc, nil
}

func (svc *service) Send(ctx context.Context, message entity.Message, options *SendOptions) (string, error) {
	svc.admissionMu.Lock()
	stopping := svc.stopping
	svc.admissionMu.Unlock()
	if stopping {
		svc.observeSubmission(channelOf(message), "shutting_down")
		return "", ErrShuttingDown
	}
	if message == nil {
		svc.observeSubmission("", "invalid")
		return "", ErrInvalidSubmission
	}
	if _, exists := svc.senders[message.Channel()]; !exists {
		svc.observeSubmission(message.Channel(), "unavailable")
		return "", ErrSenderUnavailable
	}
	id, err := svc.accept(ctx, message, options)
	svc.observeSubmission(message.Channel(), submissionOutcome(err))
	return id, err
}

func (svc *service) accept(ctx context.Context, message entity.Message, options *SendOptions) (string, error) {
	opt := SendOptions{}
	if options != nil {
		opt = *options
	}
	opt.MaxRetries = normalizeMaxRetries(opt.MaxRetries)
	if len(opt.IdempotencyKey) > 200 {
		return "", ErrInvalidSubmission
	}
	for _, character := range opt.IdempotencyKey {
		if character < 33 || character > 126 {
			return "", ErrInvalidSubmission
		}
	}
	payload, err := json.Marshal(message)
	if err != nil || string(payload) == "null" {
		return "", ErrInvalidSubmission
	}
	copyMessage, err := entity.UnmarshalMessage(message.Channel(), payload)
	if err != nil || copyMessage.Validate() != nil {
		return "", ErrInvalidSubmission
	}
	if httpMessage, ok := copyMessage.(*entity.HTTPMessage); ok {
		httpMessage.Method = httpMessage.EffectiveMethod()
		httpMessage.URL = strings.TrimSpace(httpMessage.URL)
	}
	payload, err = json.Marshal(copyMessage)
	if err != nil {
		return "", ErrInvalidSubmission
	}
	if opt.SendAt != nil {
		value := opt.SendAt.UTC().Truncate(time.Microsecond)
		if value.Year() < 1 || value.Year() > 9999 {
			return "", ErrInvalidSubmission
		}
		opt.SendAt = &value
	}
	canonical, err := json.Marshal(struct {
		Payload    json.RawMessage
		MaxRetries int
		SendAt     *time.Time
	}{payload, opt.MaxRetries, opt.SendAt})
	if err != nil {
		return "", ErrInvalidSubmission
	}
	fingerprint := sha256.Sum256(canonical)
	now := time.Now().UTC()
	eligible := now
	var nextRetry *time.Time
	if opt.SendAt != nil && opt.SendAt.After(now) {
		eligible = *opt.SendAt
		nextRetry = &eligible
	}
	var deadline *time.Time
	if svc.cfg.MessageRetryMaxAge > 0 {
		value := eligible.Add(svc.cfg.MessageRetryMaxAge)
		deadline = &value
	}
	outbox, err := svc.repo.Create(ctx, &entity.Outbox{Channel: copyMessage.Channel(), Payload: payload, State: entity.OutboxStatePending, MessageID: opt.IdempotencyKey, Fingerprint: hex.EncodeToString(fingerprint[:]), MaxRetries: opt.MaxRetries, NextRetryAt: nextRetry, EligibleAt: eligible, RetryDeadline: deadline})
	if err != nil {
		return "", err
	}
	return outbox.ID.String(), nil
}

func (svc *service) mqConnection() (*amqp.Connection, error) {
	svc.connectionMu.Lock()
	defer svc.connectionMu.Unlock()
	if svc.mqConn != nil && !svc.mqConn.IsClosed() {
		return svc.mqConn, nil
	}
	connection, err := svc.mqConnGetter()
	if err != nil {
		return nil, errors.New("broker connection unavailable")
	}
	svc.mqConn = connection
	return connection, nil
}

func declareTopology(channel *amqp.Channel, destination entity.Channel) error {
	name := destination.String()
	if _, err := channel.QueueDeclare(name, true, false, false, false, nil); err != nil {
		return err
	}
	_, err := channel.QueueDeclare(name+".delay", true, false, false, false, amqp.Table{"x-dead-letter-exchange": "", "x-dead-letter-routing-key": name})
	return err
}

func (svc *service) publish(ctx context.Context, outbox *entity.Outbox) error {
	connection, err := svc.mqConnection()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = connection.CloseDeadline(time.Now()) })
	defer stop()
	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	if err := declareTopology(channel, outbox.Channel); err != nil {
		return err
	}
	if err := channel.Confirm(false); err != nil {
		return err
	}
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	confirmation := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	closed := channel.NotifyClose(make(chan *amqp.Error, 1))
	body, err := json.Marshal(struct {
		ID      uuid.UUID      `json:"id"`
		Channel entity.Channel `json:"channel"`
	}{outbox.ID, outbox.Channel})
	if err != nil {
		return err
	}
	if err := channel.PublishWithContext(ctx, "", outbox.Channel.String(), true, false, amqp.Publishing{DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: body}); err != nil {
		return err
	}
	return awaitConfirmation(ctx, returns, confirmation, closed)
}

func awaitConfirmation(ctx context.Context, returns <-chan amqp.Return, confirmation <-chan amqp.Confirmation, closed <-chan *amqp.Error) error {
	select {
	case <-returns:
		return errors.New("broker returned unroutable notification")
	case result, open := <-confirmation:
		if !open || !result.Ack {
			return errors.New("broker did not confirm notification")
		}
		select {
		case <-returns:
			return errors.New("broker returned notification")
		default:
			return nil
		}
	case <-closed:
		return errors.New("broker channel closed before confirmation")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (svc *service) relay(ctx context.Context) {
	svc.setRelayRunning(true)
	defer svc.setRelayRunning(false)
	for ctx.Err() == nil {
		for channel := range svc.senders {
			rows, err := svc.repo.ReserveDispatch(ctx, channel, time.Now().UTC(), 10*time.Second, 32)
			if err != nil {
				svc.logger.Warn("outbox reconciliation failed")
				continue
			}
			for _, row := range rows {
				if ctx.Err() != nil {
					return
				}
				if err := svc.publish(ctx, &row); err != nil {
					svc.observeBrokerError("publish", channel)
					svc.logger.Warn("notification dispatch failed", zap.String("channel", channel.String()))
					break
				}
			}
		}
		if !pause(ctx, time.Second) {
			return
		}
	}
}

func (svc *service) consume(pollingCtx, workCtx context.Context, destination entity.Channel, sender entity.Sender) {
	svc.setConsumerHealth(destination, WorkerHealth{Running: true})
	defer svc.setConsumerHealth(destination, WorkerHealth{})
	for pollingCtx.Err() == nil {
		svc.consumeConnection(pollingCtx, workCtx, destination, sender)
		if !pause(pollingCtx, time.Second) {
			return
		}
	}
}

func (svc *service) consumeConnection(pollingCtx, workCtx context.Context, destination entity.Channel, sender entity.Sender) {
	svc.setConsumerHealth(destination, WorkerHealth{Running: true})
	connection, err := svc.mqConnection()
	if err != nil {
		svc.observeBrokerError("consume", destination)
		return
	}
	setupCtx, cancel := context.WithTimeout(pollingCtx, 5*time.Second)
	stopSetup := context.AfterFunc(setupCtx, func() { _ = connection.CloseDeadline(time.Now()) })
	channel, err := connection.Channel()
	if err != nil {
		stopSetup()
		cancel()
		return
	}
	stopWork := context.AfterFunc(workCtx, func() { _ = connection.CloseDeadline(time.Now()) })
	defer stopWork()
	defer channel.Close()
	if err = declareTopology(channel, destination); err == nil {
		err = channel.Qos(1, 0, false)
	}
	var deliveries <-chan amqp.Delivery
	if err == nil {
		deliveries, err = channel.Consume(destination.String(), "", false, false, false, false, nil)
	}
	stopSetup()
	cancel()
	if err != nil {
		svc.observeBrokerError("consume", destination)
		return
	}
	svc.setConsumerHealth(destination, WorkerHealth{Running: true, Connected: true})
	defer svc.setConsumerHealth(destination, WorkerHealth{Running: true})
	for {
		select {
		case <-pollingCtx.Done():
			return
		case delivery, open := <-deliveries:
			if !open || pollingCtx.Err() != nil {
				return
			}
			if !svc.handleDelivery(workCtx, sender, delivery) {
				if !pause(pollingCtx, time.Second) {
					return
				}
			}
		}
	}
}

func (svc *service) handleDelivery(ctx context.Context, sender entity.Sender, delivery amqp.Delivery) bool {
	svc.admissionMu.Lock()
	stopping := svc.stopping
	svc.admissionMu.Unlock()
	if stopping {
		_ = delivery.Nack(false, true)
		return false
	}
	var envelope entity.Outbox
	if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
		svc.logger.Warn("rejecting malformed delivery envelope")
		_ = delivery.Nack(false, false)
		return true
	}
	if envelope.ID == uuid.Nil {
		channel := envelope.Channel
		if channel == "" {
			channel = sender.Channel()
		}
		message, err := entity.UnmarshalMessage(channel, envelope.Payload)
		if err != nil {
			svc.logger.Warn("rejecting invalid direct delivery payload", zap.String("channel", channel.String()))
			_ = delivery.Nack(false, false)
			return true
		}
		_, err = svc.accept(ctx, message, &SendOptions{MaxRetries: envelope.MaxRetries, SendAt: envelope.NextRetryAt, IdempotencyKey: envelope.MessageID})
		svc.observeSubmission(channel, submissionOutcome(err))
		if err != nil {
			permanent := errors.Is(err, ErrInvalidSubmission) || errors.Is(err, repository.ErrIdempotencyConflict)
			if permanent {
				svc.logger.Warn("rejecting direct delivery submission", zap.String("channel", channel.String()))
			} else {
				svc.logger.Warn("direct delivery persistence failed", zap.String("channel", channel.String()))
			}
			_ = delivery.Nack(false, !permanent)
			return permanent
		}
		_ = delivery.Ack(false)
		return true
	}
	outbox, err := svc.repo.Claim(ctx, envelope.ID.String(), sender.Channel(), time.Now().UTC(), 60*time.Second)
	if err != nil {
		permanent := errors.Is(err, repository.ErrRecordNotFound)
		if permanent {
			svc.logger.Warn("rejecting unknown outbox notification", zap.String("channel", sender.Channel().String()))
		} else {
			svc.logger.Warn("outbox claim failed", zap.String("channel", sender.Channel().String()))
		}
		_ = delivery.Nack(false, !permanent)
		return permanent
	}
	if outbox == nil {
		_ = delivery.Ack(false)
		return true
	}
	message, err := entity.UnmarshalMessage(outbox.Channel, outbox.Payload)
	attemptStarted := time.Now()
	svc.observeDeliveryAttempt(sender.Channel())
	if err == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = sender.Send(attemptCtx, message)
		cancel()
	}
	now := time.Now().UTC()
	if err == nil {
		outbox.State = entity.OutboxStateSent
	} else {
		outbox.LastError = "invalid stored message"
		if message != nil {
			outbox.LastError = err.Error()
		}
		outbox.State = entity.OutboxStateFailed
		next := now.Add(durationJitter(retryDelay(outbox.RetryCount + 1)))
		outbox.NextRetryAt = &next
		if retryLimitReached(*outbox) || (outbox.RetryDeadline != nil && !now.Before(*outbox.RetryDeadline)) {
			outbox.State = entity.OutboxStateDead
		}
	}
	if err := svc.repo.Finish(ctx, outbox, now); err != nil {
		svc.observeDeliveryResult(
			sender.Channel(),
			"completion_error",
			time.Since(attemptStarted),
		)
		svc.logger.Warn("outbox completion failed", zap.String("channel", sender.Channel().String()))
		_ = delivery.Nack(false, true)
		return false
	}
	svc.observeDeliveryResult(
		sender.Channel(),
		deliveryOutcome(outbox.State),
		time.Since(attemptStarted),
	)
	_ = delivery.Ack(false)
	return true
}

func pause(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func normalizeMaxRetries(value int) int {
	if value < 0 {
		return -1
	}
	return value
}
func retryLimitReached(outbox entity.Outbox) bool {
	return outbox.MaxRetries >= 0 && outbox.RetryCount >= outbox.MaxRetries
}
func retryAgeExceeded(outbox entity.Outbox, maxAge time.Duration, now time.Time) bool {
	return maxAge > 0 && outbox.CreatedAt != nil && !now.Before(outbox.CreatedAt.Add(maxAge))
}
func retryDelay(count int) time.Duration {
	if count > 9 {
		return 15 * time.Minute
	}
	return time.Second * time.Duration(1<<count)
}
func durationJitter(duration time.Duration) time.Duration {
	return duration - duration/10 + time.Duration(rand.Int64N(int64(duration/5)))
}

func (svc *service) Status(ctx context.Context, id string) (entity.OutboxStatus, error) {
	if _, err := uuid.Parse(id); err != nil {
		return entity.OutboxStatus{}, repository.ErrRecordNotFound
	}
	rows, err := svc.repo.Get(ctx, &entity.OutboxFilter{ID: id})
	if err != nil {
		return entity.OutboxStatus{}, err
	}
	if len(rows) != 1 {
		return entity.OutboxStatus{}, repository.ErrRecordNotFound
	}
	return rows[0].Status(), nil
}

func (svc *service) BeginShutdown() {
	svc.admissionMu.Lock()
	svc.stopping = true
	svc.admissionMu.Unlock()
	svc.stopPolling()
}

func (svc *service) Health() Health {
	svc.admissionMu.Lock()
	stopping := svc.stopping
	svc.admissionMu.Unlock()

	svc.healthMu.RLock()
	defer svc.healthMu.RUnlock()
	consumers := make(map[entity.Channel]WorkerHealth, len(svc.senders))
	providers := make([]entity.Channel, 0, len(svc.senders))
	for channel := range svc.senders {
		consumers[channel] = svc.consumers[channel]
		providers = append(providers, channel)
	}
	return Health{
		ShuttingDown: stopping,
		RelayRunning: svc.relayRunning,
		Consumers:    consumers,
		Providers:    providers,
	}
}

func (svc *service) setRelayRunning(running bool) {
	svc.healthMu.Lock()
	svc.relayRunning = running
	svc.healthMu.Unlock()
	if svc.observer != nil {
		svc.observer.SetWorkerReady("relay", "", running)
	}
}

func (svc *service) setConsumerHealth(channel entity.Channel, health WorkerHealth) {
	svc.healthMu.Lock()
	if svc.consumers == nil {
		svc.consumers = make(map[entity.Channel]WorkerHealth)
	}
	svc.consumers[channel] = health
	svc.healthMu.Unlock()
	if svc.observer != nil {
		svc.observer.SetWorkerReady("consumer", channel, health.Connected)
	}
}

func (svc *service) observeSubmission(channel entity.Channel, outcome string) {
	if svc.observer != nil {
		svc.observer.ObserveSubmission(channel, outcome)
	}
}

func (svc *service) observeDeliveryAttempt(channel entity.Channel) {
	if svc.observer != nil {
		svc.observer.ObserveDeliveryAttempt(channel)
	}
}

func (svc *service) observeDeliveryResult(
	channel entity.Channel,
	outcome string,
	duration time.Duration,
) {
	if svc.observer != nil {
		svc.observer.ObserveDeliveryResult(channel, outcome, duration)
	}
}

func (svc *service) observeBrokerError(operation string, channel entity.Channel) {
	if svc.observer != nil {
		svc.observer.ObserveBrokerError(operation, channel)
	}
}

func channelOf(message entity.Message) entity.Channel {
	if message == nil {
		return ""
	}
	return message.Channel()
}

func submissionOutcome(err error) string {
	switch {
	case err == nil:
		return "accepted"
	case errors.Is(err, ErrInvalidSubmission):
		return "invalid"
	case errors.Is(err, ErrSenderUnavailable):
		return "unavailable"
	case errors.Is(err, ErrShuttingDown):
		return "shutting_down"
	case errors.Is(err, repository.ErrIdempotencyConflict):
		return "conflict"
	default:
		return "error"
	}
}

func deliveryOutcome(state entity.OutboxState) string {
	switch state {
	case entity.OutboxStateSent:
		return "sent"
	case entity.OutboxStateFailed:
		return "retry"
	case entity.OutboxStateDead:
		return "dead"
	default:
		return "invalid"
	}
}

func (svc *service) Shutdown(ctx context.Context) error {
	svc.BeginShutdown()
	select {
	case <-svc.finished:
		svc.cancelWork()
		svc.connectionMu.Lock()
		defer svc.connectionMu.Unlock()
		if svc.mqConn != nil {
			_ = svc.mqConn.CloseDeadline(time.Now().Add(time.Second))
		}
		return nil
	case <-ctx.Done():
		svc.cancelWork()
		return ctx.Err()
	}
}
