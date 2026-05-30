# 版本号升级手册

本文记录 LSYL Tunnel 发布版本号升级时需要同步修改的位置，避免下次只改了安装包版本，却漏掉 Windows 可执行文件版本、自检脚本或文档。

## 版本号格式

项目里同时使用两种版本格式：

- 发布/安装包版本：`1.0.1`
- Windows 文件版本：`1.0.1.0`

升级时先确定目标版本。例如从 `1.0.1` 升级到 `1.0.2` 时：

- `APP_VERSION=1.0.2`
- `WINDOWS_FILE_VERSION=1.0.2.0`

## 必改位置

### 1. Inno 安装包版本

修改 `#define AppVersion`：

- `deploy/windows/inno/package-client.iss`
- `deploy/windows/inno/package-server.iss`

示例：

```iss
#define AppVersion "1.0.2"
```

### 2. Windows manifest 版本

把所有 `assemblyIdentity` 的 `version` 改成四段版本：

- `src/client/cmd/lsyl-tunnel-client/app.manifest`
- `src/client/cmd/lsyl-tunnel-client-gui/app.manifest`
- `src/client/cmd/lsyl-tunnel-client-lite/app.manifest`
- `src/cmd/lsyl-tunnel-profile/app.manifest`
- `src/cmd/lsyl-tunnel-cert/app.manifest`
- `src/cmd/lsyl-tunnel-passwd/app.manifest`
- `src/server/cmd/lsyl-tunnel-server/app.manifest`
- `src/server/cmd/lsyl-tunnel-server-gui/app.manifest`
- `src/server/cmd/lsyl-tunnel-server-svc/app.manifest`

示例：

```xml
version="1.0.2.0"
```

### 3. Windows 版本资源

注意：`app.manifest` 不等于 exe 的 `FileVersion`。发布自检读取的是 exe 的 Windows 版本资源，通常来自各命令目录下的 `rsrc.syso`。

改完 manifest 后，需要用项目当前的资源生成方式重新生成对应 `rsrc.syso`，否则新构建的 exe 仍可能显示旧的 `FileVersion`。

需要关注的资源文件：

- `src/client/cmd/lsyl-tunnel-client/rsrc.syso`
- `src/client/cmd/lsyl-tunnel-client-gui/rsrc.syso`
- `src/client/cmd/lsyl-tunnel-client-lite/rsrc_windows_386.syso`
- `src/client/cmd/lsyl-tunnel-client-lite/rsrc_windows_amd64.syso`
- `src/cmd/lsyl-tunnel-profile/rsrc.syso`
- `src/cmd/lsyl-tunnel-cert/rsrc.syso`
- `src/cmd/lsyl-tunnel-passwd/rsrc.syso`
- `src/server/cmd/lsyl-tunnel-server/rsrc.syso`
- `src/server/cmd/lsyl-tunnel-server-gui/rsrc.syso`
- `src/server/cmd/lsyl-tunnel-server-svc/rsrc.syso`

### 4. 发布自检版本

修改自检脚本里期望的 `FileVersion`：

- `deploy/windows/test/selfcheck.cmd`

搜索并替换：

```text
FileVersion -ne '1.0.1.0'
```

为：

```text
FileVersion -ne '1.0.2.0'
```

### 5. 文档说明

同步更新版本说明：

- `docs/customization-zh.md`
- 本文档里的示例版本，如有必要

## 不建议手动改的位置

以下目录通常是构建产物或工具链缓存，不要把它们当作源头改版本：

- `build/`
- `dist/`
- `tmp/`

如果已经生成过发布包，升级版本后应该重新构建，让产物自然刷新。

## 推荐操作顺序

1. 确定目标版本，例如 `1.0.2` / `1.0.2.0`。
2. 修改两个 Inno 脚本的 `AppVersion`。
3. 修改所有 `app.manifest` 的 `assemblyIdentity version`。
4. 重新生成所有相关 `rsrc.syso`。
5. 修改 `deploy/windows/test/selfcheck.cmd` 的期望 `FileVersion`。
6. 更新 `docs/customization-zh.md` 和必要发布说明。
7. 重新构建。
8. 运行自检和测试。

## 检查命令

确认源码、部署脚本和文档里没有旧版本：

```powershell
rg -n "1\.0\.1|1\.0\.1\.0" src deploy docs release.cmd README.md
```

确认新版本出现的位置：

```powershell
rg -n "1\.0\.2|1\.0\.2\.0|AppVersion" src deploy docs release.cmd README.md
```

构建并验证：

```cmd
cmd /c deploy\windows\build.cmd all
cmd /c deploy\windows\test\selfcheck.cmd
go test ./...
```

发布前验证：

```cmd
cmd /c release.cmd /verify-only
```

## 当前版本

当前发布版本：

- 安装包版本：`1.0.1`
- Windows 文件版本：`1.0.1.0`

