# Android 客户端说明

## 产品边界

LSYL Tunnel Mobile 是安全隧道 / 应用代理客户端，不是系统级 VPN。

当前明确不做：

- 系统级 VPN / TUN
- 反向代理 `server_to_client`
- 密码输入或密码登录
- 日志页
- 动态目标代理
- 监听 `0.0.0.0`

## 运行边界

- 最低版本：Android 10 / API 29
- TLS 版本：强制 `TLSv1.3`
- 证书：Profile 内 `server.crt` 精确 Pin
- 认证：只发送 `saved_credential`
- 后台：Foreground Service
- 本地端口：只监听 `127.0.0.1:<port>`

连接成功后，隧道和本地监听完全由 Foreground Service 托管。LSYL 页面可以退到后台、从最近任务移除、锁屏或切换到 RDP 等其他应用，不需要保持前台窗口可见。

通知栏状态对应真实服务状态：

- `已连接`：前台服务运行，并且至少一个本地监听可用。
- `部分端口不可用`：服务仍运行，健康规则继续工作。
- `网络暂不可用，等待恢复`：本地监听仍保留，等待默认网络恢复。

网络恢复由系统网络事件触发一次复核，不执行固定间隔心跳或后台轮询。切换网络时已有 TCP/RDP 数据流可能断开，重新访问本地端口即可建立新隧道。用户主动断开后不会自动恢复；在 Android 系统设置中“强制停止”应用同样不会自动恢复。

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

分发包不再包含独立命令行 Profile 工具。GUI 导出过程会校验：客户端必须已有 `saved_credential`，TLS 不允许跳过校验，转发只允许 `client_to_server`，移动端本地监听会规范为 `127.0.0.1:<port>`，且端口必须大于等于 `1024`。

同一个 `.lsylprofile` 也可以导入 Win7 轻量客户端 `lsyl-tunnel-client-lite.exe`。轻量客户端复用移动端的安全边界，只提供导入、连接、断开，不做托盘值守。

完整发布默认把移动端 APK 放到 `dist/installers/LSYL-Tunnel-Android.apk`，不再生成 `dist/LSYL Tunnel Lightweight Clients`。发布入口会调用 Gradle 重新构建 Android APK；如果构建机没有 Gradle，需要设置 `GRADLE_EXE`，把 Gradle 放到 `tool\gradle-8.9`，或设置 `MOBILE_APK` 指向已有 APK。Win7 轻量客户端随普通客户端安装包和默认客户端 kit 目录一起交付。

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
-> 服务端授权的 server_target（可为服务端所在网络可达的内网目标）
```

首页“重新检查”执行：

```text
health
-> forward_check 每个正向端口
```

重新打开或切回 LSYL 页面只读取本地运行快照，不会自动发起检查。手动检查不会创建真实业务流；实际业务连接仍在访问 `127.0.0.1:<port>` 时按需建立。

## 文本编辑转发

断开连接后，可以通过“编辑转发”修改当前 Profile 的正向规则：

```yaml
forwards:
  - name: rdp
    listen_addr: 127.0.0.1:13389
    server_target: 192.168.1.7:3389
```

编辑器只接受 `forwards`，每条规则只允许 `name`、`listen_addr` 和 `server_target`。`direction` 固定为 `client_to_server`。保存前会整体校验未知字段、层级、重复规则、端口范围和地址格式；校验失败不会替换当前配置。

编辑不会修改服务端地址、用户、`client_id`、密封凭据、证书、TLS 或连接参数。服务端仍按账号和精确目标执行最终授权。

## 后台验收

安装新 APK 后至少验证以下场景：

1. 连接成功后按 Home，直接在 RDP 中持续使用本地端口。
2. 从最近任务移除 LSYL，再次从 RDP 建立连接。
3. 锁屏数分钟后解锁，重新建立业务连接。
4. 切换 Wi-Fi 和移动网络，确认服务与本地监听不退出，并能建立新连接。
5. 返回 LSYL 页面，确认不会因页面恢复前台而自动执行远端检查。
6. 主动断开，确认通知、服务和本地监听全部停止且不会自行恢复。
