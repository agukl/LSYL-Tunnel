# Successful IP Permanent Ban Protection Design

## Context

The server permanently blocks source IPs after repeated entry protocol failures. A public NAT, FRP endpoint, or other shared egress can carry both valid tunnel traffic and invalid probes, so an IP that has recently authenticated successfully can otherwise be added to the permanent blocklist by unrelated traffic from the same source address.

This change uses existing request logs at startup and a bounded in-memory set at runtime. It does not add a new log type, log field, configuration item, or persistence file.

## Goals

- On startup, remove permanently blocked IPs that have a successful request record in the most recent seven local calendar days.
- During runtime, prevent an IP that has succeeded on the current local calendar day from being promoted to a permanent protocol-failure block.
- Keep temporary authentication-failure blocks unchanged.
- Preserve the existing permanent block file format and all four structured log formats.
- Keep startup and runtime memory use bounded.

## Non-Goals

- Do not bypass temporary blocks caused by repeated username or password failures.
- Do not create a general trusted-IP allowlist.
- Do not change connection limiting, authentication, request authorization, or log retention.
- Do not add GUI controls or configuration for the seven-day window.

## Successful Request Definition

A request record proves success only when all of the following are true:

- `remote_ip` is a valid IP address.
- `result` is `ok`.
- `auth_result` is `ok`.
- `response.ok` is `true`.
- The request type is one of the authenticated server request types: `login`, `health`, `open`, `forward_check`, `reverse`, `reverse_listen`, or `reverse_stream`.

Denied, failed, blocked, malformed, compatibility-rejected, and incomplete request records do not qualify. A successful health request qualifies because it has passed authentication and is emitted by an active client.

## Startup Data Flow

1. Apply and validate the server configuration, create the listener, and load the existing temporary and permanent block state as today.
2. Read the request log base path from `runtime.request_log_file`.
3. Scan the dated request JSONL files for today and the previous six local calendar days. Also inspect the legacy undated base file when present, filtering its entries by their recorded timestamp.
4. For each successful request, canonicalize `remote_ip` with `net.ParseIP`.
5. Collect only successful IPs that currently appear in the permanent blocklist. This bounds the seven-day removal set by the size of the existing permanent list instead of by total log volume.
6. Seed the current-day successful-IP memory set from qualifying entries dated today.
7. Rewrite the permanent block file once, preserving comments, blank lines, ordering, and unrelated entries. Delete matching entries from the in-memory permanent set only after the file rewrite succeeds.
8. Start the accept loop only after this reconciliation completes.

Missing log files are normal and are skipped. Malformed JSONL lines, invalid timestamps, and invalid IPs are ignored. A file read or permanent-file rewrite error is written to the existing service log and leaves the affected permanent blocks in place.

## Runtime Data Flow

The failure tracker owns a current-day successful-IP set alongside its existing failure state. It stores:

- the local date represented by the set;
- canonical IP strings;
- at most `max_tracked_failure_ips` entries.

When the server records a successful request response, it marks the remote IP successful before writing the request log. This keeps protection effective even if the disk log write fails. Marking success clears only accumulated protocol-failure timestamps for that IP; authentication-failure timestamps and an active temporary block remain unchanged.

Before `addProtocolFailure` records or promotes a protocol failure, it rotates the successful set when the local date has changed and checks membership. A current-day successful IP is ignored by the permanent protocol-failure path. When the set reaches its bound, new IPs are not admitted; the security-safe fallback is to retain existing permanent-ban behavior rather than permit unbounded memory growth.

The date check is lazy and uses the failure tracker's existing clock hook, so no additional goroutine or timer is required.

## Persistence Behavior

The permanent blocklist remains a line-oriented text file. A bulk removal helper performs one read and one rewrite for all matching successful IPs. Comparison uses canonical IP values while retained lines are written back unchanged.

No successful-IP state is persisted separately. On every process start, the current day is reconstructed from request logs, and the recent seven-day history is used only to repair the existing permanent list.

## Security Properties

- A source still cannot reach authentication while actively permanently blocked; startup reconciliation is the only way historical success removes an existing permanent block automatically.
- A source with current-day success remains subject to connection concurrency/rate limits and temporary authentication blocking.
- Possession of valid credentials already permits authenticated tunnel requests; using that success to suppress protocol-based permanent escalation does not bypass account authorization.
- An old success can remove a newer permanent block because the current permanent file has no block timestamp. This is an explicit consequence of the requested seven-day rule and avoids changing the file format.

## Tests

- Successful request parsing accepts every supported authenticated request type and rejects failed, blocked, health-without-auth, malformed, old, and invalid-IP records.
- Startup scanning covers seven local calendar days, the legacy base file, missing files, and malformed lines.
- Startup reconciliation removes recent successful IPs from both file and memory while preserving comments and unrelated permanent entries.
- A success older than the seven-day calendar window does not remove a permanent block.
- A current-day successful IP cannot reach the permanent protocol-failure threshold.
- The same IP can be permanently blocked after the successful-day set rolls over.
- Authentication failures can still create a temporary block for a current-day successful IP.
- The successful-IP set does not exceed `max_tracked_failure_ips`.
- Existing block persistence, entry protection, and end-to-end server tests remain green.
