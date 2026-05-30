# 固定项与替换指南

本文整理 LSYL Tunnel 中常见的固定项：证书、服务名、产品名、安装目录、端口、签名证书和版本信息。后续如果要做客户定制、品牌替换或正式发布，先按本文判断哪些可以现场替换，哪些属于发布身份，不建议频繁改。

## 1. 替换分级

### A. 现场配置项

这些可以按客户现场修改，不需要改源码：

- 客户端连接地址：`server_addr`
- 客户端用户名：`username`
- 客户端信任证书：`cert\server.crt`
- 服务端监听地址：`listen_addr`
- 服务端监控地址：`monitor_addr`
- 服务端用户、密码哈希、禁用状态
- 服务端转发端口规则
- 服务端 TLS 证书文件和私钥文件路径

### B. 发布配置项

这些用于构建安装包，发布前可以改，但改完要重新打包、重新签名、重新验证：

- 安装包内默认配置
- 服务端 TLS 主机名/IP
- 客户端内置的 `server.crt`
- 代码签名证书
- 安装器输出名称
- 版本号
- 图标

### C. 发布身份项

这些是软件身份的一部分，不建议频繁改。改了会影响升级、服务识别、杀软信誉和客户运维：

- 产品名：`LSYL Tunnel`
- 服务名：`LSYLTunnelServer`
- 服务显示名：`LSYL Tunnel Server`
- Inno 安装器 `AppId`
- 默认安装目录
- exe 文件名
- 代码签名证书主体

## 2. 服务端 TLS 证书

### 用途

服务端 TLS 证书用于让客户端确认“连接到的是正确服务端”，同时保护账号密码和业务流量。

### 文件

```text
certs\server.crt
certs\server.key
src\client\cert\server.crt
dist\LSYL Tunnel Client\cert\server.crt
```

### 替换方式

开发/发布前推荐用：

```cmd
cmd /c release.cmd /hosts "vpn.example.com,203.0.113.10"
```

该命令会：

- 生成或更新根目录 `certs\server.crt`
- 生成或更新根目录 `certs\server.key`
- 把 `certs\server.crt` 同步到 `src\client\cert\server.crt`

### 注意

- `server.crt` 可以发给客户端管理员。
- `server.key` 只能留在服务端。
- 如果服务端域名/IP 变了，需要重新生成证书并重新下发客户端安装包或替换客户端 `cert\server.crt`。
- 服务端安装器也会在目标机器缺少证书时生成 `certs\server.crt` 和 `certs\server.key`。

## 3. 客户端连接地址

客户端默认配置文件：

```text
src\client\conf\client.yaml
```

关键项：

```yaml
server_addr: vpn.example.com:3443
tls:
  ca_cert_file: ../cert/server.crt
  server_name: ""
```

### 替换方式

- 开发默认值：改 `src\client\conf\client.yaml`
- 一次性客户安装包：改 `dist\LSYL Tunnel Client\conf\client.yaml`，再运行 `dist\LSYL Tunnel Client\make-installer.cmd`

### 注意

- `server_addr` 是客户端要连接的服务端公网地址和端口。
- 如果证书里写的是域名，但 `server_addr` 用 IP，可以配置 `tls.server_name` 指定证书校验名称。
- 如果用 `release.cmd` 重建 dist，手工改过的 `dist\LSYL Tunnel Client` 会被源码配置覆盖。

## 4. 服务端监听与监控端口

服务端默认配置：

```text
src\server\conf\server.yaml
```

关键项：

```yaml
listen_addr: 0.0.0.0:3443
monitor_addr: 127.0.0.1:19111
```

### 替换方式

- 改 `src\server\conf\server.yaml` 后重新打包。
- 或改 `dist\LSYL Tunnel Server\conf\server.yaml` 后生成一次性安装包。

### 注意

- `listen_addr` 是客户端连接服务端的入口。
- `monitor_addr` 默认只监听本机，建议保持 `127.0.0.1`。
- 防火墙只需要按业务开放 `listen_addr` 和需要对外访问的反向端口。

## 5. 登录密封密钥

服务端配置：

```yaml
credential_seal:
  keys:
    - key_id: login-key-2026-08
      private_key_file: ../../../certs/login-key-2026-08.key
      public_key_file: ../../../certs/login-key-2026-08.pub
      expires_at: "2026-08-20T00:00:00+08:00"
      active: true
```

### 用途

客户端可以保存服务端密封过的短期登录凭据，避免长期保存明文密码。

### 替换方式

- 定期生成新的密封密钥对。
- 更新 `key_id`、私钥路径、公钥路径和过期时间。
- 新密钥设为 `active: true`。
- 老密钥可以短期保留用于兼容未过期凭据，过渡后删除。

### 注意

- 密封私钥只能留在服务端。
- 客户端不需要密封私钥。
- 密钥过期后，客户端需要重新输入密码获取新的密封凭据。

## 6. Windows 服务名

当前固定服务名：

```text
LSYLTunnelServer
```

相关文件：

```text
src\server\gui\service_windows.go
src\server\cmd\lsyl-tunnel-server-svc\main.go
deploy\windows\service\server.cmd
deploy\windows\inno\package-server.iss
deploy\windows\sign\write-release-manifest.ps1
docs\internal\windows-service-zh.md
```

### 是否建议替换

不建议频繁替换。服务名属于 Windows 服务身份：

- 改服务名会导致新旧服务并存风险。
- 旧版本卸载可能删不到新服务。
- 运维脚本、安装器、文档、误报申诉材料都要同步。
- 杀软和管理员看到的服务身份会变化。

### 必须替换时

1. 停止旧服务。
2. 卸载旧服务。
3. 修改所有服务名引用。
4. 修改发布清单和文档。
5. 重新打包、签名、安装验证。
6. 明确升级方案：旧服务名如何清理，新服务名如何注册。

## 7. 产品名、安装目录和安装器身份

当前产品名：

```text
LSYL Tunnel
```

客户端安装器：

```text
AppName=LSYL Tunnel Client
DefaultDirName={autopf}\LSYL Tunnel Client
OutputBaseFilename=LSYL-Tunnel-Client-Setup
AppId={{7FBA7BC8-2117-476E-8E3B-2BC00B6F33C1}
```

服务端安装器：

```text
AppName=LSYL Tunnel Server
DefaultDirName={autopf}\LSYL Tunnel Server
OutputBaseFilename=LSYL-Tunnel-Server-Setup
AppId={{A69942C1-D831-44D1-97C5-2E0DDB9FF3CA}
```

相关文件：

```text
deploy\windows\inno\package-client.iss
deploy\windows\inno\package-server.iss
deploy\windows\app\package-client.cmd
deploy\windows\app\package-server.cmd
deploy\windows\app\write-dist-tools.cmd
release.cmd
deploy\windows\sign\sign-release.ps1
deploy\windows\sign\write-release-manifest.ps1
src\client\gui\*.go
src\server\gui\*.go
src\client\cmd\*\app.manifest
src\server\cmd\*\app.manifest
src\cmd\*\app.manifest
```

### AppId 注意

- 同一个产品升级时，应保持 Inno `AppId` 不变。
- 如果是全新品牌、全新客户产品或分叉产品，可以换新的 `AppId`。
- 换 `AppId` 后，Windows 会认为是另一个安装项，不会自动覆盖旧产品。

## 8. 版本号

当前版本：

```text
1.0.1 / 1.0.1.0
```

相关文件：

```text
deploy\windows\inno\package-client.iss
deploy\windows\inno\package-server.iss
src\client\cmd\*\app.manifest
src\server\cmd\*\app.manifest
src\cmd\*\app.manifest
src\*\cmd\*\rsrc.syso
src\cmd\*\rsrc.syso
deploy\windows\test\selfcheck.cmd
```

### 注意

- 改 manifest 后需要重新生成 `rsrc.syso`。
- `selfcheck.cmd` 当前会检查 exe 文件版本为 `1.0.1.0`。
- 后续如果要频繁发版，建议把版本号集中到一个生成脚本里，不要手工逐文件改。

## 9. 代码签名证书

开发自签文件：

```text
certs\codesign-local.cer
certs\codesign-thumbprint.txt
```

正式发布推荐环境变量：

```cmd
set "LSYL_SIGN_CERT_PFX=D:\secure\codesign.pfx"
set "LSYL_SIGN_CERT_PASSWORD=证书密码"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
```

或者：

```cmd
set "LSYL_SIGN_CERT_SHA1=证书SHA1指纹"
set "LSYL_SIGN_TIMESTAMP_URL=http://timestamp.digicert.com"
```

### 注意

- 自签证书只适合开发或内部测试。
- 正式发布必须使用正式代码签名证书。
- PFX、密码、私钥不能放入项目目录和发布包。
- 签名证书尽量保持长期稳定，频繁换证书会影响信誉积累。

## 10. 图标和版本资源

图标文件：

```text
src\client\assets\client.ico
src\client\assets\client-connected.ico
src\server\assets\server.ico
deploy\windows\inno\assets\client.ico
deploy\windows\inno\assets\server.ico
```

版本资源：

```text
src\client\cmd\lsyl-tunnel-client\rsrc.syso
src\client\cmd\lsyl-tunnel-client-gui\rsrc.syso
src\server\cmd\lsyl-tunnel-server\rsrc.syso
src\server\cmd\lsyl-tunnel-server-gui\rsrc.syso
src\server\cmd\lsyl-tunnel-server-svc\rsrc.syso
src\cmd\lsyl-tunnel-cert\rsrc.syso
src\cmd\lsyl-tunnel-passwd\rsrc.syso
```

### 注意

- 换图标后需要重新生成 `rsrc.syso`。
- 安装器图标和 exe 图标都要同步。
- `selfcheck.cmd` 会检查所有发布 exe 是否带产品名、公司名、文件说明和版本号。

## 11. 推荐替换流程

### 现场换证书/地址

1. 确认服务端对外域名/IP 和端口。
2. 执行 `release.cmd /hosts "域名,IP"`。
3. 检查 `src\client\cert\server.crt` 已更新。
4. 修改 `src\client\conf\client.yaml` 的 `server_addr`。
5. 执行 `release.cmd /package-only` 或完整 `release.cmd`。
6. 交付 `dist` 或安装器。

### 品牌或产品名替换

1. 确认是否是全新产品。如果是，生成新的 Inno `AppId`。
2. 全局替换产品名、安装目录、快捷方式名、窗口标题、文档标题。
3. 同步替换 manifest 和版本资源。
4. 同步替换发布清单脚本和签名文档。
5. 重新生成 `rsrc.syso`。
6. 运行 `selfcheck.cmd`。
7. 重新签名并做真实安装验证。

### 服务名替换

1. 先确认必须替换。能不换就不换。
2. 写清升级策略：旧服务如何停止和删除。
3. 修改服务名、显示名、描述、安装器、服务脚本、发布清单、文档。
4. 验证安装、启动、停止、卸载、升级。
5. 验证旧服务残留不会影响新服务。

## 12. 当前建议

- 证书、地址、端口：按配置替换即可。
- 正式代码签名证书：用环境变量替换，不改源码。
- 产品名和服务名：短期不建议替换，保持稳定更利于杀软信誉和客户运维。
- 版本号：后续建议做集中版本生成，避免手工改多个 manifest 和自检脚本。
