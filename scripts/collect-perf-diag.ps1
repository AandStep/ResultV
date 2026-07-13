# Copyright (C) 2026 ResultV
#
# Collects a performance-diagnostics snapshot of a running ResultVPC session.
#
# Usage:
#   1. Start the app with pprof enabled:  $env:RESULTPROXY_PPROF = "1"; .\ResultVPC.exe
#   2. Right after connecting, take a baseline:   .\scripts\collect-perf-diag.ps1 -Label baseline
#   3. When the browser starts lagging (~2h later): .\scripts\collect-perf-diag.ps1 -Label lag
#   4. Compare:  go tool pprof -base <baseline>\heap.pb.gz <lag>\heap.pb.gz
#                go tool pprof -base <baseline>\cpu15s.pb.gz <lag>\cpu15s.pb.gz   (top / web)
#      Goroutine growth: diff the first line of goroutine.txt between snapshots.
#
# Each snapshot lands in .\perf-diag\<timestamp>-<label>\ next to the repo root.

param(
    [string]$Label = "snapshot",
    [string]$PprofAddr = "127.0.0.1:6060",
    [int]$CpuSeconds = 15
)

$ErrorActionPreference = "Continue"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$out = Join-Path (Join-Path $PSScriptRoot "..\perf-diag") "$stamp-$Label"
New-Item -ItemType Directory -Force $out | Out-Null
Write-Host "Snapshot -> $out"

# --- 1. Go runtime profiles (needs RESULTPROXY_PPROF=1 on the app) -----------
$base = "http://$PprofAddr/debug/pprof"
$targets = @(
    @{ Uri = "$base/goroutine?debug=1"; File = "goroutine.txt" },      # counts per stack
    @{ Uri = "$base/goroutine?debug=2"; File = "goroutine-full.txt" }, # full stacks
    @{ Uri = "$base/heap?debug=1";      File = "heap.txt" },           # incl. runtime.MemStats (NumGC, PauseNs)
    @{ Uri = "$base/heap";              File = "heap.pb.gz" },
    @{ Uri = "$base/allocs";            File = "allocs.pb.gz" }
)
foreach ($t in $targets) {
    try {
        Invoke-WebRequest -UseBasicParsing $t.Uri -OutFile (Join-Path $out $t.File) -TimeoutSec 30
    } catch {
        Write-Warning "pprof fetch failed ($($t.File)): $($_.Exception.Message). App started with RESULTPROXY_PPROF=1?"
    }
}
try {
    Write-Host "Capturing $CpuSeconds s CPU profile (keep using the browser meanwhile)..."
    Invoke-WebRequest -UseBasicParsing "$base/profile?seconds=$CpuSeconds" -OutFile (Join-Path $out "cpu${CpuSeconds}s.pb.gz") -TimeoutSec ($CpuSeconds + 30)
} catch {
    Write-Warning "CPU profile failed: $($_.Exception.Message)"
}

# --- 2. Process table: our app + WebView2 + browsers ------------------------
Get-Process | Where-Object { $_.ProcessName -match "result|msedgewebview2|chrome|firefox|yandex|browser|opera|vivaldi|brave" } |
    Select-Object Id, ProcessName, CPU, HandleCount,
        @{n="Threads";e={$_.Threads.Count}},
        @{n="WorkingSetMB";e={[math]::Round($_.WorkingSet64/1MB,1)}},
        @{n="PrivateMB";e={[math]::Round($_.PrivateMemorySize64/1MB,1)}},
        @{n="Priority";e={$_.PriorityClass}}, StartTime |
    Sort-Object PrivateMB -Descending |
    Tee-Object -Variable procTable | Format-Table -AutoSize | Out-String -Width 200 |
    Set-Content -Encoding utf8 (Join-Path $out "processes.txt")

# --- 3. Socket states of the app process -------------------------------------
$appProcs = Get-Process | Where-Object { $_.ProcessName -match "result" } | Select-Object -ExpandProperty Id
if ($appProcs) {
    $conns = Get-NetTCPConnection -OwningProcess $appProcs -ErrorAction SilentlyContinue
    $conns | Group-Object State | Select-Object Name, Count |
        Format-Table -AutoSize | Out-String | Set-Content -Encoding utf8 (Join-Path $out "tcp-states-app.txt")
    $conns | Select-Object LocalAddress, LocalPort, RemoteAddress, RemotePort, State |
        Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $out "tcp-connections-app.csv")
    Get-NetUDPEndpoint -OwningProcess $appProcs -ErrorAction SilentlyContinue |
        Measure-Object | Select-Object Count | Out-String |
        Set-Content -Encoding utf8 (Join-Path $out "udp-endpoints-app.txt")
}

# --- 4. System-wide loopback churn (TIME_WAIT has no owning PID) -------------
Get-NetTCPConnection -ErrorAction SilentlyContinue |
    Where-Object { $_.LocalAddress -eq "127.0.0.1" -or $_.RemoteAddress -eq "127.0.0.1" } |
    Group-Object State | Select-Object Name, Count |
    Format-Table -AutoSize | Out-String | Set-Content -Encoding utf8 (Join-Path $out "tcp-states-loopback.txt")

Write-Host "Done. Files:"
Get-ChildItem $out | Select-Object Name, @{n="KB";e={[math]::Round($_.Length/1KB,1)}} | Format-Table -AutoSize
