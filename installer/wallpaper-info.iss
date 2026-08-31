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
; The same program linked for the GUI subsystem. Everything launched unattended -- the shortcuts
; and the tray -- uses this one, because Windows gives a console-subsystem exe a console window
; before any of its code runs, and a shortcut has no way to ask for that window to be hidden.
#define MyAppExeNameW  "wallpaper-infow.exe"

[Setup]
AppId={{7C1B6A54-2E4D-4C2F-9E1A-0B7D3F5A9C21}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
; Without this, Add/Remove Programs shows "wallpaper-info version 0.2.0" as the name; the
; version already has its own column.
AppVerName={#MyAppName}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
; Per-user: nothing here needs elevation, and installing under LOCALAPPDATA lets the
; running exe be replaced during self-update (the same reason lexi installs there).
PrivilegesRequired=lowest
DefaultDirName={localappdata}\Phew Blue\wallpaper-info
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
; while the tray process is alive. Windows will not overwrite a locked .exe, so the running
; copy has to go before the file step.
;
; Deliberately NO AppMutex. Inno checks it before PrepareToInstall and can only respond with a
; message box, so under /SUPPRESSMSGBOXES it defaults to Cancel and Setup aborts with exit 1
; having installed nothing -- i.e. it turned every silent upgrade into a silent no-op, the
; exact failure it was added to prevent. PrepareToInstall below does the job instead: it runs
; unattended, and taskkill closes a tray app with no main window, which Restart Manager cannot.
CloseApplications=force
RestartApplications=no

[Files]
Source: "wallpaper-info.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "wallpaper-infow.exe"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "startup"; Description: "Run wallpaper-info at logon (tray icon)"; GroupDescription: "Startup"

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeNameW}"; Parameters: "--tray"
; Startup shortcut: one long-lived tray process that refreshes the wallpaper on a timer.
Name: "{userstartup}\phew-blue wallpaper-info"; Filename: "{app}\{#MyAppExeNameW}"; Parameters: "--tray"; Tasks: startup

[Run]
; Apply the chosen preset and paint the wallpaper immediately...
Filename: "{app}\{#MyAppExeName}"; Parameters: "{code:GetPresetArg}{code:GetManifestArg}"; Flags: runhidden waituntilterminated
; ...then leave the tray running (unticked by default in silent installs).
Filename: "{app}\{#MyAppExeNameW}"; Parameters: "--tray"; Flags: runhidden nowait postinstall; Description: "Start wallpaper-info now"

[UninstallDelete]
Type: filesandordirs; Name: "{localappdata}\Phew Blue\wallpaper-info"
Type: filesandordirs; Name: "{localappdata}\wallpaper-info"
Type: dirifempty;      Name: "{localappdata}\Phew Blue"

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
  // The resident tray is the GUI-subsystem build, so killing only the console one would leave it
  // holding {app} open and every upgrade would silently install nothing.
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#MyAppExeNameW}', '',
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

// /MANIFEST=<url-or-path> points the one-shot preset render at a catalogue other than the
// published one — a USB stick or an air-gapped share. Only this first render needs it: the
// render persists the resolved preset, background and font into the config, so the Startup
// tray process still looks right long after the stick is unplugged.
function GetManifestArg(Param: string): string;
var M: string;
begin
  M := ExpandConstant('{param:MANIFEST|}');
  if M = '' then
    Result := ''
  else
    Result := ' --manifest "' + M + '"';
end;

// The preset this machine is already configured with, or '' if it has none. Read as text:
// the config is TOML carrying a single `preset = "<id>"` line, and a parser for one value
// would be more code than the scan. Both the current location and the pre-"Phew Blue"
// one are checked, because an upgraded install keeps its original directory.
function ConfiguredPreset(): string;
var
  Paths: array[0..1] of string;
  S: AnsiString;
  I, J: Integer;
begin
  Result := '';
  Paths[0] := ExpandConstant('{userappdata}') + '\Phew Blue\wallpaper-info\config.toml';
  Paths[1] := ExpandConstant('{userappdata}') + '\wallpaper-info\config.toml';
  for I := 0 to 1 do
  begin
    if FileExists(Paths[I]) and LoadStringFromFile(Paths[I], S) then
    begin
      J := Pos('preset = "', S);
      if J > 0 then
      begin
        J := J + 10;
        while (J <= Length(S)) and (S[J] <> '"') do
        begin
          Result := Result + S[J];
          J := J + 1;
        end;
        if Result <> '' then
          Exit;
      end;
    end;
  end;
end;

// The --preset argument for the post-install render, or '' to leave the machine's own
// preset alone.
//
// /PRESET=<id> supports unattended installs from provisioning; the wizard page is the
// interactive equivalent. Keep these ids in sync with presets/*.toml.
//
// The third case is the one that matters: a silent install with no /PRESET is what every
// self-update looks like. This used to fall through to the default and re-render as
// phew-blue, so a machine provisioned with a stick-only preset silently lost it the first
// time it updated itself. With a preset already in the config there is nothing to choose,
// so the render is left to use what is saved.
function GetPresetArg(Param: string): string;
var
  P: string;
begin
  P := ExpandConstant('{param:PRESET|}');
  if P = '' then
  begin
    if (not WizardSilent) and (PresetPage <> nil) then
      P := PresetPage.CheckListBox.Items[PresetPage.SelectedValueIndex]
    else if ConfiguredPreset() <> '' then
    begin
      Result := '';
      Exit;
    end
    else
      P := 'phew-blue';
  end;
  Result := ' --preset ' + P;
end;
