# Copyright (C) 2026 ResultV
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.

<#
.SYNOPSIS
  Запускает `go test` так, чтобы он не мог уронить живой туннель ResultV.

.DESCRIPTION
  Тестовый прогон роняет соединение не тем, что делает тестовый КОД (сеть и
  системные вызовы в пакете замоканы), а тем, чего стоит сама сборка: `go test`
  по умолчанию компонует пакеты с параллелизмом -p = GOMAXPROCS (здесь 12), а
  каждый link.exe для бинаря с sing-box держит порядка гигабайта. На машине с
  16 ГБ, где свободно меньше гигабайта, это выносит рабочие наборы всех
  процессов, включая ResultV: HIGH_PRIORITY_CLASS (см. internal/system/
  priority_windows.go) спасает от голода по CPU, но не от вытеснения страниц —
  движок встаёт на секунды, и сервер/ОС рвут все туннельные соединения разом.

  Скрипт стартует go через `cmd /c start`, который назначает класс приоритета и
  маску процессоров В МОМЕНТ СОЗДАНИЯ процесса. Оба атрибута наследуются всеми
  потомками (compile.exe, link.exe, сами тестовые бинари), поэтому весь
  инструментальный «куст» оказывается заперт на N логических ядрах и с
  приоритетом ниже движка. Параллелизм сборки при этом снижен до -p 1: это
  главный рычаг по памяти, один линкер вместо двенадцати.

.PARAMETER Cpus
  Сколько логических ядер отдать тулчейну (по умолчанию 4 из 12).

.PARAMETER P
  Значение -p для go test: сколько пакетов собирать/гонять параллельно.
  По умолчанию 1 — один линкер за раз.

.PARAMETER Priority
  Класс приоритета для куста go: LOW (по умолчанию) или BELOWNORMAL.

.PARAMETER GoVerbose
  Передать `-v` в go test. Отдельный ключ нужен потому, что голый -v
  PowerShell связывает со своим общим параметром -Verbose и до go он не доходит.
  Алиас -v работает: явный параметр перебивает общий.

.PARAMETER Monitor
  Писать CSV с телеметрией (свободная память, рабочий набор ResultV, жив ли
  туннель) на всё время прогона — чтобы потом было видно, что именно кончилось.

.PARAMETER MonitorDir
  Куда положить CSV (по умолчанию perf-diag/ в корне репозитория — туда же
  пишет collect-perf-diag.ps1, каталог уже в .gitignore).

.EXAMPLE
  .\scripts\run-tests-isolated.ps1 ./internal/proxy/ -run 'TestBuildRoute'

.EXAMPLE
  .\scripts\run-tests-isolated.ps1 -Cpus 2 -Monitor ./internal/config/
#>

[CmdletBinding()]
param(
    [int]$Cpus = 4,
    [int]$P = 1,
    [ValidateSet('LOW', 'BELOWNORMAL')]
    [string]$Priority = 'LOW',
    [Alias('v')]
    [switch]$GoVerbose,
    [switch]$Monitor,
    [string]$MonitorDir,
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)]
    [string[]]$GoTestArgs
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $GoTestArgs -or $GoTestArgs.Count -eq 0) {
    Write-Error 'Нечего запускать: передайте аргументы go test, например ./internal/config/ или ./internal/proxy/ -run TestFoo'
}

$logical = [int](Get-CimInstance Win32_ComputerSystem -Verbose:$false).NumberOfLogicalProcessors
if ($Cpus -lt 1) { $Cpus = 1 }
if ($Cpus -gt $logical) { $Cpus = $logical }

# Маска для `start /AFFINITY` — hex без префикса. Берём СТАРШИЕ логические
# процессоры: нулевое ядро обслуживает прерывания (в том числе сетевые), и
# отдавать его компилятору — ровно то, чего мы избегаем.
$mask = 0
for ($i = $logical - $Cpus; $i -lt $logical; $i++) { $mask = $mask -bor (1 -shl $i) }
$maskHex = '{0:X}' -f $mask

# GOMAXPROCS ограничивает и сам go, и тестовые бинари; без него рантайм видит
# все 12 ядер и плодит потоки, которых маска всё равно не пустит.
$env:GOMAXPROCS = "$Cpus"

$goArgs = @('test', "-p", "$P")
if ($GoVerbose) { $goArgs += '-v' }
$goArgs += $GoTestArgs
# cmd.exe разбирает командную строку до запуска go, поэтому всё, что для него
# метасимвол, обязано приехать в кавычках: без этого `-run 'TestA|TestB'`
# превращается в конвейер и вторая половина регекспа уходит искать программу
# с таким именем.
$quoted = $goArgs | ForEach-Object {
    if ($_ -match '[|&<>^()\s]') { '"' + $_ + '"' } else { $_ }
}
$goLine = 'go ' + ($quoted -join ' ')

Write-Host "[isolated] $goLine"
Write-Host "[isolated] CPU: $Cpus/$logical (affinity 0x$maskHex), priority $Priority, GOMAXPROCS=$Cpus"

$monitorJob = $null
$csvPath = $null
if ($Monitor) {
    if (-not $MonitorDir) { $MonitorDir = Join-Path $repoRoot 'perf-diag' }
    if (-not (Test-Path $MonitorDir)) { New-Item -ItemType Directory -Path $MonitorDir -Force | Out-Null }
    $csvPath = Join-Path $MonitorDir ("testrun-{0:yyyyMMdd-HHmmss}.csv" -f (Get-Date))
    Write-Host "[isolated] телеметрия: $csvPath"

    $monitorJob = Start-Job -ScriptBlock {
        param($out)
        'time,free_ram_mb,resultv_ws_mb,resultv_prio,go_tree_ws_mb,tunnel_ok,probe_ms' |
            Out-File -FilePath $out -Encoding utf8
        while ($true) {
            $os = Get-CimInstance Win32_OperatingSystem
            $freeMb = [int]($os.FreePhysicalMemory / 1KB)

            $rv = Get-CimInstance Win32_Process -Filter "Name='ResultV.exe'" |
                Select-Object -First 1
            $rvWs = if ($rv) { [int]($rv.WorkingSetSize / 1MB) } else { 0 }
            $rvPrio = if ($rv) { $rv.Priority } else { 0 }

            # Весь инструментальный куст: go, компилятор, линкер, тестовые бинари.
            $tree = Get-CimInstance Win32_Process -Filter "Name='go.exe' OR Name='link.exe' OR Name='compile.exe' OR Name='vet.exe'"
            $treeWs = 0
            foreach ($t in $tree) { $treeWs += [int]($t.WorkingSetSize / 1MB) }

            # Проба живости туннеля: один TCP-connect раз в тик (≈30/мин —
            # на порядки ниже любого порога, за который банят источник).
            $ok = 0
            $sw = [System.Diagnostics.Stopwatch]::StartNew()
            try {
                $client = New-Object System.Net.Sockets.TcpClient
                $iar = $client.BeginConnect('1.1.1.1', 443, $null, $null)
                if ($iar.AsyncWaitHandle.WaitOne(2000, $false) -and $client.Connected) { $ok = 1 }
                $client.Close()
            } catch { $ok = 0 }
            $sw.Stop()

            "$(Get-Date -Format o),$freeMb,$rvWs,$rvPrio,$treeWs,$ok,$([int]$sw.ElapsedMilliseconds)" |
                Out-File -FilePath $out -Append -Encoding utf8
            Start-Sleep -Seconds 2
        }
    } -ArgumentList $csvPath
}

$started = Get-Date
try {
    # `start` — единственный способ задать приоритет и аффинити В МОМЕНТ
    # создания процесса: Start-Process + присвоение свойств оставляет окно, в
    # котором go успевает развернуть сборку на всех ядрах. /B держит вывод в
    # текущей консоли, /WAIT возвращает управление только по завершении.
    $cmdLine = "start `"gotest`" /$Priority /AFFINITY $maskHex /B /WAIT $goLine"
    & cmd.exe /c $cmdLine
    $exit = $LASTEXITCODE
} finally {
    if ($monitorJob) {
        Stop-Job $monitorJob -ErrorAction SilentlyContinue
        Remove-Job $monitorJob -Force -ErrorAction SilentlyContinue
    }
}

$elapsed = [int]((Get-Date) - $started).TotalSeconds
Write-Host "[isolated] готово за ${elapsed}s, exit=$exit"

if ($csvPath -and (Test-Path $csvPath)) {
    $rows = Import-Csv $csvPath
    if ($rows) {
        $minFree = ($rows | Measure-Object free_ram_mb -Minimum).Minimum
        $minWs = ($rows | Measure-Object resultv_ws_mb -Minimum).Minimum
        $maxTree = ($rows | Measure-Object go_tree_ws_mb -Maximum).Maximum
        $drops = @($rows | Where-Object { $_.tunnel_ok -eq '0' }).Count
        Write-Host "[isolated] min free RAM: ${minFree} MB | min WS ResultV: ${minWs} MB | max WS тулчейна: ${maxTree} MB | неудачных проб: $drops из $($rows.Count)"
    }
}

exit $exit
