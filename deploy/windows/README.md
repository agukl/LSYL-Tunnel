# Windows 脚本命令地图

本文是 `deploy\windows\` 及根目录发布脚本的命令索引。生产部署流程见 `docs\deployment\windows-deployment-zh.md`。

## 推荐入口

日常发布只从根目录入口开始：

```cmd
release.cmd
release.cmd /hosts "vpn.example.com,203.0.113.10"
release.cmd /local-sign
release.cmd /skip-test
release.cmd /bundle-inno
release.cmd /no-client-kit
release.cmd /verify-only
```

命令边界：

| 命令 | 用途 | 是否改 `dist` |
| --- | --- | --- |
| `release.cmd` | 完整发布：测试、构建临时包、生成三端安装产物、保留客户端 kit、签名、校验、写清单 | 是 |
| `release.cmd /hosts "..."` | 发布前重建服务端 TLS 证书，并同步 `src\client\cert\server.crt` | 是 |
| `release.cmd /local-sign` | 创建或复用本机自签证书后完整发布 | 是 |
| `release.cmd /skip-test` | 跳过 `go test ./src/...` 后完整发布 | 是 |
| `release.cmd /bundle-inno` | 在客户端重打包目录旁额外内置 Inno Setup 编译器 | 是 |
| `release.cmd /no-client-kit` | 完整发布，但最终 `dist` 不保留客户端现场重打包目录 | 是 |
| `release.cmd /verify-only` | 只校验当前 `dist` 产物和签名 | 否 |

默认 `dist` 保留：

```text
dist\installers\LSYL-Tunnel-Client-Setup.exe
dist\installers\LSYL-Tunnel-Server-Setup.exe
dist\installers\LSYL-Tunnel-Android.apk
dist\LSYL Tunnel Client\
dist\make-installers.cmd
dist\README.txt
dist\release-manifest.txt
dist\release-manifest.json
```

使用 `/no-client-kit` 时不保留：

```text
dist\LSYL Tunnel Client\
dist\make-installers.cmd
dist\README.txt
```

实施人员拿到带客户端 kit 的 `dist` 后，只允许重新生成客户端安装器：

```cmd
dist\make-installers.cmd
dist\LSYL Tunnel Client\make-installer.cmd
```

服务端安装器和 Android APK 是发布产物，不从最终 `dist` 重新生成。

## 完整打包链路

完整 `release.cmd` 的链路是：

```text
源码 + src/client/conf + src/client/cert + src/server/conf
  -> deploy\windows\build.cmd
  -> build\bin
  -> deploy\windows\app\package-client.cmd
  -> deploy\windows\app\package-server.cmd
  -> deploy\windows\app\build-android-apk.cmd
  -> build\tmp\dist-work
  -> dist.stage
  -> dist
```

阶段职责：

| 阶段 | 入口脚本 | 主要输出 |
| --- | --- | --- |
| 构建二进制 | `deploy\windows\build.cmd client/server` | `build\bin\client`、`build\bin\server` |
| 生成临时客户端包 | `deploy\windows\app\package-client.cmd` | `build\tmp\dist-work\LSYL Tunnel Client` |
| 生成临时服务端包 | `deploy\windows\app\package-server.cmd` | `build\tmp\dist-work\LSYL Tunnel Server` |
| 生成或复制 Android APK | `deploy\windows\app\build-android-apk.cmd` | `build\tmp\dist-work\android\LSYL-Tunnel-Android.apk` |
| 组装最终发布 | `release.cmd` | `dist` |

`build\tmp\dist-work` 是临时工作目录，不作为交付物。`dist.stage` 是最终 `dist` 的暂存目录，只有全部安装产物生成并校验通过后才替换 `dist`。

## 构建与运行脚本

```cmd
deploy\windows\build.cmd all
deploy\windows\build.cmd server
deploy\windows\build.cmd client
deploy\windows\build.cmd client-package
deploy\windows\build.cmd win7-lite
deploy\windows\build.cmd profile
deploy\windows\build-win7-lite.cmd
deploy\windows\run.cmd server
deploy\windows\run.cmd server-gui
deploy\windows\run.cmd client
deploy\windows\run.cmd client-gui
deploy\windows\run.cmd client-lite
```

Win7 Lite 构建可用环境变量：

```cmd
set "GO120EXE=C:\Go120\bin\go.exe"
set "ALLOW_GO120_DOWNLOAD=1"
```

默认找不到 Go 1.20 时直接失败，不自动下载。查找顺序是 `GO120EXE`、PATH、`tool\go1.20.14`；只有显式设置 `ALLOW_GO120_DOWNLOAD=1` 时，脚本才会下载 Go 1.20 到 `tool\go1.20.14`。

## App 分发脚本

这些脚本通常由 `release.cmd` 间接调用，开发调试时才单独执行：

```cmd
deploy\windows\app\package-client.cmd
deploy\windows\app\package-client.cmd "D:\out\LSYL Tunnel Client"

deploy\windows\app\package-server.cmd
deploy\windows\app\package-server.cmd "D:\out\LSYL Tunnel Server"

deploy\windows\app\build-android-apk.cmd
deploy\windows\app\build-android-apk.cmd "D:\out\LSYL-Tunnel-Android.apk"

deploy\windows\app\write-dist-tools.cmd
deploy\windows\app\write-dist-tools.cmd "D:\out\dist"
```

说明：

| 命令 | 用途 | 默认输出 |
| --- | --- | --- |
| `package-client.cmd [目录]` | 重新构建客户端安装包所需二进制，复制客户端配置、证书、WinDivert 运行时及许可材料，清理运行态字段 | `build\tmp\dist-work\LSYL Tunnel Client` |
| `package-server.cmd [目录]` | 重新构建服务端安装包所需二进制，复制服务端配置和 Inno 模板 | `build\tmp\dist-work\LSYL Tunnel Server` |
| `build-android-apk.cmd [apk]` | 构建或复制 Android APK 到目标文件 | 指定 APK |
| `write-dist-tools.cmd [dist]` | 给带客户端 kit 的 dist 写入客户端重打包入口和 README | 指定 dist |

Android 构建可用环境变量：

```cmd
set "GRADLE_EXE=D:\gradle\bin\gradle.bat"
set "ANDROID_GRADLE_TASK=:app:assembleDebug"
set "ANDROID_GRADLE_ARGS=--stacktrace"
set "ANDROID_OFFLINE=1"
set "MOBILE_APK=D:\signed\LSYL-Tunnel-Android.apk"
```

未设置 `MOBILE_APK` 时，脚本必须能运行 Gradle，并会清理旧的 `mobile\android\app\build\outputs\apk` 后重新构建；不会默认复用历史 APK。

标准 64 位 Windows 客户端的虚拟端点接管依赖 `third_party\windivert\2.2.2\x64`。打包脚本只复制已固定版本的 DLL、驱动、许可和对应源码归档；Win7 Lite 与 Android 构建链不引用该目录。

## Inno 安装器脚本

```cmd
deploy\windows\inno\bundle-inno.cmd
deploy\windows\inno\make-client-installer.cmd
deploy\windows\inno\make-server-installer.cmd
deploy\windows\inno\make-server-installer.cmd "D:\out\installers"
```

说明：

- `make-client-installer.cmd` 会被复制为客户端包内的 `make-installer.cmd`，实施侧通常调用复制后的版本。
- `make-server-installer.cmd` 会被复制到临时服务端包，正式发布由 `release.cmd` 调用。
- `make-installers.cmd` 存在于默认带客户端 kit 的最终 `dist` 中，并且只重新生成客户端安装器。
- `package-client.iss` 和 `package-server.iss` 是 Inno 模板，不直接作为命令运行。

安装器脚本查找 `ISCC.exe` 的顺序：

```text
INNO_SETUP_ISCC
PATH 中的 iscc.exe
tool\inno\ISCC.exe
tool\Inno Setup 6\ISCC.exe
dist\tools\inno\ISCC.exe
%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe
%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe
%ProgramFiles%\Inno Setup 6\ISCC.exe
```

## 签名与发布清单脚本

```cmd
deploy\windows\sign\init-selfsigned-codesign.cmd
deploy\windows\sign\sign-release.cmd package
deploy\windows\sign\sign-release.cmd installers
deploy\windows\sign\sign-release.cmd client-package
deploy\windows\sign\sign-release.cmd server-package
deploy\windows\sign\sign-release.cmd client-installer
deploy\windows\sign\sign-release.cmd server-installer

powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\sign\write-release-manifest.ps1
```

签名范围：

| Scope | 对象 |
| --- | --- |
| `package` | 临时客户端包和临时服务端包中的 exe |
| `installers` | `dist\installers` 或当前发布暂存目录中的客户端和服务端安装器 |
| `client-package` | 客户端包 exe |
| `server-package` | 服务端包 exe |
| `client-installer` | 客户端安装器 |
| `server-installer` | 服务端安装器 |
| `all` | package + installers |

APK 不走 Authenticode 签名；发布清单会记录 APK 文件哈希。

## 自检与清理

```cmd
deploy\windows\test\selfcheck.cmd
```

需要清理本地构建缓存时可直接运行底层脚本：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\clean-build.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\clean-build.ps1 -DryRun
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\clean-build.ps1 -All
```

默认清理：

```text
build\bin
build\tmp
build\tools
build\gocache
build\gomodcache
dist.stage
dist.previous
tmp
mobile\android\.gradle
mobile\android\.kotlin
mobile\android\build
mobile\android\app\build
src\client\tmp
src\server\tmp
```
