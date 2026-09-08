# Provider Contracts and Outbound Security

Provider success means verified submission acceptance, not proof that a recipient received the message. Tests use local HTTP/SMTP fixtures and synthetic credentials; no live-provider certification is claimed. The HTTP API remains unauthenticated and must not be exposed publicly.

## Provider Behavior

| Provider | Acceptance and compatibility |
| --- | --- |
| Bale | Requires HTTP 2xx, valid JSON, `ok: true`, and a nonzero result `message_id`. Uses `/bot<token>/<method>` and accepts a configured token with or without one leading `bot`. Text receives the legacy newline; more than 4000 bytes including that newline becomes `message.txt`. A supplied attachment uses the original text as caption. |
| Mattermost | Post/file creation requires HTTP 201 and valid nonempty post/file identifiers. Text/title combination over 500 bytes becomes `message.txt`; attachment/document posts retain the legacy title-only post body. Upload acceptance alone does not mean the post succeeded. |
| SMS | Direct, context-aware Kavenegar REST submission replaces the context-blind SDK. Preserves configured sender, comma-separated recipients, and trailing newline. Requires HTTP 200, valid JSON, API `return.status = 200`, one positive message ID per submitted recipient, and entry status 1, 2, 4, 5, or 10. Other responses fail; this does not poll later delivery status. |
| Email | Preserves To/CC/BCC, HTML precedence, and nonempty MIME attachments. BCC recipients are included in the envelope but omitted from transmitted headers. Uses a cancellable, deadline-bound SMTP connection; port 465 uses implicit TLS, otherwise STARTTLS is used when available. Authentication requires TLS with certificate verification. Success requires acceptance of the SMTP DATA transaction. |
| HTTP | Requires HTTP 2xx and the configured response-size policy. Uses the destination policy below; no cookies are retained between submissions and redirects are not followed. |

Bale and Mattermost reject more than one supplied attachment before network I/O rather than silently ignore extras. Their legacy byte thresholds are compatibility choices, not claims about provider character limits. Empty Bale/Mattermost attachment data fails; email continues to omit empty attachments. Senders do not clear caller-owned attachment data on either success or failure.

Provider HTTP clients default to 30-second timeouts and a 1 MiB response limit. The service additionally bounds each entire attempt to 30 seconds, including multi-request Mattermost operations. SMTP owns/closes the underlying connection on cancellation; no detached background send is left running. Outbound HTTP retains operator-configurable request/response limits: default 1 MiB, zero empty-only, negative unlimited. Unlimited values deliberately remove that resource bound and are not recommended.

Errors and stored delivery failures contain safe provider/operation categories and numeric HTTP/API codes, never raw request URLs, credentials, provider response bodies, or submitted payloads. Context cancellation/deadline identity is retained. Partial delivery is still possible: a completed upload, accepted recipient, or successful provider request cannot be rolled back after a later failure.

## Outbound HTTP Configuration

Enabling `HTTP_SENDER_ENABLED` alone grants **no destinations**. Configure comma-separated exact `host:port` entries in `HTTP_SENDER_ALLOWED_DESTINATIONS`. There are no wildcards. Host matching is case-insensitive and ignores a trailing DNS dot; omitted URL ports mean 80 for HTTP and 443 for HTTPS.

For example, an operator may allow `hooks.example.com:443`. Publicly routable destinations need no private exception. A deliberately internal destination needs both its exact host/port entry and a matching `HTTP_SENDER_ALLOWED_PRIVATE_CIDRS` entry. The isolated validation fixture used `sink:80` with `172.16.0.0/12`; this is a Docker-test example, not a production recommendation.

- Only HTTP/HTTPS URLs without userinfo or fragments are allowed. Hop-by-hop and authority-override headers are rejected.
- Every new connection resolves the hostname, validates **all** returned addresses, and dials a validated literal IP. Mixed public/blocked answers are rejected. TLS still verifies the original hostname. Reused connections retain their already-validated peer; DNS rebinding cannot change that connection's destination.
- Private/loopback addresses require the private CIDR exception. Unspecified, multicast, link-local, special-use/translation ranges, and recognized metadata addresses are blocked even with that exception. Public IPv6 is restricted to global `2000::/3` addresses outside the blocked special-use ranges.
- Environment proxies, shared cookie storage, and automatic redirects are disabled. Caller-specified end-to-end headers such as Authorization remain supported, but are not forwarded through redirects.
- This policy applies to message-selected HTTP destinations. Operator-configured Mattermost/SMTP hosts may deliberately be internal. Fixed provider HTTP clients also disable redirects and environment proxies.

Existing unrestricted webhooks, cookie-dependent flows, redirects, missing `bot` prefixes, error-as-success behavior, and silently discarded attachments are intentionally incompatible with the migrated baseline. Review configuration before enabling a provider.

## Contract Sources

The [approved design](hardening-design.md#official-sources-reviewed) links the official Bale, Mattermost, and RabbitMQ references reviewed on September 7, 2026. The remaining SMS/SMTP review used the [Kavenegar REST contract](https://kavenegar.com/rest.html), [Gomail send interface](https://github.com/go-gomail/gomail/blob/master/send.go), and the installed Gomail MIME writer. Live accounts, production recipients, and a specific deployed Mattermost version remain outside this validation scope.
