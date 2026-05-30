# 网络连接流程与项目所在层级

本文说明一条通用网络连接从发起到关闭的生命周期，并标出 LSYL Tunnel 实际运行在哪些阶段。它用于帮助部署人员、管理员和安全审查人员理解本项目的网络边界。

## 1. 总体定位

LSYL Tunnel 不是虚拟网卡 VPN，也不接管系统路由表。

它是一个运行在应用层的 TLS 隧道和 TCP 端口代理：

- 不转发任意 IP 包。
- 不创建虚拟网卡。
- 不改变系统默认路由。
- 只代理配置好的 TCP 端口。
- 客户端和服务端之间使用 TLS 保护账号、凭据和业务字节流。
- 项目只在连接进入应用进程之后才能认证、授权、转发和统计。

## 2. 通用网络连接生命周期

```mermaid
flowchart TD
    A["业务程序发起连接<br/>例如浏览器、RDP、SQL 客户端"] --> B["本机协议栈准备连接"]
    B --> C{"是否需要 DNS 解析?"}
    C -->|是| D["DNS 查询<br/>域名 -> IP"]
    C -->|否| E["已有 IP 地址"]
    D --> E

    E --> F["路由选择<br/>本机路由表 / 默认网关 / VPN 路由"]
    F --> G["本机防火墙检查"]
    G --> H["NAT / 出口网络"]
    H --> I["公网或内网链路传输"]
    I --> J["服务端入口防火墙 / 安全组"]
    J --> K["服务端 TCP 监听端口"]

    K --> L["TCP 三次握手<br/>SYN -> SYN/ACK -> ACK"]
    L --> M{"TCP 建连成功?"}
    M -->|否| M1["连接失败<br/>timeout / refused / no route"]
    M -->|是| N["连接进入服务端进程<br/>Accept 成功"]

    N --> O{"是否启用 TLS?"}
    O -->|是| P["TLS 握手<br/>版本协商 / 证书校验 / 密钥交换"]
    O -->|否| Q["明文应用协议"]
    P --> R{"TLS 成功?"}
    R -->|否| R1["TLS 失败<br/>证书错误 / 版本不兼容 / 握手超时"]
    R -->|是| S["应用层协议握手"]
    Q --> S

    S --> T["认证与授权<br/>账号 / token / 凭据 / 权限规则"]
    T --> U{"认证授权成功?"}
    U -->|否| U1["拒绝连接<br/>auth_failed / denied / blocked"]
    U -->|是| V{"是否是代理或隧道场景?"}

    V -->|否| W["业务服务直接处理请求<br/>HTTP / SQL / RDP / 自定义协议"]
    V -->|是| X["代理端选择目标<br/>配置端口 / 用户权限 / 目标地址"]

    X --> Y["代理端连接目标服务<br/>Dial target"]
    Y --> Z{"目标连接成功?"}
    Z -->|否| Z1["目标不可达<br/>target_unreachable / refused / timeout"]
    Z -->|是| AA["双向数据转发<br/>client <-> proxy <-> target"]

    W --> AB["业务数据交换"]
    AA --> AB

    AB --> AC{"连接是否保持?"}
    AC -->|继续| AB
    AC -->|任意一端关闭| AD["连接关闭流程"]
    AC -->|网络异常| AE["异常断连<br/>reset / timeout / keepalive failed"]

    AE --> AD
    AD --> AF["释放资源<br/>关闭 socket / goroutine / fd"]
    AF --> AG["记录统计<br/>耗时 / 上下行流量 / 活跃连接减少 / 关闭原因"]

    subgraph OS_NET["系统和网络层主要负责"]
      D
      F
      G
      H
      I
      J
      L
    end

    subgraph APP_LAYER["应用或项目主要负责"]
      N
      P
      S
      T
      X
      Y
      AA
      AD
      AG
    end
```

## 3. LSYL Tunnel 所在位置

```mermaid
flowchart LR
    A["系统网络层<br/>DNS / 路由 / TCP"] --> B["TLS 层<br/>证书和加密通道"]
    B --> C["LSYL 协议层<br/>login / health / open / reverse"]
    C --> D["认证授权层<br/>用户 / 凭据 / 端口权限"]
    D --> E["目标连接层<br/>连接本机目标服务"]
    E --> F["透明转发层<br/>TCP 字节流 ProxyPair"]
    F --> G["统计日志层<br/>连接数 / 流量 / 业务记录"]

    style C fill:#e7f8f4,stroke:#0b9186
    style D fill:#e7f8f4,stroke:#0b9186
    style E fill:#e7f8f4,stroke:#0b9186
    style F fill:#e7f8f4,stroke:#0b9186
    style G fill:#e7f8f4,stroke:#0b9186
```

本项目主要运行在以下阶段：

- TLS 连接建立后，读取 LSYL 协议请求。
- 校验账号密码或客户端本地密封凭据。
- 按服务端配置和用户授权判断是否允许访问目标。
- 对目标服务发起 TCP 连接。
- 在客户端、服务端和目标服务之间透明复制 TCP 字节流。
- 在连接关闭时记录耗时、流量、连接结果和错误原因。

## 4. 正向代理连接位置

正向代理是 `client_to_server`，典型用途是用户通过客户端本机端口访问服务端侧服务。

```mermaid
sequenceDiagram
    participant App as 本机业务程序
    participant Client as LSYL 客户端
    participant Server as LSYL 服务端
    participant Target as 服务端侧目标服务

    App->>Client: 连接客户端本机监听端口
    Client->>Server: 建立 TCP + TLS
    Client->>Server: 发送 open 请求
    Server->>Server: 认证账号和凭据
    Server->>Server: 校验目标权限
    Server->>Target: 连接目标服务
    Target-->>Server: 目标连接成功
    Server-->>Client: 返回 connected
    App<<->>Client: 本地 TCP 数据
    Client<<->>Server: TLS 隧道数据
    Server<<->>Target: 目标 TCP 数据
    App-->>Client: 任意一端关闭
    Client-->>Server: 关闭隧道流
    Server-->>Target: 关闭目标连接
```

正向代理不在业务流中插入应用层心跳。业务数据进入转发阶段后，项目只做透明字节流转发。

客户端为了展示服务端可用性，会额外发起独立的 `health` 短连接探测。`health` 不打开目标端口，不计入业务连接流，也不写业务日志。

## 5. 反向代理连接位置

反向代理是 `server_to_client`，典型用途是服务端创建本机被动入口，由客户端主动连接服务端并激活该入口。

```mermaid
sequenceDiagram
    participant Inbound as 访问服务端被动端口的程序
    participant Server as LSYL 服务端
    participant Client as LSYL 客户端
    participant Target as 客户端侧目标服务

    Client->>Server: 建立反向控制连接 reverse_listen
    Server->>Server: 认证账号和端口权限
    Server->>Server: 创建或复用服务端被动监听端口
    Server-->>Client: reverse listen activated
    Server-->>Client: 周期性 reverse_ping
    Client-->>Server: reverse_pong

    Inbound->>Server: 连接服务端被动端口
    Server->>Client: 通知 reverse_connect
    Client->>Server: 新建 reverse_stream
    Server->>Server: 绑定入站连接和客户端 stream
    Client->>Target: 连接客户端侧目标服务
    Inbound<<->>Server: 服务端被动端口 TCP 数据
    Server<<->>Client: TLS 隧道数据
    Client<<->>Target: 客户端目标 TCP 数据
```

反向代理的控制通道有应用层心跳，用于检测客户端异常离线并释放激活状态。这个心跳只存在于反向控制通道，不进入正向代理业务数据流。

## 6. 项目可统计和不可统计的边界

| 阶段 | 是否由项目直接统计 | 说明 |
|---|---|---|
| DNS 解析 | 否 | 失败只能从客户端连接错误中间接看到。 |
| 路由、防火墙、NAT | 否 | 主要由系统、网络设备、云安全组或防火墙负责。 |
| TCP 半连接、SYN backlog | 否 | 未进入应用进程的连接通常不会出现在项目日志中。 |
| TCP Accept 成功后的连接 | 部分 | 服务端能看到已进入进程的连接，但未必已经完成 TLS 和协议握手。 |
| TLS 握手 | 部分 | 客户端能看到证书、版本、信任链错误；服务端能看到部分握手失败。 |
| LSYL 协议请求 | 是 | `login`、`health`、`open`、`reverse_listen`、`reverse_stream` 会进入请求日志。 |
| 认证失败和封禁 | 是 | 认证失败、封禁和解封状态由服务端维护。 |
| 授权拒绝 | 是 | 用户无权访问目标时记录策略拒绝。 |
| 目标连接失败 | 是 | 目标不可达、连接拒绝、超时会被记录。 |
| 数据转发连接 | 是 | 成功进入转发后统计活跃连接、累计连接、耗时和上下行流量。 |
| 业务协议内部行为 | 否 | 项目不解析 RDP、SQL、HTTP 等业务协议内容。 |

## 7. 统计口径

`login`：

- 用于用户登录和凭据发放。
- 成功会计入服务端认证成功。
- 成功会写业务日志。
- 不计入业务数据流连接数。

`health`：

- 用于客户端后台探测服务端是否可达、凭据是否仍有效。
- 会进入请求日志。
- 不写业务日志。
- 不计入服务端认证成功统计。
- 不计入业务数据流连接数。

`open`：

- 用于正向代理。
- 成功进入数据转发后，计入活跃连接和累计连接。
- 关闭时记录耗时、上下行流量和关闭事件。
- 授权失败或目标不可达不会计入业务数据流连接数。

`reverse_listen`：

- 用于反向代理控制通道。
- 负责激活服务端被动入口。
- 控制通道本身不计入业务数据流连接数。
- 激活、失败、关闭会写业务日志。

`reverse_stream`：

- 用于反向代理数据流。
- 成功绑定入站连接并进入数据转发后，计入活跃连接和累计连接。
- 关闭时记录耗时、上下行流量和关闭事件。

## 8. 对外说明建议

如果需要向运维、安全软件厂商或现场管理员解释网络行为，可以使用下面这段描述：

> LSYL Tunnel 是账号认证的 TLS 加密 TCP 端口代理。服务端监听配置的隧道入口，客户端主动连接服务端。项目不创建虚拟网卡，不修改系统路由，不转发任意 IP 包，只代理管理员配置的 TCP 端口。TLS 用于保护账号凭据和业务字节流；服务端根据用户授权决定是否连接目标服务；连接建立后项目透明复制 TCP 数据，并记录连接元信息、结果、耗时和流量。
