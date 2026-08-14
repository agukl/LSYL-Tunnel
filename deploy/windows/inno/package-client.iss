#define AppName "LSYL Tunnel Client"
#define AppVersion "2.2.0"
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
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-lite.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#SourceRoot}\bin\WinDivert.dll"; DestDir: "{app}\bin"; Flags: ignoreversion restartreplace; Check: ShouldInstallPinnedRuntime('{app}\bin\WinDivert.dll', '{#WinDivertDLLSHA256}')
Source: "{#SourceRoot}\bin\WinDivert64.sys"; DestDir: "{app}\bin"; Flags: ignoreversion restartreplace; Check: ShouldInstallPinnedRuntime('{app}\bin\WinDivert64.sys', '{#WinDivertDriverSHA256}')
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestName: "lsyl-tunnel-client-gui-quit.exe"; Flags: dontcopy
Source: "{#SourceRoot}\bin\lsyl-tunnel-client-gui.exe"; DestName: "lsyl-tunnel-client-gui-check.exe"; Flags: dontcopy
Source: "{#SourceRoot}\assets\client.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\client-connected.ico"; DestDir: "{app}\assets"; Flags: ignoreversion
Source: "{#SourceRoot}\conf\client.yaml"; DestDir: "{app}\conf"; Flags: ignoreversion uninsneveruninstall
Source: "{#SourceRoot}\cert\*"; DestDir: "{app}\cert"; Flags: ignoreversion recursesubdirs createallsubdirs uninsneveruninstall
Source: "{#SourceRoot}\licenses\WinDivert\*"; DestDir: "{app}\licenses\WinDivert"; Flags: ignoreversion recursesubdirs createallsubdirs

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

function ClientRollbackDir: String;
begin
  Result := ExpandConstant('{tmp}\lsyl-client-install-rollback');
end;

procedure CleanupClientQuitHelper;
begin
  DeleteFile(ClientQuitHelperPath());
end;

procedure CleanupClientConfigCheckHelper;
begin
  DeleteFile(ClientConfigCheckHelperPath());
end;

procedure CleanupClientRollbackFiles;
begin
  DelTree(ClientRollbackDir(), True, True, True);
end;

procedure CleanupClientInstallTemps;
begin
  CleanupClientQuitHelper();
  CleanupClientConfigCheckHelper();
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

function CheckClientConfigCompatibility: String;
var
  ResultCode: Integer;
  ConfigFile: String;
  CheckerExe: String;
  ResultFile: String;
  Params: String;
  Detail: AnsiString;
  DetailText: String;
begin
  Result := '';
  ConfigFile := ExpandConstant('{app}\conf\client.yaml');
  if not FileExists(ConfigFile) then
    exit;

  CleanupClientConfigCheckHelper();
  ExtractTemporaryFile('lsyl-tunnel-client-gui-check.exe');
  CheckerExe := ClientConfigCheckHelperPath();
  ResultFile := ExpandConstant('{tmp}\client-config-compat-result.txt');
  DeleteFile(ResultFile);

  Params :=
    '-config-compat-check' +
    ' -config "' + ConfigFile + '"' +
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
      DetailText := 'The installed client config requires a different client version.';
    Result :=
      'Installed client config is not compatible with this client installer. Installation stopped.' + #13#10 + #13#10 +
      DetailText + #13#10 + #13#10 +
      'Installed config: ' + ConfigFile;
  end;
  CleanupClientConfigCheckHelper();
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
  if ClientArchiveConfigFile <> '' then begin
    if ClientHadArchiveConfigFile then begin
      if FileExists(ClientRollbackArchiveConfigFile) then
        CopyFile(ClientRollbackArchiveConfigFile, ClientArchiveConfigFile, False);
    end else begin
      DeleteFile(ClientArchiveConfigFile);
    end;
  end;
  if ClientArchiveCertFile <> '' then begin
    if ClientHadArchiveCertFile then begin
      if FileExists(ClientRollbackArchiveCertFile) then
        CopyFile(ClientRollbackArchiveCertFile, ClientArchiveCertFile, False);
    end else begin
      DeleteFile(ClientArchiveCertFile);
    end;
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := RequestClientQuit();
  if Result <> '' then
    exit;
  Result := CheckClientConfigCompatibility();
  if Result <> '' then begin
    CleanupClientInstallTemps();
    exit;
  end;
  Result := BackupClientRuntimeFiles();
  if Result <> '' then begin
    if ClientInstallPrepared then begin
      RollbackClientRuntimeFiles();
      ClientInstallPrepared := False;
    end;
    CleanupClientInstallTemps();
  end;
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
