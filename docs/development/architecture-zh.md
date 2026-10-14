# 目录结构与代码边界

LSYL Tunnel 按客户端、服务端、共享层和部署脚本拆分。公网入口依次使用证书持有前置门、TLS 加密和账号认证；客户端预置服务端公开证书，服务端保存私钥，不使用客户端证书或完整 mTLS。

## 顶层结构

```text
src/
  client/
    cmd/                客户端 CLI / GUI 入口
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
    transport/          TLS 配置和双向流量复制
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

## 核心源码职责

服务端隧道核心位于 `src/server/tunnel`：

- `server.go`：启动、关闭、连接接收、握手和转发主链路。
- `authorization.go`：正向目标和反向监听授权。
- `connection_limiter.go`、`fail_tracker.go`：入口连接限制和失败来源跟踪。
- `monitor.go`、`maintenance.go`：本地监控接口、状态快照和后台清扫。
- `event_logs.go`、`runtime_events.go`：四类业务日志和内存事件。

服务端管理台后端位于 `src/server/gui`：`http_windows.go` 负责本地 HTTP 服务和路由，`admin_api_windows.go` 负责请求处理和状态组装，`admin_config_mapping_windows.go` 负责 GUI 与核心配置互转，`admin_forward_mapping_windows.go` 负责转发字段转换，`admin_monitor_client_windows.go` 负责访问服务端本地监控接口。

Profile 工具位于 `src/cmd/lsyl-tunnel-profile`：`main.go` 只负责命令分发，`commands.go` 处理命令参数，`profiles.go` 管理档案和目录切换，`inspect.go` 负责配置、证书和文件检查，`interactive.go` 提供交互菜单。

## 客户端边界

客户端负责：

- 读取 `src/client/conf/client.yaml`。
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

- 读取 `src/server/conf/server.yaml`。
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

`virtual`：标准 64 位 Windows 客户端按 `listen_addr` 接管虚拟入口。`:端口` 从服务端信任证书自动选择唯一 IPv4，`IPv4:端口` 用于多 SAN 场景显式选择；最终 IP 必须来自证书 IPv4 SAN，业务端口不能等于 `server_addr` 端口。登录成功后，同一次连接的多条规则合并到一次提权 helper 会话，通过 WinDivert 将匹配端点分别反射到随机 `127.0.0.1` 监听，流映射使用有界内存表；未匹配端口保持原网络路径，不创建网卡别名或持久路由。用户取消管理员授权、规则停止或 helper 异常时只将受影响规则标记为不可用，不终止客户端连接；重新连接时再次申请授权。发往服务端时仍映射为 `client_to_server`，不改变服务端授权规则。

`server_to_client`：服务端配置被动入口，客户端通过规则中的 `listen_addr` 选择并主动激活对应入口，同时上报本地 `server_target` 供运行记录。服务端按监听地址和登录账号校验归属；收到访问后只发送不透明流 ID，客户端再回建数据流并连接本地目标。服务端不会主动拨入客户端网络，不用客户端目标选择端口，也不会把保留端口下发或写回客户端配置。

反向代理的服务端入口只能是 `127.0.0.1:端口`，例如 `127.0.0.1:18080`。同一个反向端口必须全局唯一并且只能归属一个已存在账号，同一账号可以绑定多个反向端口；账号停用时不能认证和激活。规则名仅用于显示和日志，任何方向都不参与服务端授权。

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

`build/` 存放可重建产物，`dist.stage/` 和 `dist.previous/` 仅用于发布暂存和替换恢复，不是源码。保留失败发布目录可用于排查，不要直接在其中维护代码。

`src/client/conf/`、`src/server/conf/`、`src/client/cert/` 和根目录 `certs/` 是本地配置及交付身份输入；`runtime/` 是开发运行数据。这些内容即使被 Git 忽略也不能视为可随意删除的缓存。现场调整只在 `dist/LSYL Tunnel Client` 中进行，程序和脚本修改必须回到 `src`、`deploy`，再重新打包。

## 维护约束

- 窗口生命周期、HTTP 接口与隧道生命周期分别维护；仅在确有共享职责时提取公共模块。
- 测试与实现相邻。纯页面资源测试放在 `front`，Windows 集成测试使用 `_windows_test.go`；重构后执行 `go test ./src/...` 和 Windows 自检。
- 运行时性能边界保持不变：客户端缓存只读 TLS 配置并复用虚拟转发 helper；服务端事件使用固定环形缓冲，日志复用序列化结果，IP 表只在定时或容量触顶时清扫。
- 长期文档按功能维护，历史方案和执行计划由 Git 历史保留；不要为每轮改动另建一套平行说明。
