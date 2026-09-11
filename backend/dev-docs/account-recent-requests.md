# Account recent requests

The admin account list displays the latest ten account forwarding results. The
column loads separately from the account list, including when the list returns
304, and does not require operations monitoring or usage-log persistence.

## Recording

The outer forwarding adapter starts a request-local observation. Nested
protocol adapters share it, so a successful response produces one success
entry. New upstream error events preserve failures recovered by retries, while
account failover starts a separate observation for the next account. Duplicate
diagnostics carrying the same upstream request ID are collapsed within an
observation. Pre-forward admission failures with no account do not create rows.

WebSocket requests are recorded at the AfterTurn callback; opening or closing a
healthy session is not an inference result. A pre-turn handshake failure can
produce a failed result. HTTP 200 does not imply success: terminal response
errors, protocol error payloads and stream interruptions remain failed records.
Telemetry observations must not change forwarding or billing behavior.

When adding a forwarding path, use beginAccountRecentRequest at its outer
boundary. If a parser intentionally returns a usage result with no error after
an upstream failure, call markRecentRequestFailure while the failure is known.
Use the actual HTTP status when available, and zero when no response exists.
Do not reconstruct the historical proxy from the account's current database
binding. Error events and the observation contain credential-free proxy
snapshots; unknown routes use the existing unknown sentinel.

## Storage and API

POST /api/v1/admin/accounts/recent-requests/batch accepts account_ids (1–200)
and an optional limit (default 10, maximum 20). The normal response envelope
contains a map from account ID to a newest-first array.

The shared Redis key account:recent_requests:v1:ID is a sorted set capped at 20
entries. Completion timestamps order asynchronously written batches; a unique
member suffix preserves separate simultaneous requests. The key expires after
seven days without a write. No SQL migration or historical backfill is needed.

Four workers consume a bounded 512-batch queue. Each batch is limited to the
newest 20 records and a one-second write budget. Queue overflow, process exit
and Redis outages may lose recent-history samples but must not affect requests
or billing. This is a live troubleshooting summary, not a durable audit log.
Error messages are extracted, redacted and truncated before queueing/storage.

## Manual account tests

Manual tests choose a proxy once using the existing weighted account pool
selector, then keep that snapshot through internal retries. The proxy_info SSE
event exposes only the proxy name, ID and managed/direct/unknown route type.
A configured pool with no usable proxy fails instead of implicitly going
direct. WebSocket default-client routing is unknown when no managed proxy is
configured. This identifies the selected proxy configuration, not its public
exit IP. Manual tests do not promise that pressing Retry chooses a different
proxy.
