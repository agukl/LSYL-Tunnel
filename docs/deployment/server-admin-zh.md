# 服务端管理员指南

服务端安装、卸载、Inno Setup 和默认保留目录的完整流程见 [Windows 部署与安装](windows-deployment-zh.md)。本文聚焦服务端业务配置和运行管理。

## 1. 构建服务端

```powershell
cmd /c deploy\windows\build.cmd server
```

生成文件在：

```text
build\bin\server
```

## 2. 生成服务端 TLS 文件

正式安装包会在安装阶段自动生成自签服务端 TLS 文件。安装向导中应填写客户端实际连接的域名或 IP。

开发目录或手工重新生成时，也可以使用证书脚本。

本地测试：

```powershell
cmd /c deploy\windows\cert\init-server.cmd "localhost,127.0.0.1"
```

生产环境应把公网域名或固定 IP 写入 hosts：

```powershell
cmd /c deploy\windows\cert\init-server.cmd "vpn.example.com,203.0.113.10"
```

输出：

```text
certs\server.crt
certs\server.key
```

`server.crt` 可以发给客户端信任；`server.key` 只能留在服务端。

安装程序和脚本默认保留 `conf`、`certs`、`data`、`logs`。卸载服务端程序时会先停止 GUI/服务进程并删除 `LSYLTunnelServer` 服务注册，但不会默认删除这些运行数据。

## 3. 创建用户密码哈希

```powershell
build\bin\server\lsyl-tunnel-passwd.exe -password "强密码"
```

把输出写入 `src/server/conf/server.yaml`：

```yaml
auth:
  users:
    - username: alice
      password_hash: pbkdf2-sha256:...
```

`plain:` 只建议本地测试使用，生产环境请使用 PBKDF2 哈希。

服务端 GUI 的“用户认证”页不会展示密码或哈希。需要修改用户密码时，点击用户行后的“重置密码”，输入并确认新密码；GUI 会在本机生成新的 `password_hash`，保存配置后生效。

### 3.1 配置客户端本地密码密封

服务端可以配置 `credential_seal`，用于给客户端 GUI 下发短期公钥。客户端登录成功后会用这个公钥把密码密封保存到本地，避免长期保存明文密码。

```yaml
credential_seal:
  keys:
    - key_id: login-key-2026-08
      private_key_file: ../../certs/login-key-2026-08.key
      public_key_file: ../../certs/login-key-2026-08.pub
      expires_at: "2026-08-20T00:00:00+08:00"
      active: true
```

私钥文件只留在服务端；文件不存在时服务端会自动生成。服务端运行时会自行轮换过期登录密钥：如果当前 `active` 密钥过期，会先激活配置中已存在且未过期的下一把密钥；如果没有可用下一把，会自动生成新的 `login-key-YYYY-MM` 密钥、设为 `active`，并写回配置文件。旧密钥会保留但失活，旧客户端保存的 `saved_credential` 按原过期时间自然失效；用户在凭据过期后需要重新输入一次密码。

## 4. 配置转发端口和放通用户

服务端 GUI 中，“用户认证”页只维护账号本身；“转发端口”页根据已有用户分配端口放通关系。保存配置时，GUI 会把所选用户写入每条转发规则的 `allowed_users`。

用户配置只保存身份信息：

```yaml
auth:
  users:
    - username: alice
      password_hash: pbkdf2-sha256:...
```

转发方向说明：

- `client_to_server`：客户端监听入口，服务端连接目标。授权校验的是服务端侧目标地址。
- `server_to_client`：服务端创建被动入口，客户端连接目标。服务端入口必须由已认证客户端主动连接后激活，服务端不会主动连接客户端。简版实现里，授权校验的是服务端侧被动入口 `listen_addr`。

服务端通过 `forwards[]` 明确声明需要检查和保护的转发端口：

```yaml
forwards:
  - name: rdp
    direction: client_to_server
    server_target: 127.0.0.1:3389
    allowed_users:
      - alice

  - name: reverse-web
    direction: server_to_client
    listen_addr: 127.0.0.1:18080
    allowed_users:
      - alice
```

启动规则：

- 正向 `client_to_server`：GUI 保存时会预检 `server_target` 是否可访问；服务启动后会再次审计并记录运行事件。目标临时不可访问不拖垮服务主入口，实际连接时会返回“目标服务不可达”。GUI 页面只填写端口号；保存时会把正向端口写成 `server_target: 127.0.0.1:<端口>`，并把所选用户写入 `allowed_users`。
- 反向 `server_to_client`：服务启动时只加载配置并审计端口状态；客户端上线激活时才会尝试监听服务端被动端口。端口临时被占用时不拖垮服务端，客户端会自动重试并记录“服务端被动端口不可用”。
- 反向 `listen_addr` 只能是服务端本机回环地址，例如 `127.0.0.1`、`localhost` 或 `::1`，不能绑定到 `0.0.0.0` 或网卡 IP。
- 反向端口不能重复，并且必须只归属一个启用用户；GUI 页面只填写端口号；保存时会把反向端口写成 `listen_addr: 127.0.0.1:<端口>`，并把所选用户写入 `allowed_users`。
- 反向端口由客户端连接触发激活。客户端异常断网、崩溃或被强杀时，服务端会通过反向控制心跳自动释放该客户端的激活状态，新客户端可重新尝试激活同一端口。
- 不再支持全局放行；访问授权只来自转发规则中的 `allowed_users`。

## 5. 启动服务端

命令行启动：

```powershell
cmd /c deploy\windows\run.cmd server
```

GUI 重启：

```powershell
cmd /c deploy\windows\run.cmd server-gui
```

服务端 GUI 只保留“重启服务”按钮。点击后会保存未保存配置并重启 `LSYLTunnelServer` Windows 服务；如果服务尚未注册，会请求管理员授权完成注册并启动。命令行调试才使用 `run.cmd server`。

Windows 服务见 [Windows 服务部署与调试](../development/windows-service-zh.md)。

生成完整客户端/服务端安装器，推荐使用根目录发布入口：

```powershell
cmd /c release.cmd
```

只生成分发目录、不编译安装器：

```powershell
cmd /c release.cmd /package-only
```

开发人员可以只交付整个 `dist` 目录给实施。实施人员按现场修改 `dist\LSYL Tunnel Server\conf\server.yaml` 后，可以运行：

```cmd
dist\LSYL Tunnel Server\make-installer.cmd
```

如果开发机已安装 Inno Setup 6，`dist` 会包含 `tools\inno\ISCC.exe`，实施机器不需要额外安装 Inno；没有内置编译器时再安装 Inno Setup 6 或设置 `INNO_SETUP_ISCC`。

也可以在 `dist` 根目录一键生成客户端和服务端安装器：

```cmd
dist\make-installers.cmd
```

Inno 安装器会注册 `LSYLTunnelServer` 服务，默认启动类型为手动；只有安装时勾选开机自启动确认项，才会设置为自动启动。安装器会在安装时生成缺失的 `certs\server.crt` / `certs\server.key`。

手动注册服务端服务：

```powershell
cmd /c deploy\windows\service\server.cmd install
cmd /c deploy\windows\service\server.cmd start
cmd /c deploy\windows\service\server.cmd status
```

## 6. 运行详情

服务端 GUI 的“运行详情”页用于近期问题排查：

- 封禁 IP：显示未过期的封禁来源、剩余时间和自动解封时间。服务停止时也会从运行状态文件读取。
- 失败诊断：只展示认证失败、策略拒绝、目标不可达、反向端口无在线客户端等异常结果。
- 最近事件：展示内存滑窗中的请求、登录探测、连接建立、连接关闭和反向控制心跳超时事件。

完整文本流水仍在“运行日志”页查看；运行详情只保留最近 `runtime.recent_events` 条事件。

## 7. 监控接口

如果配置了：

```yaml
monitor_addr: 127.0.0.1:19111
```

可以访问：

```text
http://127.0.0.1:19111/status
```


