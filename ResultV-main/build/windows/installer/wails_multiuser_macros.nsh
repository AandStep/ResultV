!include "LogicLib.nsh"

!macro rp.setShellContext
  ${If} $MultiUser.InstallMode == "AllUsers"
    SetShellVarContext all
  ${Else}
    SetShellVarContext current
  ${EndIf}
!macroend

!macro rp.registerResultVProtocol
  SetRegView 64
  ; Wipe any prior registration in BOTH hives. Without this, a CurrentUser
  ; reinstall after an AllUsers install (or vice versa) leaves a stale entry
  ; that browsers prefer over the new one.
  DeleteRegKey HKLM "Software\Classes\resultv"
  DeleteRegKey HKCU "Software\Classes\resultv"

  ; HKLM is the machine-wide fallback. With MULTIUSER_EXECUTIONLEVEL=Highest
  ; we always have rights here (the installer is elevated regardless of which
  ; install mode the user picked), so write it unconditionally. This is what
  ; makes browser deep links work even when:
  ;   - user picked "Current user" install — the runtime fallback fixes HKCU
  ;     later, but until then HKLM is what answers browser lookups
  ;   - user picked a custom install path — HKLM points to whichever path
  ;     was chosen, properly quoted
  ;   - the launching user never opens the app themselves (so the runtime
  ;     fallback never writes the real user's HKCU)
  ClearErrors
  WriteRegStr HKLM "Software\Classes\resultv" "" "URL:ResultV Protocol"
  ${IfNot} ${Errors}
    WriteRegStr HKLM "Software\Classes\resultv" "URL Protocol" ""
    WriteRegStr HKLM "Software\Classes\resultv\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr HKLM "Software\Classes\resultv\shell" "" ""
    WriteRegStr HKLM "Software\Classes\resultv\shell\open" "" ""
    WriteRegStr HKLM "Software\Classes\resultv\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""
  ${EndIf}

  ; HKCU is per-process. When the installer is elevated this hive is the
  ; admin's, not the launching user's — so this write is only useful for
  ; admin-as-user testing scenarios. The real per-user registration happens
  ; in system.RegisterResultVProtocol() on first non-elevated app launch
  ; (see MUI_FINISHPAGE_RUN_FUNCTION below — we relaunch via explorer.exe
  ; to drop the admin token).
  WriteRegStr HKCU "Software\Classes\resultv" "" "URL:ResultV Protocol"
  WriteRegStr HKCU "Software\Classes\resultv" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\resultv\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
  WriteRegStr HKCU "Software\Classes\resultv\shell" "" ""
  WriteRegStr HKCU "Software\Classes\resultv\shell\open" "" ""
  WriteRegStr HKCU "Software\Classes\resultv\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""

  ; Notify Windows shell/browsers that associations changed (avoids stale cache
  ; where freshly installed custom protocol handlers are not picked up until relogin).
  System::Call 'shell32::SHChangeNotify(l, l, p, p) v (0x08000000, 0x0000, 0, 0)'
!macroend

!macro rp.unregisterResultVProtocol
  SetRegView 64
  ; Mirror rp.registerResultVProtocol — we always wrote to HKLM, so always
  ; clean it up. HKCU cleanup is best-effort: the real user's hive (where
  ; the runtime fallback wrote) may not be reachable from an elevated
  ; uninstall, but it's harmless to leave dangling — the next install or
  ; first app launch overwrites it.
  DeleteRegKey HKLM "Software\Classes\resultv"
  DeleteRegKey HKCU "Software\Classes\resultv"
  System::Call 'shell32::SHChangeNotify(l, l, p, p) v (0x08000000, 0x0000, 0, 0)'
!macroend

!macro rp.writeUninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  SetRegView 64
  ${If} $MultiUser.InstallMode == "AllUsers"
    WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKLM "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"

    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKLM "${UNINST_KEY}" "EstimatedSize" "$0"
  ${Else}
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"

    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
  ${EndIf}
!macroend

!macro rp.deleteUninstaller
  Delete "$INSTDIR\uninstall.exe"

  SetRegView 64
  ${If} $MultiUser.InstallMode == "AllUsers"
    DeleteRegKey HKLM "${UNINST_KEY}"
  ${Else}
    DeleteRegKey HKCU "${UNINST_KEY}"
  ${EndIf}
!macroend

!macro rp.webview2runtime
  !ifndef WAILS_INSTALL_WEBVIEW_DETAILPRINT
    !define WAILS_INSTALL_WEBVIEW_DETAILPRINT "Installing: WebView2 Runtime"
  !endif

  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 != ""
    Goto rp_wv_ok
  ${EndIf}

  ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 != ""
    Goto rp_wv_ok
  ${EndIf}

  SetDetailsPrint both
  DetailPrint "${WAILS_INSTALL_WEBVIEW_DETAILPRINT}"
  SetDetailsPrint listonly

  InitPluginsDir
  CreateDirectory "$pluginsdir\webview2bootstrapper"
  SetOutPath "$pluginsdir\webview2bootstrapper"
  File "tmp\MicrosoftEdgeWebview2Setup.exe"
  ExecWait '"$pluginsdir\webview2bootstrapper\MicrosoftEdgeWebview2Setup.exe" /silent /install'

  SetDetailsPrint both
  rp_wv_ok:
!macroend
