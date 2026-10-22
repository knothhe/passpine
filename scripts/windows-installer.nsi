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
!ifndef APP_CHANNEL
  !error "APP_CHANNEL is required"
!endif
!ifndef APP_NAME
  !error "APP_NAME is required"
!endif
!ifndef PACKAGE_NAME
  !error "PACKAGE_NAME is required"
!endif
!if "${APP_CHANNEL}" == "dev"
  !define HOST_NAME "com.knothhe.passpine.dev"
!else if "${APP_CHANNEL}" == "release"
  !define HOST_NAME "com.knothhe.passpine"
!else
  !error "APP_CHANNEL must be dev or release"
!endif

Name "${APP_NAME}"
OutFile "${DIST_DIR}\${PACKAGE_NAME}-${APP_VERSION}-windows-${APP_ARCH}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${APP_NAME}"
InstallDirRegKey HKCU "Software\${APP_NAME}" "InstallDir"
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
  ReadRegStr $0 HKCU "${PREFIX}\NativeMessagingHosts\${HOST_NAME}" ""
  StrCpy $1 "$LOCALAPPDATA\${APP_NAME}\NativeMessagingHosts\${BROWSER}\${HOST_NAME}.json"
  ${If} $0 == $1
    DeleteRegKey HKCU "${PREFIX}\NativeMessagingHosts\${HOST_NAME}"
    Delete "$1"
    RMDir "$LOCALAPPDATA\${APP_NAME}\NativeMessagingHosts\${BROWSER}"
  ${EndIf}
!macroend

Section
  SetShellVarContext current
!if "${APP_CHANNEL}" == "dev"
  # Never let a custom /D overwrite a formal or unrecognized installation.
  ${If} ${FileExists} "$INSTDIR\passpine.exe"
  ${OrIf} ${FileExists} "$INSTDIR\passpine-host.exe"
    ReadINIStr $0 "$INSTDIR\build-channel.ini" "build" "channel"
    ${If} $0 != "dev"
      SetErrorLevel 1
      Abort
    ${EndIf}
  ${EndIf}
!endif
  SetOutPath "$INSTDIR"
  File "${DIST_DIR}\passpine.exe"
  File "${DIST_DIR}\passpine-host.exe"
  WriteINIStr "$INSTDIR\build-channel.ini" "build" "channel" "${APP_CHANNEL}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  ExecWait '$\"$INSTDIR\passpine-host.exe$\" --register' $0
  ${If} $0 != 0
    SetErrorLevel 1
    Abort
  ${EndIf}
  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\passpine.exe"
  WriteRegStr HKCU "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayIcon" "$INSTDIR\passpine.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  ReadRegStr $2 HKCU "Software\${APP_NAME}" "InstallDir"
  ${If} $2 == $INSTDIR
    ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APP_NAME}"
    ${If} $0 == '$\"$INSTDIR\passpine.exe$\" --mygo-opened-at-login'
      DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APP_NAME}"
      DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run" "${APP_NAME}"
    ${EndIf}
  !insertmacro RemoveHostRegistration "Software\Google\Chrome" "chrome"
  !insertmacro RemoveHostRegistration "Software\Microsoft\Edge" "edge"
  !insertmacro RemoveHostRegistration "Software\BraveSoftware\Brave-Browser" "brave"
  !insertmacro RemoveHostRegistration "Software\Mozilla" "firefox"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
    DeleteRegKey HKCU "Software\${APP_NAME}"
    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    RMDir "$SMPROGRAMS\${APP_NAME}"
  ${EndIf}
  Delete "$INSTDIR\passpine.exe"
  Delete "$INSTDIR\passpine-host.exe"
  Delete "$INSTDIR\build-channel.ini"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
