#define AppName "LSYL Tunnel Client"
#define AppVersion "3.4.0"
#define WinDivertDLLSHA256 "c1e060ee19444a259b2162f8af0f3fe8c4428a1c6f694dce20de194ac8d7d9a2"
#define WinDivertDriverSHA256 "8da085332782708d8767bcace5327a6ec7283c17cfb85e40b03cd2323a90ddc2"
; This script is copied into the client distributable package:
;   LSYL Tunnel Client\installer\client.iss
; It compiles the files from that package directory, not from source.
#define SourceRoot ".."

[Setup]
AppId={{7FBA7BC8-2117-476E-8E3B-2BC00B6F33C1}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=LSYL Tunnel
DefaultDirName={autopf}\LSYL Tunnel Client
DefaultGroupName=LSYL Tunnel
DisableProgramGroupPage=yes
OutputDir=..\..\installers
OutputBaseFilename=LSYL-Tunnel-Client-Setup
SetupIconFile=..\assets\client.ico
UninstallDisplayIcon={app}\assets\client.ico
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog commandline
UsePreviousPrivileges=yes
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
CloseApplications=yes
RestartApplications=no
WizardStyle=modern

[Languages]
Name: "chinesesimplified"; MessagesFile: "Languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "快捷方式："; Flags: unchecked

[Dirs]
Name: "{app}\conf"; Permissions: users-modify; Flags: uninsneveruninstall
Name: "{app}\cert"; Permissions: users-modify; Flags: uninsneveruninstall
Name: "{app}\secrets"; Permissions: users-modify; Flags: uninsneveruninstall
Name: "{app}\tmp"
Name: "{app}\tmp\gui"; Permissions: users-modify
Name: "{app}\licenses\WinDivert\source"

[Files]
Source: "{#SourceRoot}\prerequisites\MicrosoftEdgeWebview2Setup.exe"; Flags: dontcopy
Source: "{#SourceRoot}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-lite.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\WinDivert.dll"; DestDir: "{app}\bin"; Flags: ignoreversion restartreplace; Check: ShouldInstallPinnedRuntime('{app}\bin\WinDivert.dll', '{#WinDivertDLLSHA256}')
Source: "{#SourceRoot}\bin\WinDivert64.sys"; DestDir: "{app}\bin"; Flags: ignoreversion restartreplace; Check: ShouldInstallPinnedRuntime('{app}\bin\WinDivert64.sys', '{#WinDivertDriverSHA256}')
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestName: "lsyl-tunnel-client-gui-quit.exe"; Flags: dontcopy
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestName: "lsyl-tunnel-client-gui-check.exe"; Flags: dontcopy
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestName: "lsyl-tunnel-client-gui-cleanup.exe"; Flags: dontcopy
Source: "{#SourceRoot}\assets\client.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\client-connected.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\conf\client.yaml"; DestDir: "{app}\conf"; Flags: ignoreversion uninsneveruninstall
Source: "{#SourceRoot}\cert\*"; DestDir: "{app}\cert"; Flags: ignoreversion recursesubdirs createallsubdirs uninsneveruninstall
Source: "{#SourceRoot}\licenses\WinDivert\*"; DestDir: "{app}\licenses\WinDivert"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#SourceRoot}\licenses\WebView2\*"; DestDir: "{app}\licenses\WebView2"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\LSYL Tunnel Client"; Filename: "{app}\bin\lsyl-tunnel-client-gui.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\client.ico"
Name: "{group}\LSYL Tunnel Lite (Win7)"; Filename: "{app}\bin\lsyl-tunnel-client-lite.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\client.ico"
Name: "{autodesktop}\LSYL Tunnel Client"; Filename: "{app}\bin\lsyl-tunnel-client-gui.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\client.ico"; Tasks: desktopicon

[InstallDelete]
Type: files; Name: "{app}\uninstall-client-app.cmd"
Type: files; Name: "{app}\uninstall-client-app.ps1"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\bin"
Type: filesandordirs; Name: "{app}\assets"
Type: filesandordirs; Name: "{app}\tmp"
Type: filesandordirs; Name: "{app}\licenses"

[Code]
var
  ClientInstallPrepared: Boolean;
  ClientInstallSucceeded: Boolean;
  ClientHadConfigFile: Boolean;
  ClientHadCertFile: Boolean;
  ClientHadArchiveConfigFile: Boolean;
  ClientHadArchiveCertFile: Boolean;
  ClientArchiveConfigFile: String;
  ClientArchiveCertFile: String;
  ClientRollbackConfigFile: String;
  ClientRollbackCertFile: String;
  ClientRollbackArchiveConfigFile: String;
  ClientRollbackArchiveCertFile: String;
  ClientQuitUseInstalledApp: Boolean;
  ClientMigrationNeeded: Boolean;
  ClientInstalledVersion: String;
  ClientRollbackOwned: Boolean;
  ClientRollbackFailed: Boolean;

#include "webview2-runtime.iss"

function ShouldInstallPinnedRuntime(FileName, ExpectedSHA256: String): Boolean;
var
  ExpandedFileName: String;
  ExistingSHA256: String;
begin
  ExpandedFileName := ExpandConstant(FileName);
  if not FileExists(ExpandedFileName) then
  begin
    Result := True;
    Exit;
  end;

  try
    ExistingSHA256 := GetSHA256OfFile(ExpandedFileName);
    Result := CompareText(ExistingSHA256, ExpectedSHA256) <> 0;
    if not Result then
      Log('Skipping unchanged pinned runtime: ' + ExpandedFileName);
  except
    Log('Unable to hash existing pinned runtime; replacement will be attempted: ' + ExpandedFileName);
    Result := True;
  end;
end;

function ClientQuitHelperPath: String;
begin
  Result := ExpandConstant('{tmp}\lsyl-tunnel-client-gui-quit.exe');
end;

function ClientConfigCheckHelperPath: String;
begin
  Result := ExpandConstant('{tmp}\lsyl-tunnel-client-gui-check.exe');
end;

function ClientUpgradeCleanupHelperPath: String;
begin
  Result := ExpandConstant('{tmp}\lsyl-tunnel-client-gui-cleanup.exe');
end;

function ClientRollbackDir: String;
begin
  Result := ExpandConstant('{app}\conf\.install-rollback');
end;

procedure CleanupClientQuitHelper;
begin
  DeleteFile(ClientQuitHelperPath());
end;

procedure CleanupClientConfigCheckHelper;
begin
  DeleteFile(ClientConfigCheckHelperPath());
end;

procedure CleanupClientUpgradeHelper;
begin
  DeleteFile(ClientUpgradeCleanupHelperPath());
end;

procedure CleanupClientRollbackFiles;
begin
  DelTree(ClientRollbackDir(), True, True, True);
end;

procedure CleanupClientInstallTemps;
begin
  CleanupClientQuitHelper();
  CleanupClientConfigCheckHelper();
  CleanupClientUpgradeHelper();
  if ClientRollbackOwned and not ClientRollbackFailed then
  begin
    CleanupClientRollbackFiles();
    ClientRollbackOwned := False;
  end;
end;

function RequestClientQuit: String;
var
  ResultCode: Integer;
  HelperPath: String;
begin
  Result := '';
  CleanupClientQuitHelper();
  if not DirExists(ExpandConstant('{app}')) then
    exit;
  if not FileExists(ExpandConstant('{app}\bin\lsyl-tunnel-client-gui.exe')) then
    exit;
  if ClientQuitUseInstalledApp then begin
    HelperPath := ExpandConstant('{app}\bin\lsyl-tunnel-client-gui.exe');
  end else begin
    ExtractTemporaryFile('lsyl-tunnel-client-gui-quit.exe');
    HelperPath := ClientQuitHelperPath();
  end;
  if not Exec(HelperPath, '/quit', ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
    if not ClientQuitUseInstalledApp then
      CleanupClientQuitHelper();
    Result := '无法请求 LSYL Tunnel Client 退出。安装器不会强制结束进程，请从托盘退出客户端后重试。';
    exit;
  end;
  if ResultCode <> 0 then begin
    if not ClientQuitUseInstalledApp then
      CleanupClientQuitHelper();
    Result := 'LSYL Tunnel Client 仍在运行。安装器不会强制结束进程，请从托盘退出客户端后重试。';
    exit;
  end;
  Sleep(800);
  if not ClientQuitUseInstalledApp then
    CleanupClientQuitHelper();
end;

function CleanupClientUpgradeConflicts: String;
var
  ResultCode: Integer;
  HelperPath: String;
  InstalledClientExe: String;
  ResultFile: String;
  Params: String;
  Detail: AnsiString;
  DetailText: String;
begin
  Result := '';
  CleanupClientUpgradeHelper();
  ExtractTemporaryFile('lsyl-tunnel-client-gui-cleanup.exe');
  HelperPath := ClientUpgradeCleanupHelperPath();
  InstalledClientExe := ExpandConstant('{app}\bin\lsyl-tunnel-client-gui.exe');
  ResultFile := ExpandConstant('{tmp}\client-upgrade-cleanup-result.txt');
  DeleteFile(ResultFile);
  Params :=
    '-installer-cleanup' +
    ' -installed-client-exe "' + InstalledClientExe + '"' +
    ' -result-file "' + ResultFile + '"';
  if not Exec(HelperPath, Params, ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
    Result := '无法启动客户端升级清理程序，请确认安装器具有管理员权限后重试。';
    CleanupClientUpgradeHelper();
    exit;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    DetailText := Trim(String(Detail));
    if DetailText = '' then
      DetailText := '旧客户端进程或 WinDivert 驱动仍在占用安装文件。';
    Result :=
      '无法清理旧客户端冲突，安装已停止。' + #13#10 + #13#10 +
      DetailText + #13#10 + #13#10 +
      '请关闭占用程序后重试；若驱动仍无法释放，请重启 Windows 后再次安装。';
  end;
  DeleteFile(ResultFile);
  CleanupClientUpgradeHelper();
end;

function CheckClientConfigCompatibility: String;
var
  ResultCode: Integer;
  ConfigFile: String;
  CheckerExe: String;
  ResultFile: String;
  Params: String;
  Detail: AnsiString;
  DetailText: String;
  FindRec: TFindRec;
begin
  Result := '';
  ConfigFile := ExpandConstant('{app}\conf\client.yaml');
  ClientMigrationNeeded := FileExists(ConfigFile);
  if FindFirst(ExpandConstant('{app}\conf\client.*.yaml'), FindRec) then begin
    try
      ClientMigrationNeeded := True;
    finally
      FindClose(FindRec);
    end;
  end;
  if not ClientMigrationNeeded then
    exit;

  if not GetVersionNumbersString(ExpandConstant('{app}\bin\lsyl-tunnel-client-gui.exe'), ClientInstalledVersion) then
    ClientInstalledVersion := '';

  CleanupClientConfigCheckHelper();
  ExtractTemporaryFile('lsyl-tunnel-client-gui-check.exe');
  CheckerExe := ClientConfigCheckHelperPath();
  ResultFile := ExpandConstant('{tmp}\client-config-compat-result.txt');
  DeleteFile(ResultFile);

  Params :=
    '-config-compat-check' +
    ' -config "' + ConfigFile + '"' +
    ' -source-version "' + ClientInstalledVersion + '"' +
    ' -backup-file "' + ClientRollbackDir() + '\migration.json"' +
    ' -result-file "' + ResultFile + '"';
  if not Exec(CheckerExe, Params, ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
    Result :=
      'Cannot start client config compatibility checker.' + #13#10 +
      CheckerExe;
    CleanupClientConfigCheckHelper();
    exit;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    DetailText := Trim(String(Detail));
    if DetailText = '' then
      DetailText := '无法确认已安装客户端版本是否支持升级。';
    Result :=
      '无法确认已安装客户端支持自动升级，安装已停止。' + #13#10 + #13#10 +
      DetailText + #13#10 + #13#10 +
      '当前配置：' + ConfigFile;
  end;
  CleanupClientConfigCheckHelper();
end;

function RunClientConfigMigration(Rollback: Boolean): Boolean;
var
  ResultCode: Integer;
  Params: String;
begin
  Result := True;
  ExtractTemporaryFile('lsyl-tunnel-client-gui-check.exe');
  Params := '-config-compat-check -backup-file "' + ClientRollbackDir() + '\migration.json"';
  if Rollback then
    Params := Params + ' -rollback'
  else
    Params := Params + ' -apply -config "' + ExpandConstant('{app}\conf\client.yaml') +
      '" -source-version "' + ClientInstalledVersion + '"';
  if not Exec(ClientConfigCheckHelperPath(), Params, ExpandConstant('{tmp}'), SW_HIDE,
    ewWaitUntilTerminated, ResultCode) then
    Result := False
  else
    Result := ResultCode = 0;
end;

function PreserveClientInstallAccount: String;
var
  ResultCode: Integer;
  ResultFile: String;
  Params: String;
  Detail: AnsiString;
begin
  Result := '';
  if not ClientHadConfigFile then
    exit;
  ExtractTemporaryFile('lsyl-tunnel-client-gui-check.exe');
  ResultFile := ExpandConstant('{tmp}\client-account-preserve-result.txt');
  DeleteFile(ResultFile);
  Params := '-config-compat-check -preserve-account' +
    ' -config "' + ExpandConstant('{app}\conf\client.yaml') + '"' +
    ' -source-config "' + ClientRollbackConfigFile + '"' +
    ' -result-file "' + ResultFile + '"';
  if not Exec(ClientConfigCheckHelperPath(), Params, ExpandConstant('{tmp}'), SW_HIDE,
    ewWaitUntilTerminated, ResultCode) then begin
    Result := '无法启动客户端账号迁移程序。';
    exit;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    Result := Trim(String(Detail));
    if Result = '' then
      Result := '无法将旧连接账号写入新版客户端配置。';
  end;
end;

function CheckInstalledClientConfig: String;
var
  ResultCode: Integer;
  ResultFile: String;
  Params: String;
  Detail: AnsiString;
begin
  Result := '';
  ExtractTemporaryFile('lsyl-tunnel-client-gui-check.exe');
  ResultFile := ExpandConstant('{tmp}\client-installed-config-result.txt');
  DeleteFile(ResultFile);
  Params := '-config-compat-check' +
    ' -config "' + ExpandConstant('{app}\conf\client.yaml') + '"' +
    ' -result-file "' + ResultFile + '"';
  if not Exec(ClientConfigCheckHelperPath(), Params, ExpandConstant('{tmp}'), SW_HIDE,
    ewWaitUntilTerminated, ResultCode) then begin
    Result := '无法启动新版客户端配置检查程序。';
    exit;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    Result := Trim(String(Detail));
    if Result = '' then
      Result := '新版安装包中的客户端配置不可用。';
  end;
end;

function TrimClientYamlValue(Value: String): String;
begin
  Result := Trim(Value);
  if Length(Result) >= 2 then begin
    if ((Copy(Result, 1, 1) = '"') and (Copy(Result, Length(Result), 1) = '"')) or
       ((Copy(Result, 1, 1) = #39) and (Copy(Result, Length(Result), 1) = #39)) then
      Result := Copy(Result, 2, Length(Result) - 2);
  end;
end;

function ReadClientConfigServerAddr(ConfigFile: String): String;
var
  RawText: AnsiString;
  Text: String;
  Line: String;
  Key: String;
  Value: String;
  LineEnd: Integer;
  P: Integer;
begin
  Result := '';
  if not LoadStringFromFile(ConfigFile, RawText) then
    exit;
  Text := String(RawText);
  while Text <> '' do begin
    LineEnd := Pos(#10, Text);
    if LineEnd > 0 then begin
      Line := Copy(Text, 1, LineEnd - 1);
      Delete(Text, 1, LineEnd);
    end else begin
      Line := Text;
      Text := '';
    end;
    if (Length(Line) > 0) and (Copy(Line, Length(Line), 1) = #13) then
      Delete(Line, Length(Line), 1);
    Line := Trim(Line);
    if (Line <> '') and (Copy(Line, 1, 1) <> '#') then begin
      P := Pos(':', Line);
      if P > 0 then begin
        Key := Trim(Copy(Line, 1, P - 1));
        if CompareText(Key, 'server_addr') = 0 then begin
          Value := Trim(Copy(Line, P + 1, Length(Line)));
          Result := TrimClientYamlValue(Value);
          exit;
        end;
      end;
    end;
  end;
end;

function IsClientArchiveSuffixChar(Ch: String): Boolean;
begin
  Result :=
    ((Ch >= 'a') and (Ch <= 'z')) or
    ((Ch >= 'A') and (Ch <= 'Z')) or
    ((Ch >= '0') and (Ch <= '9')) or
    (Ch = '.') or (Ch = '-') or (Ch = '_');
end;

function ClientArchiveSuffix(ServerAddr: String): String;
var
  I: Integer;
  Ch: String;
  LastUnderscore: Boolean;
begin
  Result := '';
  ServerAddr := Trim(ServerAddr);
  LastUnderscore := False;
  for I := 1 to Length(ServerAddr) do begin
    Ch := Copy(ServerAddr, I, 1);
    if IsClientArchiveSuffixChar(Ch) then begin
      Result := Result + Ch;
      LastUnderscore := False;
    end else if not LastUnderscore then begin
      Result := Result + '_';
      LastUnderscore := True;
    end;
  end;
  while (Length(Result) > 0) and
        ((Copy(Result, 1, 1) = '.') or (Copy(Result, 1, 1) = '_') or (Copy(Result, 1, 1) = '-')) do
    Delete(Result, 1, 1);
  while (Length(Result) > 0) and
        ((Copy(Result, Length(Result), 1) = '.') or (Copy(Result, Length(Result), 1) = '_') or (Copy(Result, Length(Result), 1) = '-')) do
    Delete(Result, Length(Result), 1);
  if Length(Result) > 120 then
    Result := Copy(Result, 1, 120);
  while (Length(Result) > 0) and
        ((Copy(Result, Length(Result), 1) = '.') or (Copy(Result, Length(Result), 1) = '_') or (Copy(Result, Length(Result), 1) = '-')) do
    Delete(Result, Length(Result), 1);
  if Result = '' then
    Result := 'unknown';
end;

function BackupClientRuntimeFiles: String;
var
  ConfigFile: String;
  CertFile: String;
  RollbackDir: String;
  ArchiveSuffix: String;
  ArchiveStarted: Boolean;
begin
  Result := '';
  ClientInstallPrepared := False;
  ClientInstallSucceeded := False;
  ArchiveStarted := False;
  ConfigFile := ExpandConstant('{app}\conf\client.yaml');
  CertFile := ExpandConstant('{app}\cert\server.crt');
  RollbackDir := ClientRollbackDir();
  ClientRollbackConfigFile := RollbackDir + '\client.yaml.rollback';
  ClientRollbackCertFile := RollbackDir + '\server.crt.rollback';
  ClientRollbackArchiveConfigFile := RollbackDir + '\client.archive.rollback';
  ClientRollbackArchiveCertFile := RollbackDir + '\server.archive.rollback';
  ClientHadConfigFile := FileExists(ConfigFile);
  ClientHadCertFile := FileExists(CertFile);
  ClientHadArchiveConfigFile := False;
  ClientHadArchiveCertFile := False;
  ClientArchiveConfigFile := '';
  ClientArchiveCertFile := '';
  if DirExists(RollbackDir) then begin
    Result := '存在上次安装留下的配置回滚数据，请先检查并恢复：' + RollbackDir;
    exit;
  end;
  if not ForceDirectories(ExpandConstant('{app}\conf')) then begin
    Result := '无法创建客户端配置目录，请检查安装目录权限后重试。';
    exit;
  end;
  if not CreateDir(RollbackDir) then begin
    Result := '无法创建安装回滚目录，请检查安装目录权限后重试。';
    exit;
  end;
  ClientRollbackOwned := True;
  if ClientHadConfigFile then begin
    if not CopyFile(ConfigFile, ClientRollbackConfigFile, False) then begin
      Result := '无法备份当前客户端配置，安装已停止。请检查安装目录权限后重试。';
      exit;
    end;
    ArchiveStarted := True;
    ArchiveSuffix := ClientArchiveSuffix(ReadClientConfigServerAddr(ConfigFile));
    ClientArchiveConfigFile := ExpandConstant('{app}\conf\client.' + ArchiveSuffix + '.yaml');
    ClientArchiveCertFile := ExpandConstant('{app}\cert\server.' + ArchiveSuffix + '.crt');
    ClientHadArchiveConfigFile := FileExists(ClientArchiveConfigFile);
    ClientHadArchiveCertFile := FileExists(ClientArchiveCertFile);
    if ClientHadArchiveConfigFile then begin
      if not CopyFile(ClientArchiveConfigFile, ClientRollbackArchiveConfigFile, False) then begin
        Result := '无法备份同名客户端配置归档，安装已停止。请检查安装目录权限后重试。';
        exit;
      end;
    end;
    if not CopyFile(ConfigFile, ClientArchiveConfigFile, False) then begin
      Result := '无法按服务端地址归档当前客户端配置，安装已停止。请检查安装目录权限后重试。';
      ClientInstallPrepared := ArchiveStarted;
      exit;
    end;
  end;
  if ClientHadCertFile then begin
    if not CopyFile(CertFile, ClientRollbackCertFile, False) then begin
      Result := '无法备份当前客户端证书，安装已停止。请检查安装目录权限后重试。';
      ClientInstallPrepared := ArchiveStarted;
      exit;
    end;
    ArchiveStarted := True;
    if ClientArchiveCertFile <> '' then begin
      if ClientHadArchiveCertFile then begin
        if not CopyFile(ClientArchiveCertFile, ClientRollbackArchiveCertFile, False) then begin
          Result := '无法备份同名客户端证书归档，安装已停止。请检查安装目录权限后重试。';
          ClientInstallPrepared := ArchiveStarted;
          exit;
        end;
      end;
      if not CopyFile(CertFile, ClientArchiveCertFile, False) then begin
        Result := '无法按服务端地址归档当前客户端证书，安装已停止。请检查安装目录权限后重试。';
        ClientInstallPrepared := ArchiveStarted;
        exit;
      end;
    end;
  end;
  ClientInstallPrepared := True;
end;

function RollbackClientRuntimeFiles: Boolean;
var
  ConfigFile: String;
  CertFile: String;
begin
  Result := True;
  if not ClientInstallPrepared then
    exit;
  ConfigFile := ExpandConstant('{app}\conf\client.yaml');
  CertFile := ExpandConstant('{app}\cert\server.crt');
  if ClientHadConfigFile then begin
    CreateDir(ExpandConstant('{app}\conf'));
    if not FileExists(ClientRollbackConfigFile) or
       not CopyFile(ClientRollbackConfigFile, ConfigFile, False) then
      Result := False;
  end else begin
    if FileExists(ConfigFile) and not DeleteFile(ConfigFile) then
      Result := False;
  end;
  if ClientHadCertFile then begin
    CreateDir(ExpandConstant('{app}\cert'));
    if not FileExists(ClientRollbackCertFile) or
       not CopyFile(ClientRollbackCertFile, CertFile, False) then
      Result := False;
  end else begin
    if FileExists(CertFile) and not DeleteFile(CertFile) then
      Result := False;
  end;
  if ClientArchiveConfigFile <> '' then begin
    if ClientHadArchiveConfigFile then begin
      if not FileExists(ClientRollbackArchiveConfigFile) or
         not CopyFile(ClientRollbackArchiveConfigFile, ClientArchiveConfigFile, False) then
        Result := False;
    end else begin
      if FileExists(ClientArchiveConfigFile) and not DeleteFile(ClientArchiveConfigFile) then
        Result := False;
    end;
  end;
  if ClientArchiveCertFile <> '' then begin
    if ClientHadArchiveCertFile then begin
      if not FileExists(ClientRollbackArchiveCertFile) or
         not CopyFile(ClientRollbackArchiveCertFile, ClientArchiveCertFile, False) then
        Result := False;
    end else begin
      if FileExists(ClientArchiveCertFile) and not DeleteFile(ClientArchiveCertFile) then
        Result := False;
    end;
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  QuitMessage: String;
begin
  if ClientRollbackFailed then begin
    Result := '上次安装回滚未完成，备份已保留在：' + ClientRollbackDir();
    exit;
  end;
  if ClientInstallPrepared then begin
    Result := '';
    exit;
  end;
  Result := EnsureWebView2Runtime(NeedsRestart);
  if Result <> '' then
    exit;
  QuitMessage := RequestClientQuit();
  if QuitMessage <> '' then
    Log('Cooperative client quit did not complete; continuing with targeted upgrade cleanup: ' + QuitMessage);
  Result := CleanupClientUpgradeConflicts();
  if Result <> '' then begin
    CleanupClientInstallTemps();
    exit;
  end;
  Result := CheckClientConfigCompatibility();
  if Result <> '' then begin
    CleanupClientInstallTemps();
    exit;
  end;
  Result := BackupClientRuntimeFiles();
  if Result <> '' then begin
    if ClientInstallPrepared then begin
      if RollbackClientRuntimeFiles() then
        ClientInstallPrepared := False
      else
        ClientRollbackFailed := True;
    end;
    CleanupClientInstallTemps();
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  AccountError: String;
begin
  if (CurStep = ssInstall) and ClientMigrationNeeded then begin
    WizardForm.StatusLabel.Caption := '正在升级客户端配置...';
    if not RunClientConfigMigration(False) then begin
      MsgBox('客户端配置迁移失败，安装已停止；原配置将恢复。', mbError, MB_OK);
      Abort;
    end;
  end;
  if CurStep = ssPostInstall then begin
    WizardForm.StatusLabel.Caption := '正在保留客户端连接账号...';
    AccountError := PreserveClientInstallAccount();
    if AccountError <> '' then begin
      MsgBox('客户端连接账号迁移失败，安装已停止；原配置将恢复。' + #13#10 + AccountError, mbError, MB_OK);
      Abort;
    end;
    WizardForm.StatusLabel.Caption := '正在检查新版客户端配置...';
    AccountError := CheckInstalledClientConfig();
    if AccountError <> '' then begin
      MsgBox('新版客户端配置检查失败，安装已停止；原配置将恢复。' + #13#10 + AccountError, mbError, MB_OK);
      Abort;
    end;
  end;
  if CurStep = ssDone then begin
    ClientInstallSucceeded := True;
    CleanupClientInstallTemps();
  end;
end;

procedure DeinitializeSetup;
begin
  if ClientInstallPrepared and not ClientInstallSucceeded then begin
    if FileExists(ClientRollbackDir() + '\migration.json') and
       not RunClientConfigMigration(True) then
      ClientRollbackFailed := True;
    if not RollbackClientRuntimeFiles() then
      ClientRollbackFailed := True;
    if ClientRollbackFailed then
      MsgBox('客户端配置自动回滚失败，备份已保留在：' + ClientRollbackDir(), mbError, MB_OK);
  end;
  CleanupClientInstallTemps();
end;

procedure RemoveClientAutostartValue(RootKey: Integer; Subkey: String);
var
  Command, Expected: String;
begin
  Expected := '"' + ExpandConstant('{app}\bin\lsyl-tunnel-client-gui.exe') + '" --autostart';
  if RegQueryStringValue(RootKey, Subkey, 'LSYLTunnelClient', Command) and
     (CompareText(Command, Expected) = 0) then begin
    if not RegDeleteValue(RootKey, Subkey, 'LSYLTunnelClient') then
      Log('Could not remove client autostart value: ' + Subkey);
  end;
end;

procedure RemoveClientAutostart;
var
  Users: TArrayOfString;
  I: Integer;
  RunKey: String;
begin
  RunKey := 'Software\Microsoft\Windows\CurrentVersion\Run';
  RemoveClientAutostartValue(HKCU64, RunKey);
  { Also clean loaded user hives when uninstall is elevated under another account. }
  if RegGetSubkeyNames(HKU64, '', Users) then
    for I := 0 to GetArrayLength(Users) - 1 do
      RemoveClientAutostartValue(HKU64, Users[I] + '\' + RunKey);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  QuitMessage: String;
begin
  if CurUninstallStep = usUninstall then begin
    ClientQuitUseInstalledApp := True;
    QuitMessage := RequestClientQuit();
    ClientQuitUseInstalledApp := False;
    if QuitMessage <> '' then begin
      MsgBox(QuitMessage, mbError, MB_OK);
      CleanupClientInstallTemps();
      Abort;
    end;
    RemoveClientAutostart();
  end;
end;

procedure DeinitializeUninstall;
begin
  CleanupClientInstallTemps();
end;
