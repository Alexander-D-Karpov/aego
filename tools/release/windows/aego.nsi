; Per-user installer for the Aego editor and CLI; running it again updates the installation.
; makensis -DVERSION=0.9.0 -DSRC=<dir with aego.exe, aego-editor.exe, VERSION, README.txt> -DOUT=<setup.exe> aego.nsi
Unicode true
SetCompressor /SOLID lzma
RequestExecutionLevel user

!include "MUI2.nsh"

Name "Aego ${VERSION}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\Aego"
InstallDirRegKey HKCU "Software\Aego" "InstallDir"
BrandingText "Aego ${VERSION}"

!define UNINST "Software\Microsoft\Windows\CurrentVersion\Uninstall\Aego"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\aego-editor.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open the Aego editor"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Aego"
  SetOutPath "$INSTDIR"
  ; a running editor would keep the old files locked
  nsExec::Exec 'taskkill /IM aego-editor.exe /F'
  File "${SRC}\aego.exe"
  File "${SRC}\aego-editor.exe"
  File "${SRC}\VERSION"
  File "${SRC}\README.txt"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateDirectory "$SMPROGRAMS\Aego"
  CreateShortcut "$SMPROGRAMS\Aego\Aego.lnk" "$INSTDIR\aego-editor.exe"
  CreateShortcut "$SMPROGRAMS\Aego\Uninstall Aego.lnk" "$INSTDIR\uninstall.exe"
  CreateShortcut "$DESKTOP\Aego.lnk" "$INSTDIR\aego-editor.exe"

  ; put aego.exe on the user PATH once
  nsExec::Exec `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$d='$INSTDIR'; $$p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not $$p) { $$p='' }; if (($$p -split ';') -notcontains $$d) { [Environment]::SetEnvironmentVariable('Path', ($$p.TrimEnd(';') + ';' + $$d).TrimStart(';'), 'User') }"`

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
SectionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /IM aego-editor.exe /F'
  nsExec::Exec `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$d='$INSTDIR'; $$p=[Environment]::GetEnvironmentVariable('Path','User'); if ($$p) { [Environment]::SetEnvironmentVariable('Path', (($$p -split ';') | Where-Object { $$_ -and $$_ -ne $$d }) -join ';', 'User') }"`
  Delete "$INSTDIR\aego.exe"
  Delete "$INSTDIR\aego-editor.exe"
  Delete "$INSTDIR\VERSION"
  Delete "$INSTDIR\README.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Aego\Aego.lnk"
  Delete "$SMPROGRAMS\Aego\Uninstall Aego.lnk"
  RMDir "$SMPROGRAMS\Aego"
  Delete "$DESKTOP\Aego.lnk"
  DeleteRegKey HKCU "${UNINST}"
  DeleteRegKey HKCU "Software\Aego"
SectionEnd
