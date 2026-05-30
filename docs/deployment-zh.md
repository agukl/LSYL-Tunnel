# 部署与安装指南

本文集中说明 LSYL Tunnel 的 Windows 分发、安装、卸载和安装器生成流程。开发调试请先看 [内部开发快速开始](internal/quickstart-zh.md)，服务端业务配置请看 [服务端管理员指南](server-admin-zh.md)。

## 1. 目录边界

源码和配置模板：

```text
src/client/             客户端源码、GUI、默认配置和客户端信任证书输入目录
src/server/             服务端源码、Web 运维台、默认配置和服务端图标
src/internal/           共享协议、TLS、密码哈希、凭据密封和 Windows 服务封装
src/cmd/                管理工具源码
```

Windows 部署脚本：

```text
deploy/windows/build/   构建二进制
deploy/windows/run/     开发调试启动
deploy/windows/service/ 服务端 Windows 服务手动注册入口
deploy/windows/app/     客户端/服务端分发目录生成脚本
deploy/windows/inno/    Inno Setup 安装器配置和可选内置编译器打包
deploy/windows/sign/    代码签名初始化和签名脚本
deploy/windows/test/    自检脚本
```

运行产物和安装产物：

```text
build/bin/client/
build/bin/server/
dist/
tmp/
logs/
certs/
data/
```

这些目录可以重新生成，不应作为核心源码维护。

## 2. 服务端证书边界

服务端安装时，如果目标安装目录没有：

```text
certs/server.crt
certs/server.key
```

安装脚本会自动生成自签 TLS 服务端身份。管理员只把 `server.crt` 发给客户端管理员；`server.key` 必须留在服务端。

安装器会在向导中要求填写客户端真实连接服务端所用的域名或 IP。开发目录中也可以手工生成：

```powershell
cmd /c deploy\windows\cert\init-server.cmd "vpn.example.com,203.0.113.10"
```

## 3. 客户端分发

客户端只长期维护源码中的默认配置和信任证书：

```text
src/client/conf/client.yaml
src/client/cert/server.crt
```

打包脚本会把它们复制到 `dist/LSYL Tunnel Client/conf` 和 `dist/LSYL Tunnel Client/cert`；安装器再复制到安装目录。`dist` 和安装目录用于交付或运行，不作为第二套开发配置维护。客户端打包前，先把服务端公开证书放到：

```text
src/client/cert/server.crt
```

生成客户端分发目录：

```powershell
cmd /c deploy\windows\app\package-client.cmd
```

输出目录：

```text
dist/LSYL Tunnel Client
```

`package-client.cmd` 会删除并重建这个客户端分发目录，所以后续完整构建会覆盖你手工改过的 `dist/LSYL Tunnel Client`。

打包时会清理客户端运行态字段：

- `password` 会写成空值。
- `saved_credential` 会写成空对象，不会把本机登录密文带进安装包。
- `client_id` 会写成空值，客户端首次登录保存配置时会自动使用本机名。

客户端安装包会安装普通 GUI、Win7 轻量客户端、配置、图标和 `cert/server.crt`。默认运行方式是 GUI 进程模式 + 托盘值守；轻量客户端需要导入 `.lsylprofile` 后手动连接/断开。两者都不注册客户端 Windows 服务。

Win7 轻量客户端由 `deploy\windows\build-win7-lite.cmd` 使用 Go 1.20.x 构建为 32 位 exe。构建机可通过 `GO120EXE` 指定 Go 1.20，也可以让脚本下载安装到 `build\_toolchains`；最终用户机器不需要 Go 或其他运行时。

移动端 APK 和 Win7 轻量客户端会额外一起打包到：

```text
dist/LSYL Tunnel Lightweight Clients
```

其中 `android/lsyl-tunnel-mobile.apk` 是 Android 移动端，`windows-win7/lsyl-tunnel-client-lite.exe` 是 Win7 友好的 32 位轻量客户端，`profiles/` 可用于现场放置按用户导出的 `.lsylprofile`。

客户端安装、升级和卸载前，安装器会先调用客户端内置 `/quit` 命令请求正在运行的 GUI 温和退出。退出失败时安装器会停止并提示用户从托盘手动退出，不使用 `taskkill /F` 强制结束进程。

如果 `/quit` 请求失败，安装器会清理本次提取的临时 helper 后退出，不修改安装目录。如果安装在备份后中断，安装器会尝试用临时回滚备份恢复原 `conf/client.yaml` 和 `cert/server.crt`，并清理临时回滚目录。

客户端安装或升级时会覆盖安装目录中的：

```text
conf/client.yaml
cert/server.crt
```

覆盖前会备份为：

```text
conf/client.yaml.bak
cert/server.crt.bak
```

这样新安装包中的服务端地址、端口映射和证书会真正生效，避免旧证书或旧 `saved_credential` 残留导致连接失败。

客户端安装和卸载统一由 Inno 安装器处理，不再随分发包提供手动安装/卸载脚本。卸载默认保留 `conf`、`cert`、`secrets`，并移除 `bin`、`assets`、`tmp` 等程序目录。

## 4. 服务端分发

生成服务端分发目录：

```powershell
cmd /c deploy\windows\app\package-server.cmd
```

输出目录：

```text
dist/LSYL Tunnel Server
```

服务端安装器会：

- 复制服务端 GUI、服务程序、密码工具和证书工具。
- 如果缺少服务端 TLS 文件，生成自签 `certs/server.crt` 和 `certs/server.key`。
- 注册 `LSYLTunnelServer` Windows 服务，默认启动类型为手动；安装向导中明确勾选后才设置为开机自启动。
- 创建服务端 GUI 快捷方式。
- 保留已存在的 `conf/server.yaml`。

默认安装路径：

```text
C:\Program Files\LSYL Tunnel Server
```

默认保留目录：

```text
conf/
certs/
data/
logs/
```

服务端安装和卸载统一由 Inno 安装器处理，不再随分发包提供手动安装/卸载脚本。默认卸载只移除程序、快捷方式和服务注册，保留 `conf`、`certs`、`data`、`logs`。

## 5. Inno Setup 安装器

日常发布推荐直接使用根目录入口：

```powershell
cmd /c release.cmd
```

这个命令会按顺序完成：

- 检查/同步客户端需要信任的服务端公开证书。
- 执行 `go test ./...`。
- 生成客户端和服务端分发目录。
- 编译 Inno 安装器。
- 对分发包内 exe 和安装器执行签名。
- 校验最终产物是否存在以及签名是否有效。

如果需要发布前重新生成服务端 TLS 公开证书，并同步到客户端打包输入目录：

```powershell
cmd /c release.cmd /hosts "vpn.example.com,203.0.113.10"
```

如果只想生成分发目录，不编译 Inno 安装器：

```powershell
cmd /c release.cmd /package-only
```

这个命令用于开发交付实施：它会生成自包含的 `dist` 目录，并清理旧的 `dist/installers`，避免把过期安装器一起交出去。开发机已安装 Inno Setup 6 时，会同时内置命令行编译器到：

```text
dist/tools/inno/
```

实施人员拿到 `dist` 后，不需要源码和 `deploy` 目录，通常也不需要单独安装 Inno Setup 6，直接运行：

```cmd
dist\make-installers.cmd
```

也可以只生成某一侧安装器：

```cmd
dist\LSYL Tunnel Client\make-installer.cmd
dist\LSYL Tunnel Server\make-installer.cmd
```

实施可以先按客户现场修改：

```text
dist/LSYL Tunnel Client/conf/client.yaml
dist/LSYL Tunnel Client/cert/server.crt
dist/LSYL Tunnel Server/conf/server.yaml
```

然后再生成安装器。生成结果写入：

```text
dist/installers/
```

如果只是临时定制客户端安装包，可以先生成客户端分发目录：

```powershell
cmd /c deploy\windows\app\package-client.cmd
```

然后修改：

```text
dist/LSYL Tunnel Client/conf/client.yaml
dist/LSYL Tunnel Client/cert/server.crt
```

最后只编译当前客户端分发包：

```powershell
cmd /c "dist\LSYL Tunnel Client\make-installer.cmd"
```

这个命令不会重新打包，也不会覆盖 `dist/LSYL Tunnel Client`，适合做一次性的客户定制安装包。

输出目录：

```text
dist/installers/
```

当前产物：

```text
LSYL-Tunnel-Client-Setup.exe
LSYL-Tunnel-Server-Setup.exe
```

安装器生成脚本查找 `ISCC.exe` 的顺序是：

```text
dist\tools\inno\ISCC.exe
```

然后才会尝试实施机器上的 Inno Setup 安装路径：

```text
%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe
%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe
%ProgramFiles%\Inno Setup 6\ISCC.exe
```

也可以手动指定：

```cmd
set "INNO_SETUP_ISCC=C:\Users\<user>\AppData\Local\Programs\Inno Setup 6\ISCC.exe"
```

`dist\tools\inno\license.txt` 会保留 Inno Setup 许可和版权说明。内置 Inno 只用于编译本项目安装器，不修改 Inno Setup 本身。

服务端 Inno 安装器会在向导中要求填写证书主机名/IP，并提供“设置 LSYL Tunnel Server 服务为开机自启动”确认项；安装器使用内置逻辑完成证书生成和 `LSYLTunnelServer` 服务注册，不再把服务端安装/卸载脚本打进安装器。

为了降低误报并减少部署入口，客户端和服务端分发包都不再保留手动安装/卸载脚本；安装和卸载统一由 Inno 安装器处理。

## 6. 自检

完整自检：

```powershell
cmd /c deploy\windows\test\selfcheck.cmd
```

自检会覆盖：

- Go 单元测试。
- 客户端/服务端构建。
- 密码哈希工具和证书工具。
- Windows 脚本目录结构。
- 客户端/服务端分发目录结构。
- Inno 配置和图标资源是否存在。

自检不会主动安装或启动 Windows 服务，因为这些动作需要 UAC 授权。

## 7. 签名发布

安装器未签名时容易被杀软或 Windows SmartScreen 识别为未知风险。正式发布建议配置代码签名证书后再运行安装器构建脚本。

开发或内部测试可以使用项目提供的本机自签代码签名入口：

```cmd
cmd /c release.cmd /local-sign
```

`/local-sign` 会创建或复用当前用户证书仓库里的 `CN=LSYL Tunnel Local Code Signing`，然后继续完整发布。自签证书只适合开发和内部测试，不能替代正式 CA 代码签名证书。

PFX 证书方式：

```cmd
set "LSYL_SIGN_CERT_PFX=D:\secure\codesign.pfx"
set "LSYL_SIGN_CERT_PASSWORD=证书密码"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
cmd /c release.cmd
```

证书仓库指纹方式：

```cmd
set "LSYL_SIGN_CERT_SHA1=证书SHA1指纹"
cmd /c release.cmd
```

构建脚本会先签分发包内的 exe，再编译安装器，最后签安装器。没有配置签名证书时会跳过签名，不影响开发构建。

详细说明见 [签名发布指南](internal/release-signing-zh.md)。

