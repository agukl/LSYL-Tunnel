# Windows 服务部署

完整安装包流程见 [部署与安装指南](deployment-zh.md)。本文只说明 Windows 服务本身的注册、重启、停止和日志边界。

服务脚本统一放在：

```text
deploy\windows\service\
```

## 服务名称

```text
LSYLTunnelServer
```

服务显示名和描述固定为：

```text
Display name: LSYL Tunnel Server
Description : LSYL Tunnel 服务端：提供账号认证的 TLS 加密隧道和端口转发，日志写入安装目录 logs。
```

服务名、显示名和描述不随机生成，也不伪装系统服务，便于管理员在 Windows 服务管理器、事件排查和杀软误报申诉中识别。

## 服务端服务

统一入口：

```powershell
cmd /c deploy\windows\service\server.cmd install
cmd /c deploy\windows\service\server.cmd start
cmd /c deploy\windows\service\server.cmd status
cmd /c deploy\windows\service\server.cmd stop
cmd /c deploy\windows\service\server.cmd uninstall
```

`server.cmd install` 只注册服务，不会自动启动。服务启动类型为手动。注册时会固定写入服务程序、配置文件和日志路径。

服务端 GUI 只保留“重启服务”按钮。点击后会先保存未保存配置，再重启 `LSYLTunnelServer` Windows 服务；如果服务尚未注册，会请求管理员授权完成注册并启动。

正式安装包同样注册 `LSYLTunnelServer` 服务。默认启动类型为手动；只有管理员在安装向导中勾选“设置 LSYL Tunnel Server 服务为开机自启动”时，才会改为开机自启动。安装阶段会在缺少文件时生成自签 `certs\server.crt` 和 `certs\server.key`，并提醒管理员只把 `server.crt` 发给客户端。

服务注册使用项目内置的服务注册助手，不再直接依赖安装器拼接 `sc.exe` 命令。Windows Server 上如果注册失败，安装器会直接提示具体原因；同时会写入：

```text
C:\Program Files\LSYL Tunnel Server\logs\service\service-register-error.txt
```

常见原因包括：未以管理员身份运行、服务正在删除中、服务程序缺失、配置文件缺失、安装目录权限不足。

安装、升级和卸载时，安装器只通过 `sc.exe stop LSYLTunnelServer` 正规停止服务，并依赖安装器的应用关闭机制处理正在运行的管理台；不使用强制 `taskkill /F` 清理进程，避免形成“强杀进程”的木马特征。

## 客户端运行方式

客户端只使用用户态进程：普通 GUI 是进程模式 + 托盘值守，Win7 轻量客户端是窗口内手动连接/断开。客户端不提供、不注册、不清理 Windows 服务。

## 权限说明

- `install`、`start`、`stop`、`uninstall` 通常需要管理员权限。
- `status` 通常不需要管理员权限。
- GUI 重启服务时可能弹出 UAC 授权窗口。

## 默认程序和配置

开发目录服务端：

```text
build\bin\server\lsyl-tunnel-server-svc.exe
src\server\conf\server.yaml
runtime\logs\service\server-service-YYYY-MM-DD.log
```

安装后的服务端：

```text
C:\Program Files\LSYL Tunnel Server\bin\lsyl-tunnel-server-svc.exe
C:\Program Files\LSYL Tunnel Server\conf\server.yaml
C:\Program Files\LSYL Tunnel Server\certs\server.crt
C:\Program Files\LSYL Tunnel Server\logs\service\server-service-YYYY-MM-DD.log
```

安装后的客户端：

```text
conf\client.yaml
cert\server.crt
```

服务端卸载默认保留：

```text
conf\
certs\
data\
logs\
```

服务端卸载由 Inno 安装器处理，默认保留运行数据目录；需要彻底清理时应先确认目标目录确实是 LSYL Tunnel 的安装目录，避免误删非安装目录。

## 启动前检查

服务端启动前应确认：

```text
certs\server.crt
certs\server.key
src\server\conf\server.yaml
```

客户端启动前应确认：

```text
src\client\conf\client.yaml
src\client\cert\server.crt
```

服务端启动时会根据 `forwards[]` 做端口预检：

- 正向代理：审计服务端目标端口是否可访问，失败时写入运行事件，不拖垮服务主入口。
- 反向代理：检查配置合法性并审计服务端被动端口状态；客户端上线激活时才监听该端口，端口不可用时由客户端提示、记录并自动重试。

## 日志和运行状态

服务日志按天切分。源码开发服务传入 `-log runtime\logs\service\server-service.log` 时，实际文件形如：

```text
runtime\logs\service\server-service-2026-05-20.log
```

请求、认证、连接、拒绝、目标失败等运行事件会同步写入当天日志，并在服务进程内保留最近 `runtime.recent_events` 条，供运行详情页面读取。

封禁 IP 属于业务结果，不依赖文本日志推断。服务端会写入 `runtime.state_file`，默认是：

```text
runtime\data\server-state.json
```

服务重启后，未过期封禁 IP 会从该文件恢复。

