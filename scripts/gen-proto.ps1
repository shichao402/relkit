# Regenerate Go + Dart bindings from proto/ (in-repo SSOT).
# Requires: buf on PATH (go install github.com/bufbuild/buf/cmd/buf@v1.50.0)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Proto = Join-Path $Root "proto"

Push-Location $Proto
try {
    & buf dep update 2>&1 | Out-Null
    & buf generate
    if ($LASTEXITCODE -ne 0) { throw "buf generate failed with exit $LASTEXITCODE" }
} finally {
    Pop-Location
}

Write-Host "generated Go  -> $Root\api"
Write-Host "generated Dart -> $Root\sdk\dart\lib\src\gen"
