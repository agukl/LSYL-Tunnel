# Android Config Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Place all Android profile actions in one proportional row and require two explicit confirmations before deleting a profile.

**Architecture:** Keep the change inside `MainActivity`: use one weighted horizontal `LinearLayout` for the three existing buttons, and split deletion into two dialogs before calling the existing destructive operations. No profile format, tunnel protocol, or service behavior changes.

**Tech Stack:** Kotlin, Android platform widgets, Gradle 8.9

## Global Constraints

- Layout order is `导入配置 | 编辑转发 | 删除配置`.
- Available button width uses weights `2f / 1f / 1f`, equivalent to `50% / 25% / 25%` before fixed gaps.
- Deletion executes only after two consecutive positive confirmations.
- Do not add phone settings, battery management, or vendor-specific prompts.
- Per user direction, do not add or modify automated tests for this change.

---

### Task 1: Profile Action Row And Double Confirmation

**Files:**
- Modify: `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/MainActivity.kt`

**Interfaces:**
- Consumes: existing `importBtn`, `editBtn`, `deleteBtn`, `store`, `runtimeStore`, and `TunnelForegroundService.stopIntent(Context)`.
- Produces: `weightedButtonParams(weight: Float, startMargin: Int, endMargin: Int)` and a two-stage `deleteProfile()` flow.

- [x] **Step 1: Build one weighted action row**

Add all three buttons to `configRow` in this order, using weights `2f`, `1f`, and `1f`. Apply an 8dp visual gap between adjacent buttons with 4dp margins on each side.

- [x] **Step 2: Remove the separate edit button row**

Keep the existing flexible spacer, then add only `configRow` with a 24dp top margin.

- [x] **Step 3: Add the second destructive confirmation**

Keep the first dialog non-destructive. Its positive action opens a second dialog with the irreversible warning; only the second positive action clears desired runtime state, stops the service, deletes the profile, publishes `DISCONNECTED`, refreshes the page, and shows the existing toast.

### Task 2: Compile And Device Verification

**Files:**
- Generated: `dist/installers/LSYL-Tunnel-Android.apk`

- [x] **Step 1: Compile the Android APK**

Run `gradle --no-daemon assembleDebug` from `mobile/android` and require exit code 0.

- [x] **Step 2: Refresh the distributable APK**

Run `deploy/windows/app/build-android-apk.cmd "dist/installers/LSYL-Tunnel-Android.apk"`.

- [x] **Step 3: Verify on the attached Android device**

Install the distributable APK with ADB. Confirm one horizontal row contains the three actions, the import button occupies approximately half the usable row width, and profile deletion requires both confirmation dialogs.
