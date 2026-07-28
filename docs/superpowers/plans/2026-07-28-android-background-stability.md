# Android Background Tunnel Stability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Android tunnel run independently of `MainActivity`, survive transient network loss without dropping local listeners, restore after system process reclamation, expose precise status, and support restricted forward-rule text editing.

**Architecture:** A pure Kotlin `TunnelRuntimeController` owns lifecycle policy while `TunnelForegroundService` supplies Android service, persistence, notification, and network callback adapters. `TunnelManager` owns durable loopback listeners and treats each remote stream as disposable. Structured snapshots replace string-based state, and a safe YAML parser updates only `forwards` through an atomic profile-file write.

**Tech Stack:** Kotlin 2.0.21, Android API 29-35, Java 17, JUnit 4, SnakeYAML Engine 3.0.1, Go regression tests, Gradle 8.9.

## Global Constraints

- The tunnel must continue after `MainActivity` is paused, destroyed, removed from recents, or replaced by another app.
- No 15-second health poll, fixed retry loop, `WakeLock`, or battery-optimization exemption.
- A transient network failure keeps the foreground service and every bound local listener alive.
- User disconnect and unrecoverable profile/certificate/credential/version failures set `desired_running=false`.
- Existing TCP streams may break during a network switch; a new local connection must be able to establish a new tunnel.
- Android remains `minSdk=29`, `targetSdk=35`, TLS 1.3 pinned certificate, and `client_to_server` only.
- Text editing can replace only `forwards`; server address, account, sealed credential, certificate, TLS, and connection settings remain unchanged.
- Runtime issue display must not expose `server_target`, server address, certificate details, or sealed credential.
- No server protocol or server configuration change.

---

## File Map

- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/runtime/RuntimeIssue.kt`: platform-neutral issue and recovery-disposition model shared by tunnel and service layers.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/runtime/FailureClassifier.kt`: maps typed network/TLS/protocol failures to recovery policy and Chinese copy.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/RuntimeSnapshot.kt`: structured phase, safe serialization, and display data.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/TunnelRuntimeController.kt`: pure lifecycle policy for start, refresh, network loss, fatal failure, system release, and user stop.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/RuntimePresentation.kt`: pure snapshot-to-UI projection used by `MainActivity`.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/RuntimeStateStore.kt`: SharedPreferences adapter for snapshot and `desired_running`.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/DefaultNetworkMonitor.kt`: default-network callback adapter with event coalescing.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile/ForwardTextConfig.kt`: safe YAML rendering and strict parsing.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile/ProfileFiles.kt`: filesystem-only profile operations and atomic forward replacement.
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/protocol/TunnelProtocol.kt`: testable protocol interface implemented by `ProtocolClient`.
- Existing `TunnelStats.kt`, `LocalForward.kt`, and `TunnelManager.kt`: listener state separated from per-stream issue state.
- Existing `TunnelForegroundService.kt`: Android adapter around the controller; no periodic monitor.
- Existing `MainActivity.kt`: snapshot rendering, manual recheck, and restricted text editor.

---

### Task 1: Structured Runtime State And Failure Policy

**Files:**
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/RuntimeSnapshot.kt`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/runtime/RuntimeIssue.kt`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/runtime/FailureClassifier.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/RuntimeSnapshotTest.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/runtime/FailureClassifierTest.kt`

**Interfaces:**
- Produces: `TunnelPhase`, `IssueDisposition`, `RuntimeIssue`, `RuntimeSnapshot`, `FailureClassifier.classify(Throwable, String?, Int?)`.
- `RuntimeSnapshot.toJson()` and `RuntimeSnapshot.fromJson(String)` must round-trip without a `server_target` field.

- [ ] **Step 1: Write failing snapshot codec tests**

```kotlin
@Test fun snapshotRoundTripKeepsOnlySafeRuleDetails() {
    val source = RuntimeSnapshot(
        phase = TunnelPhase.DEGRADED,
        summary = "部分端口不可用",
        listenerCount = 1,
        issues = listOf(RuntimeIssue("target_denied", "当前账号没有访问该端口的权限", IssueDisposition.RULE_DISABLED, "rdp", 13389))
    )
    val encoded = source.toJson()
    assertFalse(encoded.contains("server_target"))
    assertEquals(source, RuntimeSnapshot.fromJson(encoded))
}
```

- [ ] **Step 2: Run the snapshot test and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.service.RuntimeSnapshotTest`

Expected: compilation fails because `RuntimeSnapshot` does not exist.

- [ ] **Step 3: Implement the minimal structured snapshot model and JSON codec**

```kotlin
enum class TunnelPhase { DISCONNECTED, STARTING, CONNECTED, DEGRADED, WAITING_NETWORK, STOPPING, FAILED }
```

`RuntimeIssue.kt` contains the platform-neutral model:

```kotlin
enum class IssueDisposition { TRANSIENT, FATAL, RULE_DISABLED, CONNECTION_ONLY }

data class RuntimeIssue(
    val code: String,
    val message: String,
    val disposition: IssueDisposition,
    val ruleName: String? = null,
    val localPort: Int? = null
)
```

`RuntimeSnapshot.kt` contains the service-safe snapshot:

```kotlin
data class RuntimeSnapshot(
    val phase: TunnelPhase,
    val summary: String,
    val listenerCount: Int = 0,
    val issues: List<RuntimeIssue> = emptyList()
)
```

- [ ] **Step 4: Run the snapshot test and verify GREEN**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.service.RuntimeSnapshotTest`

Expected: PASS.

- [ ] **Step 5: Write failing typed failure-classification tests**

Cover `UnknownHostException`, `SocketTimeoutException`, refused `ConnectException`, certificate `SSLHandshakeException`, and protocol codes `credential_expired`, `auth_failed`, `auth_blocked`, `client_version_unsupported`, `protocol_version_unsupported`, `user_stream_limit`, `target_denied`, and `target_unreachable`. Assert exact disposition and safe Chinese message.

```kotlin
@Test fun timeoutWaitsForNetworkWithoutKillingRuntime() {
    val issue = FailureClassifier.classify(SocketTimeoutException("timed out"))
    assertEquals(IssueDisposition.TRANSIENT, issue.disposition)
    assertEquals("连接服务端超时，等待网络恢复", issue.message)
}
```

- [ ] **Step 6: Run classifier tests and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.runtime.FailureClassifierTest`

Expected: compilation fails because `FailureClassifier` does not exist.

- [ ] **Step 7: Implement typed failure classification and cause-chain inspection**

Protocol codes take priority over message matching. Java exception types take priority for DNS, timeout, refusal, routing, and TLS certificate failures. Unknown remote errors are transient unless they are local profile/configuration errors.

- [ ] **Step 8: Run both test classes and commit**

```powershell
gradle --no-daemon :app:testDebugUnitTest --tests 'com.lsyl.tunnel.mobile.service.*' --tests 'com.lsyl.tunnel.mobile.runtime.*'
git add mobile/android/app/src/main mobile/android/app/src/test
git commit -m "feat(android): add structured tunnel runtime state"
```

---

### Task 2: Strict Forward Text Parsing And Atomic Profile Update

**Files:**
- Modify: `mobile/android/app/build.gradle.kts`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile/ForwardTextConfig.kt`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile/ProfileFiles.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile/ProfileStore.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile/ProfileImporter.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/profile/ForwardTextConfigTest.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/profile/ProfileFilesTest.kt`

**Interfaces:**
- Produces: `ForwardTextConfig.render(List<ForwardConfig>): String` and `ForwardTextConfig.parse(String): List<ForwardConfig>`.
- Produces: `ProfileFiles.updateForwards(List<ForwardConfig>)` with same-directory temp write, `FileDescriptor.sync()`, and atomic replace.
- `ProfileStore.updateForwards` delegates to `ProfileFiles`; existing import/load/delete behavior remains.

- [ ] **Step 1: Add SnakeYAML Engine as an implementation dependency**

```kotlin
implementation("org.snakeyaml:snakeyaml-engine:3.0.1")
```

- [ ] **Step 2: Write failing strict-parser tests**

Tests must prove valid rendering/parsing and reject unknown top-level keys, unknown rule keys, duplicate YAML keys, missing fields, wrong nesting, non-loopback listeners, ports below 1024, duplicate names/listeners, reverse direction, and more than eight rules.

```kotlin
@Test fun parsesRestrictedForwardYaml() {
    val parsed = ForwardTextConfig.parse("""
        forwards:
          - name: rdp
            listen_addr: 127.0.0.1:13389
            server_target: 192.168.1.7:3389
    """.trimIndent())
    assertEquals("192.168.1.7:3389", parsed.single().serverTarget)
    assertEquals(DIRECTION_CLIENT_TO_SERVER, parsed.single().direction)
}
```

- [ ] **Step 3: Run parser tests and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.profile.ForwardTextConfigTest`

Expected: compilation fails because `ForwardTextConfig` does not exist.

- [ ] **Step 4: Implement safe YAML loading and shared forward validation**

Use `LoadSettings.builder().setAllowDuplicateKeys(false)` and `Load`; accept only `Map<String, Any?>` root with exactly `forwards`, and rule mappings with exactly `name`, `listen_addr`, and `server_target`. Reuse one validator from import and text editing so both paths enforce identical mobile boundaries.

- [ ] **Step 5: Run parser tests and verify GREEN**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.profile.ForwardTextConfigTest`

Expected: PASS.

- [ ] **Step 6: Write failing atomic-profile tests**

Use JUnit `TemporaryFolder` to create `profile.json` and `server.crt`. Assert that updating forwards preserves every non-forward JSON field and certificate byte, and that invalid replacement leaves the original files byte-for-byte unchanged.

- [ ] **Step 7: Run profile-file tests and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.profile.ProfileFilesTest`

Expected: compilation fails because `ProfileFiles` does not exist.

- [ ] **Step 8: Implement filesystem adapter and delegate `ProfileStore`**

Write `profile.json.tmp` in the active profile directory, flush and sync it, then use `Files.move(..., ATOMIC_MOVE, REPLACE_EXISTING)` with a same-filesystem `REPLACE_EXISTING` fallback only for `AtomicMoveNotSupportedException`. Always delete the temp file in `finally`.

- [ ] **Step 9: Run all profile tests and commit**

```powershell
gradle --no-daemon :app:testDebugUnitTest --tests 'com.lsyl.tunnel.mobile.profile.*'
git add mobile/android/app/build.gradle.kts mobile/android/app/src/main mobile/android/app/src/test
git commit -m "feat(android): add restricted forward text config"
```

---

### Task 3: Keep Local Listeners Alive Across Stream Failures

**Files:**
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/protocol/TunnelProtocol.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/protocol/ProtocolClient.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/tunnel/TunnelStats.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/tunnel/LocalForward.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/tunnel/TunnelManager.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/tunnel/LocalForwardResilienceTest.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/tunnel/TunnelManagerTest.kt`

**Interfaces:**
- `TunnelProtocol` exposes `health()`, `forwardCheck(ForwardConfig)`, and `open(ForwardConfig): Socket`.
- `TunnelManager.startListeners()` binds local ports without requiring network access.
- `TunnelManager.checkRemote(onStage: (String) -> Unit = {})` performs one explicit health/rule check.
- `TunnelManager.stop()` remains idempotent.
- `ForwardStatus` exposes listener state plus optional safe `RuntimeIssue`; it never exposes target data to the UI snapshot.

- [ ] **Step 1: Write failing local-forward resilience test**

Use a real loopback port and fake `TunnelProtocol` whose first `open` throws `SocketTimeoutException`. Connect a local socket twice and assert the fake protocol receives both attempts, proving the first stream failure did not close the listener.

- [ ] **Step 2: Run resilience test and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.tunnel.LocalForwardResilienceTest`

Expected: the second connection is rejected or the test cannot compile against `TunnelProtocol`.

- [ ] **Step 3: Add protocol interface and separate listener state from stream issue**

`LocalForward.handleLocal` reports classified connection issues without calling `stop`, except `target_denied`, which disables only that rule. A successful `protocol.open` clears the previous transient issue before copying bytes.

- [ ] **Step 4: Run resilience test and verify GREEN**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.tunnel.LocalForwardResilienceTest`

Expected: PASS.

- [ ] **Step 5: Write failing manager startup tests**

Assert that `startListeners()` succeeds while fake `health()` throws a timeout, `checkRemote()` propagates the timeout without stopping listeners, one `target_denied` rule does not stop another, and later successful check clears recoverable rule issues.

- [ ] **Step 6: Run manager tests and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.tunnel.TunnelManagerTest`

Expected: compilation fails because `startListeners` and `checkRemote` do not exist.

- [ ] **Step 7: Refactor `TunnelManager` around durable listeners and explicit remote checks**

Start every local listener first. Remote health failures propagate to the controller without calling `stop`. Rule-specific checks update only their own runtime. If a disabled rule becomes authorized during manual/network recovery, recreate its listener.

- [ ] **Step 8: Run tunnel tests and commit**

```powershell
gradle --no-daemon :app:testDebugUnitTest --tests 'com.lsyl.tunnel.mobile.tunnel.*'
git add mobile/android/app/src/main mobile/android/app/src/test
git commit -m "fix(android): keep listeners across network failures"
```

---

### Task 4: Pure Lifecycle Controller And Persistent Intent

**Files:**
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/TunnelRuntimeController.kt`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/RuntimeStateStore.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/TunnelRuntimeControllerTest.kt`

**Interfaces:**
- `ManagedTunnel` is implemented by `TunnelManager` and exposes `startListeners`, `checkRemote`, `stats`, and `stop`.
- `RuntimeStateSink` exposes `desiredRunning`, `setDesiredRunning`, and `publish(RuntimeSnapshot)`.
- Controller methods: `connect(userInitiated: Boolean)`, `refresh()`, `networkUnavailable()`, `disconnect()`, and `releaseForSystem()`.

- [ ] **Step 1: Write failing controller lifecycle tests**

Tests with fake runtime/store must prove:

```kotlin
@Test fun transientRefreshFailureKeepsRuntimeAndDesiredIntent() {
    val runtime = FakeRuntime(refreshFailure = SocketTimeoutException())
    val controller = controller(runtime)
    controller.connect(userInitiated = true)
    controller.refresh()
    assertFalse(runtime.stopped)
    assertTrue(store.desiredRunning)
    assertEquals(TunnelPhase.WAITING_NETWORK, store.latest.phase)
}
```

Also cover fatal certificate/credential/version failure stopping runtime and clearing desired intent, user disconnect clearing desired intent, `releaseForSystem()` stopping sockets while preserving desired intent, duplicate connect not creating a second runtime, and partial listener failure producing `DEGRADED`.

- [ ] **Step 2: Run controller tests and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.service.TunnelRuntimeControllerTest`

Expected: compilation fails because the controller does not exist.

- [ ] **Step 3: Implement controller and Android SharedPreferences adapter**

All controller calls are synchronous and must be made from the service's single command executor. The controller never references `Activity`, `Service`, `Handler`, notification APIs, or `ConnectivityManager`.

- [ ] **Step 4: Run service-domain tests and commit**

```powershell
gradle --no-daemon :app:testDebugUnitTest --tests 'com.lsyl.tunnel.mobile.service.*'
git add mobile/android/app/src/main mobile/android/app/src/test
git commit -m "feat(android): add restorable tunnel lifecycle"
```

---

### Task 5: Foreground Service And Event-Driven Network Recovery

**Files:**
- Modify: `mobile/android/app/src/main/AndroidManifest.xml`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/DefaultNetworkMonitor.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/TunnelForegroundService.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/RecoverySignalGateTest.kt`

**Interfaces:**
- Service actions remain `START`, `REFRESH`, and `STOP` for compatibility.
- `intent == null` is a restore request, not a user start request.
- `DefaultNetworkMonitor` emits only changes for the active default network.
- `RecoverySignalGate.tryQueue()` ensures repeated `onAvailable` callbacks queue at most one refresh until `complete()`.

- [ ] **Step 1: Write failing recovery-signal coalescing test**

Assert that two availability events before completion produce one queued refresh and that a new event after completion is accepted.

- [ ] **Step 2: Run test and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.service.RecoverySignalGateTest`

Expected: compilation fails because `RecoverySignalGate` does not exist.

- [ ] **Step 3: Implement network monitor and rewrite service as an adapter**

Remove `Handler`, `monitorRunnable`, `MONITOR_INTERVAL_MS`, `startRuntimeMonitor`, `runMonitorCheck`, and fatal-on-refresh behavior. Start foreground immediately, execute controller commands serially, render notifications from snapshots, and preserve `desired_running` in `onDestroy()`.

- [ ] **Step 4: Update manifest for persistent tunnel use**

```xml
<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
<uses-permission android:name="android.permission.FOREGROUND_SERVICE_SPECIAL_USE" />

<service
    android:name=".service.TunnelForegroundService"
    android:exported="false"
    android:foregroundServiceType="specialUse"
    android:stopWithTask="false">
    <property
        android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE"
        android:value="User-initiated persistent encrypted local port tunnel" />
</service>
```

Remove `FOREGROUND_SERVICE_DATA_SYNC`.

- [ ] **Step 5: Run service tests and assemble debug APK**

```powershell
gradle --no-daemon :app:testDebugUnitTest --tests 'com.lsyl.tunnel.mobile.service.*'
gradle --no-daemon :app:assembleDebug
```

Expected: tests pass and `app/build/outputs/apk/debug/app-debug.apk` exists.

- [ ] **Step 6: Commit platform integration**

```powershell
git add mobile/android/app/src/main mobile/android/app/src/test
git commit -m "fix(android): detach tunnel from activity lifecycle"
```

---

### Task 6: Status Details, Manual Recheck, And Forward Editor UI

**Files:**
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/MainActivity.kt`
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/RuntimePresentation.kt`
- Modify: `mobile/android/app/src/main/res/values/strings.xml` only if shared user-facing strings are extracted.
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/RuntimePresentationTest.kt`

**Interfaces:**
- Activity reads `RuntimeSnapshot` from `RuntimeStateStore` and receives snapshot JSON broadcasts.
- `onResume()` registers the receiver and reads local state only; it never sends `ACTION_REFRESH`.
- “重新检查” is the only UI path to `ACTION_REFRESH` besides network recovery.
- “编辑转发” opens a multiline YAML editor only while disconnected.

- [ ] **Step 1: Write failing presentation tests**

Assert button-state and issue-detail projection for `CONNECTED`, `DEGRADED`, `WAITING_NETWORK`, `FAILED`, and `DISCONNECTED`. Assert projected rule details contain name/local port/message and never target.

- [ ] **Step 2: Run presentation test and verify RED**

Run: `gradle --no-daemon :app:testDebugUnitTest --tests com.lsyl.tunnel.mobile.service.RuntimePresentationTest`

Expected: compilation fails because presentation projection is missing.

- [ ] **Step 3: Implement pure presentation projection and update Activity**

Add a compact issue text area below status, a manual “重新检查” command, and an “编辑转发” action. Remove `syncRuntimeStatus()` and string-prefix button logic. Import/delete/save success may use Toast; runtime errors remain inline.

- [ ] **Step 4: Wire restricted editor to atomic profile update**

Render current rules through `ForwardTextConfig.render`. On save, parse all text first, call `ProfileStore.updateForwards`, refresh the profile view, and keep the dialog open with the exact parser error when validation fails. Disable save while runtime is not `DISCONNECTED` or `FAILED` with no desired connection.

- [ ] **Step 5: Run all Android unit tests and assemble APK**

```powershell
gradle --no-daemon :app:testDebugUnitTest
gradle --no-daemon :app:assembleDebug
```

- [ ] **Step 6: Commit UI integration**

```powershell
git add mobile/android/app/src/main mobile/android/app/src/test
git commit -m "feat(android): improve tunnel status and config editing"
```

---

### Task 7: Documentation And Full Verification

**Files:**
- Modify: `docs/deployment/mobile-android-zh.md`
- Generated, ignored output: `dist/installers/LSYL-Tunnel-Android.apk`

**Interfaces:**
- Documentation states that the foreground service owns the tunnel after connection, Activity visibility is irrelevant, network recovery is event-driven, active streams may reconnect, and only forwards are editable.

- [ ] **Step 1: Update Android deployment documentation**

Remove the periodic-refresh description. Add exact background lifecycle, network recovery, notification semantics, restricted editor format, and force-stop limitation.

- [ ] **Step 2: Run formatting and source checks**

```powershell
git diff --check
rg -n 'MONITOR_INTERVAL_MS|runMonitorCheck|syncRuntimeStatus|FOREGROUND_SERVICE_DATA_SYNC' mobile/android/app/src
```

Expected: no runtime-polling or old FGS-type matches.

- [ ] **Step 3: Run complete Android and Go verification**

```powershell
Push-Location mobile/android
gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug
Pop-Location
go test -count=1 ./src/...
```

Expected: all commands exit 0.

- [ ] **Step 4: Build the distributable Android APK**

```powershell
cmd /c deploy\windows\app\build-android-apk.cmd "dist\installers\LSYL-Tunnel-Android.apk"
```

Expected: `dist/installers/LSYL-Tunnel-Android.apk` is replaced with the newly built APK.

- [ ] **Step 5: Inspect APK and final diff**

```powershell
Get-Item dist\installers\LSYL-Tunnel-Android.apk
git status --short
git diff --stat HEAD~6..HEAD
```

Confirm that only Android source/tests/docs and the plan changed on the feature branch.

- [ ] **Step 6: Commit documentation**

```powershell
git add docs/deployment/mobile-android-zh.md
git commit -m "docs(android): document background tunnel behavior"
```

## Manual Device Verification

An attached Android 10+ device is required for the final lifecycle proof:

1. Import a Profile and connect until the notification and UI both show `已连接`.
2. Press Home, open RDP, and use `127.0.0.1:<local_port>` for at least five minutes.
3. Remove LSYL from recents and repeat the connection without reopening LSYL.
4. Lock the screen for five minutes, unlock directly into RDP, and reconnect.
5. Switch Wi-Fi to mobile data and back. Confirm the notification changes to waiting/recovered state and a new RDP connection succeeds.
6. Return to LSYL and confirm no automatic remote refresh occurs merely because the Activity resumed.
7. Tap Disconnect and confirm notification, service, and loopback listener all stop and do not return.
