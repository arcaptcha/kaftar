# Server Development and Local Operations

## Prerequisites

- Go 1.26.5 and a C compiler for the SQLite-backed repository tests.
- Docker Engine with the Compose plugin for the local PostgreSQL/RabbitMQ environment.
- Public Go module access. There is no dependency on a private registry or the reference directory.

All commands here start at the repository root. Do not run tests against the reference checkout or production providers.

## Local Compose

```sh
cp server/.env.example server/.env
# Set nonempty PostgreSQL and RabbitMQ passwords in server/.env.
docker compose --env-file server/.env up --build -d --wait
curl --fail http://localhost:1404/health
docker compose --env-file server/.env logs server
docker compose --env-file server/.env down
```

The API binds to host loopback only. PostgreSQL and RabbitMQ are not published on host ports. Compose reads required local credentials from the ignored `server/.env`; no password is stored in tracked configuration. Compose uses Docker's init shim to forward signals and reap the standalone server process, with a 40-second stop grace for the server's 35-second shutdown window. Data persists in named volumes after `down`; add `--volumes` only when intentionally discarding that local data.

All provider flags default to false. Enable one in the ignored `server/.env` and recreate the server to use it. Only use controlled recipients and endpoints. HTTP destinations require an explicit allowlist; internal destinations also need a private CIDR exception. The API has no authentication; never expose this setup publicly.

## Build and Test

```sh
go -C server test ./...
go -C server vet ./...
go -C server build -o /tmp/kaftar ./cmd
docker build -t kaftar-server:local server
```

For formatting, run `gofmt` directly on changed Go files. No task wrapper scripts are needed. Dependency metadata must remain synchronized with `go -C server mod tidy`; do not run indiscriminate dependency upgrades during migration.

`TestIsolatedDeliveryIntegration` is opt-in and must target disposable services only. It requires `KAFTAR_INTEGRATION_URL`, `KAFTAR_INTEGRATION_AMQP`, `KAFTAR_INTEGRATION_POSTGRES`, and `KAFTAR_INTEGRATION_SINK`; the sink address must be reachable and explicitly allowlisted by the running server. The test creates records and publishes messages. See [reliability validation](../todo/done/0.1.2-review-delivery-reliability.md#validation) for the fixture configuration and executed command. Unit runs deliberately skip integration tests without their variables.

The PostgreSQL/RabbitMQ fault tests additionally require `KAFTAR_RELIABILITY_AMQP` pointing to a dedicated `kaftar-hardening` vhost on `127.0.0.1`. They create/drop isolated database schemas and purge only that vhost's HTTP queue. Do not use a shared or production database/broker. These tests cover concurrent keys/claims, deferred commit failure, commit-before-publish recovery, lost completion, reconnect, unroutable confirms, scheduling order, and graceful draining.

To run the Go executable directly, supply a separate ignored environment file with PostgreSQL and RabbitMQ hosts/ports reachable from the host:

```sh
/tmp/kaftar -env server/.env.local
```

The default Compose hostnames resolve only inside its network. Do not use the Compose example unchanged for a host-native server. `-env` loads the specified file; the process otherwise reads its environment. Existing environment variables take precedence over file values. This native startup requires separately reachable dependencies and is not the default onboarding path.

## Database Upgrades

Stop old server instances and back up the database before starting this version; old consumers must not run alongside fenced-claim consumers. Startup runs the transactional `0.1.2-durable-delivery` migration, serialized with a PostgreSQL advisory lock. It adds delivery/dispatch fields and indexes without deleting rows.

Duplicate nonempty `(channel, message_id)` keys, or legacy keyed rows without a canonical fingerprint, stop migration for owner reconciliation. Do not delete/merge those records automatically. Unkeyed legacy records backfill initial eligibility from `next_retry_at` when present, otherwise `created_at`; retry deadlines freeze from that value and the startup age configuration. This is an explicit cutover approximation because the old schema did not preserve initial eligibility separately. Terminal rows stay terminal; ambiguous sending rows become recoverable expired claims.

Keep existing RabbitMQ delay queues until drained. Test an upgrade against a restored copy first. Schema rollback is not automatic; restore the pre-upgrade backup rather than running legacy consumers against partially migrated work.

## Configuration

The complete example is [server/.env.example](../server/.env.example). Keep provider credentials in ignored files or environment injection.

| Variables | Meaning |
| --- | --- |
| `DEV_MODE`, `HTTP_PORT` | Logging mode and listener port. Compose fixes the internal port at 8080 and publishes 1404. |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB_NAME`, `POSTGRES_SCHEMA` | Required PostgreSQL connection. The migrated implementation disables transport TLS; restrict it to trusted environments. |
| `RABBITMQ_HOST`, `RABBITMQ_PORT`, `RABBITMQ_USERNAME`, `RABBITMQ_PASSWORD` | Dispatch/ingress connection; outages do not prevent durable HTTP acceptance. No AMQP TLS is configured. |
| `MESSAGE_RETRY_MAX_AGE` | Positive Go duration, such as `24h`, freezes a deadline from initial eligibility for newly accepted rows; empty/nonpositive disables the limit. |
| `SMS_SENDER_ENABLED`, `SMS_SENDER_API_KEY`, `SMS_SENDER_PHONE` | Kavenegar enablement, API credential, sender number. |
| `EMAIL_SENDER_ENABLED`, `EMAIL_SENDER_HOST`, `EMAIL_SENDER_PORT`, `EMAIL_SENDER_USERNAME`, `EMAIL_SENDER_PASSWORD`, `EMAIL_SENDER_FROM_ADDRESS` | SMTP settings; empty From falls back to username. |
| `MATTERMOST_SENDER_ENABLED`, `MATTERMOST_SENDER_URL`, `MATTERMOST_SENDER_PAT` | Mattermost enablement, base URL, personal access token. |
| `BALE_SENDER_ENABLED`, `BALE_SENDER_BOT_TOKEN` | Bale enablement and token, with or without one leading `bot`; live delivery is not verified by local tests. |
| `HTTP_SENDER_ENABLED` | Enable outbound HTTP requests only for trusted callers. |
| `HTTP_SENDER_ALLOWED_DESTINATIONS` | Comma-separated exact `host:port` destinations. Empty denies all webhook destinations. |
| `HTTP_SENDER_ALLOWED_PRIVATE_CIDRS` | Comma-separated private/loopback exceptions; an exact destination entry is still required. Metadata and special-use restrictions cannot be overridden. |
| `HTTP_SENDER_REQUEST_BODY_MAX_BYTES`, `HTTP_SENDER_RESPONSE_BODY_MAX_BYTES` | Default 1048576 each; zero permits only empty bodies, negative disables the limit. |

## Contracts and Deployment Direction

- [OpenAPI](openapi.yaml) is hand-maintained. The verified command is `npx --yes @redocly/cli lint docs/openapi.yaml --extends minimal`; its localhost-server warning is intentional. Do not regenerate Swagger artifacts.
- [Queue contract](queue-contract.md) explains direct AMQP submission and durability caveats.
- [Provider contracts](provider-contracts.md) explains success checks, SMTP TLS/cancellation, attachments, and destination restrictions.
- [Migration inventory](migration.md) records preserved behavior and known limitations.
- The console framework remains undecided. Future `console/Dockerfile` and `deploy/combined.Dockerfile` are separate work; the combined image must supervise both processes. No placeholder console or supervisor is introduced here.
