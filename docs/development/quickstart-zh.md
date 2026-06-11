# 快速开始

本文用于本机开发和功能验证。生产部署前请阅读 [Windows 部署与安装](../deployment/windows-deployment-zh.md)、[服务端管理员指南](../deployment/server-admin-zh.md)、[客户端用户指南](../deployment/client-user-zh.md) 和 [安全模型](../system/security-zh.md)。

## 1. 构建程序

```powershell
cmd /c deploy\windows\build.cmd all
```

构建后主要产物：

```text
build\bin\server\lsyl-tunnel-server.exe
build\bin\server\lsyl-tunnel-server-svc.exe
build\bin\server\lsyl-tunnel-server-gui.exe
build\bin\client\lsyl-tunnel-client.exe
build\bin\client\lsyl-tunnel-client-gui.exe
build\bin\client\lsyl-tunnel-client-lite.exe
```

## 2. 生成服务端 TLS 文件

本机测试：

```powershell
cmd /c deploy\windows\cert\init-server.cmd "localhost,127.0.0.1"
```

生成文件：

```text
certs\server.crt
certs\server.key
```

客户端需要信任 `server.crt`；`server.key` 只留在服务端。

客户端分发时会默认携带 `src\client\cert\` 目录中的公开证书。测试或打包前可把服务端公开证书复制到：

```text
src\client\cert\server.crt
```

## 3. 配置服务端

配置文件：

```text
src\server\conf\server.yaml
```

推荐通过服务端 GUI 修改用户和转发端口：

```powershell
cmd /c deploy\windows\run.cmd server-gui
```

手写配置时，用户密码建议使用哈希：

```powershell
build\bin\server\lsyl-tunnel-passwd.exe -password "强密码"
```

## 4. 运行客户端

```powershell
cmd /c deploy\windows\run.cmd client-gui
```

客户端 GUI 默认是“进程模式 + 托盘值守”：登录成功后在当前 GUI 进程内启动本地监听；关闭窗口会隐藏到托盘；退出客户端才会停止隧道。

## 5. 自检

```powershell
cmd /c deploy\windows\test\selfcheck.cmd
```

自检会执行 Go 测试、构建、密码工具、TLS 工具、客户端/服务端卸载脚本、分发包结构和精简运行目录检查。涉及 Windows 服务安装/启动/停止/卸载的动作需要管理员授权，普通自检不会主动申请 UAC。

## 6. 生成分发包和安装器

完整安装、卸载和 Inno Setup 说明见 [Windows 部署与安装](../deployment/windows-deployment-zh.md)。这里仅列常用命令。

推荐使用根目录发布入口，它会自动执行测试、打包、安装器构建、签名和产物验证：

```powershell
cmd /c release.cmd
```

如果要同时重新生成服务端 TLS 证书，并把公开证书同步到客户端打包目录：

```powershell
cmd /c release.cmd /hosts "vpn.example.com,203.0.113.10"
```

开发或内部测试需要本机自签代码签名时：

```cmd
cmd /c release.cmd /local-sign
```

只生成客户端/服务端分发目录、不编译 Inno 安装器：

```powershell
cmd /c release.cmd /package-only
```

生产环境服务端安装时会生成自签 TLS 文件。客户端实际连接的域名或 IP 应写入服务端安装向导，或者用于发布前的 `/hosts` 参数。

`release.cmd /package-only` 会在开发机已安装 Inno Setup 6 时，把命令行编译器带入 `dist\tools\inno`。实施人员拿到 `dist` 后通常可以直接运行 `dist\make-installers.cmd`；如果没有内置编译器，再安装 Inno Setup 6 或设置 `INNO_SETUP_ISCC`。服务端安装向导会询问证书主机名/IP。

