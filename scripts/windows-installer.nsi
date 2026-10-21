Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!ifndef APP_VERSION
  !error "APP_VERSION is required"
!endif
!ifndef APP_ARCH
  !error "APP_ARCH is required"
!endif
!ifndef DIST_DIR
  !error "DIST_DIR is required"
!endif

Name "Passpine"
OutFile "${DIST_DIR}\Passpine-${APP_VERSION}-windows-${APP_ARCH}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Passpine"
InstallDirRegKey HKCU "Software\Passpine" "InstallDir"
RequestExecutionLevel user
!define MUI_ABORTWARNING
!define MUI_ICON "${DIST_DIR}\windows-resources\Passpine.ico"
!define MUI_UNICON "${DIST_DIR}\windows-resources\Passpine.ico"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
!if "${APP_ARCH}" == "amd64"
  ${IfNot} ${RunningX64}
    SetErrorLevel 1633
    Quit
  ${EndIf}
!endif
FunctionEnd

!macro RemoveHostRegistration PREFIX BROWSER
  ReadRegStr $0 HKCU "${PREFIX}\NativeMessagingHosts\com.knothhe.passpine" ""
  StrCpy $1 "$LOCALAPPDATA\Passpine\NativeMessagingHosts\${BROWSER}\com.knothhe.passpine.json"
  ${If} $0 == $1
    DeleteRegKey HKCU "${PREFIX}\NativeMessagingHosts\com.knothhe.passpine"
    Delete "$1"
    RMDir "$LOCALAPPDATA\Passpine\NativeMessagingHosts\${BROWSER}"
  ${EndIf}
!macroend

Section
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File "${DIST_DIR}\passpine.exe"
  File "${DIST_DIR}\passpine-host.exe"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  ExecWait '$\"$INSTDIR\passpine-host.exe$\" --register' $0
  ${If} $0 != 0
    SetErrorLevel 1
    Abort
  ${EndIf}
  CreateDirectory "$SMPROGRAMS\Passpine"
  CreateShortcut "$SMPROGRAMS\Passpine\Passpine.lnk" "$INSTDIR\passpine.exe"
  WriteRegStr HKCU "Software\Passpine" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "DisplayName" "Passpine"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "DisplayIcon" "$INSTDIR\passpine.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine" "NoRepair" 1
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  ReadRegStr $2 HKCU "Software\Passpine" "InstallDir"
  ${If} $2 == $INSTDIR
    ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Passpine"
    ${If} $0 == '$\"$INSTDIR\passpine.exe$\" --mygo-opened-at-login'
      DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Passpine"
      DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run" "Passpine"
    ${EndIf}
  !insertmacro RemoveHostRegistration "Software\Google\Chrome" "chrome"
  !insertmacro RemoveHostRegistration "Software\Microsoft\Edge" "edge"
  !insertmacro RemoveHostRegistration "Software\BraveSoftware\Brave-Browser" "brave"
  !insertmacro RemoveHostRegistration "Software\Mozilla" "firefox"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine"
    DeleteRegKey HKCU "Software\Passpine"
    Delete "$SMPROGRAMS\Passpine\Passpine.lnk"
    RMDir "$SMPROGRAMS\Passpine"
  ${EndIf}
  Delete "$INSTDIR\passpine.exe"
  Delete "$INSTDIR\passpine-host.exe"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
