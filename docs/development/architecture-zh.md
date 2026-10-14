# 目录结构与代码边界

LSYL Tunnel 按客户端、服务端、共享层和部署脚本拆分。公网入口依次使用证书持有前置门、TLS 加密和账号认证；客户端预置服务端公开证书，服务端保存私钥，不使用客户端证书或完整 mTLS。

## 顶层结构

```text
src/
  client/
    cmd/                客户端 CLI / 标准 GUI / Win7 Lite 入口
    cert/               客户端打包用的服务端 TLS 信任文件，只放公开证书
    conf/               客户端默认配置
    front/              HTML / CSS / JavaScript，构建时嵌入标准客户端 GUI
    gui/                登录界面、托盘值守、窗口控制、本地管理 API
    lite/               Win7 原生轻量界面，不依赖 WebView2
    mobileprofile/      移动端和轻量客户端的 Profile 导入导出
    tunnel/             客户端隧道逻辑、登录探测、转发监听、配置加载
  server/
    assets/             服务端 GUI 和安装器图标资源
    cmd/                服务端 CLI / GUI / Windows 服务入口
    conf/               服务端默认配置
    front/              Web 运维平台页面资源，构建时嵌入服务端 GUI
    gui/                WebView2 外壳、本地管理 API、配置读写、Windows 服务控制
    tunnel/             服务端认证、授权、正反向转发、监控、运行事件
  internal/
    protocol/           客户端和服务端共享的握手协议
    transport/          证书持有前置门、TLS 配置、双向复制和每流速率限制
    netaddr/            Go 各入口共用的回环写法及业务目标 IP 范围规则
    version/            产品、协议、配置版本及版本比较
    passutil/           密码哈希与校验
    credseal/           客户端本地保存密码的服务端密封凭据
    winservice/         Windows 服务运行封装和按天日志
    webviewhost/        标准 Windows GUI 共用的 WebView2 宿主与运行时检查
  cmd/
    lsyl-tunnel-passwd/ 密码哈希工具
    lsyl-tunnel-cert/   服务端 TLS 文件生成工具
    lsyl-tunnel-profile/ 客户端配置档案和移动端配置导出工具

deploy/windows/
  build.cmd             标准 Go 构建入口
  build-win7-lite.cmd   Win7 独立工具链构建入口
  go-env.cmd            Go 环境选择
  run.cmd               开发运行入口
  cert/                 服务端 TLS 文件脚本
  service/              Windows 服务脚本
  app/                  源码打包、Android 构建和安装依赖准备
  inno/                 安装器模板、共享前置依赖、现场重打包脚本
  sign/                 代码签名初始化和签名脚本
  test/                 自检脚本

release.cmd             发布主入口：测试、打包、安装器、签名和验证
mobile/android/         Android 应用与 JVM 测试
third_party/            随产品分发的第三方组件及许可
tool/                   本机开发工具和依赖安装器缓存，不作为源码提交
```

## 进程与依赖

项目是模块化的客户端 / 服务端系统，不是微服务，也不是完整 VPN。三个客户端共享协议契约，但运行边界不同：

```text
标准 Windows 客户端
  WebView2 页面 -> 本地 GUI API -> client/tunnel（同一进程）
                                -> WinDivert helper（提权子进程，仅虚拟规则）

Win7 Lite -> client/tunnel（同一进程，Go 1.20 / windows/386）
          -> mobileprofile（配置导入）

Android Activity -> ForegroundService -> RuntimeController -> TunnelManager
                                                     -> Kotlin 协议 / TLS / SocketPipe

服务端管理台
  WebView2 页面 -> 本地 GUI API -> 配置文件 / 日志文件 / 服务控制
                              -> monitor_addr -> 独立 Windows 服务中的 server/tunnel
```

服务端 GUI 不承载转发，关闭管理台不应停止 Windows 服务。标准客户端 GUI 则内嵌隧道核心，隐藏窗口进入托盘与退出进程是两种不同操作。Android 转发由前台服务持有，不由 Activity 的页面生命周期持有。

Go 源码依赖方向是 `cmd -> gui/lite -> tunnel -> internal`；`mobileprofile` 复用客户端配置类型，`front` 只提供嵌入资源。核心 `tunnel` 不反向依赖 GUI、WebView2 或页面资源，客户端核心与服务端核心也不直接依赖彼此。`internal` 是共享基础能力集合，其中 `winservice` / `webviewhost` 属于平台适配，不应被网络核心引用。

Android 不复用 Go 二进制，必须独立维护协议、前置门、Profile 和错误码实现。跨端修改要同时验证 Go 与 Kotlin，不能只保证同一语言内的单元测试通过。

## 核心源码职责

服务端隧道核心位于 `src/server/tunnel`：

- `server.go`：启动、连接接收、握手和转发主链路。
- `lifecycle.go`：连接持有的入口并发名额、活动连接登记、后台任务管理和统一关闭。
- `authorization.go`：正向目标和反向监听授权。
- `connection_limiter.go`、`fail_tracker.go`：入口连接限制和失败来源跟踪。
- `monitor.go`、`maintenance.go`：本地监控接口、状态快照和后台清扫。
- `event_logs.go`、`runtime_events.go`：四类业务日志和内存事件。

服务端管理台后端位于 `src/server/gui`：`http_windows.go` 负责本地 HTTP 服务和路由，`admin_api_windows.go` 负责请求处理和状态组装，`admin_config_mapping_windows.go` 负责 GUI 与核心配置互转，`admin_forward_mapping_windows.go` 负责转发字段转换，`admin_monitor_client_windows.go` 负责访问服务端本地监控接口。

Profile 工具位于 `src/cmd/lsyl-tunnel-profile`：`main.go` 只负责命令分发，`commands.go` 处理命令参数，`profiles.go` 管理档案和目录切换，`inspect.go` 负责配置、证书和文件检查，`interactive.go` 提供交互菜单。

## 四层业务流

| 层次 | 主要工作 | 状态与记录 |
| --- | --- | --- |
| 入口连接 | TCP 接收、封禁命中、来源连接限制、证书持有前置门、TLS 握手 | 内存计数及有界 IP 表；异常记录走有界异步入口日志，永久封禁命中聚合 |
| 请求认证 | 有界 JSON 请求解析、协议版本检查、密码或密封凭据校验 | 请求流水；成功来源保护与失败追踪 |
| 业务控制 | 正向目标授权、反向端口激活、待回连流 ID 管理 | 业务流水、内存事件及按账号并发准入 |
| 数据流 | 连接目标、双向复制、单流双向合计速率限制、关闭回收 | 内存流量统计及流量日志；不逐包写业务日志 |

正向和反向的每条业务流目前各自建立隧道连接并完成认证，不是登录后复用一条多路复用连接。`saved_credential` 用于密封保存密码，不是跳过密码 KDF 的会话令牌。TLS 配置缓存、一次登录探测和共享虚拟 helper 不改变这个认证边界。

四类 JSONL 业务日志分别由 `event_logs.go` 写入 request、business、entry-traffic、flow-traffic 目录；服务日志还包含相同事件的文本归纳和服务运行信息，不能视为第五种业务模型。只有入口异常日志采用有界异步队列，其余日志仍同步写入。内存事件环用于运行展示，不代替持久日志。

入口并发名额随 TCP 连接持有，正向流、反向控制和反向数据流均在实际关闭时释放，不随请求处理函数返回提前释放。服务端拥有独立取消上下文；`Close()` 停止接收和任务登记、取消拨号与维护任务、关闭活动连接及反向监听，然后等待工作任务退出，最后排空入口日志队列并关闭四类日志文件。反向待回连的超时由工作任务持有，交接或退出后不保留游离定时回调。

## 配置契约

- Go 地址范围与回环写法由 `internal/netaddr` 统一，客户端预检不替代服务端最终授权。
- YAML 字段与版本结构、配置语义、运行时授权是不同检查阶段，不应在 GUI 中隐式修正规则来代替校验。
- 服务端安装前使用严格 YAML 结构校验；普通运行读取仍与安装检查分开，转发条目通过自定义解码拒绝未知字段。不能把安装检查当作整个运行配置解析器的替代品。
- `forwards[].name` 是客户端内唯一的运行标识和日志标签，不是服务端授权依据；两端可以不同。
- 配置归属、旧字段兼容和三种方向的地址矩阵集中维护在[配置参考](../system/config-reference-zh.md)，本页不再复制完整字段规则。

## 客户端边界

客户端负责：

- 读取指定的客户端配置；开发默认位于 `src/client/conf/client.yaml`，安装后位于应用目录的 `conf/client.yaml`。
- 校验服务端 TLS 身份。
- 使用用户名密码或短期密封凭据登录服务端。
- 按配置打开固定本地监听或 Windows 虚拟 IP 入口，或激活服务端反向被动端口。
- 在本地连接和服务端隧道之间复制流量。
- GUI 登录成功后进入托盘值守，关闭窗口不等于退出。

客户端不负责：

- 管理用户账号。
- 修改服务端授权。
- 判断最终访问权限。
- 展示复杂 TLS、密钥和服务端诊断细节。

客户端 GUI 的普通用户界面只暴露服务端地址、用户名、密码和连接状态。端口映射、TLS 文件、服务端名称等高级配置由管理员预置。

## 服务端边界

服务端负责：

- 读取指定的服务端配置；开发默认位于 `src/server/conf/server.yaml`，安装后位于应用目录的 `conf/server.yaml`。
- 监听隧道入口。
- 在 TLS 之前校验客户端是否持有与服务端私钥匹配的公开证书。
- 校验用户密码或密封凭据。
- 按用户授权决定是否允许访问目标。
- 执行正向目标可达性检查。
- 保存反向代理服务端本地端口配置，客户端激活时尝试监听；端口不可用时记录并由客户端自动重试。
- 维护运行事件、封禁 IP 和监控状态。

服务端不负责：

- 在线注册账号。
- 接收用户上传文件。
- 发放客户端证书。
- 做完整 VPN 路由。
- 主动连接客户端机器。

## 转发方向

`client_to_server`：客户端通过固定本地端口接收连接，服务端连接服务端侧目标。

`virtual`：仅在标准 Windows 客户端增加按证书授权 `IPv4:端口` 拦截和随机回环桥接。多条规则共享一次提权 helper，会话结束后撤销接管；服务端仍按 `client_to_server` 授权，不知道也不下发虚拟入口。证书选择、端口冲突及取消授权规则见[配置参考](../system/config-reference-zh.md)。

`server_to_client`：服务端配置被动入口，客户端通过规则中的 `listen_addr` 选择并主动激活对应入口，同时上报本地 `server_target` 供运行记录。服务端按监听地址和登录账号校验归属；收到访问后只发送不透明流 ID，客户端再回建数据流并连接本地目标。服务端不会主动拨入客户端网络，不用客户端目标选择端口，也不会把保留端口下发或写回客户端配置。

反向入口固定为服务端 `127.0.0.1:端口`，端口全局唯一且只归属一个已存在账号；同一账号可以拥有多个端口。客户端目标不用于选择服务端保留端口，规则名也不参与授权。

## GUI 边界

双端采用相同的源码分工，不另建前端框架或构建链：

| 修改内容 | 所在位置 |
| --- | --- |
| 页面结构、样式、交互 | 各端 `front/index.html`、`styles.css`、`app.js` |
| 静态资源嵌入 | 各端 `front/front.go`，新增文件时同步 `go:embed` |
| 应用启动与退出组织 | 各端 `gui/gui_windows.go` |
| 本地 HTTP 服务、路由与静态资源 | 各端 `gui/http_windows.go` |
| 原生窗口行为 | 各端 `gui/window_windows.go`；客户端托盘在 `tray_windows.go` |
| 开发目录与安装目录定位 | 各端 `gui/paths_windows.go` |
| 管理台内存日志与服务日志尾部读取 | 服务端 `gui/logging_windows.go`，不负责四类业务日志写入 |
| WebView2 初始化与运行时检查 | `src/internal/webviewhost`，不包含隧道业务 |

页面资源通过 `go:embed` 编入 exe，并由回环 HTTP 服务提供；更改页面后需要重新构建 GUI。发布机器不需要 Node，也不需要单独复制前端文件。`tunnel` 不依赖 `gui`、`front` 或 WebView2，Win7 Lite 和 Android 保持独立界面实现。

客户端 GUI：

- 内置隧道引擎，不再额外拉起普通客户端进程。
- 默认不注册 Windows 服务。
- 已连接时关闭窗口进入托盘值守。
- 退出客户端才会停止隧道。

服务端 GUI：

- 是本地 Web 运维平台，页面源码在 `src/server/front`。
- 只开放本机 `127.0.0.1` 管理 API。
- 只保留“重启服务”按钮，用于保存配置后重启 `LSYLTunnelServer` Windows 服务。
- 用户认证页只管理用户。
- 转发端口页按已有用户分配端口放通。
- 运行详情页展示封禁 IP、失败诊断、最近事件。

服务端安装器：

- 安装阶段生成自签服务端 TLS 文件，管理员只分发 `server.crt`。
- 默认注册手动启动的 `LSYLTunnelServer` 服务；安装向导确认后可设置为开机自启动。
- 安装、升级和默认卸载保留 `conf`、`certs`、`data`、`logs`。
- Inno 安装器不内置服务端 `.ps1/.cmd` 安装卸载脚本；分发包也不再保留手动安装/卸载入口，只保留安装器生成入口。
- 卸载前停止 GUI/服务进程并删除服务注册，默认只移除程序目录。

## 运行产物

以下目录是构建或运行产物，不是核心源码：

```text
build/bin/client/
build/bin/server/
build/tmp/
dist/
dist.stage/
dist.previous/
certs/
runtime/logs/
runtime/data/
```

`build/` 存放可重建产物，`dist.stage/` 和 `dist.previous/` 仅用于发布暂存和替换恢复，不是源码。保留失败发布目录可用于排查，不要直接在其中维护代码。新的开发临时文件统一放 `build/tmp/`，运行日志与持久数据统一放根目录 `runtime/`，不再在 `src/server/` 下创建同名产物目录。

发布由根目录 `release.cmd` 组织：源码与本地交付配置 -> `build/bin` -> `build/tmp/dist-work` -> `dist.stage` -> `dist`。安装产物验证成功后才替换最终目录；正式发布清单在替换后写入。最终 `dist` 的客户端 kit 只是现场重新生成带配置安装器的输入，不是另一套源码。脚本职责与详细参数只维护在[命令地图](../../deploy/windows/README.md)。

`src/client/conf/`、`src/server/conf/`、`src/client/cert/` 和根目录 `certs/` 是本地配置及交付身份输入；`runtime/` 是开发运行数据。这些内容即使被 Git 忽略也不能视为可随意删除的缓存。现场调整只在 `dist/LSYL Tunnel Client` 中进行，程序和脚本修改必须回到 `src`、`deploy`，再重新打包。

## 维护优先级

后续重构按职责和资源所有权推进，不为缩短文件而拆包：

1. 固定跨端契约：Go 与 Kotlin 共享协议 / Profile 测试向量，覆盖字段、错误码、地址范围、前置门和关闭语义。当前两套实现的测试各自存在，但没有统一测试向量。
2. 收窄大文件职责：优先分离服务端请求分发与认证编排、客户端连接与转发生命周期、Windows helper IPC 与数据包重定向，以及服务端页面的状态 / 表格 / 表单逻辑。保持现有包边界和无 Node 构建链。

上述内容是当前代码审查发现的维护方向，不代表已经完成运行逻辑重构。不要把增加目录或框架本身当作优化目标。

## 维护约束

- 窗口生命周期、HTTP 接口与隧道生命周期分别维护；仅在确有共享职责时提取公共模块。
- 测试与实现相邻。纯页面资源测试放在 `front`，Windows 集成测试使用 `_windows_test.go`；重构后执行 `go test ./src/...` 和 Windows 自检。
- 运行时性能边界保持不变：客户端缓存只读 TLS 配置并复用虚拟转发 helper；服务端事件使用固定环形缓冲，日志复用序列化结果，IP 表只在定时或容量触顶时清扫。
- 长期文档按功能维护，历史方案和执行计划由 Git 历史保留；不要为每轮改动另建一套平行说明。
