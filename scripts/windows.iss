#define AppVersion "0.1.0"

; Architecture is selected by the caller: ISCC scripts/windows.iss (x64)
; or ISCC /DSBArch=arm64 scripts/windows.iss (ARM64).
#ifndef SBArch
  #define SBArch "x64"
#endif

#if SBArch == "arm64"
  #define SBArchDir "arm64"
  #define SBArchAllowed "arm64"
  #define SBArchSuffix "-arm64"
#else
  #define SBArchDir "x64"
  #define SBArchAllowed "x64compatible"
  #define SBArchSuffix ""
#endif

[Setup]
AppId={{76D050DA-7C1F-4294-B731-34AC90D8C49E}
AppName=ShadowBox
AppVersion={#AppVersion}
AppPublisher=ShadowBox contributors
DefaultDirName={localappdata}\Programs\ShadowBox
DefaultGroupName=ShadowBox
PrivilegesRequired=lowest
ArchitecturesAllowed={#SBArchAllowed}
ArchitecturesInstallIn64BitMode={#SBArchAllowed}
OutputDir=..\dist\windows-{#SBArchDir}
OutputBaseFilename=ShadowBox-{#AppVersion}-Setup{#SBArchSuffix}
SetupIconFile=..\apps\desktop\windows\runner\resources\app_icon.ico
UninstallDisplayIcon={app}\shadowbox-ui.exe
Compression=lzma2
SolidCompression=yes
CloseApplications=yes
RestartApplications=no
[Files]
Source: "..\dist\windows-{#SBArchDir}\ShadowBox\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
[Icons]
Name: "{group}\ShadowBox"; Filename: "{app}\ShadowBox.exe"; IconFilename: "{app}\shadowbox-ui.exe"
[Run]
Filename: "{app}\ShadowBox.exe"; Description: "Launch ShadowBox"; Flags: nowait postinstall skipifsilent
