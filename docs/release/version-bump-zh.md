# 版本号升级手册

本文记录 LSYL Tunnel 升级发布版本时必须同步修改的位置。目标是避免只改安装器版本，却漏掉 Windows exe 文件版本、版本资源、自检脚本或文档入口。

当前版本：

- 发布/安装包版本：`1.1.0`
- Windows 文件版本：`1.1.0.0`

版本格式约定：

- `APP_VERSION`：三段版本，例如 `1.1.0`，用于 Inno 安装器、README 和发布说明。
- `WINDOWS_FILE_VERSION`：四段版本，例如 `1.1.0.0`，用于 Windows manifest、exe 版本资源和自检。

## 一页清单

升级版本时按这张表逐项处理：

| 类型 | 文件 | 更新内容 |
| --- | --- | --- |
| 根入口 | `README.md` | `Current release version` 和版本升级手册入口。 |
| 中文文档入口 | `docs/README-zh.md` | 当前版本号。 |
| 发布说明 | `docs/release/release-notes-zh.md` | 增加新版本章节，概括功能、运维和发布链路变化。 |
| 固定项说明 | `docs/deployment/customization-zh.md` | “当前版本”和 `selfcheck` 期望版本。 |
| 版本升级手册 | `docs/release/version-bump-zh.md` | 当前版本、示例版本、检查命令。 |
| Inno 安装包 | `deploy/windows/inno/package-client.iss` | `#define AppVersion`。 |
| Inno 安装包 | `deploy/windows/inno/package-server.iss` | `#define AppVersion`。 |
| 发布自检 | `deploy/windows/test/selfcheck.cmd` | `FileVersion -ne '<WINDOWS_FILE_VERSION>'`。 |
| Windows manifest | `src/client/cmd/lsyl-tunnel-client/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/client/cmd/lsyl-tunnel-client-gui/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/client/cmd/lsyl-tunnel-client-lite/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/cmd/lsyl-tunnel-profile/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/cmd/lsyl-tunnel-cert/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/cmd/lsyl-tunnel-passwd/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/server/cmd/lsyl-tunnel-server/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/server/cmd/lsyl-tunnel-server-gui/app.manifest` | `assemblyIdentity version`。 |
| Windows manifest | `src/server/cmd/lsyl-tunnel-server-svc/app.manifest` | `assemblyIdentity version`。 |
| Windows 版本资源 | `src/client/cmd/lsyl-tunnel-client/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/client/cmd/lsyl-tunnel-client-gui/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/client/cmd/lsyl-tunnel-client-lite/rsrc_windows_386.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/client/cmd/lsyl-tunnel-client-lite/rsrc_windows_amd64.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/cmd/lsyl-tunnel-profile/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/cmd/lsyl-tunnel-cert/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/cmd/lsyl-tunnel-passwd/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/server/cmd/lsyl-tunnel-server/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/server/cmd/lsyl-tunnel-server-gui/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |
| Windows 版本资源 | `src/server/cmd/lsyl-tunnel-server-svc/rsrc.syso` | exe `FileVersion` / `ProductVersion`。 |

## 推荐顺序

下面以从 `1.1.0 / 1.1.0.0` 升到 `1.1.1 / 1.1.1.0` 为例。

1. 确定版本：

```text
APP_VERSION=1.1.1
WINDOWS_FILE_VERSION=1.1.1.0
```

2. 修改文本版本号：

```powershell
rg -n "1\.1\.0|1\.1\.0\.0" README.md docs deploy src
```

需要人工确认后替换，不要替换 `go.mod` 里的 Go 语言版本，也不要替换协议字段、TLS 版本或文档示例里和产品版本无关的数字。

3. 修改两个 Inno 脚本：

```iss
#define AppVersion "1.1.1"
```

4. 修改所有 `app.manifest`：

```xml
version="1.1.1.0"
```

5. 重新生成或更新所有 `rsrc.syso`。

`app.manifest` 不等于 exe 的 `FileVersion`。发布自检读取的是 exe 的 Windows 版本资源，通常来自命令目录下的 `rsrc.syso`。如果只改了 manifest，没有同步 `rsrc.syso`，新构建的 exe 仍会显示旧版本。

当前仓库没有单独提交资源生成脚本。如果产品名、公司名、图标和 manifest 嵌入不变，只做版本号升级，可以用下面的临时 Node 脚本把现有 `.syso` 从旧四段版本更新到新四段版本：

```powershell
$env:OLD_WINDOWS_FILE_VERSION='1.1.0.0'
$env:NEW_WINDOWS_FILE_VERSION='1.1.1.0'
@'
const fs = require('fs');
const oldVersion = process.env.OLD_WINDOWS_FILE_VERSION;
const newVersion = process.env.NEW_WINDOWS_FILE_VERSION;
if (!/^\d+\.\d+\.\d+\.\d+$/.test(oldVersion) || !/^\d+\.\d+\.\d+\.\d+$/.test(newVersion)) {
  throw new Error('OLD_WINDOWS_FILE_VERSION and NEW_WINDOWS_FILE_VERSION must be four-part versions.');
}
const paths = [
  'src/client/cmd/lsyl-tunnel-client/rsrc.syso',
  'src/client/cmd/lsyl-tunnel-client-gui/rsrc.syso',
  'src/client/cmd/lsyl-tunnel-client-lite/rsrc_windows_386.syso',
  'src/client/cmd/lsyl-tunnel-client-lite/rsrc_windows_amd64.syso',
  'src/cmd/lsyl-tunnel-profile/rsrc.syso',
  'src/cmd/lsyl-tunnel-cert/rsrc.syso',
  'src/cmd/lsyl-tunnel-passwd/rsrc.syso',
  'src/server/cmd/lsyl-tunnel-server/rsrc.syso',
  'src/server/cmd/lsyl-tunnel-server-gui/rsrc.syso',
  'src/server/cmd/lsyl-tunnel-server-svc/rsrc.syso',
];
const parseVersion = (v) => v.split('.').map((n) => Number(n));
const fixedDwords = (v) => {
  const parts = parseVersion(v);
  return [((parts[0] << 16) | parts[1]) >>> 0, ((parts[2] << 16) | parts[3]) >>> 0];
};
const oldText = Buffer.from(oldVersion, 'utf16le');
const newText = Buffer.from(newVersion, 'utf16le');
const [newMS, newLS] = fixedDwords(newVersion);
const sig = Buffer.from([0xbd, 0x04, 0xef, 0xfe]);
for (const p of paths) {
  const buf = fs.readFileSync(p);
  let strings = 0;
  let idx = 0;
  while ((idx = buf.indexOf(oldText, idx)) !== -1) {
    newText.copy(buf, idx);
    strings++;
    idx += oldText.length;
  }
  let fixed = 0;
  idx = 0;
  while ((idx = buf.indexOf(sig, idx)) !== -1) {
    buf.writeUInt32LE(newMS, idx + 8);
    buf.writeUInt32LE(newLS, idx + 12);
    buf.writeUInt32LE(newMS, idx + 16);
    buf.writeUInt32LE(newLS, idx + 20);
    fixed++;
    idx += sig.length;
  }
  if (strings !== 2 || fixed !== 1) {
    throw new Error(`${p}: expected 2 version strings and 1 fixed info block, got strings=${strings}, fixed=${fixed}`);
  }
  fs.writeFileSync(p, buf);
  console.log(`${p}: ${oldVersion} -> ${newVersion}`);
}
'@ | node -
```

如果要同时替换图标、公司名、产品名或版权，不能只用上面的版本补丁，应使用资源生成工具重新生成所有 `.syso`。

6. 修改自检脚本期望版本：

```text
FileVersion -ne '1.1.1.0'
```

7. 更新发布说明和当前版本文字：

- `README.md`
- `docs/README-zh.md`
- `docs/release/release-notes-zh.md`
- `docs/deployment/customization-zh.md`
- `docs/release/version-bump-zh.md`

8. 构建并验证：

```cmd
cmd /c deploy\windows\test\selfcheck.cmd
cmd /c release.cmd
cmd /c release.cmd /verify-only
```

9. 确认 exe 文件版本：

```powershell
$targets=@(
  'build\bin\client\lsyl-tunnel-client.exe',
  'build\bin\client\lsyl-tunnel-client-gui.exe',
  'build\bin\client\lsyl-tunnel-client-lite.exe',
  'build\bin\profile\lsyl-tunnel-profile.exe',
  'build\bin\server\lsyl-tunnel-server.exe',
  'build\bin\server\lsyl-tunnel-server-gui.exe',
  'build\bin\server\lsyl-tunnel-server-svc.exe',
  'build\bin\server\lsyl-tunnel-cert.exe',
  'build\bin\server\lsyl-tunnel-passwd.exe'
)
foreach($t in $targets){
  $vi=(Get-Item $t).VersionInfo
  '{0} {1}' -f (Split-Path $t -Leaf), $vi.FileVersion
}
```

10. 确认旧版本号清零：

```powershell
rg -n "1\.1\.0|1\.1\.0\.0" README.md docs deploy src
```

如果命中旧版本，逐条判断是否应替换。正常情况下，除了历史发布说明需要保留旧版本章节外，当前版本和发布脚本不应残留旧版本。

## 不要手动改的地方

这些目录是构建产物、运行产物或本机缓存，不作为版本号源头修改：

- `build/`
- `dist/`
- `runtime/`
- `build/tmp/`
- `certs/`
- `mobile/android/app/build/`

升级版本后重新运行发布命令，让这些产物自然刷新。
