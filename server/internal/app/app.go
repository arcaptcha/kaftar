package app

import (
	"context"

	"github.com/arcaptcha/kaftar/server/internal/config"
	"github.com/arcaptcha/kaftar/server/internal/entity"
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
	Service(ctx context.Context) service.Service
	Shutdown(context.Context) error
}

type app struct {
	cfg config.Config
	log *zap.Logger
	db  *gorm.DB
	svc service.Service
}

func New(c config.Config, l *zap.Logger) App {
	if l == nil {
		panic("logger is not configured")
	}

	app := &app{
		cfg: c,
		log: l,
	}

	if err := app.initDB(); err != nil {
		l.Fatal("database initialization failed", zap.Error(err))
	}
	if err := app.initService(context.Background()); err != nil {
		l.Fatal("service initialization failed", zap.Error(err))
	}
	return app
}

func (a *app) Config() config.Config { return a.cfg }

func (a *app) Logger() *zap.Logger { return a.log }

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
	return nil
}

func (a *app) initService(ctx context.Context) error {
	if a.svc != nil {
		return nil
	}
	mqConnGetter := rabbitmq.NewConnectionGetter(&rabbitmq.Config{
		Host: a.cfg.Rabbitmq.Host,
		Port: a.cfg.Rabbitmq.Port,
		User: a.cfg.Rabbitmq.Username,
		Pass: a.cfg.Rabbitmq.Password,
	})

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

	cfg := &service.Config{MessageRetryMaxAge: a.cfg.MessageRetryMaxAge}

	svc, err := service.New(ctx, a.Logger(), outboxrepo.New(a.db), mqConnGetter, cfg, senders...)
	if err != nil {
		a.log.Fatal("service creation failed", zap.Error(err))
	}
	a.svc = svc
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
	err := a.svc.Shutdown(ctx)
	if err != nil {
		return err
	}
	connection, dbErr := a.db.DB()
	if dbErr != nil {
		return dbErr
	}
	return connection.Close()
}
