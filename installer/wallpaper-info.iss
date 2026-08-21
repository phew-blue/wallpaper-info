; wallpaper-info — per-user Windows installer (Inno Setup 6)
;
; Build:  iscc /DMyAppVersion=0.4.0 installer\wallpaper-info.iss
; Output: installer\Output\wallpaper-info-setup-{version}.exe
;
; Pre-requisite: wallpaper-info.exe next to this script (CI copies it in).
;
; Compiled by ISCC running under Wine on the Linux home-ops runner — the same
; toolchain lexi uses, via the vendored .github/actions/wine-inno action.

#define MyAppName      "wallpaper-info"
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#define MyAppPublisher "Phew Blue"
#define MyAppURL       "https://phew.blue/software"
#define MyAppExeName   "wallpaper-info.exe"

[Setup]
AppId={{7C1B6A54-2E4D-4C2F-9E1A-0B7D3F5A9C21}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
; Per-user: nothing here needs elevation, and installing under LOCALAPPDATA lets the
; running exe be replaced during self-update (the same reason lexi installs there).
PrivilegesRequired=lowest
DefaultDirName={localappdata}\wallpaper-info
DisableDirPage=yes
DefaultGroupName={#MyAppName}
OutputDir=Output
OutputBaseFilename=wallpaper-info-setup-{#MyAppVersion}
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}
; Every provisioned machine already runs a resident copy, and self-update runs this installer
; while the tray process is alive. Windows will not overwrite a locked .exe, so without these
; an upgrade silently does nothing at all. AppMutex matches InstanceMutexName in
; instance_windows.go; PrepareToInstall below is the belt-and-braces force close.
AppMutex=phew-blue-wallpaper-info
CloseApplications=force
RestartApplications=no

[Files]
Source: "wallpaper-info.exe"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "startup"; Description: "Run wallpaper-info at logon (tray icon)"; GroupDescription: "Startup"

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--tray"
; Startup shortcut: one long-lived tray process that refreshes the wallpaper on a timer.
Name: "{userstartup}\phew-blue wallpaper-info"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--tray"; Tasks: startup

[Run]
; Apply the chosen preset and paint the wallpaper immediately...
Filename: "{app}\{#MyAppExeName}"; Parameters: "--preset {code:GetPreset}"; Flags: runhidden waituntilterminated
; ...then leave the tray running (unticked by default in silent installs).
Filename: "{app}\{#MyAppExeName}"; Parameters: "--tray"; Flags: runhidden nowait postinstall; Description: "Start wallpaper-info now"

[UninstallDelete]
Type: filesandordirs; Name: "{localappdata}\wallpaper-info"

[Code]
var PresetPage: TInputOptionWizardPage;

// A resident tray/watch process holds our own .exe open. Restart Manager cannot reliably close
// a tray app with no main window, and a silent install must never stop to ask, so terminate any
// running copy before the file step. Returning '' means "carry on" — failing to kill is not a
// reason to abort, the file step will report the real problem if one remains.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#MyAppExeName}', '',
       SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(500); // let the handle actually drop before Setup opens the file for writing
  Result := '';
end;

procedure InitializeWizard;
begin
  PresetPage := CreateInputOptionPage(wpSelectTasks,
    'Choose a look', 'Which preset should the wallpaper use?',
    'You can change this later from the tray icon.', True, False);
  PresetPage.Add('phew-blue');
  PresetPage.Add('mono');
  PresetPage.SelectedValueIndex := 0;
end;

// /PRESET=<id> supports unattended installs from provisioning; the wizard page is the
// interactive equivalent. Keep these ids in sync with presets/*.toml.
function GetPreset(Param: string): string;
begin
  Result := ExpandConstant('{param:PRESET|}');
  if Result <> '' then
    Exit;
  if PresetPage <> nil then
    Result := PresetPage.CheckListBox.Items[PresetPage.SelectedValueIndex]
  else
    Result := 'phew-blue';
end;
