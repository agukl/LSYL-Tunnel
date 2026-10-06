{ Shared prerequisite code, included after each installer's global variables. }
function WebView2VersionPresent(RootKey: Integer): Boolean;
var
  Version: String;
begin
  Result := False;
  if RegQueryStringValue(RootKey,
    'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) then
    Result := (Trim(Version) <> '') and (Trim(Version) <> '0.0.0.0');
end;

function WebView2RuntimePresent: Boolean;
begin
  Result := WebView2VersionPresent(HKLM32);
  { An all-users install must not depend on the installing admin's user-only runtime. }
  if not Result and not IsAdminInstallMode then
    Result := WebView2VersionPresent(HKCU32);
end;

function EnsureWebView2Runtime(var NeedsRestart: Boolean): String;
var
  Installer: String;
  ResultCode: Integer;
begin
  Result := '';
  if WebView2RuntimePresent() then begin
    Log('WebView2 Runtime is already installed; skipping prerequisite installation.');
    exit;
  end;

  Log('WebView2 Runtime is missing; starting prerequisite installation.');
  { Inno Setup hides PreparingLabel before calling PrepareToInstall. }
  WizardForm.PreparingLabel.Caption := '正在安装 WebView2 Runtime';
  WizardForm.PreparingLabel.Visible := True;
  WizardForm.Update;
  try
    ExtractTemporaryFile('MicrosoftEdgeWebview2Setup.exe');
    Installer := ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe');
    if not Exec(Installer, '/silent /install', ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ResultCode) then begin
      Result := '无法启动 Microsoft Edge WebView2 Runtime 安装器。请检查安装权限后重试。';
      exit;
    end;
    Log('WebView2 installer exit code: ' + IntToStr(ResultCode));
    if ResultCode = 3010 then begin
      NeedsRestart := True;
      Result := 'Microsoft Edge WebView2 Runtime 已安装，需要重启 Windows 后继续安装。';
      exit;
    end;
    if not WebView2RuntimePresent() then
      Result :=
        'Microsoft Edge WebView2 Runtime 未安装成功，已停止本次安装。退出码：' + IntToStr(ResultCode) + #13#10 +
        '请检查网络和安装权限；离线服务器请先安装 Microsoft 官方 Evergreen Standalone Installer（x64），然后重试。' + #13#10 +
        'https://developer.microsoft.com/microsoft-edge/webview2/';
  finally
    WizardForm.PreparingLabel.Visible := False;
  end;
end;
