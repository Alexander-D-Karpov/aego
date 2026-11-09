; Per-user installer for the Aego editor and CLI; running it again updates the installation.
; makensis -DVERSION=0.10.0 -DSRC=<dir with aego.exe, aego-editor.exe, VERSION, README.txt> -DOUT=<setup.exe> aego.nsi
Unicode true
SetCompressor /SOLID lzma
RequestExecutionLevel user

!include "MUI2.nsh"
!include "FileFunc.nsh"

Name "Aego ${VERSION}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\Aego"
InstallDirRegKey HKCU "Software\Aego" "InstallDir"
BrandingText "Aego ${VERSION}"

!define UNINST "Software\Microsoft\Windows\CurrentVersion\Uninstall\Aego"
!define CLASSES "Software\Classes"
!define MUI_ABORTWARNING
!ifdef ICON
  !define MUI_ICON "${ICON}"
  !define MUI_UNICON "${ICON}"
!endif
!define MUI_FINISHPAGE_RUN "$INSTDIR\aego-editor.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open Aego"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

!macro Assoc EXT PROGID DESC
  WriteRegStr HKCU "${CLASSES}\${EXT}" "" "${PROGID}"
  WriteRegStr HKCU "${CLASSES}\${PROGID}" "" "${DESC}"
  WriteRegStr HKCU "${CLASSES}\${PROGID}\DefaultIcon" "" '"$INSTDIR\aego-editor.exe",0'
  WriteRegStr HKCU "${CLASSES}\${PROGID}\shell\open\command" "" '"$INSTDIR\aego-editor.exe" "%1"'
!macroend

!macro Unassoc EXT PROGID
  ReadRegStr $0 HKCU "${CLASSES}\${EXT}" ""
  StrCmp $0 "${PROGID}" 0 +2
  DeleteRegKey HKCU "${CLASSES}\${EXT}"
  DeleteRegKey HKCU "${CLASSES}\${PROGID}"
!macroend

Section "Aego" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"
  ; a running editor would keep the old files locked
  nsExec::Exec 'taskkill /IM aego-editor.exe /F'
  File "${SRC}\aego.exe"
  File "${SRC}\aego-editor.exe"
  File "${SRC}\VERSION"
  File "${SRC}\README.txt"
  ; Use a native separator: Windows makensis does not resolve mixed paths.
  File /oname=aego-path.ps1 "${__FILEDIR__}\path.ps1"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  ; the next update keeps this installer in updates\previous, so Restore Previous Version can run it again
  StrCmp "$EXEPATH" "$INSTDIR\setup.exe" +2
  CopyFiles /SILENT "$EXEPATH" "$INSTDIR\setup.exe"

  CreateDirectory "$SMPROGRAMS\Aego"
  CreateShortcut "$SMPROGRAMS\Aego\Aego.lnk" "$INSTDIR\aego-editor.exe"
  CreateShortcut "$SMPROGRAMS\Aego\Uninstall Aego.lnk" "$INSTDIR\uninstall.exe"
  CreateShortcut "$DESKTOP\Aego.lnk" "$INSTDIR\aego-editor.exe"

  ; Claim only an entry we add; preserve an existing user PATH verbatim.
  nsExec::ExecToLog '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\aego-path.ps1" -Action Install -Directory "$INSTDIR"'
  Pop $0
  StrCmp $0 "0" path_installed
  MessageBox MB_ICONSTOP "Could not safely update the user PATH. See installation details."
  Abort
  path_installed:

  !insertmacro Assoc ".aego" "Aego.Project" "Aego Project"
  !insertmacro Assoc ".aescene" "Aego.Scene" "Aego Scene"
  !insertmacro Assoc ".aeprefab" "Aego.Prefab" "Aego Prefab"

  WriteRegStr HKCU "Software\Aego" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINST}" "DisplayName" "Aego"
  WriteRegStr HKCU "${UNINST}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST}" "Publisher" "Aego"
  WriteRegStr HKCU "${UNINST}" "DisplayIcon" "$INSTDIR\aego-editor.exe"
  WriteRegStr HKCU "${UNINST}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINST}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST}" "NoRepair" 1
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
SectionEnd

; Optional rather than silently taking an existing URL scheme from another
; installation. The CLI still validates the single untrusted URL and asks for
; confirmation before network/project activity.
Section /o "Handle aego: example links" SecProtocol
  ReadRegStr $0 HKCU "${CLASSES}\aego\shell\open\command" ""
  StrCmp $0 '"$INSTDIR\aego-editor.exe" --example-link "%1"' protocol_write
  StrCmp $0 "" 0 protocol_occupied
  ; A key with unrelated values/children is not ours, even if it has no command.
  ClearErrors
  EnumRegValue $1 HKCU "${CLASSES}\aego" 0
  IfErrors protocol_no_values protocol_occupied
  protocol_no_values:
  ClearErrors
  EnumRegKey $1 HKCU "${CLASSES}\aego" 0
  IfErrors protocol_no_user_keys protocol_occupied
  protocol_no_user_keys:
  ; Do not shadow an existing machine-wide handler either.
  ReadRegStr $1 HKLM "${CLASSES}\aego\shell\open\command" ""
  StrCmp $1 "" protocol_write protocol_occupied
  protocol_write:
  WriteRegStr HKCU "${CLASSES}\aego" "" "URL:Aego Example"
  WriteRegStr HKCU "${CLASSES}\aego" "URL Protocol" ""
  WriteRegStr HKCU "${CLASSES}\aego\shell\open\command" "" '"$INSTDIR\aego-editor.exe" --example-link "%1"'
  Goto protocol_done
  protocol_occupied:
  DetailPrint "Existing aego: handler preserved; register Aego manually if desired."
  protocol_done:
SectionEnd

Section "Open with Aego in folder menus" SecFolder
  WriteRegStr HKCU "${CLASSES}\Directory\shell\Aego" "" "Open with Aego"
  WriteRegStr HKCU "${CLASSES}\Directory\shell\Aego" "Icon" '"$INSTDIR\aego-editor.exe",0'
  WriteRegStr HKCU "${CLASSES}\Directory\shell\Aego\command" "" '"$INSTDIR\aego-editor.exe" "%1"'
  WriteRegStr HKCU "${CLASSES}\Directory\Background\shell\Aego" "" "Open with Aego"
  WriteRegStr HKCU "${CLASSES}\Directory\Background\shell\Aego" "Icon" '"$INSTDIR\aego-editor.exe",0'
  WriteRegStr HKCU "${CLASSES}\Directory\Background\shell\Aego\command" "" '"$INSTDIR\aego-editor.exe" "%V"'
SectionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /IM aego-editor.exe /F'
  nsExec::ExecToLog '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\aego-path.ps1" -Action Uninstall -Directory "$INSTDIR"'
  Pop $0
  StrCmp $0 "0" path_removed
  MessageBox MB_ICONSTOP "Could not safely restore the user PATH. See uninstall details."
  Abort
  path_removed:
  !insertmacro Unassoc ".aego" "Aego.Project"
  !insertmacro Unassoc ".aescene" "Aego.Scene"
  !insertmacro Unassoc ".aeprefab" "Aego.Prefab"
  ReadRegStr $0 HKCU "${CLASSES}\aego\shell\open\command" ""
  StrCmp $0 '"$INSTDIR\aego-editor.exe" --example-link "%1"' 0 protocol_uninstall_done
  DeleteRegValue HKCU "${CLASSES}\aego\shell\open\command" ""
  DeleteRegKey /ifempty HKCU "${CLASSES}\aego\shell\open\command"
  DeleteRegKey /ifempty HKCU "${CLASSES}\aego\shell\open"
  DeleteRegKey /ifempty HKCU "${CLASSES}\aego\shell"
  ReadRegStr $0 HKCU "${CLASSES}\aego" ""
  StrCmp $0 "URL:Aego Example" 0 +2
  DeleteRegValue HKCU "${CLASSES}\aego" ""
  ReadRegStr $0 HKCU "${CLASSES}\aego" "URL Protocol"
  StrCmp $0 "" 0 +2
  DeleteRegValue HKCU "${CLASSES}\aego" "URL Protocol"
  DeleteRegKey /ifempty HKCU "${CLASSES}\aego"
  protocol_uninstall_done:
  DeleteRegKey HKCU "${CLASSES}\Directory\shell\Aego"
  DeleteRegKey HKCU "${CLASSES}\Directory\Background\shell\Aego"
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
  Delete "$INSTDIR\aego.exe"
  Delete "$INSTDIR\aego-editor.exe"
  Delete "$INSTDIR\VERSION"
  Delete "$INSTDIR\README.txt"
  Delete "$INSTDIR\aego-path.ps1"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\setup.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Aego\Aego.lnk"
  Delete "$SMPROGRAMS\Aego\Uninstall Aego.lnk"
  RMDir "$SMPROGRAMS\Aego"
  Delete "$DESKTOP\Aego.lnk"
  DeleteRegKey HKCU "${UNINST}"
  DeleteRegKey HKCU "Software\Aego"
SectionEnd

; /UPDATE: the editor or aego.exe started this installer as it quit; give it a moment to let go of its files.
Function .onInit
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/UPDATE" $1
  IfErrors done
  Sleep 2000
  done:
FunctionEnd

; /UPDATE: the editor ran this installer silently as it quit to update itself; start it again on the new version.
Function .onInstSuccess
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/UPDATE" $1
  IfErrors done
  Exec '"$INSTDIR\aego-editor.exe"'
  done:
FunctionEnd
