package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/api/dto"
	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestIsolatedDeliveryIntegration(test *testing.T) {
	baseURL := os.Getenv("KAFTAR_INTEGRATION_URL")
	if baseURL == "" {
		test.Skip("set KAFTAR_INTEGRATION_URL, KAFTAR_INTEGRATION_AMQP, KAFTAR_INTEGRATION_POSTGRES, and KAFTAR_INTEGRATION_SINK for an isolated disposable environment")
	}
	brokerURL := os.Getenv("KAFTAR_INTEGRATION_AMQP")
	databaseDSN := os.Getenv("KAFTAR_INTEGRATION_POSTGRES")
	sinkURL := os.Getenv("KAFTAR_INTEGRATION_SINK")
	if brokerURL == "" || databaseDSN == "" || sinkURL == "" {
		test.Fatal("all integration environment variables are required; never target production")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, scenario := range []struct {
		name        string
		destination string
		delay       time.Duration
		state       int
		retries     int
	}{
		{name: "immediate", destination: sinkURL, state: int(entity.OutboxStateSent)},
		{name: "scheduled", destination: sinkURL, delay: 4 * time.Second, state: int(entity.OutboxStateSent)},
		{name: "no-retries", destination: sinkURL + "/missing", state: int(entity.OutboxStateDead)},
		{name: "one-retry", destination: sinkURL + "/missing", retries: 1, state: int(entity.OutboxStateDead)},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			payload, err := json.Marshal(dto.HTTPMessage{URL: scenario.destination, Method: http.MethodGet})
			if err != nil {
				test.Fatal(err)
			}
			target := fmt.Sprintf("%s/api/v1/http/send?max_retries=%d", baseURL, scenario.retries)
			var sendAt time.Time
			if scenario.delay > 0 {
				sendAt = time.Now().Add(scenario.delay).UTC().Truncate(time.Second)
				target += "&send_at=" + url.QueryEscape(sendAt.Format(time.RFC3339))
			}
			response, err := client.Post(target, "application/json", bytes.NewReader(payload))
			if err != nil {
				test.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				test.Fatalf("submission returned %d", response.StatusCode)
			}
			var result dto.SendHTTPResult
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				test.Fatal(err)
			}
			status := awaitIntegrationState(test, client, baseURL, result.ID, scenario.state, sendAt)
			if scenario.state == int(entity.OutboxStateDead) && (status.LastError == "" || status.RetryCount != scenario.retries) {
				test.Fatalf("unexpected dead state: %+v", status)
			}
		})
	}

	test.Run("direct-queue", func(test *testing.T) {
		connection, err := amqp.Dial(brokerURL)
		if err != nil {
			test.Fatal("cannot connect to isolated broker")
		}
		defer connection.Close()
		channel, err := connection.Channel()
		if err != nil {
			test.Fatal(err)
		}
		defer channel.Close()
		payload, err := json.Marshal(entity.HTTPMessage{URL: strings.TrimRight(sinkURL, "/") + "/?migration=" + uuid.NewString(), Method: http.MethodGet})
		if err != nil {
			test.Fatal(err)
		}
		envelope, err := json.Marshal(entity.Outbox{Channel: entity.ChannelHTTP, Payload: payload, State: entity.OutboxStatePending})
		if err != nil {
			test.Fatal(err)
		}
		if err := channel.PublishWithContext(context.Background(), "", "http", false, false, amqp.Publishing{
			DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: envelope,
		}); err != nil {
			test.Fatal(err)
		}
		database, err := gorm.Open(postgres.Open(databaseDSN), &gorm.Config{})
		if err != nil {
			test.Fatal("cannot connect to isolated database")
		}
		connectionPool, err := database.DB()
		if err != nil {
			test.Fatal(err)
		}
		defer connectionPool.Close()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			var records []model.Outbox
			if err := database.Where("payload = ?", payload).Find(&records).Error; err != nil {
				test.Fatal(err)
			}
			if len(records) > 0 {
				awaitIntegrationState(test, client, baseURL, records[0].ID, int(entity.OutboxStateSent), time.Time{})
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		test.Fatal("external queue message was not persisted")
	})
}

func awaitIntegrationState(test *testing.T, client *http.Client, baseURL, id string, expected int, notBefore time.Time) dto.MessageStatus {
	test.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(fmt.Sprintf("%s/api/v1/http/status?id=%s", baseURL, url.QueryEscape(id)))
		if err != nil {
			test.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			test.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			test.Fatalf("status returned %d", response.StatusCode)
		}
		var status dto.MessageStatus
		if err := json.Unmarshal(body, &status); err != nil {
			test.Fatal(err)
		}
		if status.Status == expected {
			if !notBefore.IsZero() && time.Now().Before(notBefore) {
				test.Fatal("scheduled message delivered early")
			}
			return status
		}
		time.Sleep(100 * time.Millisecond)
	}
	test.Fatalf("message %s never reached state %d", id, expected)
	return dto.MessageStatus{}
}
