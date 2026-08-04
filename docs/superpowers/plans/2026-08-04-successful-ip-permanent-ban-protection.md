# Successful IP Permanent Ban Protection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove recently successful IPs from permanent blocks at startup and prevent current-day successful IPs from being permanently blocked by protocol-failure accumulation.

**Architecture:** A tunnel-package scanner streams the existing request JSONL files for seven local calendar days and reports canonical successful IPs. The failure tracker owns a bounded current-day success set, performs one bulk permanent-file reconciliation at startup, and checks that set only in the protocol-failure permanent-ban path.

**Tech Stack:** Go 1.25 standard library, JSONL request logs, existing `failTracker`, line-oriented permanent block persistence.

## Global Constraints

- Temporary authentication-failure blocks remain unchanged.
- No new configuration fields, persistence files, log types, or log fields.
- The permanent block text format, comments, blank lines, and unrelated entries remain unchanged.
- The success set is bounded by `security.max_tracked_failure_ips` and resets by local calendar date.
- Only authenticated requests with `result=ok`, `auth_result=ok`, and `response.ok=true` qualify.
- Do not modify or commit the existing `src/server/front/index.html` worktree change.

---

### Task 1: Bulk Permanent Block Removal

**Files:**
- Modify: `src/server/tunnel/permanent_blocks.go`
- Modify: `src/server/tunnel/permanent_blocks_test.go`

**Interfaces:**
- Produces: `removePermanentBlockedIPs(path string, ips map[string]struct{}) (map[string]struct{}, error)`.
- Preserves: `RemovePermanentBlockedIP(path, ip)` behavior by delegating to the bulk helper.

- [ ] **Step 1: Write the failing bulk-removal tests**

Add tests that pass duplicate and canonical-equivalent IP lines, then assert the helper returns the removed canonical IPs while preserving comments, blank lines, invalid text, line ordering, and unrelated addresses. Add a missing-file test that returns an empty result without creating a file.

- [ ] **Step 2: Run the focused tests and confirm the helper is missing**

Run: `go test ./src/server/tunnel -run 'TestRemovePermanentBlockedIPs|TestRemovePermanentBlockedIP' -count=1`

Expected: FAIL because `removePermanentBlockedIPs` is undefined.

- [ ] **Step 3: Implement one-read/one-write removal**

Implement canonical comparison with `net.ParseIP(strings.TrimSpace(line))`. Preserve retained lines verbatim after normalizing only line separators for the rewrite. Return the set actually removed; do not rewrite when nothing matches. Refactor the existing single-IP function to call the bulk helper and preserve its exported signature.

- [ ] **Step 4: Run the focused tests**

Run: `go test ./src/server/tunnel -run 'TestRemovePermanentBlockedIPs|TestRemovePermanentBlockedIP' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the persistence helper**

```text
feat(server): add bulk permanent block removal
```

### Task 2: Request History Scanner and Bounded Daily Success Set

**Files:**
- Create: `src/server/tunnel/successful_ip_history.go`
- Create: `src/server/tunnel/successful_ip_history_test.go`
- Modify: `src/server/tunnel/server.go`

**Interfaces:**
- Produces: `scanRecentSuccessfulRequestIPs(basePath string, now time.Time, days int, visit func(ip string, occurredAt time.Time)) error`.
- Produces: `isSuccessfulRequestForBlockProtection(entry RequestLogEntry) bool`.
- Produces: `(*failTracker).markSuccessful(key string) bool` and internal current-day membership rotation.
- Changes: `(*failTracker).addProtocolFailure` returns without recording for a current-day successful IP.

- [ ] **Step 1: Write failing scanner tests**

Create dated JSONL fixtures for today, six days ago, and seven days ago plus a legacy undated file. Assert supported authenticated request types with all three success fields qualify, today and day-minus-six are visited, day-minus-seven and failed/malformed/invalid-IP entries are excluded, and missing dated files are ignored.

- [ ] **Step 2: Run scanner tests and confirm missing symbols**

Run: `go test ./src/server/tunnel -run 'TestScanRecentSuccessfulRequestIPs|TestSuccessfulRequestForBlockProtection' -count=1`

Expected: FAIL because the scanner and predicate are undefined.

- [ ] **Step 3: Implement streaming seven-day scanning**

Use `bufio.Scanner` with an explicit 1 MiB maximum line size. Generate dated paths with `datedJSONLPath`, de-duplicate candidate paths, parse `RequestLogEntry.Time` as RFC3339, compare local calendar dates, canonicalize IPs, and continue after malformed lines. Missing files are ignored; non-`IsNotExist` file/scanner errors are joined and returned after remaining files are processed.

- [ ] **Step 4: Write failing failure-tracker tests**

Using the existing `now` hook, assert:

- repeated protocol failures do not permanently block a current-day successful IP;
- the same IP can reach the permanent threshold after advancing to the next local day;
- repeated authentication failures still create a temporary block;
- marking success clears protocol failures but preserves authentication failures and `blockedUntil`;
- the success set never exceeds `maxItems`.

- [ ] **Step 5: Run tracker tests and confirm old behavior fails**

Run: `go test ./src/server/tunnel -run 'TestSuccessfulIP' -count=1`

Expected: FAIL because the tracker has no daily success set and still creates a permanent block.

- [ ] **Step 6: Implement the bounded daily set**

Add `successfulDate string` and `successfulIPs map[string]struct{}` to `failTracker`, initialize the map in `newFailTracker`, and rotate it lazily using `now.In(time.Local).Format("2006-01-02")`. `markSuccessful` canonicalizes the IP, enforces `maxItems`, and clears only `protocolFailures`. Check membership in `addProtocolFailure` before creating or updating failure state.

- [ ] **Step 7: Run scanner and tracker tests**

Run: `go test ./src/server/tunnel -run 'TestScanRecentSuccessfulRequestIPs|TestSuccessfulRequestForBlockProtection|TestSuccessfulIP' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit history and runtime protection**

```text
feat(server): protect current-day successful IPs
```

### Task 3: Startup Reconciliation and Runtime Success Recording

**Files:**
- Modify: `src/server/tunnel/successful_ip_history.go`
- Modify: `src/server/tunnel/successful_ip_history_test.go`
- Modify: `src/server/tunnel/fail_persistence.go`
- Modify: `src/server/tunnel/event_logs.go`
- Modify: `src/server/tunnel/server.go`

**Interfaces:**
- Produces: `reconcileSuccessfulIPHistory(fails *failTracker, requestLogPath string, now time.Time) error`.
- Produces: `(*failTracker).removePermanentSuccessful(ips map[string]struct{}) (int, error)`.
- Changes: `(*Server).recordRequestLog` calls `markSuccessful` before serialization when the shared success predicate passes.

- [ ] **Step 1: Write failing startup-reconciliation tests**

Create a permanent block file containing a recent successful IP, an old-success IP, an unrelated IP, comments, and blank lines. Load the tracker, create request logs, run reconciliation, and assert only the recent successful IP disappears from both disk and `sync.Map`; today's successful IP is seeded, while a six-day-old successful IP is removed from permanent state but is not in the current-day success set.

- [ ] **Step 2: Write failing runtime-recording test**

Construct a `Server` with a tracker and a disabled JSONL path, call `recordRequestLog` with a fully successful authenticated entry, then assert subsequent protocol failures do not create a permanent block. Assert a denied request with `auth_result=ok` does not mark the IP successful.

- [ ] **Step 3: Run integration-focused tests and confirm missing behavior**

Run: `go test ./src/server/tunnel -run 'TestReconcileSuccessfulIPHistory|TestRecordRequestLogMarksSuccessfulIP' -count=1`

Expected: FAIL because reconciliation and runtime marking are not connected.

- [ ] **Step 4: Implement startup reconciliation**

Scan all qualifying entries, mark only entries from today's local date in the daily set, collect successful IPs that currently exist in `fails.permanent`, and call one bulk removal after scanning. `removePermanentSuccessful` rewrites disk first and deletes only actually removed IPs from memory. Return joined scan/rewrite errors so callers can report partial failures.

- [ ] **Step 5: Connect startup before the accept loop**

After `fails.load()` and before constructing/starting the server accept loop, call reconciliation with `cfg.Runtime.RequestLogFile` and `time.Now()`. Report errors through the existing `logf` callback and continue with security-safe retained blocks.

- [ ] **Step 6: Connect runtime request success**

At the beginning of `recordRequestLog`, call the shared predicate and then `s.fails.markSuccessful(entry.RemoteIP)` before JSON serialization and disk I/O. Guard nil server/tracker values used by unit tests.

- [ ] **Step 7: Run service tests**

Run: `go test ./src/server/tunnel -count=1`

Expected: PASS.

- [ ] **Step 8: Commit startup and runtime integration**

```text
feat(server): reconcile successful IPs on startup
```

### Task 4: Full Verification

**Files:**
- Modify only files owned by Tasks 1-3 if verification exposes a regression.

**Interfaces:**
- Produces: tested server behavior and a buildable Windows release without changing delivery contents.

- [ ] **Step 1: Format and inspect boundaries**

Run: `gofmt -w src/server/tunnel/permanent_blocks.go src/server/tunnel/permanent_blocks_test.go src/server/tunnel/successful_ip_history.go src/server/tunnel/successful_ip_history_test.go src/server/tunnel/fail_persistence.go src/server/tunnel/event_logs.go src/server/tunnel/server.go`

Run: `git diff --check`

Confirm `src/server/front/index.html` remains uncommitted and absent from feature commits.

- [ ] **Step 2: Run all Go tests**

Run: `go test ./src/... -count=1`

Expected: PASS.

- [ ] **Step 3: Run core static checks**

Run: `go vet ./src/server/tunnel`

Expected: PASS.

- [ ] **Step 4: Run Windows project self-check**

Run: `deploy\windows\test\selfcheck.cmd`

Expected: `selfcheck PASS`.

- [ ] **Step 5: Run the release build**

Run: `release.cmd`

Expected: server, standard client, Win7 Lite, and Android deliverables are produced with valid signatures and the existing dist structure.

- [ ] **Step 6: Inspect commits and worktree**

Run: `git log --oneline -6`

Run: `git status --short`

Expected: only the pre-existing `src/server/front/index.html` modification remains.
