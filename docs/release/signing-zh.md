# 签名发布指南

LSYL Tunnel 的安装器会写入程序文件、配置和证书；服务端安装器还会生成服务端 TLS 身份并注册 Windows 服务。这些动作都属于安全软件重点关注的行为。未签名、签名信誉不足、服务信息不清晰或安装行为不透明时，杀软或 Windows SmartScreen 容易把安装器或服务程序识别为“未知发布者”“可疑服务”甚至“木马”。

代码签名的目标：

- 让用户看到明确发布者。
- 降低杀软和 SmartScreen 对未知安装包的拦截概率。
- 证明安装器和落地的 exe 发布后未被篡改。
- 为后续版本积累发布信誉。

代码签名不能保证：

- 立即消除所有误报。
- 绕过服务注册、证书生成、高权限安装行为带来的风控。
- 替代供应链安全审计。
- 把本机自签证书变成正式发布信誉。

正规发布的目标不是绕过安全软件，而是让安全软件、管理员和用户都能清楚判断：这个程序是谁发布的、安装了什么、注册了什么服务、监听了什么端口、如何卸载、日志在哪里。

## 1. 当前发布优化

当前构建流程已经做了两件事：

- 构建安装器前先签分发包内的 exe。
- 构建安装器后再签最终安装器。
- 根目录 `release.cmd` 会在发布最后验证目标文件是否存在、签名是否有效。
- 根目录 `release.cmd` 会输出 `dist\release-manifest.txt` 和 `dist\release-manifest.json`，记录构建时间、SHA256、签名状态、版本资源和 Windows 服务说明。

客户端 Inno 安装器不再携带客户端卸载 `.ps1/.cmd`，卸载时由 Inno 原生逻辑保留 `conf`、`cert`、`secrets`，并移除程序文件。这能减少客户端安装器里的脚本特征。

服务端 Inno 安装器也不再携带服务端安装/卸载 `.ps1/.cmd`，证书生成使用包内 `lsyl-tunnel-cert.exe`，服务注册/删除由安装器调用 `sc.exe` 完成。分发包只保留生成安装器所需脚本，避免形成第二套部署入口。

## 2. 签名对象

发布时建议签两层：

```text
dist/LSYL Tunnel Client/bin/lsyl-tunnel-client-gui.exe
dist/LSYL Tunnel Client/bin/lsyl-tunnel-client-lite.exe
dist/LSYL Tunnel Server/bin/*.exe
dist/installers/LSYL-Tunnel-Client-Setup.exe
dist/installers/LSYL-Tunnel-Server-Setup.exe
```

## 3. 准备签名工具

优先使用 Windows SDK 中的 `signtool.exe`。脚本会按下面顺序查找：

- `LSYL_SIGNTOOL` 环境变量。
- `PATH` 中的 `signtool.exe`。
- Windows SDK 默认安装目录。

找不到时，如果配置的是证书指纹，脚本会自动降级使用 PowerShell `Set-AuthenticodeSignature`。也可以手动设置：

```cmd
set "LSYL_SIGNTOOL=C:\Program Files (x86)\Windows Kits\10\bin\x64\signtool.exe"
```

## 4. 开发用本地自签代码签名

本项目提供开发用自签代码签名证书初始化脚本。它会在当前用户证书仓库创建 `CN=LSYL Tunnel Local Code Signing`，并把公开证书导入当前用户的 TrustedPublisher/Root，随后把指纹写入 `certs\codesign-thumbprint.txt`。

```cmd
cmd /c release.cmd /local-sign
```

`/local-sign` 会先调用 `deploy\windows\sign\init-selfsigned-codesign.cmd`，然后执行完整发布。`sign-release.cmd` 会自动读取 `certs\codesign-thumbprint.txt`。如果机器没有 Windows SDK 的 `signtool.exe`，会自动使用 PowerShell Authenticode 签名。

如需重新生成本机自签代码签名证书：

```cmd
cmd /c deploy\windows\sign\init-selfsigned-codesign.cmd /force
```

注意：自签代码签名只适合开发、内部分发或本机信任测试。它能让安装包具备 Authenticode 签名，但不会替代正式 CA 代码签名证书，也不能建立公开发布信誉。服务程序被当成木马时，自签证书通常只能帮助本机验证签名链路，不能作为正式申诉或降低误报的核心依据。

## 5. 使用 PFX 签名

推荐把 PFX 放在项目目录外，不要提交到仓库。

```cmd
set "LSYL_SIGN_CERT_PFX=D:\secure\codesign.pfx"
set "LSYL_SIGN_CERT_PASSWORD=证书密码"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
cmd /c release.cmd
```

如果 PFX 没有密码，可以不设置 `LSYL_SIGN_CERT_PASSWORD`。

## 6. 使用证书指纹签名

如果证书已导入 Windows 证书仓库，可以使用 SHA1 指纹：

```cmd
set "LSYL_SIGN_CERT_SHA1=证书SHA1指纹"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
cmd /c release.cmd
```

## 7. 临时客户端安装包签名

如果已经手工改好了客户端分发目录：

```text
dist/LSYL Tunnel Client/conf/client.yaml
dist/LSYL Tunnel Client/cert/server.crt
```

可以只编译并签名客户端安装器：

```cmd
cmd /c "dist\LSYL Tunnel Client\make-installer.cmd"
cmd /c deploy\windows\sign\sign-release.cmd client-installer
```

实施人员如果只拿到了 `dist`，可以先运行包内脚本生成安装器；签名则需要在有签名证书的发布机上执行。

## 8. 单独执行签名脚本

签全部可签产物：

```cmd
cmd /c deploy\windows\sign\sign-release.cmd all
```

只签包内 exe：

```cmd
cmd /c deploy\windows\sign\sign-release.cmd package
```

只签安装器：

```cmd
cmd /c deploy\windows\sign\sign-release.cmd installers
```

只签客户端：

```cmd
cmd /c deploy\windows\sign\sign-release.cmd client-package
cmd /c deploy\windows\sign\sign-release.cmd client-installer
```

只签服务端：

```cmd
cmd /c deploy\windows\sign\sign-release.cmd server-package
cmd /c deploy\windows\sign\sign-release.cmd server-installer
```

没有配置 `LSYL_SIGN_CERT_PFX`、`LSYL_SIGN_CERT_SHA1`，且不存在 `certs\codesign-thumbprint.txt` 时，脚本会提示跳过签名，不影响开发构建。

## 9. 验证签名

推荐用发布入口统一验证：

```cmd
cmd /c release.cmd /verify-only
```

也可以手工验证：

```cmd
signtool verify /pa /v dist\installers\LSYL-Tunnel-Client-Setup.exe
signtool verify /pa /v dist\installers\LSYL-Tunnel-Server-Setup.exe
```

也可以右键文件查看“数字签名”页。

## 10. 发布清单

发布完成后会生成：

```text
dist\release-manifest.txt
dist\release-manifest.json
```

清单包含：

- 构建时间。
- 文件路径、大小和 SHA256。
- Authenticode 签名状态和签名主体。
- 产品名、公司名、文件说明、文件版本。
- 服务端 Windows 服务名、显示名、启动类型和描述。

这些内容用于实施验收、客户交付记录和杀软误报申诉。正式发布时，如果清单里出现 `NotSigned`，说明该文件还没有有效签名。

## 11. 降低误报的后续建议

- 为所有 exe 增加一致的版本资源：产品名、公司名、版权、文件说明、版本号和图标。
- 服务端 Windows 服务使用固定服务名、显示名和描述，不随机命名，不伪装系统服务。
- 服务端服务默认手动启动，除非安装向导明确告知管理员并由管理员选择开机自启动。
- 安装器不加壳、不混淆、不从临时目录释放后执行、不自删除、不隐藏日志。
- 安装器和服务程序都要能被管理员解释：安装目录、服务命令行、监听端口、证书位置、日志位置、卸载保留目录。
- 保持同一代码签名证书持续发布，积累信誉。
- 优先使用正式代码签名证书；EV 证书通常更有利于 SmartScreen 信誉。
- 发布前对安装器和落地 exe 都签名。
- 不把 PFX、私钥、密码写入脚本或仓库。
- 尽量减少安装包和分发包内的脚本行为；当前只保留生成安装器和签名所需入口。
- 对确认误报的样本，向对应杀软厂商提交误报申诉。

## 12. 误报申诉材料

如果服务程序或安装器被报毒，准备以下材料再向安全厂商提交误报申诉：

- 文件名、版本号、构建时间、SHA256。
- 数字签名验证结果和证书主体。
- 安装器来源、安装路径、落地文件列表。
- Windows 服务名 `LSYLTunnelServer`、显示名 `LSYL Tunnel Server`、启动类型、服务命令行。
- 服务用途说明：账号密码认证的 TLS 加密隧道和端口转发服务。
- 网络行为说明：服务端监听地址、监控地址、允许的转发端口。
- 日志和配置位置说明。
- 卸载行为说明：删除程序文件和服务注册，默认保留配置、证书、数据和日志。
- 安全软件名称、版本、病毒库版本、拦截图和触发步骤。
