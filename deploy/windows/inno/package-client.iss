#define AppName "LSYL Tunnel Client"
#define AppVersion "1.1.0"
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

[Files]
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-lite.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestName: "lsyl-tunnel-client-gui-quit.exe"; Flags: dontcopy
Source: "{#SourceRoot}\assets\client.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\client-connected.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\conf\client.yaml"; DestDir: "{app}\conf"; Flags: ignoreversion uninsneveruninstall
Source: "{#SourceRoot}\cert\*"; DestDir: "{app}\cert"; Flags: ignoreversion recursesubdirs createallsubdirs uninsneveruninstall

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

[Code]
var
  ClientInstallPrepared: Boolean;
  ClientInstallSucceeded: Boolean;
  ClientHadConfigFile: Boolean;
  ClientHadCertFile: Boolean;
  ClientRollbackConfigFile: String;
  ClientRollbackCertFile: String;
  ClientQuitUseInstalledApp: Boolean;

function ClientQuitHelperPath: String;
begin
  Result := ExpandConstant('{tmp}\lsyl-tunnel-client-gui-quit.exe');
end;

function ClientRollbackDir: String;
begin
  Result := ExpandConstant('{tmp}\lsyl-client-install-rollback');
end;

procedure CleanupClientQuitHelper;
begin
  DeleteFile(ClientQuitHelperPath());
end;

procedure CleanupClientRollbackFiles;
begin
  DelTree(ClientRollbackDir(), True, True, True);
end;

procedure CleanupClientInstallTemps;
begin
  CleanupClientQuitHelper();
  CleanupClientRollbackFiles();
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

function BackupClientRuntimeFiles: String;
var
  ConfigFile: String;
  CertFile: String;
  RollbackDir: String;
begin
  Result := '';
  ClientInstallPrepared := False;
  ClientInstallSucceeded := False;
  ConfigFile := ExpandConstant('{app}\conf\client.yaml');
  CertFile := ExpandConstant('{app}\cert\server.crt');
  RollbackDir := ClientRollbackDir();
  ClientRollbackConfigFile := RollbackDir + '\client.yaml.rollback';
  ClientRollbackCertFile := RollbackDir + '\server.crt.rollback';
  ClientHadConfigFile := FileExists(ConfigFile);
  ClientHadCertFile := FileExists(CertFile);
  CleanupClientRollbackFiles();
  if not CreateDir(RollbackDir) then begin
    Result := '无法创建安装回滚目录，请检查临时目录权限后重试。';
    exit;
  end;
  if ClientHadConfigFile then begin
    if not CopyFile(ConfigFile, ClientRollbackConfigFile, False) then begin
      Result := '无法备份当前客户端配置，安装已停止。请检查安装目录权限后重试。';
      exit;
    end;
    if not CopyFile(ConfigFile, ExpandConstant('{app}\conf\client.yaml.bak'), False) then begin
      Result := '无法写入客户端配置备份，安装已停止。请检查安装目录权限后重试。';
      exit;
    end;
  end;
  if ClientHadCertFile then begin
    if not CopyFile(CertFile, ClientRollbackCertFile, False) then begin
      Result := '无法备份当前客户端证书，安装已停止。请检查安装目录权限后重试。';
      exit;
    end;
    if not CopyFile(CertFile, ExpandConstant('{app}\cert\server.crt.bak'), False) then begin
      Result := '无法写入客户端证书备份，安装已停止。请检查安装目录权限后重试。';
      exit;
    end;
  end;
  ClientInstallPrepared := True;
end;

procedure RollbackClientRuntimeFiles;
var
  ConfigFile: String;
  CertFile: String;
begin
  if not ClientInstallPrepared then
    exit;
  ConfigFile := ExpandConstant('{app}\conf\client.yaml');
  CertFile := ExpandConstant('{app}\cert\server.crt');
  if ClientHadConfigFile then begin
    CreateDir(ExpandConstant('{app}\conf'));
    if FileExists(ClientRollbackConfigFile) then
      CopyFile(ClientRollbackConfigFile, ConfigFile, False);
  end else begin
    DeleteFile(ConfigFile);
  end;
  if ClientHadCertFile then begin
    CreateDir(ExpandConstant('{app}\cert'));
    if FileExists(ClientRollbackCertFile) then
      CopyFile(ClientRollbackCertFile, CertFile, False);
  end else begin
    DeleteFile(CertFile);
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := RequestClientQuit();
  if Result <> '' then
    exit;
  Result := BackupClientRuntimeFiles();
  if Result <> '' then
    CleanupClientInstallTemps();
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssDone then begin
    ClientInstallSucceeded := True;
    CleanupClientInstallTemps();
  end;
end;

procedure DeinitializeSetup;
begin
  if ClientInstallPrepared and not ClientInstallSucceeded then
    RollbackClientRuntimeFiles();
  CleanupClientInstallTemps();
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
  end;
end;

procedure DeinitializeUninstall;
begin
  CleanupClientInstallTemps();
end;
