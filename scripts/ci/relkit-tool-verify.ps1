# Fetch one relkit tool from a relkit agent server, verify it with the build
# machine's own ssh-keygen, then hand back its path (requirements 12.2, 12.3,
# 12.5).
#
# Windows twin of relkit-tool-verify.sh. The Windows agents run PowerShell and
# cannot be assumed to have a POSIX shell, but they do ship OpenSSH (Windows 10
# 1809+, Git for Windows), which is what requirement 12.3 relies on.
#
# This file plus the sh twin is the frozen body of the three commands a host CI
# yaml carries: download, verify, execute. Nothing here names a relkit version,
# a file name, a component digest, or a bundle address, so a relkit release
# never edits it.
#
# Required environment:
#   RELKIT_SERVER            absolute http(s) base URL of the agent server
#   RELKIT_ROOT_PUBLIC_KEY   "ssh-ed25519 <base64>", as /v1/tools/root.pub serves
#   RELKIT_TOOL              component: cli | updater | ops
#   RELKIT_PLATFORM          os-arch: windows-amd64 | darwin-arm64 | linux-amd64
# Optional:
#   RELKIT_TOOL_DIR          download directory (default: relkit-tools)
#
# On success the verified tool's path is the only stdout line.
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

foreach ($name in 'RELKIT_SERVER', 'RELKIT_ROOT_PUBLIC_KEY', 'RELKIT_TOOL', 'RELKIT_PLATFORM') {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "$name is required"
    }
}

$server = $env:RELKIT_SERVER
$publicKey = $env:RELKIT_ROOT_PUBLIC_KEY.Trim()
$tool = $env:RELKIT_TOOL
$platform = $env:RELKIT_PLATFORM
$toolDir = if ([string]::IsNullOrWhiteSpace($env:RELKIT_TOOL_DIR)) { 'relkit-tools' } else { $env:RELKIT_TOOL_DIR }

$namespace = 'relkit-tool'
$principal = 'relkit-root'

# `current` resolves on the server against its signed Catalog, and the
# component/platform pair resolves against the signed tool manifest, so the
# template never pins a version or a file name.
$base = "$($server.TrimEnd('/'))/v1/tools/current/$tool/$platform"

$os = $platform.Split('-')[0]
$asset = "$tool-$($platform)"
if ($os -eq 'windows') { $asset += '.exe' }

New-Item -ItemType Directory -Force -Path $toolDir | Out-Null

# The allowlist states "this is the key I trust" and is derived locally from the
# pinned project key; fetching it from the same server that serves the tool
# would make the signature check meaningless.
$allowlist = Join-Path $toolDir 'relkit_allowed_signers'
# allowed_signers format: principal first, then the key. Writing the keyId last
# as a comment makes ssh-keygen read the key type as a principal and reject every
# signature. A "*" principal matches any -I identity, so a root rename does not
# invalidate the line.
Set-Content -Path $allowlist -Value "* $publicKey" -Encoding ascii

$toolPath = Join-Path $toolDir $asset
$signaturePath = "$toolPath.sig"

# --- 1. download the tool and its signature ---------------------------------
# The manifest's declared digest and file name come from a HEAD first: Windows
# PowerShell 5.1 does not expose response headers when -OutFile is also given.
$head = Invoke-WebRequest -Uri $base -Method Head -UseBasicParsing
$declaredDigest = [string]$head.Headers['X-Relkit-Tool-SHA256']
$declaredName = [string]$head.Headers['X-Relkit-Tool-Name']
if (-not [string]::IsNullOrWhiteSpace($declaredName)) {
    $asset = $declaredName
    $toolPath = Join-Path $toolDir $asset
    $signaturePath = "$toolPath.sig"
}
Invoke-WebRequest -Uri $base -OutFile $toolPath -UseBasicParsing
Invoke-WebRequest -Uri "$base/signature" -OutFile $signaturePath -UseBasicParsing

# The signature protects whatever bytes are read, so the declared digest is
# checked as well: a truncated download that still verifies would otherwise be
# installed as a corrupt tool.
if (-not [string]::IsNullOrWhiteSpace($declaredDigest)) {
    $actual = 'sha256:' + (Get-FileHash -Path $toolPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $declaredDigest.ToLowerInvariant()) {
        Remove-Item $toolPath -ErrorAction SilentlyContinue
        throw "downloaded $asset digest $actual, manifest declares $declaredDigest"
    }
}

# --- 2. verify with the build machine's ssh-keygen ---------------------------
# Failure exits non-zero. There is no fallback to GitHub, a mirror, or an
# unverified checksum (requirement 12.2). Checking for the binary first keeps a
# missing ssh-keygen from being reported as a bad signature.
$sshKeygen = Get-Command ssh-keygen -ErrorAction SilentlyContinue
if (-not $sshKeygen) {
    throw 'ssh-keygen is not installed; requirement 12.3 requires the OpenSSH that ships with the build machine'
}

$stdoutPath = Join-Path $toolDir '.relkit-verify.out'
$stderrPath = Join-Path $toolDir '.relkit-verify.err'
$verify = Start-Process -FilePath $sshKeygen.Source `
    -ArgumentList @('-Y', 'verify', '-f', $allowlist, '-I', $principal, '-n', $namespace, '-s', $signaturePath) `
    -RedirectStandardInput (Resolve-Path $toolPath).Path `
    -RedirectStandardOutput $stdoutPath `
    -RedirectStandardError $stderrPath `
    -NoNewWindow -Wait -PassThru

if ($verify.ExitCode -ne 0) {
    $detail = (Get-Content $stdoutPath -Raw -ErrorAction SilentlyContinue) +
              (Get-Content $stderrPath -Raw -ErrorAction SilentlyContinue)
    throw "ssh-keygen rejected $asset (exit $($verify.ExitCode)): $detail"
}
Remove-Item $stdoutPath, $stderrPath -ErrorAction SilentlyContinue

# --- 3. hand the verified tool back -----------------------------------------
# The caller runs it. The tool re-checks the Catalog and Runtime Profile with
# the same root key before doing any work.
Write-Output $toolPath
