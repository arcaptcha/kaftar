package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type statsReader struct {
	stats repository.OutboxStats
	err   error
}

func (reader statsReader) Stats(context.Context) (repository.OutboxStats, error) {
	return reader.stats, reader.err
}

func TestMetricsUseBoundedLabelsAndCanBeConstructedRepeatedly(test *testing.T) {
	first := NewMetrics()
	second := NewMetrics()
	require.NotNil(test, first)
	require.NotNil(test, second)

	router := echo.New()
	router.Use(first.Middleware)
	router.GET("/items/:id", func(ctx echo.Context) error {
		return ctx.NoContent(http.StatusNoContent)
	})
	for _, id := range []string{"first-message-id", "second-message-id"} {
		request := httptest.NewRequest(http.MethodGet, "/items/"+id, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(test, http.StatusNoContent, response.Code)
	}
	request := httptest.NewRequest("CUSTOM-METHOD", "/items/third-message-id", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	body := metricsBody(test, first)
	require.Contains(
		test,
		body,
		`kaftar_http_requests_total{method="GET",route="/items/:id",status="204"} 2`,
	)
	require.NotContains(test, body, "first-message-id")
	require.NotContains(test, body, "second-message-id")
	require.NotContains(test, body, "CUSTOM-METHOD")
	require.Contains(
		test,
		body,
		`kaftar_http_requests_total{method="OTHER",route="/items/:id",status="405"} 1`,
	)
}

func TestDeliveryAndHealthMetrics(test *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveSubmission(entity.ChannelHTTP, "accepted")
	metrics.ObserveDeliveryAttempt(entity.ChannelHTTP)
	metrics.ObserveDeliveryResult(entity.ChannelHTTP, "retry", 10*time.Millisecond)
	metrics.ObserveDeliveryResult(entity.ChannelHTTP, "dead", 20*time.Millisecond)
	metrics.ObserveBrokerError("publish", entity.ChannelHTTP)
	metrics.SetWorkerReady("consumer", entity.ChannelHTTP, true)
	metrics.ObserveHealth(Check{Name: "postgres", Status: StatusHealthy})

	body := metricsBody(test, metrics)
	for _, expected := range []string{
		`kaftar_message_submissions_total{channel="http",outcome="accepted"} 1`,
		`kaftar_delivery_attempts_total{channel="http"} 1`,
		`kaftar_delivery_results_total{channel="http",outcome="retry"} 1`,
		`kaftar_delivery_retries_total{channel="http"} 1`,
		`kaftar_dead_messages_total{channel="http"} 1`,
		`kaftar_broker_errors_total{channel="http",operation="publish"} 1`,
		`kaftar_worker_ready{channel="http",worker="consumer"} 1`,
		`kaftar_health_check_status{component="postgres"} 1`,
		"kaftar_build_info",
	} {
		require.Contains(test, body, expected)
	}
}

func TestOutboxCollectorCachesBoundedStatistics(test *testing.T) {
	metrics := NewMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	done := metrics.StartOutboxCollector(ctx, statsReader{stats: repository.OutboxStats{
		Counts: []repository.OutboxCount{{
			Channel: entity.ChannelHTTP,
			State:   entity.OutboxStatePending,
			Count:   3,
		}},
		OldestPending: []repository.OldestPending{{
			Channel:    entity.ChannelHTTP,
			EligibleAt: time.Now().Add(-time.Minute),
		}},
	}}, OutboxCollectorConfig{Interval: time.Hour, Timeout: time.Second})
	test.Cleanup(func() {
		cancel()
		<-done
	})

	require.Eventually(test, func() bool {
		return strings.Contains(
			metricsBody(test, metrics),
			`kaftar_outbox_messages{channel="http",state="pending"} 3`,
		)
	}, time.Second, 10*time.Millisecond)
	body := metricsBody(test, metrics)
	require.Contains(test, body, "kaftar_outbox_collector_success 1")
	require.Contains(test, body, `kaftar_outbox_oldest_pending_age_seconds{channel="http"}`)
}

func metricsBody(test *testing.T, metrics *Metrics) string {
	test.Helper()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, request)
	require.Equal(test, http.StatusOK, response.Code)
	require.Contains(test, response.Header().Get("Content-Type"), "text/plain")
	return response.Body.String()
}
