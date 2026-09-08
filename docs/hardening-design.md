# Migration Hardening Design

**Status: approved by the owner on September 7, 2026; implementation and validation recorded in the linked tasks.** This is the design gate for completed tasks [0.1.2](../todo/done/0.1.2-review-delivery-reliability.md) and [0.1.3](../todo/done/0.1.3-harden-provider-contracts.md), a design record rather than the authoritative runtime contract. The [queue contract](queue-contract.md) and [migration inventory](migration.md) describe runtime behavior and historical migration context.

## Evidence

On September 7, 2026, temporary Go test overlays asserted the desired behavior against the migrated implementation. They reproduced nine failing checks without contacting providers:

| Area | Observed failure | Fixture |
| --- | --- | --- |
| Queue ingestion | Acknowledges a foreign message when database creation fails | Fake repository and AMQP acknowledger |
| Delivery settlement | Acknowledges delivery when saving success fails | Fake repository and AMQP acknowledger |
| Duplicate handling | Sends an envelope marked `sent` without consulting durable state | Fake sender; this does not establish database-level deduplication |
| Status | Successful delivery does not write `sent_at` | Captured repository update |
| Status clearing | Updating a sent row retains its previous error and retry timestamp | Existing SQLite repository fixture |
| Bale HTTP failure | HTTP 400 counts as success | In-memory HTTP transport |
| Bale API failure | HTTP 200 with `ok: false` counts as success | In-memory HTTP transport |
| Bale error safety | A transport error contains the synthetic token from its request URL | In-memory HTTP transport |
| Mattermost failure | HTTP 401 counts as successful posting | In-memory HTTP transport |

These were diagnostic assertions at proposal time, not integration validation. Permanent regression coverage now lives in the service, repository, and provider tests. The original cached migration suite did not detect these gaps; fresh race/integration results and operational checks are recorded in the linked task validation sections.

## 0.1.2: Recommended Delivery Design

Keep PostgreSQL as durable authority and RabbitMQ as dispatch/ingress transport. Do not attempt to make a database write and broker publication one atomic operation.

1. **Accept on database commit.** HTTP submission persists a pending row and returns its ID. A broker outage no longer causes deletion of an accepted row. Database failure returns an error without claiming acceptance. A lost HTTP response can still leave a committed row; idempotent resubmission handles that ambiguity.
2. **Reconcile due rows.** A bounded database-backed relay dispatches due pending/failed rows, with a transactional dispatch reservation and an expiring redispatch deadline. Use persistent, mandatory publications, publisher confirms, returned-message handling, and finite timeouts. Revisit nonterminal rows even after a confirmed publication until a delivery claim exists. This recovers both commit-before-publish crashes and later broker loss; duplicates are expected.
3. **Claim before sending.** Load payload, channel, schedule, and state from the database, not the queue envelope. Atomically claim an eligible row using a lease and fencing token; competing deliveries cannot start another unexpired claim. Acknowledge terminal, early, or already-claimed duplicate notifications only because reconciliation retains responsibility. Never deliver an unknown nonempty ID or let an envelope replace a stored payload.
4. **Persist before settling.** Direct AMQP submissions are acknowledged only after durable insertion or an identical idempotent submission is found. Temporary database failures requeue with bounded consumer backoff; malformed/conflicting submissions are rejected without requeue and logged without payloads. Retry and terminal transitions must commit before acknowledgment. Persist `sending`, `sent_at`, and explicit clearing of obsolete error/retry fields; replace sparse struct updates with explicit transition methods.
5. **Recover expired attempts.** Record attempt count when claiming. An expired lease is an uncertain attempt, consumes the same retry budget as a failed attempt, and is rescheduled or made dead. Fence all completion writes. A successful external call followed by process/database failure can still be repeated: the guarantee is at-least-once attempts, **not exactly-once external delivery**. Lease fencing cannot undo external side effects.
6. **Schedule in PostgreSQL.** Use due timestamps for new HTTP submissions and retries instead of mixed per-message TTLs. Preserve retry backoff, jitter, and negative/unlimited retry-count conventions. Set the retry-age deadline from initial eligibility (`send_at`, otherwise acceptance time), check it before claims, and persist it so later configuration changes do not move it. Zero max age remains unlimited. Long scheduling does not consume retry age before eligibility.
7. **Make identity explicit.** Scope nonempty idempotency keys to a channel. Accept optional HTTP `Idempotency-Key` and direct-envelope `message_id`; atomically enforce uniqueness with a partial database index. Same key plus identical payload/options returns the original ID; conflicting content returns HTTP 409 or rejects AMQP ingestion. Define canonical comparison before implementation. Without a key, repeated submissions and commit-before-ack crashes can create separate messages. Keep keys with rows; any future retention policy must state its deduplication window.
8. **Own lifecycle and connections.** Use one synchronized connection manager and bounded consumer prefetch. Reconnect and relay loops must honor cancellation. On SIGTERM, stop HTTP admission and new claims, drain existing work within a fixed grace period, then cancel I/O and close broker/database resources. Start with 30-second provider deadlines, 60-second attempt leases, and a 35-second shutdown grace; test these relationships rather than rely on unbounded background contexts.

### Upgrade and Compatibility

- Add explicit schema migration for leases, attempts, eligibility/deadlines, dispatch reservations, and idempotency. Inventory existing duplicate nonempty keys before creating the unique index; stop for an owner decision rather than silently delete or merge rows.
- Backfill existing pending/failed rows using stored retry timestamps and creation time. Existing terminal rows stay terminal; existing ambiguous `sending` rows require recovery as uncertain attempts. Freeze a cutover time/configuration for age backfill and test it with representative rows.
- Retain legacy queue envelope decoding while existing queues drain. Known IDs become notifications only. Direct producers should submit to the main queue with `next_retry_at`; legacy `.delay` queues can still exhibit head-of-line delay until drained. Do not delete queues or promise precise timing for messages still held there.
- A disabled provider retains accepted rows without consuming attempts. New HTTP submissions to disabled providers still return 503. Document outage/re-enable and age-expiration behavior explicitly.
- Update OpenAPI and queue documentation with commit-based acceptance, optional idempotency, conflicts, timestamp meanings, and remaining ambiguous-delivery risks in the implementation change.

### Required Reliability Regressions

Use an isolated PostgreSQL/RabbitMQ stack and a local HTTP sink, with barriers or controlled failure injection rather than timing-only sleeps:

| Scenario | Required result |
| --- | --- |
| Commit fails; broker unavailable after commit; process exits before publish | No false acceptance on failed commit; committed rows recover without manual resubmission |
| Unroutable publish, negative/missing confirm, connection loss | Due row remains recoverable; no unsafe acknowledgment or deletion |
| Restart after confirmation or provider response | Recovery progresses; ambiguous external duplicates are documented |
| Concurrent duplicate deliveries and submissions | One active claim; identical keyed submissions share an ID; conflicts are rejected |
| Lease expiry and late worker completion | Retry budget enforced; stale worker cannot overwrite newer state |
| Future message followed by earlier due message | New database scheduling has no FIFO TTL head-of-line blockage |
| Retry limit/age boundary and successful retry | Correct attempt count, dead state, `sent_at`, and cleared error/retry fields |
| Broker restart and SIGTERM during work | Reconnect without races; bounded shutdown; unfinished work recoverable |

Run focused tests, full tests, race tests, integration tests, and an image build before archiving the task. The migration integration test alone does not exercise these faults.

## 0.1.3: Recommended Provider Policy

- **Success means verified provider acceptance, not recipient receipt.** Bale requires HTTP success and a valid bounded JSON response with `ok: true` and a result. Use the documented `/bot<token>/<method>` URL; normalize an existing `bot` prefix once for compatibility. Mattermost post/upload requires HTTP 201 and valid nonempty post/file identifiers. Reject malformed/truncated success responses. SMS requires both valid JSON and the API's success status, not merely HTTP 200. SMTP success means successful completion of the SMTP submission transaction.
- **Make requests bounded and cancellable.** Give provider HTTP clients a 30-second default timeout and a 1 MiB response limit; honor earlier caller deadlines. Replace context-blind SMS calls with a context-aware request path that preserves recipient/text formatting. Use an owned, deadline-bound SMTP connection which is closed on cancellation; do not return early while a detached send goroutine continues delivery.
- **Keep diagnostics credential-safe.** Return provider name, operation, safe status/error code, and stable failure category. Never return/log request URLs, tokens, passwords, provider response bodies, or submitted payloads. Preserve `errors.Is` for cancellation/deadline errors without wrapping credential-bearing URL errors. Apply the same policy to stored `last_error` and outbound HTTP errors.
- **Do not silently lose attachments.** Initially reject more than one attachment for Bale/Mattermost before any network call rather than silently discard extras. Keep existing 4000-byte Bale and 500-byte Mattermost long-text conversions as explicit compatibility thresholds, not advertised provider limits. Test below/at/above each boundary, including multibyte text and Bale's appended newline. Preserve attachment data on failure and do not mutate caller-owned payloads. Verify SMTP MIME filename/type/bytes, HTML preference, CC, and BCC envelope/header behavior with a local SMTP fixture.
- **Outbound HTTP is stateless and deny-by-default.** Require operator-configured exact host/port allowlists; reject URL userinfo and unsupported schemes. Allow public destinations only by default. Private destinations need both an explicit destination entry and approved private CIDRs; never allow unspecified, multicast, link-local, or metadata destinations. Resolve and validate every candidate address, then dial only a checked IP while retaining the original TLS hostname; reject mixed allowed/blocked DNS answers. Do not use environment proxies, a shared cookie jar, or automatic redirects. Reject hop-by-hop headers. Existing unrestricted webhooks, redirects, and cookie-dependent flows intentionally stop working under this policy.
- **Separate operator configuration from message input.** Fixed Bale/SMS endpoints and operator-configured Mattermost/SMTP hosts are not message-selected destinations. Disable automatic provider redirects to avoid forwarding credentials; do not apply public-only webhook rules to a deliberately configured internal Mattermost/SMTP deployment.

Provider regressions use local transports/HTTP/SMTP fixtures: HTTP/API failures, malformed and oversized bodies, synthetic-secret redaction, attachment content, text boundaries, canceled/expired requests, stalled I/O, redirect refusal, cookie isolation, private/IPv6/mixed DNS rejection, rebinding-safe dialing, proxy independence, and explicit private-destination exceptions. The implementation review additionally checked the Kavenegar REST response and Gomail transport/MIME contracts, linked from [provider contracts](provider-contracts.md#contract-sources). Real recipients remain outside approval scope.

## Official Sources Reviewed

Reviewed September 7, 2026. These sources justify design decisions, not claims of live provider compatibility:

- [RabbitMQ publisher confirms](https://github.com/rabbitmq/rabbitmq-website/blob/main/docs/confirms.md): confirms and consumer acknowledgments cover different boundaries; mandatory returns matter even with confirms.
- [RabbitMQ TTL](https://github.com/rabbitmq/rabbitmq-website/blob/main/docs/ttl.md): per-message expiry can remain behind earlier messages. The documentation website returned HTTP 403; its official source repository was reviewed instead.
- [Bale Bot API](https://docs.bale.ai/): `bot` URL prefix and JSON `ok`/error response convention.
- [Mattermost posts](https://github.com/mattermost/mattermost/blob/master/api/v4/source/posts.yaml) and [files](https://github.com/mattermost/mattermost/blob/master/api/v4/source/files.yaml): creation response contracts. A deployed Mattermost version is not yet selected.
- [Kavenegar Go client](https://github.com/kavenegar/kavenegar-go/blob/master/client.go): the reviewed client constructs requests without caller context and ignores success-body decode errors. This is not sufficient to claim full SMS contract conformance.

## Approval Boundary

The owner approved the delivery design and provider compatibility/security policy on September 7, 2026. Canonical comparison and exact runtime policies are recorded in the delivery and provider contracts. The reference checkout was later retired through [task 0.1.1](../todo/done/0.1.1-retire-reference-safely.md).

Implementation details: dispatch reservations last ten seconds with one-second relay scans; expired uncertain attempts recover immediately within their budget; empty keys bypass deduplication; typed message JSON and normalized options define comparison. Legacy keyed rows without fingerprints require owner reconciliation rather than guessed equivalence. See [delivery](queue-contract.md) and [provider](provider-contracts.md) contracts.
