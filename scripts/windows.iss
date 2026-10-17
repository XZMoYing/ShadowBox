#define AppVersion "0.1.0"
[Setup]
AppId={{76D050DA-7C1F-4294-B731-34AC90D8C49E}
AppName=ShadowBox
AppVersion={#AppVersion}
AppPublisher=ShadowBox contributors
DefaultDirName={localappdata}\Programs\ShadowBox
DefaultGroupName=ShadowBox
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\dist\windows-x64
OutputBaseFilename=ShadowBox-{#AppVersion}-Setup
SetupIconFile=..\apps\desktop\windows\runner\resources\app_icon.ico
UninstallDisplayIcon={app}\shadowbox-ui.exe
Compression=lzma2
SolidCompression=yes
CloseApplications=yes
RestartApplications=no
[Files]
Source: "..\dist\windows-x64\ShadowBox\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
[Icons]
Name: "{group}\ShadowBox"; Filename: "{app}\ShadowBox.exe"; IconFilename: "{app}\shadowbox-ui.exe"
[Run]
Filename: "{app}\ShadowBox.exe"; Description: "Launch ShadowBox"; Flags: nowait postinstall skipifsilent
