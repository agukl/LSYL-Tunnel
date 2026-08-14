# Client Repackage Identity Sanitization Design

## Goal

Ensure every Windows client installer is built without a device identity. This applies both to the default release build and to an installer regenerated from `dist\LSYL Tunnel Client` after on-site configuration edits. The on-site source `conf\client.yaml` must also be cleared before compilation so the distribution kit does not retain a device identity.

The installed client sees an empty `client_id` and uses the target machine hostname as its runtime identity. On-site edits to server addresses, accounts, certificates, and forwarding rules remain unchanged.

## Current Gap

`package-client.cmd` clears `client_id` when the release package is first assembled. The copied distribution kit therefore starts with an empty identity. However, `make-installer.cmd` currently compiles the kit's current `conf\client.yaml` directly. If an operator adds a `client_id`, the regenerated installer carries it.

## Design

Add a small PowerShell helper that reads the on-site `conf\client.yaml` and replaces only the top-level `client_id` value with an empty string. It writes a sibling temporary file first and then replaces the source file, preventing a partial write from damaging the configuration.

`make-client-installer.cmd` will:

1. Clear `client_id` in the package's original `conf\client.yaml`.
2. Stop before compilation if identity sanitization fails.
3. Compile the installer from the now identity-free package configuration.

The same helper will be used when the default package is assembled and will be copied into `dist\LSYL Tunnel Client` for on-site repackaging. `package-client.iss` continues to package the existing package-relative configuration, so no alternate configuration source is introduced.

## Failure Handling

- A missing or unreadable configuration stops the build before Inno Setup runs.
- A configuration without exactly one top-level `client_id` field stops the build instead of producing an ambiguous package.
- Failure to produce an identity-free temporary configuration stops the build.
- Compiler failure preserves its existing nonzero exit status.
- A failed atomic replacement leaves the original configuration unchanged and removes the sibling temporary file.
- Once sanitization succeeds, the original on-site configuration remains identity-free even if compiler execution later fails.

## Verification

Extend the Windows packaging self-check to use a temporary client configuration containing a nonempty `client_id`, then verify:

- the original test configuration is rewritten with `client_id: ""`;
- other configuration fields remain present;
- a missing or duplicate top-level `client_id` is rejected without corrupting the source;
- the installer build script invokes identity sanitization before Inno Setup;
- the default source packaging path still clears runtime identity and credential data.

No client protocol, server behavior, installed configuration schema, or runtime identity logic changes are included.
