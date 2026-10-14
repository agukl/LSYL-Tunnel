# 配置参考

## 说明

配置字段仍然使用 `tls`，因为底层传输保护由 Go 标准库 TLS 完成。但在业务语义上，TLS 只负责传输保护和服务端识别；客户端身份认证仍然是用户名和密码，或由服务端密封过的短期登录凭证还原出密码后再校验。

## 地址写法

配置中的回环地址只接受 `127.0.0.1`，带端口时写为 `127.0.0.1:端口`。`localhost`（含大小写、尾点变体）、`::1`、其他 `127.0.0.0/8` 地址及 IPv4 映射回环地址均明确报错，不自动转换。此规则适用于连接、监听、目标及 `tls.server_name` 配置，也适用于安装兼容检查和配置导入、导出。

`server_addr` 仍支持普通域名；`0.0.0.0` 和 `::` 是全网卡监听，不属于回环地址，原有监听规则不变。虚拟入口仍必须是证书授权的非本地 IPv4，不能使用 `127.0.0.1`。

升级前需手工修正旧写法；若将 `tls.server_name` 改为 `127.0.0.1`，证书也必须包含相应 IP SAN。程序不会改写已签发证书，不会关闭证书校验。历史日志及封禁数据中的 IP 不做文本替换，回环封禁清理仍识别完整回环范围。

## 转发配置职责

| 方向 | 客户端负责 | 服务端负责 | 协议边界 |
| --- | --- | --- | --- |
| `client_to_server` | 配置并监听 `listen_addr`；提交 `server_target` | 配置相同的 `server_target` 和 `allowed_users`；按账号、方向、目标精确授权并连接目标 | 客户端提交目标，服务端不信任规则名 |
| `virtual` | 校验证书 IP，接管 `listen_addr` 指定的 `IP:端口`，再通过随机本地桥接端口转发 | 与 `client_to_server` 相同，只校验服务端目标和账号授权 | 线上按 `client_to_server` 处理，服务端不感知虚拟入口 |
| `server_to_client` | 配置服务端 `127.0.0.1:端口` 的 `listen_addr` 和客户端同格式的 `server_target`；收到流 ID 后连接本地目标 | 配置全局唯一的 `listen_port` 和单一归属账号；按客户端提交的监听地址与账号精确授权 | 客户端提交监听地址和本地目标；服务端只返回不透明流 ID，不下发或改写规则 |

`server_addr`、账号和 TLS 信任用于连接并识别隧道服务端；它们不是业务目标。`forwards[].name` 不参与服务端授权，两端名称可以不同。

### 地址与端口边界

| 配置位置 | 可用写法 | 不代表什么 |
| --- | --- | --- |
| 客户端 `server_addr` | 域名或 IP，加隧道端口 | 不是业务目标或虚拟入口 |
| 客户端 `tls.server_name` | 证书允许的域名或 IP，不带端口；留空按 `server_addr` 校验 | 不参与端口映射 |
| 正向 / 虚拟 `server_target` | `127.0.0.1:端口` 或私有 IP:端口，与服务端授权目标一致 | 不支持域名、公网 IP 或任意目标 |
| 普通 Windows 正向 `listen_addr` | 本机监听地址:端口；通常用 `127.0.0.1`，全网卡监听也保留支持 | 不需要等于目标端口，也不由服务端下发 |
| 虚拟 `listen_addr` | `:端口` 或证书授权的 `IPv4:端口` | 不支持域名接管、整个 IP 接管或隧道端口接管 |
| 客户端反向 `listen_addr` / `server_target` | 均为 `127.0.0.1:端口`；前者在服务端，后者在客户端 | 两者不是同一台机器上的地址 |
| 服务端反向 `listen_port` | 单个数字端口 | 不是客户端字段 |

业务目标和反向端口范围为 `1-65535`，不支持端口范围字符串；多个端口分别配置规则。普通 Windows 正向监听保留端口 `0` 的随机分配能力，固定业务入口应填写具体端口；Win7 Lite / Android 的本地监听地址固定为 `127.0.0.1`，端口范围为 `1024-65535`。

私有目标包括 IPv4 的 `10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16` 和 IPv6 ULA `fc00::/7`；IPv6 地址带端口时必须用方括号。虚拟入口仍仅支持 IPv4。客户端提前校验格式与地址范围，服务端仍独立执行最终的账号及精确目标授权。

服务端正向规则使用 `server_target`，反向规则使用 `listen_port`；不要把客户端的监听地址和反向目标填到服务端对应规则中。服务端为兼容旧配置仍接受反向 `listen_addr: 127.0.0.1:端口`，两种字段同时出现时必须指向相同端口，否则读取、安装检查和保存均报错。客户端任何方向都不接受 `listen_port` 或 `virtual_ip` 配置字段。

两端只校验自己掌握的安全边界，不互相同步、推导或修正业务字段。客户端规则存在错误时，错误会定位到具体规则，本次配置拒绝加载且原文件保持不变；不会跳过错误规则继续连接，也不会静默改写。服务端配置错误则阻止保存、启动或覆盖安装。

## 服务端配置

`config_version`：服务端配置结构版本。当前支持 `1`；缺省时按历史 V0 配置处理并套用当前默认值。覆盖安装会先按对应版本的字段集做严格 YAML 校验：未知字段、错误层级、V1 缺失必填结构或运行语义不合法时均会停止安装；高于当前程序支持的版本同样拒绝。

`requires.min_server_version`：该配置要求的最低服务端程序版本。当前分发默认 `"3.1.0"`；配置结构、协议基线或字段含义升级时才应上调。

`compatibility.min_client_version`：允许连接本服务端的最低客户端版本。当前默认 `"3.0.0"`。

`compatibility.max_client_version`：允许连接本服务端的最高客户端版本，默认留空表示不限制新客户端。只有确认新客户端不能安全连接旧服务端时才填写。

`compatibility.protocol_version`：服务端要求的 LSYL 握手协议版本。当前为 `3`；客户端握手中的 `protocol_version` 不匹配时会被拒绝。协议 3 在 TLS 前增加证书持有前置门，不兼容协议 2 客户端。

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

`security.max_concurrent_connections`：服务端入口允许的最大并发 TCP 连接数，包含尚未完成前置门/TLS/认证的连接、正向数据连接、反向控制连接和反向数据连接。名额在连接实际关闭时释放；反向控制连接也占一个入口名额，但不占账号业务流名额。

`security.max_concurrent_connections_per_ip`：同一来源 IP 允许的最大并发入口连接数。

`security.connection_rate_window_sec`：来源 IP 新建连接速率统计窗口秒数。

`security.max_new_connections_per_ip_window`：同一来源 IP 在速率窗口内允许的新建连接数。该限制只控制新建连接速率，不限制已建立隧道的业务带宽。

`security.max_tracked_connection_ips`：入口连接限流器可保留的来源 IP 状态上限。达到上限时优先逐出最久未活动的来源；若所有记录都有活动连接，则拒绝新的来源连接并在监控状态中计数。

`security.connection_limiter_cleanup_sec`：入口连接限流来源状态的过期清扫周期。速率窗口已过且没有活动连接的来源会被移出内存。

`security.max_tracked_failure_ips`：认证失败与异常协议失败的来源 IP 内存记录上限。达到上限时优先逐出最久未活动且未处于临时封禁的记录；若只剩有效临时封禁，新的失败记录会被丢弃并在监控状态中计数。

`security.failure_tracker_cleanup_sec`：失败来源记录的过期清扫周期，过期的失败时间序列和临时封禁会从内存中移除。

`security.entry_traffic_log_queue_size`：入口异常日志异步队列容量。入口保护不会等待日志落盘；队列满时仅丢弃入口异常明细并在监控状态中计数，不影响入口拒绝动作。

`security.max_concurrent_streams_per_user`：单个账号允许同时保持的业务连接数，`0` 表示不限制。只统计正向 `open` 和反向 `reverse_stream` 数据通道，不统计登录、健康检查和反向控制连接。

`security.stream_rate_limit_bytes_per_sec`：每条业务连接的总速率上限，单位字节/秒，`0` 表示不限制。该上限按单条连接上下行合计统计。

`security.auth_fail_window_sec`：登录失败统计窗口。

`security.auth_fail_threshold`：窗口内失败次数阈值。

`security.auth_fail_block_sec`：触发阈值后的封禁时间。

`credential_seal.keys[]`：客户端本地加密保存密码使用的服务端密封密钥。客户端只拿到公钥，私钥只留在服务端。

`credential_seal.keys[].key_id`：密封密钥标识，建议按月份、季度或批次命名。

`credential_seal.keys[].private_key_file`：服务端私钥路径。文件不存在时服务端会自动生成。

`credential_seal.keys[].public_key_file`：服务端公钥路径。文件不存在时会由私钥自动导出。

`credential_seal.keys[].expires_at`：密封密钥过期时间，格式为 RFC3339。新安装配置默认从打包时间顺延三个月；过期后客户端需要重新输入密码。

`credential_seal.keys[].active`：当前下发给客户端的新密封公钥，只允许一个 active。

`runtime.state_file`：服务端运行状态持久化文件。源码开发配置默认位于 `runtime\data\server-state.json`；安装包内配置默认位于 `data\server-state.json`。当前用于保存已触发封禁的来源 IP，避免服务重启后立刻失效。

`runtime.request_log_file`：请求认证层流水日志。源码开发配置默认位于 `runtime\logs\request\request.jsonl`；安装包内配置默认位于 `logs\request\request.jsonl`。

`runtime.business_log_file`：业务控制层流水日志。源码开发配置默认位于 `runtime\logs\business\business.jsonl`；安装包内配置默认位于 `logs\business\business.jsonl`。

`runtime.entry_traffic_log_file`：入口连接层流量和异常日志。源码开发配置默认位于 `runtime\logs\entry-traffic\entry-traffic.jsonl`；安装包内配置默认位于 `logs\entry-traffic\entry-traffic.jsonl`。用于记录连接限制拒绝、永久封禁命中聚合、非 TLS、HTTP 探测、TLS/协议握手异常等入口层事件。该文件使用有界异步写入，不改变既有字段或文件类别。

`runtime.flow_traffic_log_file`：业务数据流层流量和异常日志。源码开发配置默认位于 `runtime\logs\flow-traffic\flow-traffic.jsonl`；安装包内配置默认位于 `logs\flow-traffic\flow-traffic.jsonl`。用于记录 `open`、`reverse_stream` 数据流关闭时的字节数、时长、平均速率，以及单账号并发限制、目标不可达、反向流超时等异常。

`runtime.recent_events`：服务端内存中保留的最近运行事件数量，默认 500。请求、认证、连接、拒绝、目标失败等运行详情只做滑窗保留，并同步写入按天切分的服务日志。

`forwards[]`：服务端侧转发清单，用于启动前检查、反向端口授权，以及 GUI 中的端口放通管理。客户端仍需要配置自己的 `forwards[]` 来决定登录后启用哪些映射。

服务端 `forwards[].allowed_users`：允许使用该转发规则的用户名列表。用户账号本身只负责认证；端口访问关系统一写在转发规则上。核心配置校验要求每条规则至少绑定一个已存在用户，不能依赖 GUI 页面兜底。

服务端 `forwards[].direction: client_to_server`：GUI 保存时校验目标格式和授权关系，不要求目标当时在线；服务启动后会做可达性审计并记录运行事件。目标临时不通不会拖垮服务主入口，实际连接时会返回“目标服务不可达”。目标只能填写 `127.0.0.1` 或私有网段 IP:端口，例如 `127.0.0.1:3389`、`192.168.10.20:3389`、`10.20.30.40:5432`；不接受域名、公网 IP 或任意未配置目标。

服务端 `forwards[].direction: server_to_client`：只配置 `listen_port`，运行时固定使用 `127.0.0.1:<listen_port>`。端口不能重复，每条规则必须且只能归属一个已存在账号，同一账号可以绑定多个反向端口；账号停用时无法认证和激活，重新启用后无需重配端口。客户端上线时提交对应的 `listen_addr`，服务端校验该监听地址已配置且归属登录账号后才尝试监听；端口被占用时，客户端只收到脱敏后的“服务端被动端口不可用”，具体端口和底层错误仅记录在服务端日志中。这些约束由核心配置校验和 GUI 同时执行。

反向控制连接带有服务端心跳。客户端异常离线且没有正常释放控制连接时，服务端会在心跳超时后释放该客户端的激活状态；新的客户端连接会重新尝试激活同一配置端口。

## 客户端配置

`config_version`：客户端配置结构版本。当前版本为 `2`，并继续读取严格合法的 V1 和无版本旧配置。V1/V2 会校验完整字段结构，所有版本都拒绝未知字段、错误层级、重复字段和多份 YAML 文档；高于当前程序支持值时拒绝启动、覆盖安装或切换到该配置。

`requires.min_client_version`：该配置要求的最低客户端程序版本。配置结构 V2 允许的最低值为 `"2.1.0"`，当前分发默认 `"3.1.0"`；配置结构或字段含义升级时同步上调。

`server_addr`：服务端隧道地址。

`remark`：可选界面备注。仅在 Windows 客户端切换配置时跟在服务端地址后显示，不参与连接、认证、证书校验或转发。包含该字段的配置需要 3.1.0 或更高版本客户端。

`username`：用户名。

`password`：密码。GUI 登录成功后会清空该字段，不建议长期保存明文。

`password_env`：读取密码的环境变量名。管理员预置场景可用。

`password_file`：读取密码的文件路径。管理员预置场景可用。

`saved_credential`：GUI 登录成功后写入的短期密文凭证。它不是明文密码，但在过期前仍可用于登录；泄露时应修改账号密码并轮换服务端密封密钥。

`client_id`：客户端标识，仅用于日志和排查，不作为安全凭据。Win7 轻量客户端导入 `.lsylprofile` 时会使用目标机器的本机名，不继承导出机器的标识。

`tls.ca_cert_file`：客户端信任的服务端 TLS 公开文件，通常就是服务端生成的 `server.crt`。它同时用于 TLS 前的证书持有前置门，因此始终必填，即使临时设置 `insecure_skip_verify: true` 也不能省略。客户端默认配置指向 `../cert/server.crt`，打包和安装时会把 `src\client\cert\server.crt` 放到客户端目录的 `cert\server.crt`。

`tls.server_name`：TLS 校验使用的服务端名称，应匹配生成服务端 TLS 文件时的 `-hosts`。

`tls.insecure_skip_verify`：跳过 TLS 服务端校验，仅允许临时测试使用，生产必须为 `false`。

`connection.dial_timeout_sec`：客户端连接服务端超时。

`forwards[].name`：客户端本地规则标识，也用于显示和日志，不参与服务端授权，无需与服务端规则名一致。同一客户端内必须唯一；名称值可留空，此时以 `listen_addr` 作为标识，也不能与其他规则标识冲突。版本化 YAML 的规则仍需包含 `name` 字段，可写 `name: ""`；不是省略整个字段。

`forwards[].direction`：转发方向。`client_to_server` 表示客户端监听普通本地入口、服务端连接目标；`virtual` 表示标准 64 位 Windows 客户端接管 `listen_addr` 指定的虚拟入口，服务端业务方向仍按 `client_to_server` 处理；`server_to_client` 表示服务端创建被动入口、客户端连接目标。留空时默认为 `client_to_server`。

`forwards[].listen_addr`：`client_to_server` 时表示普通客户端入口；`server_to_client` 时必须填写服务端已预留的 `127.0.0.1:端口`，例如 `127.0.0.1:13306`，客户端会在控制连接和数据流请求中提交该值，由服务端精确校验端口配置及账号归属。服务端不会把监听地址下发给客户端。客户端反向规则不支持 `listen_port`，该字段即使为空也会按规则报错，配置文件保持原样。`virtual` 时支持 `":<业务端口>"` 和 `"<证书 IPv4>:<业务端口>"` 两种格式，不支持域名或 IPv6。`:端口` 会优先采用 `server_addr` 中已被证书授权的 IPv4；否则证书必须只有一个可用的非本地 IPv4 SAN。证书存在多个候选且无法由 `server_addr` 确定时，必须填写完整的 `IPv4:端口`。显式 IPv4 必须存在于 `tls.ca_cert_file` 所引用服务端证书的 IP SAN 中。虚拟端口不从 `server_target` 推导，也不能使用 `server_addr` 的隧道端口。同一证书 IP 可以配置不同业务端口，但同一 IP:端口只能出现一次；单个客户端配置最多 48 个虚拟端点。

`forwards[].server_target`：目标地址。`client_to_server` 时表示服务端侧要访问的目标，必须是 `127.0.0.1` 或私有 IP:端口；`server_to_client` 时表示客户端侧要访问的 `127.0.0.1:端口`，例如 `127.0.0.1:3307`。反向目标由客户端实际使用，并随反向请求上报用于运行日志和诊断；服务端不连接该目标，也不以它选择保留端口或修改客户端配置。客户端可以配置多条 `server_to_client` 规则，每条规则都必须填写对应的 `listen_addr` 和 `server_target`。

`direction: virtual` 是标准 64 位 Windows 客户端的会话级端点接管。客户端每次连接时先完成登录，将本次准备就绪的多条虚拟规则合并到一次提权 helper 会话，统一请求管理员授权。WinDivert 只拦截证书 IP 与配置业务端口的组合，分别反射到本次会话随机分配的 `127.0.0.1` 回环端口。取消授权或共享 helper 在运行中异常退出会使本批虚拟规则不可用，不影响普通和反向规则；单独停止一条规则只关闭对应入口。它不添加网卡 IP、不修改路由，也不接管同一 IP 的其他端口；因此 `server_addr` 的隧道端口仍按原网络路径访问。过滤句柄随提权子进程关闭，断开或异常退出后不保留持久状态。

