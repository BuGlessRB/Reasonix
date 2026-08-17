; Reasonix Studio's Windows installer.
;
; Studio installs one executable beside the SPA it draws its UI from, so this is
; a plain per-machine install rather than the desktop line's versioned layout —
; that one exists to swap a running install under a launcher, which Studio does
; not have yet. Windows artifacts from this line are unsigned: the production
; SignPath policy allows exactly one origin (protected main-v2), so a build from
; the studio branch cannot request it and SmartScreen will warn.
;
; Invoked by scripts/studio-build.sh:
;   makensis -DVERSION=2.0.0 -DPAYLOAD=<dir> -DOUTFILE=<path> studio.nsi

Unicode true
ManifestDPIAware true

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!insertmacro GetSize

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef PAYLOAD
  !error "PAYLOAD is required"
!endif
!ifndef OUTFILE
  !error "OUTFILE is required"
!endif

!define APPNAME "Reasonix Studio"
!define BINNAME "reasonix-studio.exe"
!define PUBLISHER "Reasonix Contributors"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\ReasonixStudio"

Name "${APPNAME}"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\${APPNAME}"
InstallDirRegKey HKLM "Software\ReasonixStudio" "InstallDir"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APPNAME}"
VIAddVersionKey "FileDescription" "${APPNAME} installer"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "LegalCopyright" "Copyright © 2026 ${PUBLISHER}"

!define MUI_ABORTWARNING
!define MUI_ICON "${PAYLOAD}\appicon.ico"
!define MUI_UNICON "${PAYLOAD}\appicon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${BINNAME}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${APPNAME}"

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "TradChinese"

; WebView2 is what Wails renders into. A machine without it shows an empty
; window rather than an error, so the bootstrapper runs before the first launch
; and is skipped when any runtime is already registered. WEBVIEW2 is defined
; only when the build could fetch the bootstrapper; without it the installer
; still installs, and Windows 11 already ships the runtime.
Function EnsureWebView2
!ifdef WEBVIEW2
  ReadRegStr $0 HKLM \
    "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 != ""
    Return
  ${EndIf}
  ReadRegStr $0 HKCU \
    "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 != ""
    Return
  ${EndIf}
  DetailPrint "Installing the WebView2 runtime…"
  InitPluginsDir
  File /oname=$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe "${WEBVIEW2}"
  ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
!endif
FunctionEnd

; An in-app update starts this installer and then exits, so for a moment the old
; Studio still holds its executable open and Windows refuses to replace it.
; Waiting for the file to become writable is what turns that race into a pause;
; giving up after the timeout lets the File command report the real error rather
; than looping forever behind an app the user never closed.
Function WaitForRunningStudio
  StrCpy $R1 0
  wait:
    IfFileExists "$INSTDIR\${BINNAME}" 0 done
    ClearErrors
    FileOpen $R2 "$INSTDIR\${BINNAME}" a
    IfErrors busy
    FileClose $R2
    Goto done
  busy:
    IntOp $R1 $R1 + 1
    IntCmp $R1 40 done done 0
    DetailPrint "Waiting for ${APPNAME} to close…"
    Sleep 500
    Goto wait
  done:
FunctionEnd

Section "install"
  SetOutPath "$INSTDIR"
  Call WaitForRunningStudio
  File "${PAYLOAD}\${BINNAME}"
  File "${PAYLOAD}\appicon.ico"
  ; frontendAssets() resolves the SPA next to the executable, so the tree has to
  ; land as a sibling and not be flattened into $INSTDIR.
  SetOutPath "$INSTDIR\frontend-next\dist"
  File /r "${PAYLOAD}\frontend-next\dist\*"

  SetOutPath "$INSTDIR"
  Call EnsureWebView2

  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${BINNAME}" "" "$INSTDIR\appicon.ico"

  WriteRegStr HKLM "Software\ReasonixStudio" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTKEY}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\appicon.ico"
  WriteRegStr HKLM "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKLM "${UNINSTKEY}" "EstimatedSize" "$0"

  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "uninstall"
  Delete "$INSTDIR\${BINNAME}"
  Delete "$INSTDIR\appicon.ico"
  Delete "$INSTDIR\uninstall.exe"
  RMDir /r "$INSTDIR\frontend-next"
  ; Only the directory this installer created; /r on $INSTDIR would take a
  ; user's files with it if they ever installed into a shared folder.
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  RMDir "$SMPROGRAMS\${APPNAME}"

  DeleteRegKey HKLM "${UNINSTKEY}"
  DeleteRegKey HKLM "Software\ReasonixStudio"
SectionEnd
