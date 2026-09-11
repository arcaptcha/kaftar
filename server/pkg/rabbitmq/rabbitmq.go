package rabbitmq

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
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

func Check(ctx context.Context, config *Config, channels []entity.Channel) error {
	dialer := &net.Dialer{}
	connection, err := amqp.DialConfig(config.URL(), amqp.Config{
		Heartbeat: 10 * time.Second,
		Dial: func(network, address string) (net.Conn, error) {
			connection, dialErr := dialer.DialContext(ctx, network, address)
			if dialErr != nil {
				return nil, dialErr
			}
			if deadline, ok := ctx.Deadline(); ok {
				if deadlineErr := connection.SetDeadline(deadline); deadlineErr != nil {
					_ = connection.Close()
					return nil, deadlineErr
				}
			}
			return connection, nil
		},
	})
	if err != nil {
		return err
	}
	defer connection.Close()
	amqpChannel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer amqpChannel.Close()
	for _, channel := range channels {
		if _, err := amqpChannel.QueueInspect(channel.String()); err != nil {
			return err
		}
		if _, err := amqpChannel.QueueInspect(channel.String() + ".delay"); err != nil {
			return err
		}
	}
	return nil
}
