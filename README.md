# LSYL Tunnel

LSYL Tunnel is a username/password-authenticated TCP tunnel and port-forwarding project protected by TLS.

The identity boundary is the account password, not a client certificate. TLS protects tunnel traffic and lets the client verify the server. The client does not present a certificate or private key.

Chinese documentation: [docs/README-zh.md](docs/README-zh.md)
System flow overview (Chinese): [docs/system-flow-zh.md](docs/system-flow-zh.md)

## Layout

```text
src/client/             Client CLI, GUI, Win7 Lite UI, config, bundled trust cert, tunnel logic
src/server/             Server CLI, Web admin console, service wrapper, config, tunnel logic
src/internal/           Shared protocol, transport, password, credential sealing, service helpers
src/cmd/                Admin tools: lsyl-tunnel-passwd, lsyl-tunnel-cert
build/bin/              Local build outputs, split by client and server
deploy/windows/         Windows scripts grouped by build/run/cert/service/app/inno/test
docs/                   Chinese documentation
```

Generated/runtime directories such as `build/bin/client`, `build/bin/server`, `dist`, `tmp`, `logs`, root `certs`, and `data` are ignored by `.gitignore`.

## Main Windows Entry Points

Daily release build:

```powershell
cmd /c release.cmd
```

Common release variants:

```powershell
cmd /c release.cmd /hosts "vpn.example.com,203.0.113.10" /local-sign
cmd /c release.cmd /skip-test
cmd /c release.cmd /package-only
cmd /c release.cmd /verify-only
```

`release.cmd` is the recommended entry for packaging, installer build, signing, and output verification. Lower-level scripts under `deploy/windows` are kept for development, troubleshooting, and one-off custom packages.

```powershell
cmd /c deploy\windows\build.cmd all
cmd /c deploy\windows\cert\init-server.cmd "localhost,127.0.0.1"
cmd /c deploy\windows\run.cmd server-gui
cmd /c deploy\windows\run.cmd client-gui
cmd /c deploy\windows\run.cmd client-lite
cmd /c deploy\windows\test\selfcheck.cmd
```

Package and installer entry points:

```powershell
cmd /c release.cmd
cmd /c release.cmd /package-only
```

`/package-only` creates a self-contained `dist` handoff directory. When Inno Setup 6 exists on the build machine, its command-line compiler is copied to `dist\tools\inno` so implementation staff can build installers without installing Inno separately. Implementation staff can copy only `dist`, adjust package-local config/cert files, and run:

The mobile APK and Win7 Lite client are also copied together into `dist\LSYL Tunnel Lightweight Clients` for direct handoff.

```cmd
dist\make-installers.cmd
```

or build one side:

```cmd
dist\LSYL Tunnel Client\make-installer.cmd
dist\LSYL Tunnel Server\make-installer.cmd
```

Detailed Windows deployment notes: [docs/deployment-zh.md](docs/deployment-zh.md)

Internal archive and handoff checklist: [docs/internal/archive-checklist-zh.md](docs/internal/archive-checklist-zh.md)

Server service entry:

```powershell
cmd /c deploy\windows\service\server.cmd install
cmd /c deploy\windows\service\server.cmd start
cmd /c deploy\windows\service\server.cmd status
cmd /c deploy\windows\service\server.cmd stop
cmd /c deploy\windows\service\server.cmd uninstall
```

## Runtime Model

The client GUI runs the tunnel engine in-process by default. After login it opens local forwarding listeners inside `lsyl-tunnel-client-gui.exe`; closing the window keeps the tunnel guarded in the tray, and exiting the client stops it. The Windows client app package does not install or register a client service.

`lsyl-tunnel-client-lite.exe` is a Win7-oriented lightweight client. It imports a `.lsylprofile`, then offers only connect/disconnect and exits when the window closes.
The Lite binary is built separately with Go 1.20.x as `windows/386` and `CGO_ENABLED=0`, so the target PC does not need Go, WebView, .NET, Java, or any bundled runtime.

The server GUI is a local Web operations console. Frontend assets live in `src/server/front` and are embedded into `lsyl-tunnel-server-gui.exe`; the GUI exposes only a local `127.0.0.1` management API for status, configuration editing, and restarting the `LSYLTunnelServer` service.

The server installer generates a self-signed server TLS identity during install when `certs/server.crt` or `certs/server.key` is missing. Administrators distribute only `server.crt` to clients; `server.key` stays on the server. Server uninstall keeps `conf`, `certs`, `data`, and `logs` by default.

Forward entries support two directions:

- `client_to_server`: the client listens locally, and the server connects to a server-side target.
- `server_to_client`: the server creates a passive local listening port, and an authenticated client activates it; the server never dials the client.

## Security Model

- Client identity: username + password.
- Transport protection: TLS.
- Server verification: client trusts the configured server TLS certificate and checks `server_name`.
- Access boundary: allow rules are assigned directly to users through configured forward entries.
- Local password storage: after GUI login, the client saves a short-lived server-sealed credential instead of plaintext password.
- Recommended password storage on server: `pbkdf2-sha256:<iterations>:<salt-base64>:<hash-base64>`.

## Verification

```powershell
go test ./...
cmd /c deploy\windows\test\selfcheck.cmd
```

Windows service install/start/stop/uninstall requires Administrator/UAC.

