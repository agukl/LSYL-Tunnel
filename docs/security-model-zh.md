# 安全模型

## 核心边界

LSYL Tunnel 的身份认证核心是用户名和密码，不是客户端证书。

TLS 的职责是：

- 加密账号密码和业务流量。
- 让客户端验证服务端身份。
- 防止链路中间人直接读取或篡改流量。

TLS 不负责：

- 证明客户端机器身份。
- 代替用户名密码认证。
- 防止账号密码被用户复制或共享。
- 防止客户端机器被攻陷后泄露本地凭据。

## 密码存储

服务端配置中推荐保存 PBKDF2-SHA256 哈希：

```powershell
build\bin\server\lsyl-tunnel-passwd.exe -password "your-password"
```

格式：

```text
pbkdf2-sha256:<iterations>:<salt-base64>:<hash-base64>
```

`plain:` 只建议本地测试使用，不建议生产使用。

## 客户端本地密封凭据

客户端 GUI 登录成功后，会使用服务端下发的短期公钥，把本次输入的密码密封成 `saved_credential`，并清空本地 `password` 明文字段。

下一次连接时，客户端在 TLS 通道内提交这个密封凭据；服务端用私钥解开后，再按原来的密码哈希校验。

这个设计只解决“客户端配置文件长期保存明文密码”的问题，不改变账号密码认证边界，也不替代 TLS。密封凭据在过期前仍可能被重放使用，因此泄露时应修改账号密码并废弃对应的服务端 `credential_seal` 密钥。

服务端会自行维护 `credential_seal` 的激活密钥：运行中每分钟检查一次，登录响应下发公钥前也会兜底检查。如果当前 `active` 密钥已经过期，服务端会优先激活配置中已存在且未过期的下一把密钥；如果没有可用下一把，会自动生成新的 `login-key-YYYY-MM` 密钥文件、设为 `active`，并写回配置文件。旧密钥会保留但失活，旧客户端保存的 `saved_credential` 按原过期时间自然失效。

## 服务端 TLS 文件

`lsyl-tunnel-cert` 会生成：

- `server.crt`：客户端用于信任和识别服务端的公开文件。
- `server.key`：服务端私钥，只能保存在服务端。

泄露 `server.crt` 通常不是严重问题，因为客户端本来就需要它识别服务端。

泄露 `server.key` 是严重问题，攻击者可能伪装服务端。应立即重新生成 TLS 文件，并更新客户端信任文件。

## 访问授权

服务端默认拒绝未配置的转发请求。用户账号只负责认证，端口授权统一写在 `forwards[].allowed_users` 中。

服务端 GUI 中，“用户认证”页只维护账号；“转发端口”页按用户分配端口放通。服务端目标默认限制在本机回环地址，核心校验不会接受全局放行策略。

## 连接级限流

服务端在接受 TCP 连接后、进入 TLS 和认证握手前，会先执行轻量连接限制：

- `security.max_concurrent_connections`
- `security.max_concurrent_connections_per_ip`
- `security.connection_rate_window_sec`
- `security.max_new_connections_per_ip_window`

达到阈值的连接会被直接关闭，不进入请求解析和认证流程，也不会写入请求流水。这一层用于限制慢 TLS、慢握手和短时间新建连接洪峰对文件描述符、goroutine 和握手处理资源的消耗。

这里的“速率”只表示新建连接次数速率，不表示隧道内业务数据的带宽上限。业务流量统计在连接关闭事件里记录 `bytes_up`、`bytes_down` 和 `duration_ms`。

## 账号业务流限制

认证和转发授权通过后，服务端会对真正进入数据转发的业务连接执行账号维度限制：

- `security.max_concurrent_streams_per_user`
- `security.stream_rate_limit_bytes_per_sec`

`max_concurrent_streams_per_user` 限制单个账号同时保持的正向 `open` 和反向 `reverse_stream` 数据通道数量。登录、健康检查、转发检查和反向控制连接不占用该额度。

`stream_rate_limit_bytes_per_sec` 限制每条业务连接的上下行合计速率。它不会改变业务日志口径；连接关闭时仍按实际通过的字节数记录 `bytes_up`、`bytes_down` 和 `duration_ms`。

## 登录失败封禁

服务端按来源 IP 统计认证失败：

- `security.auth_fail_window_sec`
- `security.auth_fail_threshold`
- `security.auth_fail_block_sec`

同一来源 IP 在窗口期内失败次数达到阈值后，会被临时封禁。

封禁 IP 属于业务结果，会写入 `runtime.state_file`。服务重启后，未过期的封禁记录仍会恢复，避免攻击者通过重启服务绕过封禁窗口。

## 运行事件

请求、认证、连接、策略拒绝、目标失败等属于运行事件：

- 服务进程内保留最近 `runtime.recent_events` 条。
- 同步写入按天切分的服务日志。
- 用于运行详情页和故障排查。

运行事件不应该记录密码、密封凭据密文或完整业务数据包。

## 泄露处理建议

- 泄露客户端密码：修改该用户密码哈希，重启服务端。
- 泄露客户端 `saved_credential`：修改该用户密码，并轮换服务端 `credential_seal` 密钥。
- 泄露服务端 TLS 私钥：重新生成服务端 TLS 文件，更新客户端信任文件。
- 泄露服务端密码哈希：要求用户更换强密码，并确认哈希算法使用 PBKDF2。
