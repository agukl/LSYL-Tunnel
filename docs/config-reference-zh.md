# 配置参考

## 说明

配置字段仍然使用 `tls`，因为底层传输保护由 Go 标准库 TLS 完成。但在业务语义上，TLS 只负责传输保护和服务端识别；客户端身份认证仍然是用户名和密码，或由服务端密封过的短期登录凭证还原出密码后再校验。

## 服务端配置

`listen_addr`：服务端隧道入口监听地址。

`monitor_addr`：本机监控 HTTP 地址，留空表示关闭。

`tls.cert_file`：服务端 TLS 公开身份文件，客户端会信任它来识别服务端。

`tls.key_file`：服务端 TLS 私钥，必须保护好，不能发给客户端。

`tls.min_version`：推荐 `"1.3"`，兼容旧环境可设为 `"1.2"`。

`auth.users[].username`：用户名。

`auth.users[].password_hash`：密码哈希，推荐使用 `lsyl-tunnel-passwd` 生成。

`auth.users[].disabled`：禁用用户。

`security.handshake_timeout_sec`：认证握手超时。

`security.dial_timeout_sec`：服务端连接目标服务超时。

`security.max_handshake_bytes`：认证请求最大字节数。

`security.max_concurrent_connections`：服务端入口允许的最大并发连接数，包含尚未完成 TLS/认证握手的连接。

`security.max_concurrent_connections_per_ip`：同一来源 IP 允许的最大并发入口连接数。

`security.connection_rate_window_sec`：来源 IP 新建连接速率统计窗口秒数。

`security.max_new_connections_per_ip_window`：同一来源 IP 在速率窗口内允许的新建连接数。该限制只控制新建连接速率，不限制已建立隧道的业务带宽。

`security.max_concurrent_streams_per_user`：单个账号允许同时保持的业务连接数，`0` 表示不限制。只统计正向 `open` 和反向 `reverse_stream` 数据通道，不统计登录、健康检查和反向控制连接。

`security.stream_rate_limit_bytes_per_sec`：每条业务连接的总速率上限，单位字节/秒，`0` 表示不限制。该上限按单条连接上下行合计统计。

`security.auth_fail_window_sec`：登录失败统计窗口。

`security.auth_fail_threshold`：窗口内失败次数阈值。

`security.auth_fail_block_sec`：触发阈值后的封禁时间。

`credential_seal.keys[]`：客户端本地加密保存密码使用的服务端密封密钥。客户端只拿到公钥，私钥只留在服务端。

`credential_seal.keys[].key_id`：密封密钥标识，建议按月份、季度或批次命名。

`credential_seal.keys[].private_key_file`：服务端私钥路径。文件不存在时服务端会自动生成。

`credential_seal.keys[].public_key_file`：服务端公钥路径。文件不存在时会由私钥自动导出。

`credential_seal.keys[].expires_at`：密封密钥过期时间，格式为 RFC3339，例如 `2026-08-20T00:00:00+08:00`。过期后客户端需要重新输入密码。

`credential_seal.keys[].active`：当前下发给客户端的新密封公钥，只允许一个 active。

`runtime.state_file`：服务端运行状态持久化文件。源码开发配置默认位于 `runtime\data\server-state.json`；安装包内配置默认位于 `data\server-state.json`。当前用于保存已触发封禁的来源 IP，避免服务重启后立刻失效。

`runtime.request_log_file`：请求认证层流水日志。源码开发配置默认位于 `runtime\logs\request\request.jsonl`；安装包内配置默认位于 `logs\request\request.jsonl`。

`runtime.business_log_file`：业务控制层流水日志。源码开发配置默认位于 `runtime\logs\business\business.jsonl`；安装包内配置默认位于 `logs\business\business.jsonl`。

`runtime.entry_traffic_log_file`：入口连接层流量和异常日志。源码开发配置默认位于 `runtime\logs\entry-traffic\entry-traffic.jsonl`；安装包内配置默认位于 `logs\entry-traffic\entry-traffic.jsonl`。用于记录连接限制拒绝、永久封禁命中、非 TLS、HTTP 探测、TLS/协议握手异常等入口层事件。

`runtime.flow_traffic_log_file`：业务数据流层流量和异常日志。源码开发配置默认位于 `runtime\logs\flow-traffic\flow-traffic.jsonl`；安装包内配置默认位于 `logs\flow-traffic\flow-traffic.jsonl`。用于记录 `open`、`reverse_stream` 数据流关闭时的字节数、时长、平均速率，以及单账号并发限制、目标不可达、反向流超时等异常。

`runtime.recent_events`：服务端内存中保留的最近运行事件数量，默认 500。请求、认证、连接、拒绝、目标失败等运行详情只做滑窗保留，并同步写入按天切分的服务日志。

`forwards[]`：服务端侧转发清单，用于启动前检查、反向端口授权，以及 GUI 中的端口放通管理。客户端仍需要配置自己的 `forwards[]` 来决定登录后启用哪些映射。

服务端 `forwards[].allowed_users`：允许使用该转发规则的用户名列表。用户账号本身只负责认证；端口访问关系统一写在转发规则上。

服务端 `forwards[].direction: client_to_server`：配置保存时会预检 `server_target` 是否能从服务端访问；服务启动后也会做审计并记录运行事件。目标临时不通不会拖垮服务主入口，实际连接时会返回“目标服务不可达”。

服务端 `forwards[].direction: server_to_client`：服务端只保存被动入口配置和用户绑定；客户端上线激活时才会尝试监听 `listen_addr`。如果此时端口被其他程序占用，客户端会自动重试并记录“服务端被动端口不可用”。该地址只能是服务端本机回环地址，例如 `127.0.0.1:18080`、`localhost:18080` 或 `[::1]:18080`，不能配置为 `0.0.0.0` 或公网/内网网卡地址。

服务端反向 `listen_addr` 和端口都不能重复。客户端激活反向转发时，请求的 `listen_addr` 必须已经存在于服务端 `forwards[]` 中。

反向端口必须且只能绑定到一个启用用户。GUI 会把所选用户写入该 `server_to_client` 规则的 `allowed_users`；手写配置时，同一个反向端口只能出现在一条转发规则中，并且该规则只能绑定一个用户。这样端口归属在配置阶段就固定下来，不靠客户端上线先后抢占。

反向控制连接带有服务端心跳。客户端异常离线且没有正常释放控制连接时，服务端会在心跳超时后释放该客户端的激活状态；新的客户端连接会重新尝试激活同一配置端口。

## 客户端配置

`server_addr`：服务端隧道地址。

`username`：用户名。

`password`：密码。GUI 登录成功后会清空该字段，不建议长期保存明文。

`password_env`：读取密码的环境变量名。管理员预置场景可用。

`password_file`：读取密码的文件路径。管理员预置场景可用。

`saved_credential`：GUI 登录成功后写入的短期密文凭证。它不是明文密码，但在过期前仍可用于登录；泄露时应修改账号密码并轮换服务端密封密钥。

`client_id`：客户端标识，仅用于日志和排查，不作为安全凭据。

`tls.ca_cert_file`：客户端信任的服务端 TLS 公开文件，通常就是服务端生成的 `server.crt`。客户端默认配置指向 `../cert/server.crt`，打包和安装时会把 `src\client\cert\server.crt` 放到客户端目录的 `cert\server.crt`。

`tls.server_name`：TLS 校验使用的服务端名称，应匹配生成服务端 TLS 文件时的 `-hosts`。

`tls.insecure_skip_verify`：跳过 TLS 服务端校验，仅允许临时测试使用，生产必须为 `false`。

`connection.dial_timeout_sec`：客户端连接服务端超时。

`forwards[].name`：转发名称。

`forwards[].direction`：转发方向。`client_to_server` 表示客户端监听入口、服务端连接目标；`server_to_client` 表示服务端创建被动入口、客户端连接目标。留空时默认为 `client_to_server`。

`forwards[].listen_addr`：入口监听地址。`client_to_server` 时监听在客户端；`server_to_client` 时监听在服务端，并且该服务端被动端口必须由已认证客户端连接后激活。

`forwards[].server_target`：目标地址。`client_to_server` 时表示服务端侧要访问的目标；`server_to_client` 时表示客户端侧要访问的目标。当前字段名保留兼容，后续可再考虑改名。

反向转发不会让服务端主动连接客户端。`server_to_client` 的实际流程是：服务端启动时加载配置但不主动占端口；客户端主动连接服务端并尝试激活这个被动端口；有人访问该服务端端口时，服务端通知客户端；客户端再主动建立一条流连接回来并访问自己的本地目标。

