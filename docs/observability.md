# Health Checks and Prometheus Metrics

Kaftar exposes unauthenticated operational endpoints on the main HTTP listener. Keep the listener
on a trusted network and do not publish health details or metrics directly to the internet.

## Health Endpoints

| Endpoint | Purpose | Dependency calls |
| --- | --- | --- |
| `GET /health/live` | Process liveness | None |
| `GET /health/ready` | Dependency and delivery-service readiness | PostgreSQL and RabbitMQ |
| `GET /health` | Compatibility alias for readiness | Same as `/health/ready` |

Liveness returns HTTP 200 while the process can serve requests. It returns HTTP 503 after graceful
shutdown begins. Dependency outages do not fail liveness.

Readiness executes checks concurrently. The whole request defaults to three seconds and each
component defaults to two seconds. A failed or timed-out critical check returns HTTP 503. HTTP 200
can contain aggregate `degraded` status when only informational provider checks are degraded.

The JSON response contains `status`, `checked_at`, and stable per-component entries with `name`,
`kind`, `status`, `critical`, `reason`, and `duration_ms`. Reasons are fixed codes rather than raw
dependency errors, so credentials, connection strings, provider responses, and payload data are
not returned.

## Readiness Components

| Component | Critical | Behavior |
| --- | --- | --- |
| `postgres` | Yes | Executes `PingContext` within the component timeout. |
| `rabbitmq` | Yes | Opens a bounded connection and inspects enabled channel and delay queues. |
| `relay` | Yes | Confirms that the outbox relay goroutine is running and not shutting down. |
| `consumer_<channel>` | Yes | Checks each enabled consumer is running and connected. |
| `provider_<channel>` | No | Reports disabled or initialized; remote health is not probed. |

Provider checks never send messages or make remote provider requests. An enabled provider reports
`degraded` with reason `remote_unverified`; this is informational because restarting Kaftar cannot
repair a remote provider outage. Failed deliveries remain visible through delivery metrics.

Readiness can use these reason codes: `canceled`, `disabled`, `disconnected`, `not_configured`,
`remote_unverified`, `shutting_down`, `stopped`, `timeout`, and `unavailable`.

## Prometheus Endpoint

`GET /metrics` uses the Prometheus text exposition format. Kaftar registers a private registry per
application instance, so constructing multiple servers or tests does not create duplicate global
collector registrations.

| Metric | Type | Labels |
| --- | --- | --- |
| `kaftar_build_info` | gauge | `version`, `revision`, `modified` |
| `kaftar_http_requests_total` | counter | `route`, `method`, `status` |
| `kaftar_http_request_duration_seconds` | histogram | `route`, `method` |
| `kaftar_http_requests_in_flight` | gauge | `route`, `method` |
| `kaftar_message_submissions_total` | counter | `channel`, `outcome` |
| `kaftar_delivery_attempts_total` | counter | `channel` |
| `kaftar_delivery_results_total` | counter | `channel`, `outcome` |
| `kaftar_delivery_duration_seconds` | histogram | `channel`, `outcome` |
| `kaftar_delivery_retries_total` | counter | `channel` |
| `kaftar_dead_messages_total` | counter | `channel` |
| `kaftar_broker_errors_total` | counter | `operation`, `channel` |
| `kaftar_health_check_status` | gauge | `component` |
| `kaftar_health_check_duration_seconds` | histogram | `component` |
| `kaftar_worker_ready` | gauge | `worker`, `channel` |
| `kaftar_outbox_messages` | gauge | `channel`, `state` |
| `kaftar_outbox_oldest_pending_age_seconds` | gauge | `channel` |
| `kaftar_outbox_collector_success` | gauge | None |
| `kaftar_outbox_collector_last_success_unixtime` | gauge | None |

Standard Go runtime and process collectors are also exposed. HTTP routes use Echo route templates,
not request paths. Unknown HTTP methods use `OTHER`. Labels never include message IDs, recipients,
URLs, hosts, payloads, error text, or queue-generated identifiers.

Submission outcomes are `accepted`, `conflict`, `error`, `invalid`, `shutting_down`, and
`unavailable`. Delivery outcomes are `sent`, `retry`, `dead`, `invalid`, and `completion_error`.
Broker operations are `publish` and `consume`. Channels are limited to configured Kaftar channels
plus `all` for process-wide workers or unknown submissions.

## Backlog Collection

Outbox gauges are refreshed in the background, not during Prometheus scrapes. The default refresh
interval is 15 seconds with a two-second query timeout. Counts are grouped by bounded channel and
state labels. Oldest pending age includes pending and retryable failed rows and never reports a
negative age for future schedules.

If a refresh fails, `kaftar_outbox_collector_success` becomes zero and the last successful values
remain available. The last success timestamp allows alerts to distinguish a stable empty outbox
from stale statistics.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `HEALTH_CHECK_TIMEOUT` | `3s` | Maximum duration of a readiness request. |
| `HEALTH_COMPONENT_TIMEOUT` | `2s` | Maximum duration of one dependency check. |
| `METRICS_OUTBOX_REFRESH_INTERVAL` | `15s` | Backlog statistics refresh interval. |
| `METRICS_OUTBOX_REFRESH_TIMEOUT` | `2s` | Timeout for one backlog statistics query. |

All values use Go duration syntax. Nonpositive values fall back to their defaults. A component
timeout longer than the whole readiness timeout is reduced to the whole timeout.

Example Prometheus scrape configuration:

```yaml
scrape_configs:
  - job_name: kaftar
    static_configs:
      - targets: [kaftar:8080]
```

Alert on sustained readiness failure, disconnected workers, collector staleness, retry/dead rates,
and oldest pending age. A single provider failure should be diagnosed from delivery outcomes rather
than by restarting an otherwise healthy Kaftar process.
