# Taking over the Wails install, once.
#
# It is per-machine under Program Files with an uninstall key of its own
# (studio.nsi writes Software\ReasonixStudio and ...\Uninstall\ReasonixStudio),
# while this one is per-user with a key electron-builder derives from the appId.
# Nothing about installing this one would ever replace that one: both would sit
# in Add/Remove Programs, both would stay on disk, and only one of them would
# ever be updated again.
#
# Its uninstaller manifests admin, so ExecShellWait rather than ExecWait --
# CreateProcess refuses that image from an installer running as the user. A
# declined prompt leaves the old install where it was and this one working: the
# takeover never fails the install it is cleaning up after.

# view is the registry view to look in. The Wails installer never called
# SetRegView, so on x64 it wrote through WOW64 redirection into
# Software\WOW6432Node; electron-builder has selected the 64-bit view by the
# time this runs, which is where that key is not.
!macro takeOverLegacyStudio view
  SetRegView ${view}
  ReadRegStr $R9 HKLM "Software\ReasonixStudio" "InstallDir"
  ${If} $R9 != ""
  ${AndIf} ${FileExists} "$R9\uninstall.exe"
    DetailPrint "Removing the previous Reasonix Studio installation"
    ExecShellWait "open" "$R9\uninstall.exe" "/S" SW_HIDE
  ${EndIf}
!macroend

# A register rather than a Var: this file is compiled into the uninstaller
# script as well, where customInstall is never inserted and a declared Var is
# an unused one -- which electron-builder builds with warnings as errors.
!macro customInstall
  Push $R9
  !insertmacro takeOverLegacyStudio 32
  !insertmacro takeOverLegacyStudio 64
  # Leave the view electron-builder chose, not the one looked in last.
  ${If} ${RunningX64}
    SetRegView 64
  ${Else}
    SetRegView 32
  ${EndIf}
  Pop $R9
!macroend
