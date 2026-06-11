#define AppName "LSYL Tunnel Server"
#define AppVersion "1.1.0"
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
Name: "{app}\logs\flow-traffic"; Flags: uninsneveruninstall
Name: "{app}\logs\service"; Flags: uninsneveruninstall

[Files]
Source: "{#SourceRoot}\bin\lsyl-tunnel-server.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-server-svc.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-server-gui.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-passwd.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-cert.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\server.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\server.svg"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\conf\server.yaml"; DestDir: "{app}\conf"; Flags: ignoreversion onlyifdoesntexist uninsneveruninstall

[Icons]
Name: "{group}\LSYL Tunnel Server"; Filename: "{app}\bin\lsyl-tunnel-server-gui.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\server.ico"
Name: "{autodesktop}\LSYL Tunnel Server"; Filename: "{app}\bin\lsyl-tunnel-server-gui.exe"; WorkingDir: "{app}"; IconFilename: "{app}\assets\server.ico"; Tasks: desktopicon

[InstallDelete]
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

procedure InitializeWizard;
begin
  CertPage := CreateInputQueryPage(
    wpSelectDir,
    '服务端证书',
    '生成自签 TLS 服务端身份',
    '请输入客户端连接服务端时使用的域名或 IP，多个值用英文逗号分隔。安装时会生成 certs\server.crt 和 certs\server.key；只把 server.crt 发给客户端。');
  CertPage.Add('证书主机名/IP：', False);
  CertPage.Values[0] := 'localhost,127.0.0.1';
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
    Result := 'localhost,127.0.0.1';
end;

procedure StopServerService;
var
  ResultCode: Integer;
begin
  Exec('sc.exe', 'stop LSYLTunnelServer', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(1000);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  StopServerService();
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

procedure EnsureServerCertificate;
var
  ResultCode: Integer;
  CertFile: String;
  KeyFile: String;
  Params: String;
begin
  CertFile := ExpandConstant('{app}\certs\server.crt');
  KeyFile := ExpandConstant('{app}\certs\server.key');
  if (not FileExists(CertFile)) or (not FileExists(KeyFile)) then begin
    Params := '-out "' + ExpandConstant('{app}\certs') + '" -hosts "' + GetCertHosts('') + '"';
    WizardForm.StatusLabel.Caption := '正在生成服务端证书...';
    if not Exec(ExpandConstant('{app}\bin\lsyl-tunnel-cert.exe'), Params, ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
      MsgBox('服务端证书生成工具启动失败，请检查安装目录权限。', mbError, MB_OK);
      Abort;
    end;
    if ResultCode <> 0 then begin
      MsgBox('服务端证书生成失败，请检查证书主机名/IP 是否正确，并以管理员身份重新运行安装器。', mbError, MB_OK);
      Abort;
    end;
  end;
  LockCertDirectory();
end;

procedure RegisterServerService;
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
  HelperExe := ExpandConstant('{app}\bin\lsyl-tunnel-server-gui.exe');
  ServiceExe := ExpandConstant('{app}\bin\lsyl-tunnel-server-svc.exe');
  ConfigFile := ExpandConstant('{app}\conf\server.yaml');
  LogFile := ExpandConstant('{app}\logs\service\server-service.log');
  ResultFile := ExpandConstant('{app}\logs\service\service-register-error.txt');
  DeleteFile(ResultFile);
  if not FileExists(HelperExe) then begin
    MsgBox('服务端 Windows 服务注册失败：缺少管理台程序。' + #13#10 + HelperExe, mbError, MB_OK);
    Abort;
  end;
  if not FileExists(ServiceExe) then begin
    MsgBox('服务端 Windows 服务注册失败：缺少服务程序。' + #13#10 + ServiceExe, mbError, MB_OK);
    Abort;
  end;
  if not FileExists(ConfigFile) then begin
    MsgBox('服务端 Windows 服务注册失败：缺少配置文件。' + #13#10 + ConfigFile, mbError, MB_OK);
    Abort;
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
    MsgBox('服务端 Windows 服务注册助手启动失败，请确认安装器以管理员身份运行。', mbError, MB_OK);
    Abort;
  end;
  if ResultCode <> 0 then begin
    Detail := '';
    if FileExists(ResultFile) then
      LoadStringFromFile(ResultFile, Detail);
    DetailText := Trim(String(Detail));
    if DetailText = '' then
      DetailText := '请确认安装器以管理员身份运行，并检查是否存在正在删除中的 LSYLTunnelServer 服务。';
    MsgBox('服务端 Windows 服务注册失败：' + #13#10 + DetailText, mbError, MB_OK);
    Abort;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    WizardForm.StatusLabel.Caption := '正在准备服务端配置...';
    EnsureServerCertificate();
    WizardForm.StatusLabel.Caption := '正在注册 Windows 服务...';
    RegisterServerService();
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
