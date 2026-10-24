Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!ifndef APP_VERSION
  !error "APP_VERSION is required"
!endif
!ifndef APP_BASE_VERSION
  !error "APP_BASE_VERSION is required"
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
VIProductVersion "${APP_BASE_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "${APP_NAME}"
VIAddVersionKey /LANG=1033 "FileDescription" "${APP_NAME} installer"
VIAddVersionKey /LANG=1033 "FileVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=1033 "ProductVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Passpine contributors"
# The updater refuses to execute older installers that lack this signed marker:
# NSIS versions without the dedicated mode otherwise ignore unknown arguments.
VIAddVersionKey /LANG=1033 "PasspineExtractProtocol" "passpine-update-extract-v1-a8e19eb9-3dc3-4ccf-8b58-97546f3051f0"
!define MUI_ABORTWARNING
!define MUI_ICON "${DIST_DIR}\windows-resources\Passpine.ico"
!define MUI_UNICON "${DIST_DIR}\windows-resources\Passpine.ico"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

# Both entry points share one payload definition, so the ordinary installer is
# also the exact archive verified by the updater and release publication job.
Var ExtractDirectory
Var ExtractSeen
Var ExtractSilent
Var ExtractNormalArguments
Var ExtractInstallDirectorySeen
Var ExtractArgv
Var ExtractDirectoryHandle

Function ExtractApplicationFiles
  ClearErrors
  SetOutPath "$INSTDIR"
  IfErrors payload_extraction_failed
  File "${DIST_DIR}\passpine.exe"
  IfErrors payload_extraction_failed
  File "${DIST_DIR}\passpine-host.exe"
  IfErrors payload_extraction_failed
  Return
  payload_extraction_failed:
    SetErrorLevel 23
    Quit
FunctionEnd

# Refuse invalid extraction requests before any installation side effect. Use
# GetCommandLineW rather than $CMDLINE: NSIS removes /D= from $CMDLINE.
Function CheckExtractionRequest
  StrCpy $ExtractSeen 0
  StrCpy $ExtractSilent 0
  StrCpy $ExtractNormalArguments 0
  StrCpy $ExtractInstallDirectorySeen 0
  StrCpy $ExtractDirectoryHandle 0
  System::Call 'kernel32::GetCommandLineW() p .r0'
  System::Call 'shell32::CommandLineToArgvW(p r0, *i .r1) p .r2'
  StrCpy $ExtractArgv $2
  StrCmp $2 0 invalid_extract_arguments
  IntCmp $1 1 normal_install invalid_extract_arguments
  IntOp $1 $1 - 1
  IntOp $2 $2 + ${NSIS_PTR_SIZE}
  next_extract_argument:
    System::Call '*$2(p .r3)'
    System::Call 'kernel32::lstrlenW(p r3) i .r4'
    IntCmp $4 ${NSIS_MAX_STRLEN} invalid_extract_arguments 0 invalid_extract_arguments
    System::Call 'kernel32::lstrcpynW(w .r5, p r3, i ${NSIS_MAX_STRLEN})'
    StrCmpS $5 "/S" silent_extract_argument
    StrCpy $4 $5 22
    StrCmpS $4 "/PASSPINE_EXTRACT_DIR=" 0 not_extract_argument
      StrCmp $ExtractSeen 0 0 invalid_extract_arguments
      StrCpy $ExtractSeen 1
      StrCpy $ExtractDirectory $5 "" 22
      StrCmp $ExtractDirectory "" invalid_extract_arguments
      Goto extract_argument_done
    not_extract_argument:
      # Ordinary NSIS arguments remain available for manual installation, but
      # are never accepted alongside extraction mode.
      StrCpy $4 $5 3
      StrCmpS $4 "/D=" normal_install_directory_argument
      StrCmpS $5 "/NCRC" normal_extract_argument
      # /D= is the final NSIS parameter and intentionally has no quotes even
      # when its directory contains spaces. CommandLineToArgvW splits these
      # path fragments: preserve them for manual installation, while scanning
      # the entire original command line for attempted extraction parameters.
      System::Call 'shlwapi::StrStrIW(w r5, w "PASSPINE_EXTRACT") p .r4'
      StrCmp $4 0 0 invalid_extract_arguments
      StrCmp $ExtractInstallDirectorySeen 1 normal_extract_argument invalid_extract_arguments
    normal_install_directory_argument:
      StrCpy $ExtractInstallDirectorySeen 1
    normal_extract_argument:
      StrCpy $ExtractNormalArguments 1
      Goto extract_argument_done
    silent_extract_argument:
      IntOp $ExtractSilent $ExtractSilent + 1
    extract_argument_done:
      IntOp $2 $2 + ${NSIS_PTR_SIZE}
      IntOp $1 $1 - 1
      IntCmp $1 0 extract_arguments_done invalid_extract_arguments next_extract_argument
  extract_arguments_done:
    System::Call 'kernel32::LocalFree(p $ExtractArgv)'
    StrCpy $ExtractArgv 0
    StrCmp $ExtractSeen 0 normal_install
    StrCmp $ExtractSilent 1 0 invalid_extract_arguments
    StrCmp $ExtractNormalArguments 0 0 invalid_extract_arguments
    Call ValidateExtractionDirectory
    # This handle denies deletion/renaming of the staging directory while its
    # files are written. No application is launched in extraction mode.
    System::Call 'kernel32::CreateFileW(w "$ExtractDirectory", i 0, i 3, p 0, i 3, i 0x02200000, p 0) p .r0'
    StrCpy $ExtractDirectoryHandle $0
    StrCmp $0 -1 invalid_extract_arguments
    StrCpy $INSTDIR $ExtractDirectory
    Call ExtractApplicationFiles
    System::Call 'kernel32::CloseHandle(p $ExtractDirectoryHandle)'
    SetErrorLevel 0
    Quit
  invalid_extract_arguments:
    StrCmp $ExtractArgv 0 +2
      System::Call 'kernel32::LocalFree(p $ExtractArgv)'
    SetErrorLevel 87
    Quit
  normal_install:
    StrCmp $ExtractArgv 0 +2
      System::Call 'kernel32::LocalFree(p $ExtractArgv)'
!ifdef PASSPINE_TEST_PARSE_ONLY
  !if "${APP_NAME}" != "Passpine NSIS Parser Test"
    !error "PASSPINE_TEST_PARSE_ONLY is reserved for the isolated parser fixture"
  !endif
    # The CI parser fixture accepts ordinary install arguments without ever
    # entering installation sections or writing to their destination.
    SetErrorLevel 0
    Quit
!endif
FunctionEnd

Function ValidateExtractionDirectory
  # A canonical drive-absolute path only: no UNC, device names, ADS, aliases,
  # wildcard characters, trailing dots/spaces, or existing reparse points.
  StrLen $0 $ExtractDirectory
  IntCmp $0 4 0 invalid_extract_directory
  IntCmp $0 240 invalid_extract_directory 0 invalid_extract_directory
  StrCpy $1 $ExtractDirectory 2 1
  StrCmp $1 ":\" 0 invalid_extract_directory
  System::Call 'shlwapi::PathGetDriveNumberW(w "$ExtractDirectory") i .r1'
  IntCmp $1 -1 invalid_extract_directory
  System::Call 'kernel32::GetFullPathNameW(w "$ExtractDirectory", i ${NSIS_MAX_STRLEN}, w .r1, p 0) i .r2'
  StrCmp $1 $ExtractDirectory 0 invalid_extract_directory
  StrCmp $1 $INSTDIR invalid_extract_directory
  StrCmp $1 $EXEDIR invalid_extract_directory
  StrCpy $2 3
  StrCpy $3 ""
  extract_directory_char:
    StrCpy $1 $ExtractDirectory 1 $2
    StrCmp $1 "" extract_directory_chars_done
    StrCmp $1 ":" invalid_extract_directory
    StrCmp $1 "/" invalid_extract_directory
    StrCmp $1 '*' invalid_extract_directory
    StrCmp $1 '?' invalid_extract_directory
    StrCmp $1 '<' invalid_extract_directory
    StrCmp $1 '>' invalid_extract_directory
    StrCmp $1 '|' invalid_extract_directory
    StrCmp $1 '$\"' invalid_extract_directory
    StrCmp $1 '$\r' invalid_extract_directory
    StrCmp $1 '$\n' invalid_extract_directory
    StrCmp $1 '$\t' invalid_extract_directory
    StrCmp $1 "\" 0 extract_directory_char_done
      StrCmp $3 "." invalid_extract_directory
      StrCmp $3 " " invalid_extract_directory
      StrCmp $3 "\" invalid_extract_directory
    extract_directory_char_done:
      StrCpy $3 $1
      IntOp $2 $2 + 1
      Goto extract_directory_char
  extract_directory_chars_done:
    StrCmp $3 "." invalid_extract_directory
    StrCmp $3 " " invalid_extract_directory
    StrCmp $3 "\" invalid_extract_directory
    # Check every ancestor, not only the final directory, for junctions and
    # other reparse points. The destination must already exist and be empty.
    StrCpy $2 $ExtractDirectory
  extract_directory_parent:
    System::Call 'kernel32::GetFileAttributesW(w r2) i .r1'
    IntCmp $1 -1 invalid_extract_directory
    IntOp $1 $1 & 0x410
    IntCmp $1 0x10 0 invalid_extract_directory invalid_extract_directory
    StrLen $1 $2
    IntCmp $1 3 extract_directory_empty
    ${GetParent} "$2" $2
    StrLen $1 $2
    IntCmp $1 2 0 extract_directory_parent extract_directory_parent
    StrCpy $2 "$2\"
    Goto extract_directory_parent
  extract_directory_empty:
    ClearErrors
    FindFirst $0 $1 "$ExtractDirectory\*"
    IfErrors invalid_extract_directory
  extract_directory_entry:
    StrCmp $1 "" extract_directory_is_empty
    StrCmp $1 "." extract_directory_next
    StrCmp $1 ".." extract_directory_next
    FindClose $0
    Goto invalid_extract_directory
  extract_directory_next:
    ClearErrors
    FindNext $0 $1
    IfErrors extract_directory_is_empty extract_directory_entry
  extract_directory_is_empty:
    FindClose $0
    Return
  invalid_extract_directory:
    SetErrorLevel 87
    Quit
FunctionEnd

Function .onInit
  Call CheckExtractionRequest
!if "${APP_ARCH}" == "amd64"
  ${IfNot} ${RunningX64}
    SetErrorLevel 1633
    Quit
  ${EndIf}
!endif
FunctionEnd

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
  Call ExtractApplicationFiles
  WriteINIStr "$INSTDIR\build-channel.ini" "build" "channel" "${APP_CHANNEL}"
  WriteINIStr "$INSTDIR\build-channel.ini" "build" "directory" "$INSTDIR"
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
  ReadINIStr $0 "$INSTDIR\build-channel.ini" "build" "channel"
  ReadINIStr $1 "$INSTDIR\build-channel.ini" "build" "directory"
  ${If} $2 != $INSTDIR
  ${OrIf} $0 != "${APP_CHANNEL}"
  ${OrIf} $1 != $INSTDIR
    SetErrorLevel 5
    Abort
  ${EndIf}
  # Reject junctions/symlinks in the installation and all ancestor paths.
  StrCpy $3 "$INSTDIR"
  check_uninstall_parent:
    System::Call 'kernel32::GetFileAttributesW(w r3) i .r0'
    IntOp $0 $0 & 0x400
    ${If} $0 != 0
      SetErrorLevel 5
      Abort
    ${EndIf}
    ${GetParent} "$3" $4
    StrCmp $4 "" uninstall_path_checked
    StrCmp $4 $3 uninstall_path_checked
    StrCpy $3 $4
    Goto check_uninstall_parent
  uninstall_path_checked:
!if "${APP_CHANNEL}" == "dev"
  ${If} ${FileExists} "$INSTDIR\passpine.exe"
    ExecWait '$\"$INSTDIR\passpine.exe$\" --quit-for-uninstall' $0
    ${If} $0 != 0
      SetErrorLevel 6
      Abort
    ${EndIf}
  ${EndIf}
!endif
  ${If} ${FileExists} "$INSTDIR\passpine-host.exe"
    ExecWait '$\"$INSTDIR\passpine-host.exe$\" --unregister' $0
    ${If} $0 != 0
      SetErrorLevel 7
      Abort
    ${EndIf}
  ${EndIf}
  # Delete program files first; on failure keep the installation records and
  # uninstaller so the same operation can be retried without losing ownership.
  ClearErrors
  Delete "$INSTDIR\passpine.exe"
  Delete "$INSTDIR\passpine-host.exe"
  ${If} ${Errors}
    SetErrorLevel 8
    Abort
  ${EndIf}
  ${If} $2 == $INSTDIR
    ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APP_NAME}"
    ${If} $0 == '$\"$INSTDIR\passpine.exe$\" --mygo-opened-at-login'
      DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APP_NAME}"
      DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run" "${APP_NAME}"
    ${EndIf}
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
    DeleteRegKey HKCU "Software\${APP_NAME}"
    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    RMDir "$SMPROGRAMS\${APP_NAME}"
  ${EndIf}
  Delete "$INSTDIR\build-channel.ini"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
