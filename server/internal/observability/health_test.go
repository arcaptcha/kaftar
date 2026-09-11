package observability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/stretchr/testify/require"
)

type pingerFunc func(context.Context) error

func (function pingerFunc) PingContext(ctx context.Context) error {
	return function(ctx)
}

type healthServiceStub struct {
	mutex    sync.RWMutex
	snapshot service.Health
}

func (stub *healthServiceStub) Health() service.Health {
	stub.mutex.RLock()
	defer stub.mutex.RUnlock()
	return stub.snapshot
}

func (stub *healthServiceStub) set(snapshot service.Health) {
	stub.mutex.Lock()
	stub.snapshot = snapshot
	stub.mutex.Unlock()
}

func TestReadinessChecksDependenciesWorkersAndProviders(test *testing.T) {
	backend := &healthServiceStub{snapshot: service.Health{
		RelayRunning: true,
		Providers:    []entity.Channel{entity.ChannelHTTP},
		Consumers: map[entity.Channel]service.WorkerHealth{
			entity.ChannelHTTP: {Running: true, Connected: true},
		},
	}}
	var checkedChannels []entity.Channel
	health := NewHealth(
		pingerFunc(func(context.Context) error { return nil }),
		func(_ context.Context, channels []entity.Channel) error {
			checkedChannels = append(checkedChannels, channels...)
			return nil
		},
		backend,
		nil,
		HealthConfig{},
	)

	report := health.Readiness(context.Background())

	require.True(test, report.Ready())
	require.Equal(test, StatusDegraded, report.Status)
	require.Equal(test, []entity.Channel{entity.ChannelHTTP}, checkedChannels)
	require.Equal(test, StatusHealthy, findCheck(test, report, "postgres").Status)
	require.Equal(test, StatusHealthy, findCheck(test, report, "rabbitmq").Status)
	require.Equal(test, StatusHealthy, findCheck(test, report, "relay").Status)
	require.Equal(test, StatusHealthy, findCheck(test, report, "consumer_http").Status)
	provider := findCheck(test, report, "provider_http")
	require.Equal(test, StatusDegraded, provider.Status)
	require.False(test, provider.Critical)
	require.Equal(test, "remote_unverified", provider.Reason)
	disabled := findCheck(test, report, "provider_sms")
	require.Equal(test, StatusDisabled, disabled.Status)
	require.Equal(test, "disabled", disabled.Reason)
}

func TestReadinessFailureIsSanitizedAndRecovers(test *testing.T) {
	backend := &healthServiceStub{snapshot: service.Health{RelayRunning: true}}
	var mutex sync.RWMutex
	databaseErr := errors.New("postgres://user:secret@internal.example/private")
	health := NewHealth(
		pingerFunc(func(context.Context) error {
			mutex.RLock()
			defer mutex.RUnlock()
			return databaseErr
		}),
		func(context.Context, []entity.Channel) error { return nil },
		backend,
		nil,
		HealthConfig{},
	)

	report := health.Readiness(context.Background())
	require.False(test, report.Ready())
	require.Equal(test, StatusUnhealthy, report.Status)
	require.Equal(test, "unavailable", findCheck(test, report, "postgres").Reason)

	mutex.Lock()
	databaseErr = nil
	mutex.Unlock()
	recovered := health.Readiness(context.Background())
	require.True(test, recovered.Ready())
	require.Equal(test, StatusHealthy, recovered.Status)
}

func TestReadinessReportsWorkerFailure(test *testing.T) {
	backend := &healthServiceStub{snapshot: service.Health{
		RelayRunning: true,
		Providers:    []entity.Channel{entity.ChannelSMS},
		Consumers: map[entity.Channel]service.WorkerHealth{
			entity.ChannelSMS: {Running: true},
		},
	}}
	health := NewHealth(
		pingerFunc(func(context.Context) error { return nil }),
		func(context.Context, []entity.Channel) error { return nil },
		backend,
		nil,
		HealthConfig{},
	)

	report := health.Readiness(context.Background())

	require.False(test, report.Ready())
	worker := findCheck(test, report, "consumer_sms")
	require.Equal(test, StatusUnhealthy, worker.Status)
	require.Equal(test, "disconnected", worker.Reason)
}

func TestReadinessTimeoutIsBounded(test *testing.T) {
	backend := &healthServiceStub{snapshot: service.Health{RelayRunning: true}}
	health := NewHealth(
		pingerFunc(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}),
		func(context.Context, []entity.Channel) error { return nil },
		backend,
		nil,
		HealthConfig{
			CheckTimeout:     100 * time.Millisecond,
			ComponentTimeout: 20 * time.Millisecond,
		},
	)

	started := time.Now()
	report := health.Readiness(context.Background())

	require.Less(test, time.Since(started), 100*time.Millisecond)
	require.Equal(test, "timeout", findCheck(test, report, "postgres").Reason)
}

func TestLivenessChangesDuringShutdown(test *testing.T) {
	backend := &healthServiceStub{snapshot: service.Health{RelayRunning: true}}
	health := NewHealth(nil, nil, backend, nil, HealthConfig{})

	require.True(test, health.Liveness(context.Background()).Ready())
	backend.set(service.Health{ShuttingDown: true})
	require.False(test, health.Liveness(context.Background()).Ready())
}

func findCheck(test *testing.T, report Report, name string) Check {
	test.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	test.Fatalf("check %q not found in %+v", name, report.Checks)
	return Check{}
}
