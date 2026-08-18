#!/usr/bin/env bash
# RUP / relkit Agent 开箱探测：只打印下一步，不修改宿主仓库。
set -euo pipefail

HOST_ROOT="${1:-.}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RELKIT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== RUP Agent bootstrap (read-only probe) ==="
echo "relkit repo : $RELKIT_ROOT"
echo "host root   : $HOST_ROOT"
echo

have() { command -v "$1" >/dev/null 2>&1; }

echo "-- tools --"
if have relkit; then
  echo "OK  relkit: $(relkit --version 2>/dev/null || relkit version 2>/dev/null || echo present)"
else
  echo "MISSING  relkit (go install cnb.cool/shichao402/relkit/cmd/relkit@latest)"
fi
if have relkit-serve; then
  echo "OK  relkit-serve: $(relkit-serve -version 2>/dev/null || echo present)"
else
  echo "INFO  relkit-serve not in PATH (only needed for self-hosted http-put)"
fi
echo

echo "-- host signals --"
[[ -f "$HOST_ROOT/relkit.json" ]] && echo "FOUND  relkit.json" || echo "MISSING  relkit.json"
[[ -f "$HOST_ROOT/VERSION.json" ]] && echo "FOUND  VERSION.json" || echo "MISSING  VERSION.json"
[[ -f "$HOST_ROOT/go.mod" ]] && echo "FOUND  go.mod (Go host?)"
[[ -f "$HOST_ROOT/pubspec.yaml" ]] && echo "FOUND  pubspec.yaml (Dart/Flutter host?)"

DART_GUIDE=""
if [[ -f "$HOST_ROOT/packages/rup_client/AGENT-QUICKSTART.md" ]]; then
  DART_GUIDE="$HOST_ROOT/packages/rup_client/AGENT-QUICKSTART.md"
  echo "FOUND  Dart SDK guide: $DART_GUIDE"
elif [[ -f "$HOST_ROOT/AGENT-QUICKSTART.md" ]] && grep -q rup_client "$HOST_ROOT/pubspec.yaml" 2>/dev/null; then
  DART_GUIDE="$HOST_ROOT/AGENT-QUICKSTART.md"
fi
echo

echo "-- next documents (open in order) --"
echo "0. $SCRIPT_DIR/README.md"
NEED_TOOLCHAIN=0
if [[ ! -f "$HOST_ROOT/relkit.json" || ! -f "$HOST_ROOT/VERSION.json" ]]; then
  NEED_TOOLCHAIN=1
fi
if [[ "$NEED_TOOLCHAIN" -eq 1 ]]; then
  echo "1. $SCRIPT_DIR/toolchain-onboard.md   # path A: toolchain"
else
  echo "1. (toolchain present) use: relkit agent-guide for day-to-day publish"
fi
echo "2. $SCRIPT_DIR/sdk-cascade.md"
if [[ -n "$DART_GUIDE" ]]; then
  echo "3. $DART_GUIDE"
elif [[ -f "$HOST_ROOT/pubspec.yaml" ]]; then
  echo "3. (Dart host) add/copy rup_client then open its AGENT-QUICKSTART.md"
fi
if [[ -f "$HOST_ROOT/go.mod" ]]; then
  echo "3. $RELKIT_ROOT/sdk/AGENT-QUICKSTART.md"
fi
if [[ ! -f "$HOST_ROOT/pubspec.yaml" && ! -f "$HOST_ROOT/go.mod" ]]; then
  echo "3. $SCRIPT_DIR/sdk-cascade.md  # pick language manually"
fi
echo
echo "Done criteria: $SCRIPT_DIR/README.md §3"
echo "This script did not modify any files."
