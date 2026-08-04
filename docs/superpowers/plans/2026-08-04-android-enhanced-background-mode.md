# Android Enhanced Background Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a user-controlled Android background mode that keeps the tunnel awake for the full connected lifecycle and requests battery-optimization exemption without changing normal background behavior.

**Architecture:** Persist the user setting separately from tunnel profiles and runtime snapshots. A focused resource controller owns idempotent CPU/Wi-Fi lock acquisition, while `TunnelForegroundService` maps connection and network lifecycle events into that controller. `MainActivity` owns the switch and system authorization flow; Android 14+ deliberately skips the ineffective background Wi-Fi performance lock.

**Tech Stack:** Kotlin, Android SDK 29-35, foreground service, `PowerManager.WakeLock`, `WifiManager.WifiLock`, SharedPreferences, JUnit 4, Gradle.

## Global Constraints

- Preserve `minSdk=29`, `targetSdk=35`, and version `2.1.0`.
- Keep the existing tunnel protocol, server behavior, profile format, and forward-rule format unchanged.
- Default enhanced background mode to disabled.
- Hold enhanced resources for the full desired-running lifecycle, not only while a business stream is active.
- Never use overlays, transparent activities, picture-in-picture, hidden APIs, or periodic heartbeats to imitate foreground state.
- On Android 10-13, use `WIFI_MODE_FULL_HIGH_PERF` only while the default network is Wi-Fi.
- On Android 14+, do not acquire a background Wi-Fi performance lock because the public API is replaced by a foreground-only low-latency lock.
- A rejected battery-optimization request must not disable enhanced mode or prevent tunnel connection.
- Every disconnect, fatal failure, service destruction, and mode-disable path must release all acquired resources.
- Do not stage or modify the existing user change in `src/server/front/index.html`.

---

### Task 1: Enhanced Background Setting and Presentation

**Files:**
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundSettings.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundPresentationTest.kt`

**Interfaces:**
- Produces: `EnhancedResourceStatus(cpuHeld: Boolean, wifiHeld: Boolean, warning: String)`.
- Produces: `EnhancedBackgroundSettings.enabled()`, `setEnabled(Boolean)`, `resourceStatus()`, and `publishResourceStatus(EnhancedResourceStatus)`.
- Produces: `EnhancedBackgroundPresenter.present(enabled, batteryExempt, desiredRunning, resourceStatus): EnhancedBackgroundPresentation`.

- [ ] **Step 1: Write failing presentation tests**

```kotlin
class EnhancedBackgroundPresentationTest {
    @Test fun disabledHasNoDetail() {
        assertEquals(
            EnhancedBackgroundPresentation(false, ""),
            EnhancedBackgroundPresenter.present(false, false, false, EnhancedResourceStatus())
        )
    }

    @Test fun enabledWithoutBatteryExemptionShowsIncompleteRestriction() {
        assertEquals(
            "增强后台已开启，系统限制未完全解除",
            EnhancedBackgroundPresenter.present(true, false, true, EnhancedResourceStatus(cpuHeld = true)).detail
        )
    }

    @Test fun resourceFailureTakesPriorityWhileRunning() {
        assertEquals(
            "增强后台已开启，CPU 保持失败",
            EnhancedBackgroundPresenter.present(
                true,
                true,
                true,
                EnhancedResourceStatus(warning = "CPU 保持失败")
            ).detail
        )
    }
}
```

- [ ] **Step 2: Run tests and verify the red state**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest --tests "com.lsyl.tunnel.mobile.service.EnhancedBackgroundPresentationTest"`

Expected: compilation fails because the presentation and settings types do not exist.

- [ ] **Step 3: Implement settings and pure presentation**

```kotlin
data class EnhancedResourceStatus(
    val cpuHeld: Boolean = false,
    val wifiHeld: Boolean = false,
    val warning: String = ""
)

data class EnhancedBackgroundPresentation(val checked: Boolean, val detail: String)

object EnhancedBackgroundPresenter {
    fun present(
        enabled: Boolean,
        batteryExempt: Boolean,
        desiredRunning: Boolean,
        resourceStatus: EnhancedResourceStatus
    ): EnhancedBackgroundPresentation {
        if (!enabled) return EnhancedBackgroundPresentation(false, "")
        val detail = when {
            desiredRunning && resourceStatus.warning.isNotBlank() ->
                "增强后台已开启，${resourceStatus.warning}"
            !batteryExempt -> "增强后台已开启，系统限制未完全解除"
            else -> "增强后台已开启"
        }
        return EnhancedBackgroundPresentation(true, detail)
    }
}
```

`EnhancedBackgroundSettings` uses its own `lsyl_tunnel_background` SharedPreferences file. `setEnabled()` uses `commit()` because the service may be started immediately after the UI toggle. Runtime resource status uses `apply()` and defaults to all resources released after process recreation.

- [ ] **Step 4: Run the focused and full Android unit tests**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest`

Expected: all tests pass.

- [ ] **Step 5: Commit Task 1**

```bash
git add mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundSettings.kt mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundPresentationTest.kt
git commit -m "feat(android): add enhanced background setting"
```

### Task 2: Idempotent Enhanced Resource Controller

**Files:**
- Create: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundResources.kt`
- Create: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundResourcesTest.kt`
- Modify: `mobile/android/app/src/main/AndroidManifest.xml`

**Interfaces:**
- Consumes: `EnhancedResourceStatus` and `EnhancedBackgroundSettings.publishResourceStatus()` from Task 1.
- Produces: `EnhancedBackgroundResourceController.update(enabled: Boolean, desiredRunning: Boolean, wifiAvailable: Boolean)` and `release()`.
- Produces: `EnhancedBackgroundResourceController.create(context, statusSink)` for Android resource wrappers.

- [ ] **Step 1: Write failing resource-controller tests**

Use fake non-reference-counted locks and cover these exact cases:

```kotlin
private class FakeLock(private val failAcquire: Boolean = false) : BackgroundResourceLock {
    override var isHeld = false
    var acquireCount = 0
    var releaseCount = 0

    override fun acquire() {
        if (failAcquire) error("acquire failed")
        if (!isHeld) {
            isHeld = true
            acquireCount++
        }
    }

    override fun release() {
        if (isHeld) {
            isHeld = false
            releaseCount++
        }
    }
}

@Test fun enabledRunningAcquiresCpuAndSupportedWifi() {
    val cpu = FakeLock()
    val wifi = FakeLock()
    val statuses = mutableListOf<EnhancedResourceStatus>()
    EnhancedBackgroundResourceController(cpu, wifi, statuses::add).update(true, true, true)
    assertTrue(cpu.isHeld)
    assertTrue(wifi.isHeld)
    assertEquals(EnhancedResourceStatus(true, true, ""), statuses.last())
}

@Test fun repeatedUpdateIsIdempotent() {
    val cpu = FakeLock()
    val wifi = FakeLock()
    val controller = EnhancedBackgroundResourceController(cpu, wifi) {}
    controller.update(true, true, true)
    controller.update(true, true, true)
    assertEquals(1, cpu.acquireCount)
    assertEquals(1, wifi.acquireCount)
}

@Test fun mobileNetworkReleasesOnlyWifi() {
    val cpu = FakeLock()
    val wifi = FakeLock()
    val controller = EnhancedBackgroundResourceController(cpu, wifi) {}
    controller.update(true, true, true)
    controller.update(true, true, false)
    assertTrue(cpu.isHeld)
    assertFalse(wifi.isHeld)
}

@Test fun disabledOrNotDesiredReleasesEverything() {
    val cpu = FakeLock()
    val wifi = FakeLock()
    val controller = EnhancedBackgroundResourceController(cpu, wifi) {}
    controller.update(true, true, true)
    controller.update(false, true, true)
    assertFalse(cpu.isHeld)
    assertFalse(wifi.isHeld)
}

@Test fun android14PolicyNeverAcquiresWifiLock() {
    val cpu = FakeLock()
    EnhancedBackgroundResourceController(cpu, null) {}.update(true, true, true)
    assertTrue(cpu.isHeld)
}

@Test fun partialAcquireFailurePublishesSpecificWarning() {
    val statuses = mutableListOf<EnhancedResourceStatus>()
    EnhancedBackgroundResourceController(FakeLock(failAcquire = true), null, statuses::add)
        .update(true, true, false)
    assertEquals("CPU 保持失败", statuses.last().warning)
}
```

- [ ] **Step 2: Run the focused test and verify failure**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest --tests "com.lsyl.tunnel.mobile.service.EnhancedBackgroundResourcesTest"`

Expected: compilation fails because resource-controller interfaces do not exist.

- [ ] **Step 3: Implement the pure controller and Android lock wrappers**

```kotlin
internal interface BackgroundResourceLock {
    val isHeld: Boolean
    fun acquire()
    fun release()
}

internal class EnhancedBackgroundResourceController(
    private val cpuLock: BackgroundResourceLock,
    private val wifiLock: BackgroundResourceLock?,
    private val statusSink: (EnhancedResourceStatus) -> Unit
) {
    @Synchronized
    fun update(enabled: Boolean, desiredRunning: Boolean, wifiAvailable: Boolean) {
        if (!enabled || !desiredRunning) {
            release()
            return
        }
        val warnings = mutableListOf<String>()
        acquire(cpuLock, "CPU 保持失败", warnings)
        if (wifiAvailable && wifiLock != null) acquire(wifiLock, "Wi-Fi 保持失败", warnings)
        else releaseQuietly(wifiLock)
        statusSink(EnhancedResourceStatus(cpuLock.isHeld, wifiLock?.isHeld == true, warnings.joinToString("，")))
    }

    @Synchronized
    fun release() {
        releaseQuietly(wifiLock)
        releaseQuietly(cpuLock)
        statusSink(EnhancedResourceStatus())
    }
}
```

The Android factory creates a non-reference-counted `PARTIAL_WAKE_LOCK`. It creates `WIFI_MODE_FULL_HIGH_PERF` only when `Build.VERSION.SDK_INT < 34`. All acquire/release exceptions are converted to status warnings and never escape into tunnel startup.

- [ ] **Step 4: Add required manifest permissions**

```xml
<uses-permission android:name="android.permission.WAKE_LOCK" />
<uses-permission android:name="android.permission.REQUEST_IGNORE_BATTERY_OPTIMIZATIONS" />
```

- [ ] **Step 5: Run focused and full tests**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest`

Expected: all tests pass, including lock failure and idempotency cases.

- [ ] **Step 6: Commit Task 2**

```bash
git add mobile/android/app/src/main/AndroidManifest.xml mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundResources.kt mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundResourcesTest.kt
git commit -m "feat(android): control enhanced background resources"
```

### Task 3: Foreground Service and Network Lifecycle Integration

**Files:**
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/DefaultNetworkMonitor.kt`
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/TunnelForegroundService.kt`
- Modify: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/RecoverySignalGateTest.kt`

**Interfaces:**
- Consumes: `EnhancedBackgroundSettings` and `EnhancedBackgroundResourceController` from Tasks 1-2.
- Extends: `DefaultNetworkMonitor(..., onWifiChanged: (Boolean) -> Unit = {})`.
- Produces: `TunnelForegroundService.ACTION_UPDATE_BACKGROUND_MODE` and `updateBackgroundModeIntent(context)`.

- [ ] **Step 1: Add a failing Wi-Fi transition test**

Extend pure tracker coverage so repeated capabilities do not queue duplicate Wi-Fi state changes:

```kotlin
@Test fun wifiTrackerPublishesOnlyTransitions() {
    val published = mutableListOf<Boolean?>()
    val tracker = NetworkChangeTracker<Boolean>()
    tracker.publishIfChanged(false, published::add)
    tracker.publishIfChanged(true, published::add)
    tracker.publishIfChanged(true, published::add)
    tracker.publishIfChanged(false, published::add)
    assertEquals(listOf(false, true, false), published)
}
```

- [ ] **Step 2: Run focused tests and verify failure**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest --tests "com.lsyl.tunnel.mobile.service.RecoverySignalGateTest"`

Expected: compilation fails because `publishIfChanged()` does not exist.

- [ ] **Step 3: Extend `DefaultNetworkMonitor` with capability tracking**

Handle `onCapabilitiesChanged(network, capabilities)` and publish `capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)`. On loss, inspect the current active network and capabilities before publishing. Keep the existing availability recovery callbacks and deduplicate both network and Wi-Fi transitions.

- [ ] **Step 4: Integrate resources into service commands**

Initialize settings and resources before starting the network monitor. Add:

```kotlin
private fun syncEnhancedResources() {
    enhancedResources.update(
        enabled = enhancedSettings.enabled(),
        desiredRunning = stateStore.desiredRunning(),
        wifiAvailable = wifiAvailable
    )
}
```

Call it after `ensureForeground()` for start and sticky recovery, on `ACTION_UPDATE_BACKGROUND_MODE`, and on Wi-Fi transitions. Release after disconnect, in `finishIfInactive()`, and before executor/controller shutdown in `onDestroy()`. A fatal controller failure clears desired-running; `finishIfInactive()` must therefore release resources before removing the notification.

- [ ] **Step 5: Run all Android unit tests**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest`

Expected: all existing lifecycle, recovery, tunnel, profile, and new enhancement tests pass.

- [ ] **Step 6: Commit Task 3**

```bash
git add mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/DefaultNetworkMonitor.kt mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service/TunnelForegroundService.kt mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/RecoverySignalGateTest.kt
git commit -m "feat(android): apply enhanced background lifecycle"
```

### Task 4: User Switch and Battery Optimization Authorization

**Files:**
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/MainActivity.kt`
- Modify: `mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundPresentationTest.kt`

**Interfaces:**
- Consumes: `EnhancedBackgroundSettings`, `EnhancedBackgroundPresenter`, and `TunnelForegroundService.updateBackgroundModeIntent(context)`.

- [ ] **Step 1: Complete presentation tests for disconnected, authorized, and warning states**

Add exact assertions for enabled/disconnected, enabled/authorized/running, and disabled states so UI copy remains stable.

- [ ] **Step 2: Add the native switch and status text**

Create a `Switch` labeled `增强后台` in the operation area above connect/disconnect controls. Use a listener-suppression flag while refreshing from persisted state. Do not create a second menu or modal setting page.

- [ ] **Step 3: Implement toggle behavior**

On enable: persist first, notify a running service immediately, then request battery-optimization exemption if needed. On disable: persist, notify the running service to release locks, and keep the tunnel connected.

```kotlin
private fun setEnhancedBackground(enabled: Boolean) {
    enhancedSettings.setEnabled(enabled)
    if (runtimeStore.desiredRunning()) {
        startTunnelService(TunnelForegroundService.updateBackgroundModeIntent(this))
    }
    if (enabled) requestBatteryOptimizationExemptionIfNeeded()
    refreshEnhancedBackgroundView()
}
```

- [ ] **Step 4: Implement system authorization flow**

Check `PowerManager.isIgnoringBatteryOptimizations(packageName)`. Open `Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` with `package:$packageName`; if unavailable, fall back to `Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS`. Catch failures, leave the switch enabled, and show the incomplete-system-restriction detail. Refresh real authorization state in `onResume()`.

- [ ] **Step 5: Run tests and build the debug APK**

Run: `mobile\android\gradlew.bat -p mobile\android testDebugUnitTest assembleDebug`

Expected: all tests pass and `mobile/android/app/build/outputs/apk/debug/app-debug.apk` is generated.

- [ ] **Step 6: Commit Task 4**

```bash
git add mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/MainActivity.kt mobile/android/app/src/test/java/com/lsyl/tunnel/mobile/service/EnhancedBackgroundPresentationTest.kt
git commit -m "feat(android): expose enhanced background mode"
```

### Task 5: Full Verification and Real-Device ADB Validation

**Files:**
- Modify only if verification exposes a proven defect in files already listed above.

**Interfaces:**
- Consumes the completed Android feature from Tasks 1-4.

- [ ] **Step 1: Run repository Android verification**

Run: `mobile\android\gradlew.bat -p mobile\android clean testDebugUnitTest assembleDebug`

Expected: Gradle reports `BUILD SUCCESSFUL`.

- [ ] **Step 2: Install over the current debug app without clearing data**

Run: `adb install -r mobile\android\app\build\outputs\apk\debug\app-debug.apk`

Expected: `Success`; existing profile and desired-running setting remain present.

- [ ] **Step 3: Verify standard mode**

Connect with enhanced mode off, move the app to background, and check:

```bash
adb shell dumpsys activity services com.lsyl.tunnel.mobile
adb shell dumpsys power
```

Expected: service is `isForeground=true`; no `LSYL Tunnel:enhanced-background` wake lock is held.

- [ ] **Step 4: Verify enhanced mode and authorization**

Enable the switch, accept the system exemption, keep the tunnel connected, and re-run:

```bash
adb shell dumpsys deviceidle whitelist
adb shell dumpsys power
adb shell dumpsys wifi
```

Expected on the current Android 15 device: app is battery-exempt and the CPU wake lock is visible; no claim or expectation is made for a background Wi-Fi performance lock.

- [ ] **Step 5: Verify release and persistence paths**

Disable the switch while connected and verify the wake lock disappears without breaking RDP. Re-enable it, force-stop/reopen or let sticky recovery recreate the service, and verify the setting and lock are restored. Finally disconnect and verify every enhanced lock disappears.

- [ ] **Step 6: Run a bounded RDP stability comparison**

Use the same `127.0.0.1:3388` RDP profile for ordinary background and enhanced background. Observe at least one network transition and one screen-off interval in each mode. Record whether the tunnel process survives, the local listener remains, and the RDP session recovers; do not infer long-term battery life from this short run.

- [ ] **Step 7: Inspect final diff and status**

Run: `git diff --check` and `git status --short`.

Expected: no whitespace errors; only the pre-existing `src/server/front/index.html` modification remains outside committed Android work.


