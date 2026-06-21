Unicode true

!ifndef INFO_COMPANYNAME
  !define INFO_COMPANYNAME "ResultV"
!endif
!ifndef INFO_PRODUCTNAME
  !define INFO_PRODUCTNAME "ResultV"
!endif
!ifndef UNINST_KEY_NAME
  !define UNINST_KEY_NAME "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
!endif
!define LEGACY_UNINST_KEY_NAME "ResultProxyResultProxy"
!define LEGACY_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${LEGACY_UNINST_KEY_NAME}"

!define MULTIUSER_EXECUTIONLEVEL Highest
!define MULTIUSER_MUI
!define MULTIUSER_INSTALLMODE_COMMANDLINE
; Default to "All users" — writes the resultv:// handler to HKLM at install
; time, which is the only path that works reliably for browser deep links
; before the user has opened the app for the first time. Per-user is still
; selectable on the install-mode page (or via /CurrentUser on the cmdline).
!define MULTIUSER_USE_PROGRAMFILES64
!define MULTIUSER_INSTALLMODEPAGE_SHOWUSERNAME
!define MULTIUSER_INSTALLMODE_INSTDIR "${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!define MULTIUSER_INSTALLMODE_INSTDIR_REGISTRY_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}"
!define MULTIUSER_INSTALLMODE_INSTDIR_REGISTRY_VALUENAME "InstallLocation"
!define MULTIUSER_INSTALLMODE_DEFAULT_REGISTRY_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}"
!define MULTIUSER_INSTALLMODE_DEFAULT_REGISTRY_VALUENAME "InstallLocation"

!include "MultiUser.nsh"

!define REQUEST_EXECUTION_LEVEL highest
!include "wails_tools.nsh"
!include "wails_multiuser_macros.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
; Empty MUI_FINISHPAGE_RUN + MUI_FINISHPAGE_RUN_FUNCTION makes MUI render the
; "Run X" checkbox but delegate the launch to our function instead of doing
; a direct Exec. We need that delegation so the app does NOT inherit the
; installer's admin token — otherwise the runtime resultv:// protocol
; registration (system.RegisterResultVProtocol) writes to admin's HKCU, not
; the launching user's, and browser deep links break for "Current user"
; installs and for any first launch right after install.
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Run ${INFO_PRODUCTNAME} after closing the installer"
!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchResultVAsUser"

!define MUI_LANGDLL_ALLLANGUAGES

!insertmacro MUI_PAGE_WELCOME

!insertmacro MULTIUSER_PAGE_INSTALLMODE

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
  !insertmacro MULTIUSER_INIT
  !insertmacro wails.checkArchitecture
  Call CheckLegacyResultProxyInstall
  Call CheckWebView2Present
FunctionEnd

Function un.onInit
  !insertmacro MULTIUSER_UNINIT
FunctionEnd

; Launch the app via explorer.exe so the new process runs under the *launching*
; user's token rather than inheriting the elevated installer's admin token.
; This is required for the runtime resultv:// fallback in
; system.RegisterResultVProtocol() to write to the real user's HKCU. Direct
; Exec from an elevated installer hands the app an admin token and the HKCU
; write lands in admin's hive, where the user's browser can't see it.
Function LaunchResultVAsUser
  Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Function CheckWebView2Present
  IfSilent silent done
  silent:
    Return
  done:
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 != ""
    Return
  ${EndIf}
  ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 != ""
    Return
  ${EndIf}
  MessageBox MB_OK|MB_ICONINFORMATION "WebView2 Runtime was not detected. It will be installed during setup."
FunctionEnd

Function CheckLegacyResultProxyInstall
  SetRegView 64
  ReadRegStr $0 HKLM "${LEGACY_UNINST_KEY}" "QuietUninstallString"
  ReadRegStr $1 HKLM "${LEGACY_UNINST_KEY}" "UninstallString"
  ReadRegStr $2 HKLM "${LEGACY_UNINST_KEY}" "DisplayName"
  ${If} $0 == ""
    ReadRegStr $0 HKCU "${LEGACY_UNINST_KEY}" "QuietUninstallString"
  ${EndIf}
  ${If} $1 == ""
    ReadRegStr $1 HKCU "${LEGACY_UNINST_KEY}" "UninstallString"
  ${EndIf}
  ${If} $2 == ""
    ReadRegStr $2 HKCU "${LEGACY_UNINST_KEY}" "DisplayName"
  ${EndIf}
  ${If} $0 == ""
  ${AndIf} $1 == ""
    Return
  ${EndIf}
  ${If} $2 == ""
    StrCpy $2 "ResultProxy"
  ${EndIf}
  MessageBox MB_YESNO|MB_ICONQUESTION "$2 is already installed. Remove old version before installing ${INFO_PRODUCTNAME}?" IDNO legacy_skip
  Call PreserveLegacyUserData
  ${If} $0 != ""
    ExecWait '$0'
  ${Else}
    ExecWait '$1 /S'
  ${EndIf}
  SetShellVarContext all
  Delete "$SMPROGRAMS\ResultProxy.lnk"
  Delete "$DESKTOP\ResultProxy.lnk"
  SetShellVarContext current
  Delete "$SMPROGRAMS\ResultProxy.lnk"
  Delete "$DESKTOP\ResultProxy.lnk"
legacy_skip:
FunctionEnd

Function PreserveLegacyUserData
  CreateDirectory "$APPDATA\ResultV"
  IfFileExists "$APPDATA\ResultProxy\proxy_config.json" check_config keep_config
  check_config:
    StrCpy $3 ""
    StrCpy $4 ""
    IfFileExists "$APPDATA\ResultV\proxy_config.json" 0 copy_config
    ${GetSize} "$APPDATA\ResultV\proxy_config.json" "/S=0K" $3 $0 $1
    ${GetSize} "$APPDATA\ResultProxy\proxy_config.json" "/S=0K" $4 $0 $1
    IntCmp $3 1 copy_config 0 size_compare
  size_compare:
    IntCmp $4 $3 copy_config keep_config keep_config
  copy_config:
    CopyFiles /SILENT "$APPDATA\ResultProxy\proxy_config.json" "$APPDATA\ResultV\proxy_config.json"
  keep_config:
  IfFileExists "$APPDATA\ResultV\.machine-fallback-id" keep_fallback copy_fallback
  copy_fallback:
    IfFileExists "$APPDATA\ResultProxy\.machine-fallback-id" 0 keep_fallback
    CopyFiles /SILENT "$APPDATA\ResultProxy\.machine-fallback-id" "$APPDATA\ResultV\.machine-fallback-id"
  keep_fallback:
  IfFileExists "$APPDATA\ResultV\blocked_cache.json" keep_blocked copy_blocked
  copy_blocked:
    IfFileExists "$APPDATA\ResultProxy\blocked_cache.json" 0 keep_blocked
    CopyFiles /SILENT "$APPDATA\ResultProxy\blocked_cache.json" "$APPDATA\ResultV\blocked_cache.json"
  keep_blocked:
  IfFileExists "$APPDATA\ResultV\sing-box-cache.db" keep_cache copy_cache
  copy_cache:
    IfFileExists "$APPDATA\ResultProxy\sing-box-cache.db" 0 keep_cache
    CopyFiles /SILENT "$APPDATA\ResultProxy\sing-box-cache.db" "$APPDATA\ResultV\sing-box-cache.db"
  keep_cache:
  CreateDirectory "$LOCALAPPDATA\ResultV"
  IfFileExists "$LOCALAPPDATA\ResultV\webview\*.*" done copy_webview
  copy_webview:
    IfFileExists "$LOCALAPPDATA\ResultProxy\webview\*.*" 0 done
    CreateDirectory "$LOCALAPPDATA\ResultV\webview"
    CopyFiles /SILENT "$LOCALAPPDATA\ResultProxy\webview\*.*" "$LOCALAPPDATA\ResultV\webview"
  done:
FunctionEnd

Section
  !insertmacro rp.setShellContext

  !insertmacro rp.webview2runtime

  SetOutPath $INSTDIR

  !insertmacro wails.files
  ; sing-box naive (Cronet / purego): must sit next to ResultV.exe (see scripts/ensure-libcronet-windows.ps1)
  File "/oname=libcronet.dll" "..\libcronet.dll"

  CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  Delete "$SMPROGRAMS\ResultProxy.lnk"
  Delete "$DESKTOP\ResultProxy.lnk"

  !insertmacro wails.associateFiles
  !insertmacro wails.associateCustomProtocols
  !insertmacro rp.registerResultVProtocol

  !insertmacro rp.writeUninstaller
  SetRegView 64
  ${If} $MultiUser.InstallMode == "AllUsers"
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  ${Else}
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  ${EndIf}
SectionEnd

Section "uninstall"
  !insertmacro rp.setShellContext

  ; Kill Switch firewall cleanup (best-effort): these rules persist beyond
  ; process lifetime and must be removed on uninstall/reinstall.
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_BlockAll"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_AllowLocal"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_AllowProxy"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_AllowDNS"'

  StrCpy $R0 0
ks_proxy_loop:
  IntCmp $R0 64 ks_proxy_next ks_dns_loop ks_dns_loop
ks_proxy_next:
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_AllowProxy_$R0"'
  IntOp $R0 $R0 + 1
  Goto ks_proxy_loop

ks_dns_loop:
  StrCpy $R0 0
ks_dns_item_loop:
  IntCmp $R0 16 ks_dns_next ks_cleanup_done ks_cleanup_done
ks_dns_next:
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_AllowDNS_$R0_udp"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="ResultV_KillSwitch_AllowDNS_$R0_tcp"'
  IntOp $R0 $R0 + 1
  Goto ks_dns_item_loop

ks_cleanup_done:

  RMDir /r $INSTDIR

  Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
  Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

  !insertmacro wails.unassociateFiles
  !insertmacro wails.unassociateCustomProtocols
  !insertmacro rp.unregisterResultVProtocol

  !insertmacro rp.deleteUninstaller
SectionEnd
