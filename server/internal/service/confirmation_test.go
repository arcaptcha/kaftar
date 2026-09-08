package service

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestConfirmationFailures(test *testing.T) {
	for _, scenario := range []string{"negative", "missing", "closed", "returned-and-confirmed"} {
		test.Run(scenario, func(test *testing.T) {
			returns := make(chan amqp.Return, 1)
			confirmations := make(chan amqp.Confirmation, 1)
			closed := make(chan *amqp.Error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "negative":
				confirmations <- amqp.Confirmation{Ack: false}
			case "missing":
				cancel()
			case "closed":
				close(closed)
			case "returned-and-confirmed":
				returns <- amqp.Return{}
				confirmations <- amqp.Confirmation{Ack: true}
			}
			require.Error(test, awaitConfirmation(ctx, returns, confirmations, closed))
		})
	}
}
