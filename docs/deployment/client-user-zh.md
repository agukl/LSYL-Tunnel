# 客户端用户指南

客户端安装、卸载、分发包和 Inno Setup 的完整流程见 [Windows 部署与安装](windows-deployment-zh.md)。本文聚焦普通用户使用和客户端配置边界。

## 1. 普通用户需要什么

普通用户只需要三项信息：

- 服务端地址，例如 `vpn.example.com:3443`。
- 用户名。
- 密码。

管理员需要提前把服务端 TLS 信任文件和端口映射放进客户端配置或安装包里。`server.crt` 只是用来识别服务端，不是客户端身份证书。

如果服务端使用安装程序生成自签证书，管理员需要从服务端安装目录复制：

```text
certs\server.crt
```

然后放入客户端源码或分发目录的 `cert\server.crt`。不要复制服务端 `server.key`。

## 2. 启动客户端

开发目录中启动 GUI：

```powershell
cmd /c deploy\windows\run.cmd client-gui
```

构建客户端：

```powershell
cmd /c deploy\windows\build.cmd client
```

命令行调试：

```powershell
cmd /c deploy\windows\run.cmd client
```

## 3. 图形界面怎么用

1. 输入服务端地址、用户名、密码。
2. 点击“连接”。
3. 登录成功后，客户端在当前 GUI 进程内启动本地监听。
4. 关闭窗口不会断开连接，会隐藏到托盘。
5. 需要断开时点击“断开连接”。
6. 需要彻底退出时点击“退出客户端”或托盘菜单退出。

GUI 登录时会先做一次轻量 `login` 探测，只校验账号密码和服务端身份，不访问具体业务目标端口。

客户端 GUI 默认不注册、不启动 Windows 服务，也不会额外启动 `lsyl-tunnel-client.exe`。

## 4. Win7 轻量客户端

安装包同时提供：

```text
bin\lsyl-tunnel-client-lite.exe
```

这个程序专门给老 Windows 桌面使用：不依赖 WebView，不进托盘，不做后台值守。双击启动后只有“导入配置”“连接”“断开”三个核心动作；关闭窗口会直接断开连接并退出。

发布包中的轻量客户端使用 Go 1.20.x 按 `windows/386` 构建，并关闭 CGO。目标电脑不需要安装 Go、WebView、.NET、Java 或其他运行环境。

轻量客户端不让用户输入密码，它导入管理员下发的 `.lsylprofile`。这个文件可以由普通 GUI 在登录成功后生成：右键窗口左上角 logo，确认后会导出到系统“下载”目录，文件名为 `用户名_到期日期.lsylprofile`。

导入成功后，轻量客户端会把配置保存到当前用户目录：

```text
%APPDATA%\LSYL Tunnel Lite\conf\client.yaml
%APPDATA%\LSYL Tunnel Lite\cert\server.crt
```

如果 `.lsylprofile` 中的登录凭据过期，需要在普通 GUI 重新登录成功后再次导出，再导入轻量客户端。

## 5. 管理员预置配置

默认配置只维护源码中的这一份：

```text
src\client\conf\client.yaml
```

打包脚本会把它复制到 `dist\LSYL Tunnel Client\conf\client.yaml`，安装器再复制到安装目录。`dist` 中的配置只用于现场一次性定制安装包，不建议作为长期维护入口。

关键字段示例：

```yaml
server_addr: 127.0.0.1:3443
username: alice
password: ""
client_id: demo-client

tls:
  ca_cert_file: ../cert/server.crt
  server_name: localhost

forwards:
  - name: rdp
    direction: client_to_server
    listen_addr: 127.0.0.1:3388
    server_target: 127.0.0.1:3389
```

反向转发客户端通过 `listen_addr` 选择服务端已预留端口，并用 `server_target` 配置客户端本地目标：

```yaml
forwards:
  - name: mysql
    direction: server_to_client
    listen_addr: 127.0.0.1:13306
    server_target: 127.0.0.1:3307
```

`name` 只用于显示和日志，不参与服务端授权，也不需要与服务端规则名一致。客户端会在反向控制连接和数据流请求中提交 `listen_addr`，服务端按该地址与登录账号精确校验端口归属，但不会把端口信息返回客户端或改写客户端配置。`server_target` 由客户端本地使用，同时随反向请求上报供服务端记录和诊断；服务端不会连接或下发这个目标。同一账号可以配置多条反向规则，每条规则都必须填写服务端回环 `listen_addr` 和客户端回环 `server_target`。反向规则不支持 `listen_port`，出现该字段时会明确指出对应规则并保持文件原样。配置错误会阻止本次连接，不会静默修改或跳过该规则。

Windows 客户端需要让业务程序继续访问原地址时，可改用虚拟 IP 接管：

```yaml
forwards:
  - direction: virtual
    listen_addr: ":22"
    server_target: 10.20.30.40:22
```

此时客户端从当前 `server.crt` 自动选择虚拟 IPv4，业务程序继续访问解析后的 `IPv4:22`。客户端优先采用 `server_addr` 中已被证书授权的 IPv4；否则证书必须只有一个可用的非本地 IPv4 SAN。证书有多个候选且 `server_addr` 不能唯一确定时，改为显式填写，例如 `listen_addr: "192.0.2.22:22"`。虚拟入口不支持域名或 IPv6，也不会接受证书以外的虚拟 IP。`server_target` 继续表示服务端侧目标，两边端口可以不同。名称是可选项。每次点击连接时，客户端先完成登录认证，再按虚拟规则请求管理员授权启动本次会话的端点过滤。取消管理员授权只会使对应虚拟转发规则不可用，客户端仍保持连接，其他已授权规则不受影响；断开后重新连接会再次触发授权。内部随机端口只绑定 `127.0.0.1`，不需要也不能写入配置。

接管范围精确到 `IP:端口`：同一证书 IP 的其他端口不受影响，客户端连接服务端所用的 `server_addr` 端口禁止配置为虚拟业务端口。断开连接或退出客户端时，WinDivert 过滤句柄随提权子进程关闭；异常退出也不会留下网卡地址或路由。该模式只由标准 64 位 Windows 客户端提供，Win7 Lite 和 Android 不支持。

GUI 只会改写 `server_addr`、`username` 和登录凭据。登录成功后会清空 `password`，并写入 `saved_credential`，不会改写端口映射、TLS 信任文件等高级配置。

用于分发的安装包不会携带本机 `saved_credential`。默认打包会自动清理登录密文，并把 `client_id` 留空；现场从 `dist\LSYL Tunnel Client` 重新生成安装器时，也会先把该目录原始 `conf\client.yaml` 中的 `client_id` 清空并保存。客户端首次成功登录保存配置时会使用目标机器的本机名作为 `client_id`。

## 6. 本地密码保存

GUI 登录成功后，客户端会把本次输入的密码转换成 `saved_credential` 短期密文凭据，并清空 `password` 明文字段。

下次连接时，只要密封凭据未过期，客户端可以继续登录；过期后界面会提示用户重新输入密码。

密封凭据不是明文密码，但过期前仍可能被重放使用。如果客户端配置文件泄露，应联系管理员修改账号密码，并轮换服务端 `credential_seal` 密钥。

管理员也可以使用环境变量或密码文件预置密码：

```yaml
password: ""
password_env: LSYL_TUNNEL_PASSWORD
```

或：

```yaml
password: ""
password_file: ../../secrets/client-password.txt
```

## 7. 客户端分发包

生成完整客户端/服务端安装器，推荐使用根目录发布入口：

```powershell
cmd /c release.cmd
```

如需保留内部临时构建目录用于排查：

```powershell
cmd /c release.cmd /keep-work
```

默认执行 `release.cmd` 后会保留客户端分发目录：

```text
dist\LSYL Tunnel Client
```

实施人员只拿到 `dist` 也可以按需生成客户端安装器：

```cmd
dist\LSYL Tunnel Client\make-installer.cmd
```

如果执行 `release.cmd /bundle-inno`，发布脚本会把安装器编译器内置到 `dist\tools\inno`，实施机器通常不需要再安装 Inno。

也可以从 `dist` 根目录重新生成客户端安装器：

```cmd
dist\make-installers.cmd
```

服务端和 Android 安装产物在完整发布组装 `dist` 时生成，最终以 `dist\installers` 中的文件为准。

生成安装器后，把 `dist\installers\LSYL-Tunnel-Client-Setup.exe` 复制到用户机器并以管理员身份运行。安装器会复制 GUI、配置、图标和 `cert\server.crt`，客户端不提供 Windows 服务模式。

安装或升级会覆盖安装目录中的 `conf\client.yaml` 和 `cert\server.crt`，并先按旧配置的服务端地址归档为 `conf\client.<旧服务地址>.yaml`、`cert\server.<旧服务地址>.crt`。如果同名归档已经存在，会被最近一次覆盖安装的旧配置覆盖。重装后原名文件立即使用新配置，用户也可以在客户端登录页切回已归档的旧配置。

安装器会先请求正在运行的客户端温和退出；如果退出失败，会清理本次临时文件并提示用户从托盘退出后重试。安装中断时会尽量恢复原配置和原服务端证书，不默认强杀客户端进程。

生成前仍需要确保 `src\client\cert\server.crt` 已经放好。`release.cmd` 会从 `src\client\conf` 和 `src\client\cert` 重新生成 `dist\LSYL Tunnel Client`，并把 `make-installer.cmd` 与 Inno 模板放进分发包。

如果已经手工改好了 `dist\LSYL Tunnel Client` 中的配置和证书，并且不希望客户端分发目录被源码配置输入覆盖，可以改用：

```powershell
cmd /c "dist\LSYL Tunnel Client\make-installer.cmd"
```

该命令会保留现场填写的服务端地址、账号和转发规则，但会在编译前直接清空并保存 `dist\LSYL Tunnel Client\conf\client.yaml` 中的 `client_id`，确保分发目录和生成的安装包都不携带实施机器标识。

Inno 安装器内置卸载逻辑，不会在安装目录落地客户端卸载 `.ps1/.cmd`。需要卸载时使用 Windows “应用和功能”或开始菜单中的卸载入口。

## 8. 常见错误

`用户名或密码不正确`：账号或密码填错。

`登录失败次数过多，请稍后再试`：短时间失败过多，服务端临时封禁当前来源 IP。

`连接不上服务端`：服务端地址、端口、网络或服务端进程异常。

`服务端身份校验失败`：服务端 TLS 文件或 `server_name` 不匹配。

`账号没有访问该目标的权限`：服务端没有给当前用户放通该端口。

`服务端无法访问目标服务`：服务端到目标业务端口不通，通常需要管理员检查目标服务或防火墙。

