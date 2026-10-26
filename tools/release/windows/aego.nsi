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
  WriteUninstaller "$INSTDIR\uninstall.exe"
  ; the next update keeps this installer in updates\previous, so Restore Previous Version can run it again
  StrCmp "$EXEPATH" "$INSTDIR\setup.exe" +2
  CopyFiles /SILENT "$EXEPATH" "$INSTDIR\setup.exe"

  CreateDirectory "$SMPROGRAMS\Aego"
  CreateShortcut "$SMPROGRAMS\Aego\Aego.lnk" "$INSTDIR\aego-editor.exe"
  CreateShortcut "$SMPROGRAMS\Aego\Uninstall Aego.lnk" "$INSTDIR\uninstall.exe"
  CreateShortcut "$DESKTOP\Aego.lnk" "$INSTDIR\aego-editor.exe"

  ; put aego.exe on the user PATH once
  nsExec::Exec `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$d='$INSTDIR'; $$p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not $$p) { $$p='' }; if (($$p -split ';') -notcontains $$d) { [Environment]::SetEnvironmentVariable('Path', ($$p.TrimEnd(';') + ';' + $$d).TrimStart(';'), 'User') }"`

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
  nsExec::Exec `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$d='$INSTDIR'; $$p=[Environment]::GetEnvironmentVariable('Path','User'); if ($$p) { [Environment]::SetEnvironmentVariable('Path', (($$p -split ';') | Where-Object { $$_ -and $$_ -ne $$d }) -join ';', 'User') }"`
  !insertmacro Unassoc ".aego" "Aego.Project"
  !insertmacro Unassoc ".aescene" "Aego.Scene"
  !insertmacro Unassoc ".aeprefab" "Aego.Prefab"
  DeleteRegKey HKCU "${CLASSES}\Directory\shell\Aego"
  DeleteRegKey HKCU "${CLASSES}\Directory\Background\shell\Aego"
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
  Delete "$INSTDIR\aego.exe"
  Delete "$INSTDIR\aego-editor.exe"
  Delete "$INSTDIR\VERSION"
  Delete "$INSTDIR\README.txt"
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
