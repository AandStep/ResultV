// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// currentPlatformKey returns the platforms map key for the current Windows binary.
func currentPlatformKey() string {
	if isInstalledBuild() {
		return "windows-amd64-installer"
	}
	return "windows-amd64-portable"
}

// isInstalledBuild returns true when the running exe lives under Program Files.
func isInstalledBuild() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	return isPathUnderProgramFiles(exePath)
}

func isPathUnderProgramFiles(path string) bool {
	path = strings.ToLower(filepath.Clean(path))
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		pf := os.Getenv(env)
		if pf != "" && strings.HasPrefix(path, strings.ToLower(filepath.Clean(pf))+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// installUpdate installs the downloaded artifact.
// For portable builds: stages detached copy+restart handover script.
// For NSIS installer builds: stages detached silent-installer handover script.
// Caller performs graceful process quit after this returns nil.
func installUpdate(newExePath string) error {
	if isInstalledBuild() {
		return installNSIS(newExePath)
	}
	return installPortable(newExePath)
}

// installPortable stages the update: launches a PowerShell script that waits
// for the process to fully exit before copying the new exe, then returns nil
// so the caller can do a graceful wailsRuntime.Quit instead of os.Exit.
// The script waits up to 15 s for the file lock to be released.
func installPortable(newExePath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get current exe: %w", err)
	}
	currentExe = filepath.Clean(currentExe)
	logPath := filepath.Join(os.TempDir(), "resultv-updater.log")
	script := buildPortableHandoverScript(newExePath, currentExe, logPath)
	if err := runPowerShellDetached(script); err != nil {
		return err
	}

	// Return nil — the caller (app.go) must call wailsRuntime.Quit to
	// properly release all WebView2 handles before the script copies the exe.
	return nil
}

// installNSIS stages silent NSIS install in a detached script. The script waits
// for this process to exit, runs installer /S, and attempts to relaunch app.
// Caller does graceful quit after this returns.
func installNSIS(installerPath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get current exe: %w", err)
	}
	currentExe = filepath.Clean(currentExe)
	installForAllUsers := isPathUnderProgramFiles(currentExe)
	logPath := filepath.Join(os.TempDir(), "resultv-updater.log")
	script := buildInstallerHandoverScript(os.Getpid(), installerPath, currentExe, logPath, installForAllUsers)
	return runPowerShellDetached(script)
}

func buildPortableHandoverScript(srcPath, dstPath, logPath string) string {
	src := powershellEscapeSingleQuoted(srcPath)
	dst := powershellEscapeSingleQuoted(dstPath)
	log := powershellEscapeSingleQuoted(logPath)
	return fmt.Sprintf(`$ErrorActionPreference = 'Continue'
$logPath = '%s'
function Write-UpdateLog([string]$msg) {
  $ts = Get-Date -Format 'yyyy-MM-dd HH:mm:ss.fff'
  Add-Content -LiteralPath $logPath -Value ("[$ts] " + $msg)
}
Write-UpdateLog 'portable handover started'
Start-Sleep -Seconds 8
$ok = $false
for($i=0;$i-lt 20;$i++){
  try {
    [IO.File]::Copy('%s','%s',$true)
    $ok = $true
    Write-UpdateLog ("portable update copy succeeded on attempt " + ($i + 1))
    break
  } catch {
    Write-UpdateLog ("portable update copy failed attempt " + ($i + 1) + ": " + $_.Exception.Message)
    Start-Sleep -Seconds 1
  }
}
if($ok){
  Remove-Item -LiteralPath '%s' -Force -ErrorAction SilentlyContinue
  Write-UpdateLog 'starting updated executable'
  Start-Process -FilePath '%s' | Out-Null
  Write-UpdateLog 'restart launched'
} else {
  Write-UpdateLog 'portable handover aborted: copy never succeeded'
}
`, log, src, dst, src, dst)
}

func buildInstallerHandoverScript(pid int, installerPath, currentExePath, logPath string, installForAllUsers bool) string {
	installer := powershellEscapeSingleQuoted(installerPath)
	currentExe := powershellEscapeSingleQuoted(currentExePath)
	log := powershellEscapeSingleQuoted(logPath)
	scopeArg := "/CURRENTUSER"
	registryOrder := "@('HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ResultVResultV','HKLM:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ResultVResultV')"
	scopeLog := "current-user"
	if installForAllUsers {
		scopeArg = "/ALLUSERS"
		registryOrder = "@('HKLM:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ResultVResultV','HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ResultVResultV')"
		scopeLog = "all-users"
	}
	return fmt.Sprintf(`$ErrorActionPreference = 'Continue'
$logPath = '%s'
function Write-UpdateLog([string]$msg) {
  $ts = Get-Date -Format 'yyyy-MM-dd HH:mm:ss.fff'
  Add-Content -LiteralPath $logPath -Value ("[$ts] " + $msg)
}
Write-UpdateLog 'installer handover started'
Write-UpdateLog 'installer mode: %s'
$proc = Get-Process -Id %d -ErrorAction SilentlyContinue
if($proc){
  try {
    Wait-Process -Id %d -Timeout 45 -ErrorAction Stop
    Write-UpdateLog 'wait-process completed'
  } catch {
    Write-UpdateLog ("wait-process warning: " + $_.Exception.Message)
  }
} else {
  Write-UpdateLog 'wait-process skipped: process already exited'
}
$installerProc = Start-Process -FilePath '%s' -ArgumentList '/S', '%s' -Wait -PassThru
Write-UpdateLog ("installer exit code: " + $installerProc.ExitCode)
if($installerProc.ExitCode -eq 0){
  $launchExe = '%s'
  foreach($rk in %s){
    try {
      $reg = Get-ItemProperty -Path $rk -ErrorAction Stop
      if($reg.InstallLocation){
        $cand = Join-Path $reg.InstallLocation 'ResultV.exe'
        if(Test-Path $cand){
          $launchExe = $cand
          Write-UpdateLog ("relaunch path from " + $rk + ": " + $launchExe)
          break
        }
      }
    } catch {}
  }
  Start-Process -FilePath $launchExe | Out-Null
  Write-UpdateLog ("relaunch requested: " + $launchExe)
} else {
  Write-UpdateLog 'installer reported non-zero exit code, relaunch skipped'
}
`, log, scopeLog, pid, pid, installer, scopeArg, currentExe, registryOrder)
}

func runPowerShellDetached(script string) error {
	psExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(psExe); err != nil {
		psExe = "powershell"
	}

	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("resultv-updater-%d.ps1", time.Now().UnixNano()))
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("write updater script: %w", err)
	}

	cmd := exec.Command(psExe,
		"-NonInteractive", "-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updater script: %w", err)
	}
	return nil
}

func powershellEscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
