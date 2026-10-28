# Смоук-тест окна: фейковые логи и скрины в %TEMP%\tm-smoke, exe с флагами -game/-shots.
# Без параметров — с нуля: рейд на Развязке и 3 скрина.
# -Restart — только перезапуск exe над теми же папками (маршрут должен восстановиться).
param([switch]$Restart)

$ErrorActionPreference = 'Stop'
$root = Join-Path $env:TEMP 'tm-smoke'
$game = Join-Path $root 'build'
$logDir = Join-Path $game 'Logs\log_smoke'
$log = Join-Path $logDir 'smoke application_000.log'
$shots = Join-Path $root 'shots'
$exe = Join-Path $PSScriptRoot '..\build\bin\TarkovMap.exe'

if (-not (Test-Path $exe)) { throw "Нет $exe — сначала ./scripts/build.sh в WSL" }

if ($Restart) {
    Start-Process -FilePath $exe -ArgumentList @('-game', "`"$game`"", '-shots', "`"$shots`"")
    return
}

if (Test-Path $root) { Remove-Item -Recurse -Force $root }
New-Item -ItemType Directory -Force $logDir, $shots | Out-Null
$utf8 = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($log, '', $utf8)

Start-Process -FilePath $exe -ArgumentList @('-game', "`"$game`"", '-shots', "`"$shots`"")
Start-Sleep -Seconds 3

function Add-LogLine([string]$text) {
    $ts = Get-Date -Format 'yyyy-MM-dd HH:mm:ss.fff'
    [IO.File]::AppendAllText($log, "$ts|1.2.5.0.48146|Info|application|$text`r`n", $utf8)
}
Add-LogLine 'Session mode: Pve'
Add-LogLine 'scene preset path:maps/shopping_mall.bundle rcid:Shopping_Mall.ScenesPreset.asset'
Start-Sleep -Milliseconds 500
Add-LogLine 'GameStarted:69.1(0) real:89.98(0) diff:20.88'

# Реальные координаты с Развязки; второй скрин — у выхода «Безопасная комната».
$tails = @(
    '_-312.74, 23.88, -111.95_-0.04637, 0.85289, 0.07698, 0.51430_9.75 (0).png',
    '_-51.12, 22.88, 47.90_0.07448, 0.32045, -0.02528, 0.94399_13.27 (0).png',
    '_292.29, 23.44, -252.49_0.00568, -0.99400, 0.07629, 0.07817_13.67 (0).png'
)
foreach ($t in $tails) {
    Start-Sleep -Seconds 2
    $name = (Get-Date -Format 'yyyy-MM-dd[HH-mm]') + $t
    [IO.File]::WriteAllBytes((Join-Path $shots $name), [byte[]]@())
    Write-Host "скрин: $name"
}
