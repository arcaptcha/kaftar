package rabbitmq

import (
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/service"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	Host string
	Port int16
	User string
	Pass string
}

func (c *Config) URL() string {
	address := &url.URL{Scheme: "amqp", Host: net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port))), User: url.UserPassword(c.User, c.Pass), Path: "/"}
	return address.String()
}

func New(c *Config) (*amqp.Connection, error) {
	return amqp.DialConfig(c.URL(), amqp.Config{Heartbeat: 10 * time.Second, Dial: amqp.DefaultDial(5 * time.Second)})
}

func NewConnectionGetter(c *Config) service.MQConnectionGetter {
	return func() (*amqp.Connection, error) {
		return New(c)
	}
}
