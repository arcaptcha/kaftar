# Reference Migration

## Status

The server source is migrated from reference revision `fa45c7a` (tag `v1.8.0`) into `server/`. The module is `github.com/arcaptcha/kaftar/server`; the toolchain baseline is Go 1.26.5 as requested. This is a compatibility migration, not a claim of production readiness.

Local tests, Docker startup, and five PostgreSQL/RabbitMQ delivery scenarios pass. Source-migration task [0.1](../todo/done/0.1-migrate-reference-service.md) and reference-retirement task [0.1.1](../todo/done/0.1.1-retire-reference-safely.md) are complete. The retired checkout must not be restored inside this repository.

## File Disposition

| Reference area | Migration decision |
| --- | --- |
| `cmd/`, `internal/`, `pkg/` | Migrated with canonical imports; HTTP annotation metadata removed. |
| Core entity, repository, service, HTTP sender tests | Preserved. Repository tests use in-memory SQLite and require CGO. |
| Email, SMS, Mattermost, Bale adapters | Replaced private notifier wrappers with direct gomail, Kavenegar, and HTTP usage. New tests use local mocks only. |
| Legacy Bale tests | Rejected: embedded credentials, live provider traffic, personal paths, and no failure assertions. |
| `go.mod`, `go.sum` | New public-only module graph; no notifier or Swagger dependency, no `replace` pointing at reference. Go 1.26.5 is intentional. Direct dependency versions retain reference versions; some transitive versions differ in the resolved graph. |
| `vendor/` | Not copied: public dependencies resolve without vendoring; no private notifier source is imported. |
| `.env.example` | Rewritten with disabled providers, example domains, and explicitly disposable local database/broker credentials. |
| Dockerfile, Compose, RabbitMQ config | Replaced private mirrors and generated-documentation services; standalone server image and local-only Compose with the same queue configuration. |
| Generated Swagger/HTML and handler annotations | Rejected. One reviewed, human-maintained OpenAPI 3.1 contract lives in `docs/openapi.yaml`. It includes Bale and full status responses missing from the reference YAML. |
| README, curl examples, direct-queue documentation | Reviewed; relevant behavior is consolidated into the root README, development guide, OpenAPI, and queue contract. Personal examples and unsupported guarantees are not copied. |
| Bruno collection/environments | Not migrated: duplicated contracts and internal environment assumptions. OpenAPI is canonical. |
| `client/` | Owner-confirmed exclusion: client SDKs will be implemented in independent repositories, one per language. Their implementation does not block this server migration. |
| `.gitlab-ci.yml` | Not copied: pushes to internal registries and modifies a separate deployment repository. A public CI/release design is separate work. |
| `.git/`, `.env`, editor files, build output | Never copied. The owner later declared the retired reference and its local-only files unnecessary. |

## Migration Baseline (Historical)

The following records task 0.1 before approved hardening. Current behavior is defined by the [delivery contract](queue-contract.md) and [provider contracts](provider-contracts.md).

- HTTP: `GET /health`; `POST /api/v1/{sms,email,mattermost,bale,http}/send`; `GET /api/v1/{channel}/status?id=...`. Successful submissions return HTTP 200 with an ID, not HTTP 202. No authentication is present. Health returns the JSON string `"Ok"`, not a readiness diagnosis.
- Status lookup is global by ID; the channel in the URL does not constrain it. Message validation errors remain inconsistent: generally 500 for non-HTTP providers, 400 for outbound HTTP; disabled senders return 503 after message validation.
- PostgreSQL remains mandatory. Startup runs GORM AutoMigrate on `outboxes`. Preserve the existing schema and payload encoding; do not point a smoke test at production data.
- RabbitMQ remains mandatory, even with every provider disabled. Each enabled provider creates durable `<channel>` and `<channel>.delay` queues. Persistent AMQP publications and TTL/dead-letter routing are preserved.
- Message states are 1 pending, 2 sending, 3 failed, 4 sent, 5 dead. The consumer does not currently set sending or populate a sent timestamp. Do not infer guarantees from the state names.
- Retry count zero means no retries; negative limits normalize to -1 (unlimited). Initial future `send_at` becomes a per-message TTL. Retry delays use exponential seconds plus jitter, switching to 15 minutes above attempt 9. A positive maximum retry age is checked after failed attempts.
- Email retains recipients, CC/BCC, HTML precedence, SMTP configuration validation, and nonempty attachments. SMS preserves the legacy trailing newline in provider text.
- Mattermost uses posts and file uploads; text longer than 500 bytes becomes `message.txt`; attachment posts contain the title and only the first attachment. Bale uses the first attachment or sends text longer than 4000 bytes as a document; short text retains the trailing newline and the legacy token URL shape.
- Outbound HTTP retains configured body limits, cookies shared across sends, redirect behavior, a 30-second default timeout, and non-2xx failure detection. Destination filtering is not implemented.

## Known Limitations and Follow-up

Completed tasks [0.1.2](../todo/done/0.1.2-review-delivery-reliability.md) and [0.1.3](../todo/done/0.1.3-harden-provider-contracts.md) implement durable database reconciliation, publisher confirms, fenced delivery claims, explicit idempotency, bounded shutdown, strict provider acceptance, cancellable SMTP/SMS, and destination-restricted webhooks. The Kavenegar SDK is no longer a dependency. See their validation records; this supersedes the historical baseline above.

Exactly-once external delivery is not promised. The API remains unauthenticated; database/broker TLS, production rollout, and live provider certification remain outside these tasks.

Credential-shaped values were found in tracked reference examples and tests but were never copied into the migrated server. The owner approved discarding the reference history and local-only files, and the checkout was retired through [task 0.1.1](../todo/done/0.1.1-retire-reference-safely.md). Deleting repository copies does not revoke any externally valid credential; revocation status was not independently verified.

## Validation Record

Migration commands and outcomes are recorded in [completed task 0.1](../todo/done/0.1-migrate-reference-service.md#validation); retirement evidence is recorded in [completed task 0.1.1](../todo/done/0.1.1-retire-reference-safely.md#validation); hardening evidence is recorded in completed tasks [0.1.2](../todo/done/0.1.2-review-delivery-reliability.md#validation) and [0.1.3](../todo/done/0.1.3-harden-provider-contracts.md#validation). Mock provider tests are not proof of live third-party delivery.
