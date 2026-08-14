# Distribution License Packaging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure every LSYL Tunnel release carries the project root `LICENSE`, including the final distribution directory, the reusable Windows client kit, and installed Windows client/server applications.

**Architecture:** Keep the repository root `LICENSE` as the only maintained source. Windows package builders copy it into their temporary package roots, Inno Setup installs that package copy, and `release.cmd` copies it into the final staged distribution. Existing self-check and release verification paths compare copied files byte-for-byte with the root source.

**Tech Stack:** Windows batch scripts, Inno Setup 6, PowerShell-based checks invoked by existing batch workflows.

## Global Constraints

- Do not create or maintain a second tracked copy of the license text.
- Do not enable an installer license-acceptance page.
- Do not change WinDivert or Inno Setup third-party license handling.
- Android APK remains unchanged; `dist\LICENSE` accompanies it in the release directory.
- Copied license files must be byte-for-byte identical to the repository root `LICENSE`.

---

### Task 1: Lock Windows Package License Requirements With Failing Checks

**Files:**
- Modify: `deploy/windows/test/selfcheck.cmd`

**Interfaces:**
- Consumes: project root `LICENSE`, output directories produced by `package-client.cmd` and `package-server.cmd`.
- Produces: package assertions that fail if `LICENSE` is missing, differs from the root file, or is absent from either Inno `[Files]` section.

- [ ] **Step 1: Add package and installer assertions**

Add checks equivalent to:

```batch
if not exist LICENSE (echo [ERROR] Missing: project LICENSE & exit /b 1)
findstr /c:"{#SourceRoot}\LICENSE" deploy\windows\inno\package-client.iss >nul || (echo [ERROR] Client installer is missing project LICENSE & exit /b 1)
findstr /c:"{#SourceRoot}\LICENSE" deploy\windows\inno\package-server.iss >nul || (echo [ERROR] Server installer is missing project LICENSE & exit /b 1)
if not exist "%SELF_TMP%\selfcheck-client-package\LICENSE" (echo [ERROR] Missing: client package project LICENSE & exit /b 1)
fc /b "LICENSE" "%SELF_TMP%\selfcheck-client-package\LICENSE" >nul || (echo [ERROR] Client package project LICENSE differs from source & exit /b 1)
if not exist "%SELF_TMP%\selfcheck-server-package\LICENSE" (echo [ERROR] Missing: server package project LICENSE & exit /b 1)
fc /b "LICENSE" "%SELF_TMP%\selfcheck-server-package\LICENSE" >nul || (echo [ERROR] Server package project LICENSE differs from source & exit /b 1)
```

- [ ] **Step 2: Run the self-check and verify the new assertion fails**

Run:

```batch
cmd /c deploy\windows\test\selfcheck.cmd
```

Expected: FAIL after creating the client package because `selfcheck-client-package\LICENSE` does not yet exist.

- [ ] **Step 3: Commit the failing checks**

```batch
git add deploy/windows/test/selfcheck.cmd
git commit -m "test: require project license in Windows packages"
```

---

### Task 2: Carry the License Through Windows Packages and Installers

**Files:**
- Modify: `deploy/windows/app/package-client.cmd`
- Modify: `deploy/windows/app/package-server.cmd`
- Modify: `deploy/windows/inno/package-client.iss`
- Modify: `deploy/windows/inno/package-server.iss`

**Interfaces:**
- Consumes: repository root `LICENSE`.
- Produces: `LICENSE` at each temporary package root and `{app}\LICENSE` after either Windows installer runs.

- [ ] **Step 1: Copy the root license into each temporary package**

Add the following command after each package directory is created:

```batch
copy /y ".\LICENSE" "%PACKAGE_DIR%\LICENSE" >nul || exit /b 1
```

- [ ] **Step 2: Add the package copy to both installers**

Add this entry to each Inno `[Files]` section without adding `LicenseFile` under `[Setup]`:

```iss
Source: "{#SourceRoot}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
```

- [ ] **Step 3: Run the self-check and verify package assertions pass**

Run:

```batch
cmd /c deploy\windows\test\selfcheck.cmd
```

Expected: `selfcheck PASS`, including successful client and server package generation and client installer regeneration.

- [ ] **Step 4: Commit Windows package support**

```batch
git add deploy/windows/app/package-client.cmd deploy/windows/app/package-server.cmd deploy/windows/inno/package-client.iss deploy/windows/inno/package-server.iss
git commit -m "build: include project license in Windows packages"
```

---

### Task 3: Carry and Verify the License in the Final Distribution

**Files:**
- Modify: `release.cmd`

**Interfaces:**
- Consumes: repository root `LICENSE` and the existing `STAGE_DIR`/`VERIFY_DIR` release paths.
- Produces: `dist\LICENSE`; `release.cmd /verify-only` rejects a missing or modified distribution license.

- [ ] **Step 1: Add final distribution validation before the copy implementation**

At the start of `:verify_dist`, add:

```batch
if not exist "%VERIFY_DIR%\LICENSE" (
  echo [ERROR] Missing project license: %VERIFY_DIR%\LICENSE
  exit /b 1
)
fc /b "%WORKSPACE%\LICENSE" "%VERIFY_DIR%\LICENSE" >nul
if errorlevel 1 (
  echo [ERROR] Distribution project license differs from source: %VERIFY_DIR%\LICENSE
  exit /b 1
)
```

- [ ] **Step 2: Verify the existing distribution fails the new requirement**

Run:

```batch
cmd /c release.cmd /verify-only
```

Expected: FAIL with `Missing project license` because the current `dist` predates this feature.

- [ ] **Step 3: Copy the root license into the staged release**

After creating `STAGE_DIR`, add:

```batch
copy /y "%WORKSPACE%\LICENSE" "%STAGE_DIR%\LICENSE" >nul || goto fail
```

- [ ] **Step 4: Regenerate and verify the distribution**

Run:

```batch
cmd /c release.cmd
cmd /c release.cmd /verify-only
fc /b LICENSE dist\LICENSE
fc /b LICENSE "dist\LSYL Tunnel Client\LICENSE"
```

Expected: release generation succeeds; verify-only succeeds; both `fc` commands report no differences.

- [ ] **Step 5: Run final checks**

```batch
cmd /c deploy\windows\test\selfcheck.cmd
git diff --check
git status --short
```

Expected: `selfcheck PASS`, no whitespace errors, and only `release.cmd` plus this plan file remain uncommitted.

- [ ] **Step 6: Commit final distribution support**

```batch
git add release.cmd docs/superpowers/plans/2026-08-14-distribution-license.md
git commit -m "build: include project license in distributions"
```
