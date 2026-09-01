# RUP / relkit Agent 开箱探测：只打印下一步，不修改宿主仓库。
param(
    [string]$HostRoot = "."
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RelkitRoot = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path
$HostRoot = (Resolve-Path $HostRoot).Path

Write-Output "=== RUP Agent bootstrap (read-only probe) ==="
Write-Output "relkit repo : $RelkitRoot"
Write-Output "host root   : $HostRoot"
Write-Output ""

function Test-Cmd($Name) {
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

Write-Output "-- tools --"
if (Test-Cmd "relkit") {
    try { Write-Output ("OK  relkit: " + (relkit --version 2>$null)) }
    catch { Write-Output "OK  relkit: present" }
} else {
    Write-Output "MISSING  relkit (go install cnb.cool/shichao402/relkit/cmd/relkit@latest)"
}
if (Test-Cmd "relkit-serve") {
    try { Write-Output ("OK  relkit-serve: " + (relkit-serve -version 2>$null)) }
    catch { Write-Output "OK  relkit-serve: present" }
} else {
    Write-Output "INFO  relkit-serve not in PATH (only needed for self-hosted http-put)"
}
Write-Output ""

Write-Output "-- host signals --"
if (Test-Path (Join-Path $HostRoot "relkit.json")) { Write-Output "FOUND  relkit.json" } else { Write-Output "MISSING  relkit.json" }
if (Test-Path (Join-Path $HostRoot "VERSION.json")) { Write-Output "FOUND  VERSION.json" } else { Write-Output "MISSING  VERSION.json" }
if (Test-Path (Join-Path $HostRoot "go.mod")) { Write-Output "FOUND  go.mod (Go host?)" }
if (Test-Path (Join-Path $HostRoot "pubspec.yaml")) { Write-Output "FOUND  pubspec.yaml (Dart/Flutter host?)" }
$PkgJson = Join-Path $HostRoot "package.json"
$IsElectron = $false
if (Test-Path $PkgJson) {
    if (Select-String -Path $PkgJson -Pattern '"electron"' -Quiet) { $IsElectron = $true }
    if ($IsElectron) {
        Write-Output "FOUND  package.json (Electron host? -> apply is host-side)"
    } else {
        Write-Output "FOUND  package.json (Node/TypeScript host?)"
    }
}

$DartGuide = $null
$Vendored = Join-Path $HostRoot "packages\rup_client\AGENT-QUICKSTART.md"
if (Test-Path $Vendored) {
    $DartGuide = $Vendored
    Write-Output "FOUND  Dart SDK guide: $DartGuide"
}
Write-Output ""

Write-Output "-- next documents (open in order) --"
Write-Output "0. $(Join-Path $ScriptDir 'README.md')"
$NeedToolchain = -not (Test-Path (Join-Path $HostRoot "relkit.json")) -or -not (Test-Path (Join-Path $HostRoot "VERSION.json"))
if ($NeedToolchain) {
    Write-Output "1. $(Join-Path $ScriptDir 'toolchain-onboard.md')   # path A: toolchain"
} else {
    Write-Output "1. (toolchain present) use: relkit agent-guide for day-to-day publish"
}
Write-Output "2. $(Join-Path $ScriptDir 'sdk-cascade.md')"
if ($DartGuide) {
    Write-Output "3. $DartGuide"
} elseif (Test-Path (Join-Path $HostRoot "pubspec.yaml")) {
    Write-Output "3. (Dart host) add/copy rup_client then open its AGENT-QUICKSTART.md"
}
if (Test-Path (Join-Path $HostRoot "go.mod")) {
    Write-Output "3. $(Join-Path $RelkitRoot 'sdk\AGENT-QUICKSTART.md')"
}
if (Test-Path $PkgJson) {
    Write-Output "3. $(Join-Path $RelkitRoot 'sdk\node\AGENT-QUICKSTART.md')"
    if ($IsElectron) {
        Write-Output "   note: rup-client has no apply; host owns install/restart"
    }
}
if (-not (Test-Path (Join-Path $HostRoot "pubspec.yaml")) -and -not (Test-Path (Join-Path $HostRoot "go.mod")) -and -not (Test-Path $PkgJson)) {
    Write-Output "3. $(Join-Path $ScriptDir 'sdk-cascade.md')  # pick language manually"
}
Write-Output ""
Write-Output "Done criteria: $(Join-Path $ScriptDir 'README.md') §3"
Write-Output "This script did not modify any files."
