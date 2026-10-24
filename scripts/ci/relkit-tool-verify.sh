#!/bin/sh
# Fetch one relkit tool from a relkit agent server, verify it with the build
# machine's own ssh-keygen, then hand back its path (requirements 12.2, 12.3,
# 12.5).
#
# POSIX twin of relkit-tool-verify.ps1, for the linux and macOS build machines.
# This file is the frozen body of the three commands a host CI yaml carries:
# download, verify, execute. Nothing here names a relkit version, a file name, a
# component digest, or a bundle address, so a relkit release never edits it.
#
# Required environment:
#   RELKIT_SERVER            absolute http(s) base URL of the agent server
#   RELKIT_ROOT_PUBLIC_KEY   "ssh-ed25519 <base64>", as /v1/tools/root.pub serves
#   RELKIT_TOOL              component: cli | updater | ops
#   RELKIT_PLATFORM          os-arch: darwin-arm64 | linux-amd64
# Optional:
#   RELKIT_TOOL_DIR          download directory (default: ./relkit-tools)
#
# On success the verified tool's path is the only stdout line, so a host can
# write: tool="$(sh relkit-tool-verify.sh)"
set -eu

: "${RELKIT_SERVER:?RELKIT_SERVER is required}"
: "${RELKIT_ROOT_PUBLIC_KEY:?RELKIT_ROOT_PUBLIC_KEY is required}"
: "${RELKIT_TOOL:?RELKIT_TOOL is required}"
: "${RELKIT_PLATFORM:?RELKIT_PLATFORM is required}"

TOOL_DIR="${RELKIT_TOOL_DIR:-relkit-tools}"
NAMESPACE="relkit-tool"
PRINCIPAL="relkit-root"

# `current` resolves on the server against its signed Catalog, and the
# component/platform pair resolves against the signed tool manifest, so the
# template never pins a version or a file name.
BASE="${RELKIT_SERVER%/}/v1/tools/current/${RELKIT_TOOL}/${RELKIT_PLATFORM}"

OS="${RELKIT_PLATFORM%%-*}"
ASSET="${RELKIT_TOOL}-${RELKIT_PLATFORM}"
if [ "${OS}" = "windows" ]; then
    ASSET="${ASSET}.exe"
fi

mkdir -p "${TOOL_DIR}"

# The allowlist states "this is the key I trust" and is derived locally from the
# pinned project key; downloading it from the same server that serves the tool
# would make the signature check meaningless.
#
# Format is allowed_signers, not authorized_keys: principal first, then the key.
# Writing the keyId last as a comment makes ssh-keygen read the key type as a
# principal and reject every signature ("Could not verify signature."). A "*"
# principal matches any -I identity, so a root rename does not invalidate the
# line.
ALLOWLIST="${TOOL_DIR}/relkit_allowed_signers"
printf '* %s\n' "${RELKIT_ROOT_PUBLIC_KEY}" > "${ALLOWLIST}"

TOOL_PATH="${TOOL_DIR}/${ASSET}"
SIGNATURE_PATH="${TOOL_PATH}.sig"

# --- 1. download the tool and its signature ------------------------------
curl -fsSL -D "${TOOL_DIR}/.relkit-headers" "${BASE}" -o "${TOOL_PATH}"
curl -fsSL "${BASE}/signature" -o "${SIGNATURE_PATH}"

# The declared digest is checked as well as the signature: a truncated download
# that still verifies would otherwise be installed as a corrupt tool.
DECLARED="$(sed -n 's/^[Xx]-[Rr]elkit-[Tt]ool-[Ss][Hh][Aa]256:[[:space:]]*//p' "${TOOL_DIR}/.relkit-headers" | tr -d '\r')"
if [ -n "${DECLARED}" ]; then
    ACTUAL="sha256:$(sha256sum "${TOOL_PATH}" | cut -d' ' -f1)"
    if [ "${ACTUAL}" != "${DECLARED}" ]; then
        rm -f "${TOOL_PATH}"
        echo "downloaded ${ASSET} digest ${ACTUAL}, manifest declares ${DECLARED}" >&2
        exit 1
    fi
fi
rm -f "${TOOL_DIR}/.relkit-headers"

# --- 2. verify with the build machine's ssh-keygen -----------------------
# Failure exits non-zero. There is no fallback to GitHub, a mirror, or an
# unverified checksum (requirement 12.2).
ssh-keygen -Y verify \
    -f "${ALLOWLIST}" \
    -I "${PRINCIPAL}" \
    -n "${NAMESPACE}" \
    -s "${SIGNATURE_PATH}" \
    < "${TOOL_PATH}" > /dev/null

chmod +x "${TOOL_PATH}"

# --- 3. hand the verified tool back --------------------------------------
# The caller runs it. The tool re-checks the Catalog and Runtime Profile with
# the same root key before doing any work.
printf '%s\n' "${TOOL_PATH}"
