package observability

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusDisabled  Status = "disabled"
	StatusUnhealthy Status = "unhealthy"
)

var providerChannels = []entity.Channel{
	entity.ChannelBale,
	entity.ChannelEmail,
	entity.ChannelHTTP,
	entity.ChannelMattermost,
	entity.ChannelSMS,
}

type Check struct {
	Name           string        `json:"name"`
	Kind           string        `json:"kind"`
	Status         Status        `json:"status"`
	Critical       bool          `json:"critical"`
	Reason         string        `json:"reason,omitempty"`
	DurationMillis float64       `json:"duration_ms"`
	Duration       time.Duration `json:"-"`
}

type Report struct {
	Status    Status    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
	Checks    []Check   `json:"checks"`
}

func (report Report) Ready() bool {
	for _, check := range report.Checks {
		if check.Critical && check.Status != StatusHealthy {
			return false
		}
	}
	return true
}

type HealthConfig struct {
	CheckTimeout     time.Duration
	ComponentTimeout time.Duration
}

type SQLPinger interface {
	PingContext(context.Context) error
}

type ServiceHealth interface {
	Health() service.Health
}

type RabbitChecker func(context.Context, []entity.Channel) error

type Health struct {
	database SQLPinger
	rabbit   RabbitChecker
	service  ServiceHealth
	metrics  *Metrics
	config   HealthConfig
}

type checkSpec struct {
	name     string
	kind     string
	critical bool
	run      func(context.Context) (Status, string)
}

func NewHealth(
	database SQLPinger,
	rabbit RabbitChecker,
	serviceHealth ServiceHealth,
	metrics *Metrics,
	config HealthConfig,
) *Health {
	if config.CheckTimeout <= 0 {
		config.CheckTimeout = 3 * time.Second
	}
	if config.ComponentTimeout <= 0 || config.ComponentTimeout > config.CheckTimeout {
		config.ComponentTimeout = config.CheckTimeout
	}
	return &Health{
		database: database,
		rabbit:   rabbit,
		service:  serviceHealth,
		metrics:  metrics,
		config:   config,
	}
}

func (health *Health) Liveness(_ context.Context) Report {
	snapshot := health.service.Health()
	status, reason := StatusHealthy, ""
	if snapshot.ShuttingDown {
		status, reason = StatusUnhealthy, "shutting_down"
	}
	check := Check{
		Name:     "process",
		Kind:     "process",
		Status:   status,
		Critical: true,
		Reason:   reason,
	}
	health.observe(check)
	return Report{
		Status:    status,
		CheckedAt: time.Now().UTC(),
		Checks:    []Check{check},
	}
}

func (health *Health) Readiness(ctx context.Context) Report {
	snapshot := health.service.Health()
	specs := []checkSpec{
		health.databaseCheck(),
		health.rabbitCheck(snapshot.Providers),
		health.relayCheck(snapshot),
	}
	channels := append([]entity.Channel(nil), snapshot.Providers...)
	sort.Slice(channels, func(left, right int) bool {
		return channels[left].String() < channels[right].String()
	})
	for _, channel := range channels {
		specs = append(specs, health.consumerCheck(snapshot, channel))
	}
	enabledProviders := make(map[entity.Channel]bool, len(channels))
	for _, channel := range channels {
		enabledProviders[channel] = true
	}
	for _, channel := range providerChannels {
		specs = append(specs, providerCheck(channel, enabledProviders[channel]))
	}

	checkCtx, cancel := context.WithTimeout(ctx, health.config.CheckTimeout)
	defer cancel()
	results := make(chan Check, len(specs))
	for _, spec := range specs {
		go health.runCheck(checkCtx, spec, results)
	}
	checks := make([]Check, 0, len(specs))
	for range specs {
		checks = append(checks, <-results)
	}
	sort.Slice(checks, func(left, right int) bool {
		return checks[left].Name < checks[right].Name
	})
	return Report{
		Status:    aggregateStatus(checks),
		CheckedAt: time.Now().UTC(),
		Checks:    checks,
	}
}

func (health *Health) runCheck(
	ctx context.Context,
	spec checkSpec,
	results chan<- Check,
) {
	started := time.Now()
	componentCtx, cancel := context.WithTimeout(ctx, health.config.ComponentTimeout)
	defer cancel()
	type result struct {
		status Status
		reason string
	}
	completed := make(chan result, 1)
	go func() {
		status, reason := spec.run(componentCtx)
		completed <- result{status: status, reason: reason}
	}()
	status, reason := StatusUnhealthy, "timeout"
	select {
	case checkResult := <-completed:
		status, reason = checkResult.status, checkResult.reason
	case <-componentCtx.Done():
		_, reason = failedCheck(componentCtx, componentCtx.Err())
	}
	duration := time.Since(started)
	check := Check{
		Name:           spec.name,
		Kind:           spec.kind,
		Status:         status,
		Critical:       spec.critical,
		Reason:         reason,
		Duration:       duration,
		DurationMillis: float64(duration.Microseconds()) / 1000,
	}
	health.observe(check)
	results <- check
}

func (health *Health) databaseCheck() checkSpec {
	return checkSpec{
		name:     "postgres",
		kind:     "dependency",
		critical: true,
		run: func(ctx context.Context) (Status, string) {
			if health.database == nil {
				return StatusUnhealthy, "not_configured"
			}
			if err := health.database.PingContext(ctx); err != nil {
				return failedCheck(ctx, err)
			}
			return StatusHealthy, ""
		},
	}
}

func (health *Health) rabbitCheck(channels []entity.Channel) checkSpec {
	return checkSpec{
		name:     "rabbitmq",
		kind:     "dependency",
		critical: true,
		run: func(ctx context.Context) (Status, string) {
			if health.rabbit == nil {
				return StatusUnhealthy, "not_configured"
			}
			if err := health.rabbit(ctx, channels); err != nil {
				return failedCheck(ctx, err)
			}
			return StatusHealthy, ""
		},
	}
}

func (health *Health) relayCheck(snapshot service.Health) checkSpec {
	return checkSpec{
		name:     "relay",
		kind:     "service",
		critical: true,
		run: func(context.Context) (Status, string) {
			if snapshot.ShuttingDown {
				return StatusUnhealthy, "shutting_down"
			}
			if !snapshot.RelayRunning {
				return StatusUnhealthy, "stopped"
			}
			return StatusHealthy, ""
		},
	}
}

func (health *Health) consumerCheck(
	snapshot service.Health,
	channel entity.Channel,
) checkSpec {
	return checkSpec{
		name:     "consumer_" + channel.String(),
		kind:     "service",
		critical: true,
		run: func(context.Context) (Status, string) {
			if snapshot.ShuttingDown {
				return StatusUnhealthy, "shutting_down"
			}
			worker := snapshot.Consumers[channel]
			if !worker.Running {
				return StatusUnhealthy, "stopped"
			}
			if !worker.Connected {
				return StatusUnhealthy, "disconnected"
			}
			return StatusHealthy, ""
		},
	}
}

func providerCheck(channel entity.Channel, enabled bool) checkSpec {
	return checkSpec{
		name:     "provider_" + channel.String(),
		kind:     "provider",
		critical: false,
		run: func(context.Context) (Status, string) {
			if !enabled {
				return StatusDisabled, "disabled"
			}
			return StatusDegraded, "remote_unverified"
		},
	}
}

func failedCheck(ctx context.Context, err error) (Status, string) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(err, context.DeadlineExceeded) {
		return StatusUnhealthy, "timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return StatusUnhealthy, "canceled"
	}
	return StatusUnhealthy, "unavailable"
}

func aggregateStatus(checks []Check) Status {
	status := StatusHealthy
	for _, check := range checks {
		if check.Critical && check.Status != StatusHealthy {
			return StatusUnhealthy
		}
		if check.Status != StatusHealthy && check.Status != StatusDisabled {
			status = StatusDegraded
		}
	}
	return status
}

func (health *Health) observe(check Check) {
	if health.metrics != nil {
		health.metrics.ObserveHealth(check)
	}
}
