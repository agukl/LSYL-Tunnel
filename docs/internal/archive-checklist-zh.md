# 归档清单

本文用于项目交付、备份或迁移前检查。归档时要区分“源码归档”和“发布归档”，不要把本机运行态、私钥、日志和临时文件混入源码包。

## 1. 当前项目边界

项目根目录：

```text
<project-root>
```

项目名称：

```text
LSYL Tunnel
```

核心业务：

- 客户端：GUI 进程模式 + 托盘值守，账号密码登录，正向/反向端口转发。
- 服务端：Windows 服务承载隧道入口，Web 运维台管理配置、用户、转发端口、安全参数和运行详情。
- 身份认证：用户名 + 密码哈希。
- 传输保护：TLS。
- 客户端本地密码保护：服务端公钥密封的短期凭据。
- 服务端反向代理：服务端保存本机回环端口配置，客户端主动激活时尝试监听；异常离线后由反向控制心跳释放激活状态。

## 2. 源码归档必须包含

```text
README.md
go.mod
go.sum
.gitignore
release.cmd
src/
deploy/
docs/
```

说明：

- `src/client/` 包含客户端 GUI、隧道逻辑、配置模板、图标和客户端打包用公开服务端证书目录。
- `src/server/` 包含服务端 CLI、Windows 服务入口、GUI 外壳、Web 运维台、配置模板和隧道逻辑。
- `src/internal/` 包含协议、传输、密码哈希、凭据密封和 Windows 服务公共代码。
- `src/cmd/` 包含管理工具：密码哈希工具和 TLS 文件生成工具。
- `deploy/` 包含 Windows 构建、运行、服务、打包、安装器、签名和自检脚本。
- `docs/` 包含中文设计、部署、配置、安全和运维文档。
- `release.cmd` 是发布主入口，用于一键测试、打包、签名和验证。

## 3. 源码归档建议排除

```text
build/bin/client/
build/bin/server/
dist/
tmp/
logs/
data/
certs/
secrets/
*.key
*.p12
*.pfx
*.pem
```

原因：

- `build/bin/client/`、`build/bin/server/`、`dist/` 是构建和打包产物，可以重新生成。
- `tmp/` 是 GUI 临时地址、测试截图或临时运行文件。
- `logs/` 是本机运行日志。
- `data/` 是服务端运行状态，例如封禁 IP 持久化结果。
- `certs/` 可能包含 `server.key`、登录密封私钥等本机私钥，不能进入源码归档。
- `secrets/` 是客户端本地密封凭据目录，不应归档。

## 4. 发布归档必须包含

安装器：

```text
dist/installers/LSYL-Tunnel-Client-Setup.exe
dist/installers/LSYL-Tunnel-Server-Setup.exe
```

免安装分发包：

```text
dist/LSYL Tunnel Client/
dist/LSYL Tunnel Server/
```

发布前确认：

- 客户端安装器只包含普通 GUI、Win7 轻量客户端、图标、配置和公开服务端证书；卸载由 Inno 内置逻辑完成，不再随安装器落地客户端卸载脚本。
- 服务端安装器包含 `lsyl-tunnel-server-svc.exe`，并使用 Inno 内置逻辑生成证书、注册服务和卸载服务，不再随安装器落地服务端安装/卸载脚本。
- 客户端安装器不包含任何客户端 Windows 服务注册逻辑。
- 客户端分发包不包含客户端服务程序。
- 客户端 `cert/server.crt` 必须是准备发给客户端的公开服务端证书。
- 服务端 `server.key`、登录密封私钥和运行数据不得放入客户端发布包。
- 正式对外发布时，应配置代码签名证书并签名包内 exe 与安装器。
- 正式发布前应按 `docs/internal/release-readiness-checklist-zh.md` 清零阻断项。

## 5. 发布前自检命令

```powershell
cmd /c deploy\windows\test\selfcheck.cmd
cmd /c release.cmd
cmd /c release.cmd /verify-only
```

客户端服务禁用检查已经固化在：

```text
deploy/windows/test/selfcheck.cmd
```

自检通过时，表示源码、脚本、文档和客户端分发包没有重新引入客户端 Windows 服务。

## 6. 证书和密钥归档规则

可以归档或分发：

```text
src/client/cert/server.crt
dist/LSYL Tunnel Client/cert/server.crt
certs/server.crt
```

禁止进入普通源码归档或客户端包：

```text
certs/server.key
certs/login-*.key
server.key
*.key
secrets/
```

如果需要做灾备密钥归档，应单独加密保存，并限制管理员访问，不能和源码包或客户端安装包混放。

## 7. 归档前清理建议

安全清理临时目录：

```powershell
Remove-Item -Recurse -Force .\tmp\* -ErrorAction SilentlyContinue
```

不要默认清理：

```text
certs/
logs/
data/
dist/
```

这些目录可能用于本机运行、问题追踪、安装验证或发布交付。只有确认不再需要时再手动删除。

## 8. 当前推荐交付包

给开发人员：

```text
源码归档：包含第 2 节，排除第 3 节。
```

给部署人员：

```text
发布归档：dist/installers/ 下两个安装器，加 docs/deployment-zh.md、docs/internal/release-signing-zh.md 和 docs/internal/release-readiness-checklist-zh.md。
```
