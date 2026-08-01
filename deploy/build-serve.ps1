# Cross-compiles relkit-serve for Linux from any machine with a Go toolchain.
#
# CGO_ENABLED=0 is what makes the result a single file that runs on any Linux:
# without it the binary links against the build host's libc and will fail on a
# musl distribution such as Alpine, and on any glibc older than the builder's.
# Everything this server uses has a pure-Go implementation, including DNS
# resolution, so there is nothing to give up.

param(
    [string]$Version = "0.1.0",
    [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
# This script lives in deploy/; the module root is one level up.
Set-Location (Join-Path $PSScriptRoot "..")

if (-not (Test-Path $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir | Out-Null
}

$commit = "unknown"
try {
    $commit = (& git rev-parse --short HEAD 2>$null)
    if ($LASTEXITCODE -ne 0) { $commit = "unknown" }
} catch { $commit = "unknown" }

$stamp = "$Version+$commit"

# -s -w drop the symbol table and DWARF data, roughly a third of the size for a
# binary nobody debugs in place. -trimpath keeps build paths out of it, which
# makes the output reproducible.
$ldflags = "-s -w -X main.version=$stamp"

$targets = @(
    @{ os = "linux";   arch = "amd64" },
    @{ os = "linux";   arch = "arm64" },
    @{ os = "windows"; arch = "amd64" },
    @{ os = "darwin";  arch = "arm64" }
)

Write-Host "building relkit-serve $stamp"
foreach ($t in $targets) {
    $name = "relkit-serve-$($t.os)-$($t.arch)"
    if ($t.os -eq "windows") { $name += ".exe" }
    $out = Join-Path $OutDir $name

    $env:GOOS = $t.os
    $env:GOARCH = $t.arch
    $env:CGO_ENABLED = "0"

    & go build -trimpath -ldflags $ldflags -o $out ./cmd/relkit-serve
    if ($LASTEXITCODE -ne 0) {
        throw "build failed for $($t.os)/$($t.arch)"
    }

    $size = [math]::Round((Get-Item $out).Length / 1MB, 1)
    Write-Host ("  {0,-34} {1} MB" -f $name, $size)
}

Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
Write-Host "output in $OutDir"
