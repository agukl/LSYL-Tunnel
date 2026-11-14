#define AppName "LSYL Tunnel Server"
#define AppVersion "3.5.0"
; This script is copied into the server distributable package:
;   LSYL Tunnel Server\installer\server.iss
; It compiles the files from that package directory, not from source.
#define SourceRoot ".."

[Setup]
AppId={{A69942C1-D831-44D1-97C5-2E0DDB9FF3CA}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=LSYL Tunnel
DefaultDirName={autopf}\LSYL Tunnel Server
DefaultGroupName=LSYL Tunnel
DisableProgramGroupPage=yes
OutputDir=..\..\installers
OutputBaseFilename=LSYL-Tunnel-Server-Setup
SetupIconFile=..\assets\server.ico
UninstallDisplayIcon={app}\assets\server.ico
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
CloseApplications=yes
RestartApplications=no
WizardStyle=modern

[Languages]
Name: "chinesesimplified"; MessagesFile: "Languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "快捷方式："; Flags: unchecked
Name: "autostartservice"; Description: "设置 LSYL Tunnel Server 服务为开机自启动"; GroupDescription: "Windows 服务："; Flags: unchecked

[Dirs]
Name: "{app}\conf"; Flags: uninsneveruninstall
Name: "{app}\certs"; Flags: uninsneveruninstall
Name: "{app}\data"; Flags: uninsneveruninstall
Name: "{app}\logs"; Flags: uninsneveruninstall
Name: "{app}\logs\request"; Flags: uninsneveruninstall
Name: "{app}\logs\business"; Flags: uninsneveruninstall
Name: "{app}\logs\entry-traffic"; Flags: uninsneveruninstall
Name: "{app}\logs\service"; Flags: uninsneveruninstall

[Files]
Source: "{#SourceRoot}\prerequisites\MicrosoftEdgeWebview2Setup.exe"; Flags: dontcopy
Source: "{#SourceRoot}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\licenses\WebView2\*"; DestDir: "{app}\licenses\WebView2"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#SourceRoot}\licenses\Wails\*"; DestDir: "{app}\licenses\Wails"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#SourceRoot}\bin\lsyl-tunnel-server-svc.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-server-gui.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-cert.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\server.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\server.svg"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\conf\server.yaml"; DestDir: "{app}\conf"; Flags: ignoreversion onlyifdoesntexist uninsneveruninstall
Source: "{#SourceRoot}\conf\server.yaml"; DestName: "server-config-template.yaml"; Flags: dontcopy
Source: "{#SourceRoot}\bin\lsyl-tunnel-server-gui.exe"; DestName: "lsyl-tunnel-server-gui-check.exe"; Flags: dontcopy

[Icons]
Name: "{group}\LSYL Tunnel Server"; Filename: "{app}\bin\lsyl-tunnel-server-gui.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\server.ico"
Name: "{autodesktop}\LSYL Tunnel Server"; Filename: "{app}\bin\lsyl-tunnel-server-gui.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\server.ico"; Tasks: desktopicon

[InstallDelete]
Type: files; Name: "{app}\bin\lsyl-tunnel-server.exe"
Type: files; Name: "{app}\bin\lsyl-tunnel-passwd.exe"
Type: files; Name: "{app}\install-server-app.cmd"
Type: files; Name: "{app}\uninstall-server-app.cmd"
Type: files; Name: "{app}\uninstall-server-app.ps1"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\bin"
Type: filesandordirs; Name: "{app}\assets"
Type: filesandordirs; Name: "{app}\tmp"
Type: files; Name: "{app}\install-server-app.cmd"
Type: files; Name: "{app}\uninstall-server-app.cmd"
Type: files; Name: "{app}\uninstall-server-app.ps1"

[Code]
var
  CertPage: TInputQueryWizardPage;
  ServerMigrationNeeded: Boolean;
  ServerInstallSucceeded: Boolean;
  ServerInstalledVersion: String;
  ServerPostInstallError: String;

#include "webview2-runtime.iss"

procedure InitializeWizard;
begin
  CertPage := CreateInputQueryPage(
    wpSelectDir,
    '服务端证书',
    '生成自签 TLS 服务端身份',
    '请输入客户端连接服务端时使用的域名或 IP，多个值用英文逗号分隔。安装时会生成 certs\server.crt 和 certs\server.key；只把 server.crt 发给客户端。');
  CertPage.Add('证书主机名/IP：', False);
  CertPage.Values[0] := '127.0.0.1';
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = CertPage.ID then begin
    if Trim(CertPage.Values[0]) = '' then begin
      MsgBox('请至少填写一个证书主机名或 IP。', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

function GetCertHosts(Param: String): String;
begin
  Result := Trim(CertPage.Values[0]);
  if Result = '' then
    Result := '127.0.0.1';
end;

function CheckServerConfigCompatibility: String;
var
  ResultCode: Integer;
  ConfigFile: String;
  CheckerExe: String;
  TemplateFile: String;
  ResultFile: String;
  Params: String;
  Detail: AnsiString;
  DetailText: String;
begin
  Result := '';
  ConfigFile := ExpandConstant('{app}\conf\server.yaml');
  if not FileExists(ConfigFile) then
    exit;

  if FileExists(ExpandConstant('{app}\conf\.install-rollback.json')) then begin
    Result := '存在上次安装留下的配置回滚数据，请先检查并恢复：' +
      ExpandConstant('{app}\conf\.install-rollback.json');
    exit;
  end;

  ServerMigrationNeeded := True;
  if not GetVersionNumbersString(ExpandConstant('{app}\bin\lsyl-tunnel-server-gui.exe'), ServerInstalledVersion) then
    ServerInstalledVersion := '';

  ExtractTemporaryFile('lsyl-tunnel-server-gui-check.exe');
  ExtractTemporaryFile('server-config-template.yaml');
  CheckerExe := ExpandConstant('{tmp}\lsyl-tunnel-server-gui-check.exe');
  TemplateFile := ExpandConstant('{tmp}\server-config-template.yaml');
  ResultFile := ExpandConstant('{tmp}\server-config-compat-result.txt');
  DeleteFile(ResultFile);

  Params :=
    '-config-compat-check' +
    ' -config "' + ConfigFile + '"' +
    ' -template "' + TemplateFile + '"' +
    ' -source-version "' + ServerInstalledVersion + '"' +
    ' -backup-file "' + ExpandConstant('{app}\conf\.install-rollback.json') + '"' +
    ' -result-file "' + ResultFile + '"';
  if not Exec(CheckerExe, Params, ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
    Result :=
      'Cannot start server config compatibility checker.' + #13#10 +
      CheckerExe;
    exit;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    DetailText := Trim(String(Detail));
    if DetailText = '' then
      DetailText := 'The installed server config requires a different server version.';
    Result :=
      'Installed server config is not compatible with this server installer. Installation stopped.' + #13#10 + #13#10 +
      DetailText + #13#10 + #13#10 +
      'Installed config: ' + ConfigFile;
  end;
end;

function RunServerConfigMigration(Rollback: Boolean): Boolean;
var
  ResultCode: Integer;
  Params: String;
  CheckerExe: String;
  TemplateFile: String;
begin
  Result := True;
  ExtractTemporaryFile('lsyl-tunnel-server-gui-check.exe');
  ExtractTemporaryFile('server-config-template.yaml');
  CheckerExe := ExpandConstant('{tmp}\lsyl-tunnel-server-gui-check.exe');
  TemplateFile := ExpandConstant('{tmp}\server-config-template.yaml');
  Params := '-config-compat-check -backup-file "' +
    ExpandConstant('{app}\conf\.install-rollback.json') + '"';
  if Rollback then
    Params := Params + ' -rollback'
  else
    Params := Params + ' -apply -config "' + ExpandConstant('{app}\conf\server.yaml') +
      '" -template "' + TemplateFile +
      '" -source-version "' + ServerInstalledVersion + '"';
  if not Exec(CheckerExe, Params, ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then
    Result := False
  else
    Result := ResultCode = 0;
end;

procedure StopServerService;
var
  ResultCode: Integer;
begin
  Exec('sc.exe', 'stop LSYLTunnelServer', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(1000);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  CompatError: String;
begin
  CompatError := CheckServerConfigCompatibility();
  if CompatError <> '' then begin
    Result := CompatError;
    exit;
  end;
  Result := EnsureWebView2Runtime(NeedsRestart);
  if Result <> '' then
    exit;
  Result := '';
end;

procedure LockCertDirectory;
var
  ResultCode: Integer;
  CertDir: String;
begin
  CertDir := ExpandConstant('{app}\certs');
  Exec('icacls.exe', '"' + CertDir + '" /inheritance:r', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec('icacls.exe', '"' + CertDir + '" /grant:r *S-1-5-18:(OI)(CI)F *S-1-5-32-544:(OI)(CI)F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function EnsureServerCertificate: String;
var
  ResultCode: Integer;
  CertFile: String;
  KeyFile: String;
  Params: String;
begin
  Result := '';
  CertFile := ExpandConstant('{app}\certs\server.crt');
  KeyFile := ExpandConstant('{app}\certs\server.key');
  if (not FileExists(CertFile)) or (not FileExists(KeyFile)) then begin
    Params := '-out "' + ExpandConstant('{app}\certs') + '" -hosts "' + GetCertHosts('') + '"';
    WizardForm.StatusLabel.Caption := '正在生成服务端证书...';
    if not Exec(ExpandConstant('{app}\bin\lsyl-tunnel-cert.exe'), Params, ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
      Result := '服务端证书生成工具启动失败，请检查安装目录权限。';
      exit;
    end;
    if ResultCode <> 0 then begin
      Result := '服务端证书生成失败，请检查证书主机名/IP 是否正确。';
      exit;
    end;
  end;
  LockCertDirectory();
end;

function RegisterServerService: String;
var
  ResultCode: Integer;
  HelperExe: String;
  ServiceExe: String;
  ConfigFile: String;
  LogFile: String;
  ResultFile: String;
  Params: String;
  StartType: String;
  Detail: AnsiString;
  DetailText: String;
begin
  Result := '';
  HelperExe := ExpandConstant('{app}\bin\lsyl-tunnel-server-gui.exe');
  ServiceExe := ExpandConstant('{app}\bin\lsyl-tunnel-server-svc.exe');
  ConfigFile := ExpandConstant('{app}\conf\server.yaml');
  LogFile := ExpandConstant('{app}\logs\service\server-service.log');
  ResultFile := ExpandConstant('{app}\logs\service\service-register-error.txt');
  DeleteFile(ResultFile);
  if not FileExists(HelperExe) then begin
    Result := '服务端 Windows 服务注册失败：缺少管理台程序。' + #13#10 + HelperExe;
    exit;
  end;
  if not FileExists(ServiceExe) then begin
    Result := '服务端 Windows 服务注册失败：缺少服务程序。' + #13#10 + ServiceExe;
    exit;
  end;
  if not FileExists(ConfigFile) then begin
    Result := '服务端 Windows 服务注册失败：缺少配置文件。' + #13#10 + ConfigFile;
    exit;
  end;
  StartType := 'manual';
  if WizardIsTaskSelected('autostartservice') then
    StartType := 'auto';
  Params :=
    '-service-action install' +
    ' -service-exe "' + ServiceExe + '"' +
    ' -service-name LSYLTunnelServer' +
    ' -start-type ' + StartType +
    ' -config "' + ConfigFile + '"' +
    ' -log "' + LogFile + '"' +
    ' -result-file "' + ResultFile + '"';
  if not Exec(HelperExe, Params, ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
    Result := '服务端 Windows 服务注册助手启动失败，请确认安装器以管理员身份运行。';
    exit;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    DetailText := Trim(String(Detail));
    if DetailText = '' then
      DetailText := '请确认安装器以管理员身份运行，并检查是否存在正在删除中的 LSYLTunnelServer 服务。';
    Result := '服务端 Windows 服务注册失败：' + #13#10 + DetailText;
    exit;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssInstall) and ServerMigrationNeeded then begin
    WizardForm.StatusLabel.Caption := '正在升级服务端配置...';
    if not RunServerConfigMigration(False) then begin
      MsgBox('服务端配置迁移失败，安装已停止；原配置将恢复。', mbError, MB_OK);
      Abort;
    end;
  end;
  if CurStep = ssInstall then
    StopServerService();
  if CurStep = ssPostInstall then begin
    WizardForm.StatusLabel.Caption := '正在准备服务端配置...';
    ServerPostInstallError := EnsureServerCertificate();
    if ServerPostInstallError = '' then begin
      WizardForm.StatusLabel.Caption := '正在注册 Windows 服务...';
      ServerPostInstallError := RegisterServerService();
    end;
    if ServerPostInstallError <> '' then
      MsgBox('程序文件已更新，但后置设置未完成。请修复后重新运行安装包：' + #13#10 +
        ServerPostInstallError, mbError, MB_OK);
  end;
  if CurStep = ssDone then begin
    ServerInstallSucceeded := True;
    DeleteFile(ExpandConstant('{app}\conf\.install-rollback.json'));
  end;
end;

function GetCustomSetupExitCode: Integer;
begin
  Result := 0;
  if ServerPostInstallError <> '' then
    Result := 10;
end;

procedure DeinitializeSetup;
begin
  if ServerMigrationNeeded and not ServerInstallSucceeded and
     FileExists(ExpandConstant('{app}\conf\.install-rollback.json')) then begin
    if RunServerConfigMigration(True) then
      DeleteFile(ExpandConstant('{app}\conf\.install-rollback.json'))
    else
      MsgBox('服务端配置自动回滚失败，备份已保留在：' +
        ExpandConstant('{app}\conf\.install-rollback.json'), mbError, MB_OK);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  ResultCode: Integer;
begin
  if CurUninstallStep = usUninstall then begin
    StopServerService();
    Exec('sc.exe', 'delete LSYLTunnelServer', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
end;
