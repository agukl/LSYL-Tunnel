# Android 移动端一期方案

## 产品边界

LSYL Tunnel Mobile 是安全隧道 / 应用代理客户端，不是系统级 VPN。

一期明确不做：

- 系统级 VPN / TUN
- 反向代理 `server_to_client`
- 密码输入或密码登录
- 日志页
- 动态目标代理
- 监听 `0.0.0.0`

## 技术决策

- 语言：Kotlin 原生
- 最低版本：Android 10 / API 29
- TLS：Android 平台 `SSLSocket`
- TLS 版本：强制 `TLSv1.3`
- 协议：Kotlin 原生实现 LSYL 4 字节 big-endian frame + JSON
- 证书：Profile 内 `server.crt` 精确 Pin
- 认证：只发送 `saved_credential`
- 后台：Foreground Service
- 本地端口：只监听 `127.0.0.1:<port>`

## Profile 格式

`.lsylprofile` 是 zip 包：

```text
profile.json
server.crt
```

该文件由 Windows 客户端侧生成，不建议手工编辑 zip。用户先在客户端 GUI 成功连接一次，让本机 `conf/client.yaml` 写入未过期的 `saved_credential`。实际 GUI 触发方式是：连接成功后右键客户端窗口左上角 LSYL 图标，确认弹窗后自动导出到当前用户系统“下载”目录，文件名格式为：

```text
<用户名>_<凭据到期日期>.lsylprofile
```

也可以在客户端安装目录或分发包中用命令行生成：

```powershell
bin\lsyl-tunnel-profile.exe export-mobile -out .\mobile.lsylprofile
```

如果需要从某个已导入的客户端 profile 生成：

```powershell
bin\lsyl-tunnel-profile.exe export-mobile -profile PROFILE_NAME -out .\mobile.lsylprofile
```

输出文件已存在时需要加 `-force`。生成过程会校验：客户端必须已有 `saved_credential`，TLS 不允许跳过校验，转发只允许 `client_to_server`，移动端本地监听会规范为 `127.0.0.1:<port>`，且端口必须大于等于 `1024`。

同一个 `.lsylprofile` 也可以导入 Win7 轻量客户端 `lsyl-tunnel-client-lite.exe`。轻量客户端复用移动端的安全边界，只提供导入、连接、断开，不做托盘值守。

发布时，移动端 APK 和 Win7 轻量客户端会一起进入 `dist/LSYL Tunnel Lightweight Clients`，便于现场把两个轻量入口和用户专属 `.lsylprofile` 放在同一个交付目录里。

`profile.json` 示例：

```json
{
  "version": 1,
  "profile_id": "hzls-mobile-001",
  "server_addr": "example.com:3443",
  "username": "hzls",
  "client_id": "mobile-hzls-001",
  "saved_credential": {
    "type": "server_sealed",
    "key_id": "login-key-2026-08",
    "expires_at": "2026-08-20T00:00:00+08:00",
    "ciphertext": "..."
  },
  "tls": {
    "ca_cert_file": "server.crt",
    "server_name": "",
    "min_version": "1.3",
    "insecure_skip_verify": false
  },
  "connection": {
    "dial_timeout_sec": 5
  },
  "forwards": [
    {
      "name": "sql",
      "direction": "client_to_server",
      "listen_addr": "127.0.0.1:5432",
      "server_target": "127.0.0.1:5432"
    }
  ]
}
```

## 导入确认页

只展示：

```text
用户：<username>
有效期至：<saved_credential.expires_at>
```

不展示服务端地址、证书指纹和端口列表。

## 导入校验

移动端导入时仍在后台强校验：

- 必须有 `profile.json`
- 必须有 `server.crt`
- 必须有 `saved_credential.ciphertext`
- `saved_credential.type` 必须是 `server_sealed`
- `saved_credential.expires_at` 必须未过期
- 禁止 `password` / `password_env` / `password_file`
- 禁止 `insecure_skip_verify`
- `tls.min_version` 必须是 `1.3`
- 只允许 `client_to_server`
- `listen_addr` 必须是 `127.0.0.1:<port>`
- 本地端口必须大于等于 `1024`
- 本地监听端口不可重复

## 运行链路

```text
手机 App / 工具
-> 127.0.0.1:local_port
-> LSYL Tunnel Mobile Foreground Service
-> TLS 1.3
-> 服务端 3443
-> 服务端本机目标端口
```

手动刷新执行：

```text
health
-> forward_check 每个正向端口
```

刷新不会连接目标服务，也不会创建真实业务流。

## 当前代码位置

```text
mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/profile
mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/security
mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/protocol
mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/tunnel
mobile/android/app/src/main/java/com/lsyl/tunnel/mobile/service
```
