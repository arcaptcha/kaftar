# Delivery and Direct Queue Contract

PostgreSQL owns accepted messages, schedules, delivery attempts, and status. RabbitMQ carries ingress and dispatch notifications for enabled `sms`, `email`, `mattermost`, `bale`, and `http` providers. Telegram/Discord constants have no provider or HTTP route. This is **at-least-once attempt processing**, not exactly-once external delivery.

## Acceptance

HTTP send routes return **200 with an ID after the database insert commits**, not after broker publication or provider delivery. A broker outage does not delete accepted rows. A database failure returns 500 without claiming acceptance; an interrupted response/commit can still have an uncertain outcome, so retry with an idempotency key. Disabled senders and shutdown return 503; invalid submissions return 400.

The relay scans due rows every second in bounded batches, reserves dispatch for ten seconds, and publishes persistent, mandatory notifications with publisher confirms and a five-second confirmation timeout. Unroutable, failed, missing, or ambiguous confirmations leave rows recoverable. Nonterminal rows are reconsidered even after confirmed publication until claimed or terminal. There is no atomic database/broker transaction.

## External Submission

Publish persistent (`delivery_mode=2`) JSON to the default exchange with the enabled channel as routing key. Producers should use mandatory routing, returns, and confirms themselves. Broker acceptance is not the same as Kaftar database acceptance. Leave `id` absent or use the nil UUID:

```json
{
  "channel": "sms",
  "message_id": "order-123-notification",
  "payload": "eyJ0byI6WyIxMDAiXSwidGV4dCI6ImhlbGxvIn0=",
  "max_retries": 0
}
```

`payload` is base64 of `{"to":["100"],"text":"hello"}`; the address is illustrative. It is not a nested JSON object. Attachments inside payload JSON also use base64. Message fields follow [OpenAPI](openapi.yaml).

Optional `next_retry_at` is the initial scheduled time. Publish to the **main queue** and let PostgreSQL schedule it. An omitted channel uses the consumer's channel; an invalid/unsupported channel is rejected. A different supported channel is persisted under that channel without republishing the incoming envelope. If its provider is disabled, the row waits for enablement.

The consumer acknowledges only after persistence or recognition of an identical keyed submission. Temporary persistence failures are requeued with consumer backoff. Invalid or conflicting envelopes are rejected without requeue; there is no automatic quarantine queue. Direct AMQP submission does not return the database ID to the publisher.

## Idempotency

- HTTP uses optional `Idempotency-Key`; AMQP uses envelope `message_id`. Empty/missing keys disable deduplication. Nonempty keys are case-sensitive printable ASCII without spaces, up to 200 characters.
- A partial unique index enforces `(channel, message_id)` atomically. Same key/content/options returns the original HTTP ID or acknowledges AMQP ingestion without inserting another row. Different content/options gives HTTP 409 or AMQP rejection. Different channels or different keys represent separate submissions.
- Comparison uses a SHA-256 fingerprint of the validated, typed message JSON plus retry limit and requested scheduling time. JSON property/map order and omitted zero-valued optional fields normalize through Go encoding. Negative retry limits all become -1. HTTP methods normalize to uppercase with POST as default, and surrounding URL whitespace is trimmed.
- Requested timestamps normalize to UTC with microsecond precision, even if they are now in the past. An omitted time and an explicitly supplied past time remain different options. Other content is compared as preserved by message validation: body whitespace, header-name case, recipient/attachment order, filenames, MIME types, and attachment bytes matter. Unknown JSON fields are not part of the typed message.
- Keys remain with their rows; no automatic retention/deletion policy exists. Deleting a row ends its deduplication protection. Without a key, repeated submissions or a crash between insertion and broker acknowledgment can create separate rows.

## Claims, Retries, and Status

Internal notifications carry only `id` and `channel`; legacy full envelopes are still decoded. A nonempty ID never authorizes payload replacement or insertion. Workers load authoritative payload/state from PostgreSQL and atomically acquire a 60-second fenced lease before sending. Competing, early, or terminal duplicate notifications cannot bypass the database check. Unknown IDs and wrong-queue IDs are rejected.

States are 1 pending, 2 sending, 3 failed, 4 sent, and 5 dead. Attempts are counted at claim time; `retry_count` is attempts after the first. Failed attempts use exponential seconds with jitter, then 15 minutes beyond retry 9. Zero retries allows one attempt; negative limits normalize to -1. Expired leases count as uncertain attempts and become immediately eligible for recovery, subject to the same budget. A stale worker cannot overwrite a newer claim.

A positive `MESSAGE_RETRY_MAX_AGE` freezes a deadline at initial eligibility (`send_at` or acceptance) plus that duration. No new attempt starts at/after its deadline. Scheduling does not consume retry age before initial eligibility, and configuration changes do not move existing deadlines. An already-started attempt may finish after the deadline. Disabled providers consume no attempts; expiry is evaluated when their rows are next considered after enablement.

Successful completion writes `sent_at` and clears `last_error`/`next_retry_at`. Retry/dead/sent transitions must commit before acknowledgment. A provider acceptance followed by failed persistence, an expired lease, or a process crash can cause **another external delivery**. Fencing protects database updates, not external side effects. Multi-recipient/attachment submissions can also have partial external effects before failure.

## Scheduling, Shutdown, and Limits

New schedules/retries use database due timestamps, avoiding FIFO TTL head-of-line blocking. Existing `<channel>.delay` queues are retained for compatibility; messages already held there can still be delayed behind earlier messages. Do not delete them until drained.

SIGTERM/SIGINT stops HTTP admission and new claims, drains in-flight work, then cancels remaining I/O after a 35-second grace. Compose grants 40 seconds before forced termination. Provider attempts have 30-second deadlines. Reconnect/relay loops honor shutdown. Pending and uncertain rows recover after restart; migrations and outages can delay scheduling, so due times are not exact-delivery promises. Lease/deadline comparisons assume synchronized server clocks.

Status lookup remains global by ID regardless of the route's channel. `/health` is liveness, not a database/broker readiness check. The API is unauthenticated and intended for trusted development environments only. See [provider contracts](provider-contracts.md), [upgrade instructions](development.md#database-upgrades), and the [approved design](hardening-design.md).
