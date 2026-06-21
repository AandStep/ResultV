# Windows Installer Validation

This directory contains NSIS installer customizations and validation helpers.

## Verify `resultv://` protocol registration

Use `verify_resultv_protocol.ps1` after installation to validate that registry keys are written to the expected hive(s) and command quoting is correct.

The installer always writes to `HKLM\Software\Classes\resultv` (machine-wide fallback used by browsers when the user's HKCU is empty). For CurrentUser installs it additionally writes the elevated user's HKCU. The real per-user HKCU registration is performed by the app on first launch via `system.RegisterResultVProtocol()`; the `MUI_FINISHPAGE_RUN` checkbox launches the app via `explorer.exe` so this happens under the launching user's token, not the elevated admin's.

### Current user install

1. Install with CurrentUser mode (UI or command line):
   - `installer.exe /CurrentUser`
2. Run (elevated shell, since we check HKLM too):
   - `powershell -ExecutionPolicy Bypass -File build/windows/installer/verify_resultv_protocol.ps1 -InstallMode CurrentUser`
3. Optional strict command check:
   - `powershell -ExecutionPolicy Bypass -File build/windows/installer/verify_resultv_protocol.ps1 -InstallMode CurrentUser -ExpectedExePath "$env:LOCALAPPDATA\\ResultV\\ResultV\\ResultV.exe"`

### All users install

1. Install with AllUsers mode:
   - `installer.exe /AllUsers`
2. Run (elevated shell):
   - `powershell -ExecutionPolicy Bypass -File build/windows/installer/verify_resultv_protocol.ps1 -InstallMode AllUsers`
3. Optional strict command check with your install path:
   - `powershell -ExecutionPolicy Bypass -File build/windows/installer/verify_resultv_protocol.ps1 -InstallMode AllUsers -ExpectedExePath "C:\\Program Files\\ResultV\\ResultV\\ResultV.exe"`

## Browser deep-link smoke tests

After each install mode check:

1. Open browser and navigate to:
   - `resultv://import/<valid_payload>`
2. Verify app opens (or activates existing instance).
3. Verify import flow is triggered.
4. Copy the same raw `resultv://import/<valid_payload>` URL to clipboard and paste it into app import.
5. Verify import path succeeds without `unsupported subscription format` for valid payloads.

## Custom path with spaces

Repeat the CurrentUser and AllUsers scenarios with an install directory containing spaces, for example:

- `C:\\Users\\<name>\\AppData\\Local\\Result V Custom\\`
- `C:\\Program Files\\ResultV Custom\\`

Then re-run the same script and browser checks.
