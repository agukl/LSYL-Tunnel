# 版本发布说明

## 2.0.1

本版本为兼容性补丁发布。

- 程序版本升级为 `2.0.1`，Windows 文件版本同步为 `2.0.1.0`，Android 客户端 `versionCode` 递增为 `3`，`versionName` 和 `client_version` 同步为 `2.0.1`。
- LSYL 握手协议保持 `protocol_version: 2`；服务端 `compatibility.min_client_version` 保持 `"2.0.0"`，2.0.0 客户端可以连接 2.0.1 服务端。
- 客户端和服务端配置结构版本均不变。服务端新增的入口连接追踪限制字段均有默认值，保留的 2.0.0 配置可直接使用。
- 服务端强化入口资源保护：失败追踪和连接来源追踪均增加过期清扫、容量上限、逐出统计；入口异常日志改为有界异步写入，永久封禁命中仅记录聚合事件。
- 服务端核心层强制 `monitor_addr` 使用回环地址；反向转发约束下沉到配置校验，安装覆盖时使用严格 YAML 结构兼容检查。
- Android 客户端使用有上限工作线程池，任一方向复制结束即关闭对端连接，避免半关闭连接长期占用资源。

## 2.0.0

本版本补充客户端右下角低调版本展示。客户端未连接时显示客户端版本；登录成功后，服务端会在成功握手响应中返回 `server_version`，客户端右下角同步显示客户端和服务端版本。

- 程序版本升级为 `2.0.0`，Windows 文件版本同步为 `2.0.0.0`；Android 客户端 `versionName`、`client_version` 和 `protocol_version` 同步到本版本。
- LSYL 握手成功响应新增 `server_version`，本次按协议字段变化处理，`protocol_version` 升级为 `2`。
- 服务端默认 `compatibility.min_client_version` 升级为 `"2.0.0"`，默认 `compatibility.protocol_version` 升级为 `2`；旧协议客户端会被拒绝连接。
- 客户端配置结构未变化，客户端 `config_version` 和 `requires.min_client_version` 不变。
- 服务端配置结构未变化，服务端 `config_version` 不变；默认服务端配置要求 `requires.min_server_version: "2.0.0"`。
- 覆盖安装保留旧服务端配置时，未写 `compatibility.protocol_version` 的旧配置会按当前默认值补齐；如果旧配置显式保留 `compatibility.protocol_version: 1`，需要同步改为 `2`。

## 1.1.0

本版本聚焦服务端防护、日志目录收口、GUI 运维入口和 Windows 发布链路。

### 服务端防护

- 增加入口连接层内存限制：全局并发、单 IP 并发、单 IP 新连接速率。
- 增加业务数据层内存限制：单账号并发连接和每连接传输速率。
- 将永久封禁命中、非 TLS、HTTP 探测等明显无效请求前移到入口连接层处理。
- 正常放通连接仍由后续认证、业务控制和数据流日志记录，入口层不额外落正常放通流水。

### 服务端 GUI

- “服务安全”页增加入口连接层并发和速率限制配置。
- 新增“用户并发”页，用于查看和调节单账号并发、每连接速率限制。
- 运行总览去掉监控地址统计块，减少重复信息。

### 日志与目录

- 生产服务端日志按业务类型拆到 `logs/request`、`logs/business`、`logs/entry-traffic`、`logs/flow-traffic`、`logs/service`。
- 生产持久化数据继续收口到 `data`，例如运行状态和永久封禁名单。
- 源码开发运行产物继续使用 `runtime/data` 和 `runtime/logs`，避免和分发包路径混用。

### 发布链路

- 一键发布入口保持为根目录 `release.cmd`。
- 打包脚本在旧 `dist` 目录被占用时支持原地刷新，减少 Windows 资源管理器占用导致的失败。
- `release.cmd` 默认对安装包内主线 Go 二进制启用 `garble` 混淆和 `-trimpath -ldflags "-s -w"` 瘦身。
- Windows exe 版本资源、manifest、Inno 安装包版本和自检脚本统一到 `1.1.0 / 1.1.0.0`。
