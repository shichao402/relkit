# Regenerate Go bindings from proto/ (in-repo SSOT).
# Requires: buf on PATH (go install github.com/bufbuild/buf/cmd/buf@v1.50.0)
#
# Only Go is generated here. Hosts generate their own types from the schema
# the engine prints (`relkit-updater proto`), see ADR 0026.

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Proto = Join-Path $Root "proto"

Push-Location $Proto
try {
    # buf dep update may write WARN to stderr when there are no deps; ignore it.
    & buf dep update 2>$null | Out-Null
    & buf generate
    if ($LASTEXITCODE -ne 0) { throw "buf generate failed with exit $LASTEXITCODE" }
} finally {
    Pop-Location
}

Write-Host "generated Go -> $Root\api"
