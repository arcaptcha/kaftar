package observability

import (
	"context"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry                   *prometheus.Registry
	httpRequests               *prometheus.CounterVec
	httpDuration               *prometheus.HistogramVec
	httpInFlight               *prometheus.GaugeVec
	submissions                *prometheus.CounterVec
	deliveryAttempts           *prometheus.CounterVec
	deliveryResults            *prometheus.CounterVec
	deliveryDuration           *prometheus.HistogramVec
	retries                    *prometheus.CounterVec
	deadMessages               *prometheus.CounterVec
	brokerErrors               *prometheus.CounterVec
	healthStatus               *prometheus.GaugeVec
	healthDuration             *prometheus.HistogramVec
	workerReady                *prometheus.GaugeVec
	outboxMessages             *prometheus.GaugeVec
	outboxOldestPendingAge     *prometheus.GaugeVec
	outboxCollectorSuccess     prometheus.Gauge
	outboxCollectorLastSuccess prometheus.Gauge
}

type OutboxCollectorConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

type OutboxStatsReader interface {
	Stats(context.Context) (repository.OutboxStats, error)
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		registry: prometheus.NewRegistry(),
		httpRequests: counterVec(
			"http", "requests_total",
			"Total HTTP requests handled by route, method, and status code.",
			"route", "method", "status",
		),
		httpDuration: histogramVec(
			"http", "request_duration_seconds",
			"HTTP request duration in seconds by route and method.",
			"route", "method",
		),
		httpInFlight: gaugeVec(
			"http", "requests_in_flight",
			"Current HTTP requests in flight by route and method.",
			"route", "method",
		),
		submissions: counterVec(
			"", "message_submissions_total",
			"Total message submissions by channel and outcome.",
			"channel", "outcome",
		),
		deliveryAttempts: counterVec(
			"", "delivery_attempts_total",
			"Total provider delivery attempts by channel.",
			"channel",
		),
		deliveryResults: counterVec(
			"", "delivery_results_total",
			"Total provider delivery results by channel and outcome.",
			"channel", "outcome",
		),
		deliveryDuration: histogramVec(
			"", "delivery_duration_seconds",
			"Provider delivery duration in seconds by channel and outcome.",
			"channel", "outcome",
		),
		retries: counterVec(
			"", "delivery_retries_total",
			"Total retryable delivery failures by channel.",
			"channel",
		),
		deadMessages: counterVec(
			"", "dead_messages_total",
			"Total messages transitioned to dead by channel.",
			"channel",
		),
		brokerErrors: counterVec(
			"broker", "errors_total",
			"Total RabbitMQ operation errors by operation and channel.",
			"operation", "channel",
		),
		healthStatus: gaugeVec(
			"health", "check_status",
			"Whether the latest health check was healthy by component.",
			"component",
		),
		healthDuration: histogramVec(
			"health", "check_duration_seconds",
			"Health check duration in seconds by component.",
			"component",
		),
		workerReady: gaugeVec(
			"", "worker_ready",
			"Whether an internal worker is ready by worker type and channel.",
			"worker", "channel",
		),
		outboxMessages: gaugeVec(
			"outbox", "messages",
			"Current outbox messages by channel and state.",
			"channel", "state",
		),
		outboxOldestPendingAge: gaugeVec(
			"outbox", "oldest_pending_age_seconds",
			"Age in seconds of the oldest pending or retryable message by channel.",
			"channel",
		),
		outboxCollectorSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "kaftar",
			Subsystem: "outbox",
			Name:      "collector_success",
			Help:      "Whether the latest bounded outbox statistics refresh succeeded.",
		}),
		outboxCollectorLastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "kaftar",
			Subsystem: "outbox",
			Name:      "collector_last_success_unixtime",
			Help:      "Unix timestamp of the latest successful outbox statistics refresh.",
		}),
	}
	metrics.register()
	return metrics
}

func (metrics *Metrics) register() {
	metrics.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metrics.httpRequests,
		metrics.httpDuration,
		metrics.httpInFlight,
		metrics.submissions,
		metrics.deliveryAttempts,
		metrics.deliveryResults,
		metrics.deliveryDuration,
		metrics.retries,
		metrics.deadMessages,
		metrics.brokerErrors,
		metrics.healthStatus,
		metrics.healthDuration,
		metrics.workerReady,
		metrics.outboxMessages,
		metrics.outboxOldestPendingAge,
		metrics.outboxCollectorSuccess,
		metrics.outboxCollectorLastSuccess,
	)
	buildInfo := gaugeVec(
		"", "build_info", "Build information for the running Kaftar server.",
		"version", "revision", "modified",
	)
	metrics.registry.MustRegister(buildInfo)
	version, revision, modified := readBuildInfo()
	buildInfo.WithLabelValues(version, revision, modified).Set(1)
}

func (metrics *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{})
}

func (metrics *Metrics) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		route := normalizedRoute(ctx.Path())
		method := normalizedMethod(ctx.Request().Method)
		started := time.Now()
		inFlight := metrics.httpInFlight.WithLabelValues(route, method)
		inFlight.Inc()
		defer inFlight.Dec()

		err := next(ctx)
		status := responseStatus(ctx, err)
		metrics.httpRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
		metrics.httpDuration.WithLabelValues(route, method).Observe(time.Since(started).Seconds())
		return err
	}
}

func (metrics *Metrics) ObserveBrokerError(operation string, channel entity.Channel) {
	metrics.brokerErrors.WithLabelValues(operation, channelLabel(channel)).Inc()
}

func (metrics *Metrics) ObserveDeliveryAttempt(channel entity.Channel) {
	metrics.deliveryAttempts.WithLabelValues(channelLabel(channel)).Inc()
}

func (metrics *Metrics) ObserveDeliveryResult(
	channel entity.Channel,
	outcome string,
	duration time.Duration,
) {
	label := channelLabel(channel)
	metrics.deliveryResults.WithLabelValues(label, outcome).Inc()
	metrics.deliveryDuration.WithLabelValues(label, outcome).Observe(duration.Seconds())
	if outcome == "retry" {
		metrics.retries.WithLabelValues(label).Inc()
	}
	if outcome == "dead" {
		metrics.deadMessages.WithLabelValues(label).Inc()
	}
}

func (metrics *Metrics) ObserveSubmission(channel entity.Channel, outcome string) {
	metrics.submissions.WithLabelValues(channelLabel(channel), outcome).Inc()
}

func (metrics *Metrics) SetWorkerReady(
	worker string,
	channel entity.Channel,
	ready bool,
) {
	metrics.workerReady.WithLabelValues(worker, channelLabel(channel)).Set(boolValue(ready))
}

func (metrics *Metrics) ObserveHealth(check Check) {
	metrics.healthStatus.WithLabelValues(check.Name).Set(boolValue(check.Status == StatusHealthy))
	metrics.healthDuration.WithLabelValues(check.Name).Observe(check.Duration.Seconds())
}

func (metrics *Metrics) StartOutboxCollector(
	ctx context.Context,
	reader OutboxStatsReader,
	config OutboxCollectorConfig,
) <-chan struct{} {
	if config.Interval <= 0 {
		config.Interval = 15 * time.Second
	}
	if config.Timeout <= 0 {
		config.Timeout = 2 * time.Second
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		metrics.refreshOutbox(ctx, reader, config.Timeout)
		ticker := time.NewTicker(config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.refreshOutbox(ctx, reader, config.Timeout)
			}
		}
	}()
	return done
}

func (metrics *Metrics) refreshOutbox(
	ctx context.Context,
	reader OutboxStatsReader,
	timeout time.Duration,
) {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stats, err := reader.Stats(checkCtx)
	if err != nil {
		metrics.outboxCollectorSuccess.Set(0)
		return
	}

	metrics.outboxMessages.Reset()
	metrics.outboxOldestPendingAge.Reset()
	for _, count := range stats.Counts {
		metrics.outboxMessages.WithLabelValues(
			channelLabel(count.Channel),
			count.State.String(),
		).Set(float64(count.Count))
	}
	now := time.Now().UTC()
	for _, oldest := range stats.OldestPending {
		age := max(now.Sub(oldest.EligibleAt).Seconds(), 0)
		metrics.outboxOldestPendingAge.WithLabelValues(
			channelLabel(oldest.Channel),
		).Set(age)
	}
	metrics.outboxCollectorSuccess.Set(1)
	metrics.outboxCollectorLastSuccess.Set(float64(now.Unix()))
}

func counterVec(subsystem, name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaftar",
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	}, labels)
}

func histogramVec(subsystem, name, help string, labels ...string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "kaftar",
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
		Buckets:   prometheus.DefBuckets,
	}, labels)
}

func gaugeVec(subsystem, name, help string, labels ...string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kaftar",
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	}, labels)
}

func normalizedRoute(route string) string {
	if route == "" {
		return "unmatched"
	}
	return route
}

func normalizedMethod(method string) string {
	switch method {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

func responseStatus(ctx echo.Context, err error) int {
	if err == nil {
		if ctx.Response().Status == 0 {
			return http.StatusOK
		}
		return ctx.Response().Status
	}
	if httpError, ok := err.(*echo.HTTPError); ok {
		return httpError.Code
	}
	return http.StatusInternalServerError
}

func channelLabel(channel entity.Channel) string {
	if channel.IsValid() {
		return channel.String()
	}
	return "all"
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func readBuildInfo() (string, string, string) {
	version, revision, modified := "devel", "unknown", "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version, revision, modified
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	return version, revision, modified
}
