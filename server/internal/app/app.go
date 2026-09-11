package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/arcaptcha/kaftar/server/internal/config"
	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/observability"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	outboxrepo "github.com/arcaptcha/kaftar/server/internal/repository/outbox_repo"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/arcaptcha/kaftar/server/internal/service/bale"
	"github.com/arcaptcha/kaftar/server/internal/service/email"
	"github.com/arcaptcha/kaftar/server/internal/service/http"
	"github.com/arcaptcha/kaftar/server/internal/service/mattermost"
	"github.com/arcaptcha/kaftar/server/internal/service/sms"
	"github.com/arcaptcha/kaftar/server/pkg/postgres"
	"github.com/arcaptcha/kaftar/server/pkg/rabbitmq"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type App interface {
	Logger() *zap.Logger
	Config() config.Config
	Health() *observability.Health
	Metrics() *observability.Metrics
	Service(ctx context.Context) service.Service
	Shutdown(context.Context) error
}

type app struct {
	cfg  config.Config
	log  *zap.Logger
	db   *gorm.DB
	svc  service.Service
	repo repository.DurableOutbox

	health         *observability.Health
	metrics        *observability.Metrics
	runtimeCtx     context.Context
	cancelRuntime  context.CancelFunc
	collectorDone  <-chan struct{}
	rabbitmqConfig *rabbitmq.Config
}

func New(c config.Config, l *zap.Logger) App {
	if l == nil {
		panic("logger is not configured")
	}

	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	app := &app{
		cfg:           c,
		log:           l,
		metrics:       observability.NewMetrics(),
		runtimeCtx:    runtimeCtx,
		cancelRuntime: cancelRuntime,
	}

	if err := app.initDB(); err != nil {
		l.Fatal("database initialization failed", zap.Error(err))
	}
	if err := app.initService(runtimeCtx); err != nil {
		l.Fatal("service initialization failed", zap.Error(err))
	}
	if err := app.initObservability(); err != nil {
		l.Fatal("observability initialization failed", zap.Error(err))
	}
	return app
}

func (a *app) Config() config.Config { return a.cfg }

func (a *app) Logger() *zap.Logger { return a.log }

func (a *app) Health() *observability.Health { return a.health }

func (a *app) Metrics() *observability.Metrics { return a.metrics }

func (a *app) initDB() error {
	if a.db != nil {
		return nil
	}
	db, err := postgres.NewPsqlGormConnection(&postgres.DBConnOptions{
		User:   a.cfg.Postgres.User,
		Pass:   a.cfg.Postgres.Pass,
		Host:   a.cfg.Postgres.Host,
		Port:   a.cfg.Postgres.Port,
		DBName: a.cfg.Postgres.DBName,
		Schema: a.cfg.Postgres.Schema,
	})
	if err != nil {
		return err
	}
	if err := outboxrepo.Migrate(db, a.cfg.MessageRetryMaxAge); err != nil {
		return err
	}
	a.db = db
	a.repo = outboxrepo.New(db)
	return nil
}

func (a *app) initService(ctx context.Context) error {
	if a.svc != nil {
		return nil
	}
	a.rabbitmqConfig = &rabbitmq.Config{
		Host: a.cfg.Rabbitmq.Host,
		Port: a.cfg.Rabbitmq.Port,
		User: a.cfg.Rabbitmq.Username,
		Pass: a.cfg.Rabbitmq.Password,
	}
	mqConnGetter := rabbitmq.NewConnectionGetter(a.rabbitmqConfig)

	senders := make([]entity.Sender, 0)
	if a.cfg.SMS.Enabled {
		smsSender := sms.New(a.cfg.SMS.ApiKey, a.cfg.SMS.Phone)
		senders = append(senders, smsSender)
	}
	if a.cfg.Email.Enabled {
		emailSender, err := email.New(email.Config{
			Host:        a.cfg.Email.Host,
			Port:        a.cfg.Email.Port,
			Username:    a.cfg.Email.Username,
			Password:    a.cfg.Email.Password,
			FromAddress: a.cfg.Email.FromAddress,
		}, a.Logger())
		if err != nil {
			return err
		}
		senders = append(senders, emailSender)
	}
	if a.cfg.Mattermost.Enabled {
		mattermostSender, err := mattermost.New(mattermost.Config{
			MattermostURL: a.cfg.Mattermost.MattermostURL,
			PAT:           a.cfg.Mattermost.PAT,
		})
		if err != nil {
			return err
		}
		senders = append(senders, mattermostSender)
	}
	if a.cfg.Bale.Enabled {
		baleSender, err := bale.New(bale.Config{BotToken: a.cfg.Bale.BotToken})
		if err != nil {
			return err
		}
		senders = append(senders, baleSender)
	}
	if a.cfg.HttpSender.Enabled {
		httpSender, err := http.New(&http.Config{
			AllowedDestinations:  a.cfg.HttpSender.AllowedDestinations,
			AllowedPrivateCIDRs:  a.cfg.HttpSender.AllowedPrivateCIDRs,
			RequestBodyMaxBytes:  a.cfg.HttpSender.RequestBodyMaxBytes,
			ResponseBodyMaxBytes: a.cfg.HttpSender.ResponseBodyMaxBytes,
		})
		if err != nil {
			return err
		}
		senders = append(senders, httpSender)
	}

	cfg := &service.Config{
		MessageRetryMaxAge: a.cfg.MessageRetryMaxAge,
		Observer:           a.metrics,
	}

	svc, err := service.New(ctx, a.Logger(), a.repo, mqConnGetter, cfg, senders...)
	if err != nil {
		a.log.Fatal("service creation failed", zap.Error(err))
	}
	a.svc = svc
	return nil
}

func (a *app) initObservability() error {
	database, err := a.db.DB()
	if err != nil {
		return err
	}
	a.health = observability.NewHealth(
		database,
		func(ctx context.Context, channels []entity.Channel) error {
			return rabbitmq.Check(ctx, a.rabbitmqConfig, channels)
		},
		a.svc,
		a.metrics,
		observability.HealthConfig{
			CheckTimeout:     a.cfg.Health.CheckTimeout,
			ComponentTimeout: a.cfg.Health.ComponentTimeout,
		},
	)
	a.collectorDone = a.metrics.StartOutboxCollector(
		a.runtimeCtx,
		a.repo,
		observability.OutboxCollectorConfig{
			Interval: a.cfg.Metrics.OutboxRefreshInterval,
			Timeout:  a.cfg.Metrics.OutboxRefreshTimeout,
		},
	)
	return nil
}

func (a *app) Service(ctx context.Context) service.Service {
	if a.svc != nil {
		return a.svc
	}
	if err := a.initService(ctx); err != nil {
		a.log.Fatal("service initialization failed", zap.Error(err))
	}
	return a.svc
}

func (a *app) Shutdown(ctx context.Context) error {
	serviceErr := a.svc.Shutdown(ctx)
	a.cancelRuntime()
	collectorErr := waitForCollector(ctx, a.collectorDone)
	connection, dbErr := a.database()
	if dbErr != nil {
		return errors.Join(serviceErr, collectorErr, dbErr)
	}
	return errors.Join(serviceErr, collectorErr, connection.Close())
}

func (a *app) database() (*sql.DB, error) {
	return a.db.DB()
}

func waitForCollector(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
