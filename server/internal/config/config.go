package config

import (
	"time"

	"github.com/arcaptcha/kaftar/server/pkg/fp"
	"github.com/caarlos0/env/v11"
)

type Config struct {
	DevMode            bool                   `env:"DEV_MODE"`
	MessageRetryMaxAge time.Duration          `env:"MESSAGE_RETRY_MAX_AGE"`
	Http               HttpConfig             `envPrefix:"HTTP_"`
	Postgres           PostgresConfig         `envPrefix:"POSTGRES_"`
	SMS                SMSSenderConfig        `envPrefix:"SMS_SENDER_"`
	Email              EmailSenderConfig      `envPrefix:"EMAIL_SENDER_"`
	Mattermost         MattermostSenderConfig `envPrefix:"MATTERMOST_SENDER_"`
	Bale               BaleSenderConfig       `envPrefix:"BALE_SENDER_"`
	HttpSender         HttpSenderConfig       `envPrefix:"HTTP_SENDER_"`
	Rabbitmq           RabbitmqConfig         `envPrefix:"RABBITMQ_"`
	Health             HealthConfig           `envPrefix:"HEALTH_"`
	Metrics            MetricsConfig          `envPrefix:"METRICS_"`
}

type HttpConfig struct {
	Port uint16 `env:"PORT"`
}

type HealthConfig struct {
	CheckTimeout     time.Duration `env:"CHECK_TIMEOUT" envDefault:"3s"`
	ComponentTimeout time.Duration `env:"COMPONENT_TIMEOUT" envDefault:"2s"`
}

type MetricsConfig struct {
	OutboxRefreshInterval time.Duration `env:"OUTBOX_REFRESH_INTERVAL" envDefault:"15s"`
	OutboxRefreshTimeout  time.Duration `env:"OUTBOX_REFRESH_TIMEOUT" envDefault:"2s"`
}

type PostgresConfig struct {
	Host   string `env:"HOST"`
	Port   uint   `env:"PORT"`
	User   string `env:"USER"`
	Pass   string `env:"PASSWORD"`
	DBName string `env:"DB_NAME"`
	Schema string `env:"SCHEMA"`
}

type SMSSenderConfig struct {
	Enabled bool   `env:"ENABLED"`
	ApiKey  string `env:"API_KEY"`
	Phone   string `env:"PHONE"`
}

type EmailSenderConfig struct {
	Enabled     bool   `env:"ENABLED"`
	Host        string `env:"HOST"`
	Port        uint16 `env:"PORT"`
	Username    string `env:"USERNAME"`
	Password    string `env:"PASSWORD"`
	FromAddress string `env:"FROM_ADDRESS"`
}

type MattermostSenderConfig struct {
	Enabled       bool   `env:"ENABLED"`
	MattermostURL string `env:"URL"`
	PAT           string `env:"PAT"` // Personal Access Token
}

type RabbitmqConfig struct {
	Host     string `env:"HOST"`
	Port     int16  `env:"PORT"`
	Username string `env:"USERNAME"`
	Password string `env:"PASSWORD"`
}

type BaleSenderConfig struct {
	Enabled  bool   `env:"ENABLED"`
	BotToken string `env:"BOT_TOKEN"`
}

type HttpSenderConfig struct {
	AllowedDestinations  []string `env:"ALLOWED_DESTINATIONS"`
	AllowedPrivateCIDRs  []string `env:"ALLOWED_PRIVATE_CIDRS"`
	Enabled              bool     `env:"ENABLED"`
	RequestBodyMaxBytes  int64    `env:"REQUEST_BODY_MAX_BYTES" envDefault:"1048576"`
	ResponseBodyMaxBytes int64    `env:"RESPONSE_BODY_MAX_BYTES" envDefault:"1048576"`
}

func ReadEnv() (Config, error) {
	return env.ParseAs[Config]()
}

func MustReadEnv() Config {
	return fp.Must(ReadEnv())
}
