# Windows 脚本索引

Windows 相关辅助脚本统一放在 `deploy\windows\`。

发布主入口只有一个：

```cmd
release.cmd
```

`release.cmd` 负责测试、打包、安装器构建、签名和产物校验；这里的其它脚本主要用于开发调试、手动服务管理和一次性定制。

## 当前保留的入口

### 1. 开发构建

```cmd
deploy\windows\build.cmd all
deploy\windows\build.cmd server
deploy\windows\build.cmd client
deploy\windows\build.cmd profile
```

说明：

- `all`：构建服务端、客户端、Profile 工具
- `server`：构建服务端 CLI、GUI、Windows 服务和管理工具
- `client`：构建客户端 CLI、GUI
- `profile`：构建独立的 Profile 工具

### 2. 开发运行

```cmd
deploy\windows\run.cmd server
deploy\windows\run.cmd server-gui
deploy\windows\run.cmd client
deploy\windows\run.cmd client-gui
```

说明：

- `server`：前台启动服务端 CLI
- `server-gui`：启动服务端本地管理台
- `client`：前台启动客户端 CLI
- `client-gui`：启动客户端 GUI

### 3. 开发证书

```cmd
deploy\windows\cert\init-server.cmd "localhost,127.0.0.1"
deploy\windows\cert\init-server.cmd "vpn.example.com,203.0.113.10"
```

用途：

- 在开发目录生成或重建服务端 TLS 文件
- 输出到根目录 `certs\server.crt` 和 `certs\server.key`

### 4. 手动服务管理

```cmd
deploy\windows\service\server.cmd install
deploy\windows\service\server.cmd start
deploy\windows\service\server.cmd status
deploy\windows\service\server.cmd stop
deploy\windows\service\server.cmd uninstall
```

只保留服务端 Windows 服务管理脚本；客户端不注册 Windows 服务。

### 5. 分发包生成

```cmd
deploy\windows\app\package-client.cmd
deploy\windows\app\package-server.cmd
deploy\windows\app\package-profile.cmd
deploy\windows\app\write-dist-tools.cmd
```

用途：

- 刷新 `dist\LSYL Tunnel Client`
- 刷新 `dist\LSYL Tunnel Server`
- 刷新 `dist\LSYL Tunnel Profile Tool`
- 向 `dist` 根目录写入安装器构建入口和可选的 Inno 编译器

### 6. 安装器构建

源码侧通常不直接调用安装器脚本，优先使用：

```cmd
release.cmd
release.cmd /package-only
```

实施侧如果只拿到 `dist`，使用：

```cmd
dist\make-installers.cmd
dist\LSYL Tunnel Client\make-installer.cmd
dist\LSYL Tunnel Server\make-installer.cmd
```

源码中的对应模板和复制入口保留在：

```text
deploy\windows\inno\
```

### 7. 签名

```cmd
deploy\windows\sign\init-selfsigned-codesign.cmd
deploy\windows\sign\sign-release.cmd
```

常见做法：

- 开发机自签：`release.cmd /local-sign`
- 正式签名：配置证书后执行 `release.cmd`

### 8. 自检

```cmd
deploy\windows\test\selfcheck.cmd
```

它会检查：

- Go 测试
- 二进制构建
- 关键脚本布局
- 分发包结构
- 安装器模板
- 签名与文本资源基本完整性

## 推荐使用方式

日常最常用的其实只有这几条：

```cmd
release.cmd
release.cmd /package-only
deploy\windows\build.cmd all
deploy\windows\run.cmd server-gui
deploy\windows\run.cmd client-gui
deploy\windows\test\selfcheck.cmd
```

如果只是部署交付，优先围绕 `release.cmd` 和 `dist\make-installers.cmd` 工作；如果只是开发调试，优先围绕 `build.cmd` 和 `run.cmd` 工作。
