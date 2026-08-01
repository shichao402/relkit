#!/bin/sh
# Cross-compiles relkit-serve for every supported target.
#
# CGO_ENABLED=0 is what makes the result a single file that runs on any Linux:
# without it the binary links against the build host's libc and will fail on a
# musl distribution such as Alpine, and on any glibc older than the builder's.
# Everything this server uses has a pure-Go implementation, including DNS
# resolution, so there is nothing to give up.

set -eu

VERSION="${1:-0.1.0}"
OUT_DIR="${2:-dist}"

# This script lives in deploy/; the module root is one level up.
cd "$(dirname "$0")/.."
mkdir -p "$OUT_DIR"

commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
stamp="$VERSION+$commit"

# -s -w drop the symbol table and DWARF data, roughly a third of the size for a
# binary nobody debugs in place. -trimpath keeps build paths out of it, which
# makes the output reproducible.
ldflags="-s -w -X main.version=$stamp"

echo "building relkit-serve $stamp"
for target in linux/amd64 linux/arm64 windows/amd64 darwin/arm64; do
	os=${target%/*}
	arch=${target#*/}
	name="relkit-serve-$os-$arch"
	if [ "$os" = windows ]; then
		name="$name.exe"
	fi

	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
		go build -trimpath -ldflags "$ldflags" -o "$OUT_DIR/$name" ./cmd/relkit-serve

	size=$(du -h "$OUT_DIR/$name" | cut -f1)
	printf '  %-34s %s\n' "$name" "$size"
done

echo "output in $OUT_DIR"
