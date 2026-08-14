# Client Repackage Identity Sanitization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clear `client_id` from both default and on-site Windows client package configurations before compiling an installer.

**Architecture:** A focused PowerShell helper atomically clears exactly one top-level `client_id` field in place. Both source package assembly and the distributable `make-installer.cmd` call this helper, while Inno Setup continues to package the normal `conf\client.yaml`.

**Tech Stack:** Windows batch, PowerShell 5.1-compatible scripting, Inno Setup 6, existing Windows packaging self-check.

## Global Constraints

- The on-site `conf\client.yaml` is intentionally updated and remains identity-free after the build command runs.
- Only the top-level `client_id` value is changed; server address, account, certificate references, and forwarding rules are preserved.
- Missing or duplicate top-level `client_id` fields stop the build before Inno Setup runs.
- No protocol, server, config schema, or runtime identity behavior changes are included.
- Do not include the existing unrelated `src/server/front/index.html` worktree change in commits.

---

### Task 1: Atomic Client Identity Sanitizer

**Files:**
- Create: `deploy/windows/inno/clear-client-id.ps1`
- Create: `deploy/windows/test/test-clear-client-id.ps1`

**Interfaces:**
- Consumes: `-ConfigFile <path>` pointing to a UTF-8 client YAML file.
- Produces: exit code `0` with the same file containing exactly `client_id: ""`; nonzero exit without modifying invalid input.

- [ ] **Step 1: Write the failing sanitizer test**

Create `test-clear-client-id.ps1` with three real-file cases: a nonempty identity is cleared while neighboring text and line endings remain intact; a missing field fails without changing bytes; duplicate top-level fields fail without changing bytes. Invoke `deploy/windows/inno/clear-client-id.ps1` directly and fail the test process on any unmet assertion.

- [ ] **Step 2: Run the test to verify it fails**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File deploy/windows/test/test-clear-client-id.ps1
```

Expected: nonzero exit because `deploy/windows/inno/clear-client-id.ps1` does not exist.

- [ ] **Step 3: Implement the minimal sanitizer**

Implement a PowerShell 5.1-compatible script that:

```powershell
param([Parameter(Mandatory = $true)][string]$ConfigFile)

$pattern = [regex]::new('(?m)^(client_id[ \t]*:[ \t]*)[^\r\n]*(\r?)$')
```

Read UTF-8 bytes with strict decoding, preserve an existing UTF-8 BOM and original line endings, require exactly one match, replace only that match, write a sibling GUID-named temporary file, then use `[IO.File]::Replace` to atomically replace the source. Remove the temporary file in `finally`. If the value is already `""`, return successfully without rewriting the file.

- [ ] **Step 4: Run the focused test to verify it passes**

Run the command from Step 2. Expected: exit `0` with all three cases reported as passed.

### Task 2: Package Both Build Paths Through the Sanitizer

**Files:**
- Modify: `deploy/windows/app/package-client.cmd:51-74`
- Modify: `deploy/windows/inno/make-client-installer.cmd:1-80`
- Modify: `deploy/windows/test/selfcheck.cmd:105-115,178-214`

**Interfaces:**
- Consumes: `clear-client-id.ps1 -ConfigFile <package>\conf\client.yaml`.
- Produces: default and on-site package configurations with an empty identity before Inno compilation.

- [ ] **Step 1: Add failing integration assertions**

Update `selfcheck.cmd` to require and run `test-clear-client-id.ps1`, require `clear-client-id.ps1`, verify `make-client-installer.cmd` invokes it, and verify `package-client.cmd` copies it. After creating `selfcheck-client-package`, require the copied helper and assert that the packaged YAML contains exactly one empty top-level `client_id`.

- [ ] **Step 2: Run the self-check to verify the new assertions fail**

Run:

```powershell
cmd /c deploy\windows\test\selfcheck.cmd
```

Expected: nonzero exit because neither packaging script is wired to the sanitizer yet.

- [ ] **Step 3: Wire default package assembly**

In `package-client.cmd`, copy `clear-client-id.ps1` into the client package root. Remove the existing inline `client_id` regex from the credential-cleaning one-liner and invoke the copied helper against `%PACKAGE_DIR%\conf\client.yaml`, preserving existing password and sealed-credential cleaning.

- [ ] **Step 4: Wire on-site installer regeneration**

In `make-client-installer.cmd`, require `%PACKAGE_DIR%clear-client-id.ps1` and invoke it against `%PACKAGE_DIR%conf\client.yaml` before resolving or running Inno Setup. Propagate a nonzero sanitizer exit code and do not add an alternate Inno configuration source.

- [ ] **Step 5: Run focused and packaging verification**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File deploy/windows/test/test-clear-client-id.ps1
cmd /c deploy\windows\test\selfcheck.cmd
```

Expected: both commands exit `0`.

### Task 3: Documentation and End-to-End Repackage Check

**Files:**
- Modify: `docs/deployment/client-user-zh.md:135,182-206`
- Modify: `docs/deployment/windows-deployment-zh.md:95,193-249`

**Interfaces:**
- Consumes: the completed package scripts from Task 2.
- Produces: operator documentation stating that on-site repackaging clears and saves `client_id` before compilation.

- [ ] **Step 1: Update operator documentation**

State that both default packaging and `dist\LSYL Tunnel Client\make-installer.cmd` clear `client_id`; on-site repackaging updates the kit's original `conf\client.yaml`; runtime uses the target machine hostname after installation.

- [ ] **Step 2: Build a temporary on-site package with a seeded identity**

Copy the generated client package to `build\tmp\client-id-repackage-check`, set its top-level identity to `site-machine`, and run its `make-installer.cmd` with output directed to a temporary installer directory.

- [ ] **Step 3: Verify source cleanup and installer creation**

Assert that the temporary package's original `conf\client.yaml` now contains exactly one empty top-level identity, still contains its server and forwarding fields, and that `LSYL-Tunnel-Client-Setup.exe` exists.

- [ ] **Step 4: Run final repository checks**

Run:

```powershell
git diff --check
go test ./src/...
cmd /c deploy\windows\test\selfcheck.cmd
git status --short
```

Expected: all test commands exit `0`; status contains only the intended implementation files plus the pre-existing `src/server/front/index.html` modification.
