# Regenerate Go + Dart updater bindings from proto/ (in-repo SSOT).
# Requires: buf on PATH (go install github.com/bufbuild/buf/cmd/buf@v1.50.0)
#
# Go gets every schema (RUP + updater). Dart gets only updater/v1: the Dart
# client is a thin sidecar facade and never parses index/manifest, so shipping
# rup/v2 types there would only invite a second implementation.

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Proto = Join-Path $Root "proto"

Push-Location $Proto
try {
    # buf dep update may write WARN to stderr when there are no deps; ignore it.
    & buf dep update 2>$null | Out-Null
    & buf generate
    if ($LASTEXITCODE -ne 0) { throw "buf generate failed with exit $LASTEXITCODE" }
    & buf generate --template buf.gen.dart.yaml --path updater
    if ($LASTEXITCODE -ne 0) { throw "buf generate (dart) failed with exit $LASTEXITCODE" }
} finally {
    Pop-Location
}

Write-Host "generated Go   -> $Root\api"
Write-Host "generated Dart -> $Root\sdk\dart\lib\src\gen (updater/v1 only)"
& go run "$Root\cmd\relkit-facade-gen" "$Root"
