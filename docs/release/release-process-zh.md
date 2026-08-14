# 发版手册

本文是 LSYL Tunnel 发版、签名、兼容判断和交付校验的长期入口。只记录每次发布都需要重复使用的规则，不记录阶段性待办和实现细节。

当前版本：

```text
APP_VERSION=2.2.0
WINDOWS_FILE_VERSION=2.2.0.0
PROTOCOL_VERSION=2
```

## 版本规则

产品版本使用三段号：

| 位置 | 什么时候升级 |
| --- | --- |
| 第一位 | 协议不兼容，或旧客户端不能按原协议连接新服务端。 |
| 第二位 | 客户端或服务端配置结构、字段含义、默认行为发生需要关注的变化。 |
| 第三位 | UI、安装器、文档、辅助工具等兼容改动。 |

Windows `FileVersion` 使用四段号，由产品版本派生，例如 `2.2.0 -> 2.2.0.0`。

每次升版至少检查这些位置：

- `README.md`
- `docs/README-zh.md`
- `docs/release/release-notes-zh.md`
- `deploy/windows/inno/package-client.iss`
- `deploy/windows/inno/package-server.iss`
- `mobile/android/app/build.gradle.kts`
- `mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/protocol/ProtocolClient.kt`
- `deploy/windows/test/selfcheck.cmd`
- 各 Windows `app.manifest` 和 `rsrc.syso`

## 兼容判断

发版前必须给出两个结论：

- 配置兼容：旧 `client.yaml`、旧 `server.yaml` 是否能被新程序直接使用。
- 协议兼容：旧客户端能否连接新服务端，新客户端能否连接旧服务端。

判断原则：

- 配置结构版本高于当前程序支持范围时，安装器或程序启动必须阻止并提示升级程序。
- `requires.min_client_version`、`requires.min_server_version` 高于当前程序版本时必须阻止。
- 修改 LSYL 握手字段、请求类型、错误码语义、转发语义或连接行为时，默认按协议升级处理。
- 只新增可选且旧程序能安全忽略的字段时，可以作为兼容扩展，但发布说明必须写明结论。

## 发布命令

常用完整发布：

```cmd
cmd /c release.cmd
```

常用变体：

```cmd
cmd /c release.cmd /hosts "vpn.example.com,203.0.113.10"
cmd /c release.cmd /local-sign
cmd /c release.cmd /bundle-inno
cmd /c release.cmd /no-client-kit
cmd /c release.cmd /verify-only
```

默认 `dist` 包含三端安装产物、发布清单和客户端现场重打包目录：

```text
dist/installers/LSYL-Tunnel-Client-Setup.exe
dist/installers/LSYL-Tunnel-Server-Setup.exe
dist/installers/LSYL-Tunnel-Android.apk
dist/LSYL Tunnel Client/
dist/make-installers.cmd
dist/release-manifest.txt
dist/release-manifest.json
```

服务端只交付安装器。客户端 kit 只用于现场调整客户端配置或证书后重新生成客户端安装器。

## 签名

正式发布应使用正式代码签名证书。开发或内部测试可以使用：

```cmd
cmd /c release.cmd /local-sign
```

正式证书推荐通过环境变量传入：

```cmd
set "LSYL_SIGN_CERT_PFX=D:\secure\codesign.pfx"
set "LSYL_SIGN_CERT_PASSWORD=证书密码"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
cmd /c release.cmd
```

也可以使用证书仓库指纹：

```cmd
set "LSYL_SIGN_CERT_SHA1=证书SHA1指纹"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
cmd /c release.cmd
```

发布脚本会签分发包内 exe 和最终 Windows 安装器。Android APK 由 Android 构建链处理，发布清单只记录文件哈希和状态。

## 发布校验

发布前至少执行：

```cmd
go test ./src/...
cmd /c deploy\windows\test\selfcheck.cmd
cmd /c release.cmd
cmd /c release.cmd /verify-only
powershell -NoProfile -ExecutionPolicy Bypass -File deploy/windows/test/check-text.ps1
```

正式交付前还需要人工确认：

- 客户端安装、连接、退出和卸载正常。
- 服务端安装、升级、服务启动、停止和卸载正常。
- 服务端覆盖安装不会覆盖已有 `conf`、`certs`、`data`、`logs`。
- 配置不兼容时安装器或程序启动会给出明确提示。
- 安装器、落地 exe、发布清单的版本和签名状态一致。

## 禁止事项

- 不加壳、不压缩可执行文件。
- 不随机化服务名、进程名、安装路径。
- 不隐藏 UAC、服务注册、证书生成、日志路径和卸载行为。
- 不把 PFX、私钥、密码或生产证书放入仓库或发布包。
- 不把阶段性待办、实现过程和调试记录写入长期文档；只保留结论、命令和交付边界。
