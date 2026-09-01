#!/bin/sh
# Install relkit-serve as a systemd service and verify it works.
#
# Idempotent: re-running will not regenerate the token or overwrite an existing
# config. Rotating the token is a deliberate act (see AGENT-GUIDE.md section 5)
# because it breaks every publisher until they are updated.
#
#   sudo ./install.sh --binary ./dist/relkit-serve-linux-amd64
#
# POSIX sh rather than bash: some minimal images ship only /bin/sh, and there
# is nothing here that needs more.

set -eu

BINARY=""
DIR=/srv/releases
CONFIG_DIR=/etc/relkit-serve
ADDR=":8080"
USER_NAME=relkit
PREFIX=/usr/local/bin
ROTATE=0

usage() {
	cat <<EOF
Usage: sudo ./install.sh --binary PATH [options]

  --binary PATH     relkit-serve binary to install (required)
  --dir PATH        directory to serve (default $DIR)
  --config-dir PATH where config and token live (default $CONFIG_DIR)
  --addr ADDR       listen address (default $ADDR)
  --user NAME       service account (default $USER_NAME)
  --prefix PATH     where to install the binary (default $PREFIX)
  --rotate-token    generate a new token, invalidating the current one
  -h, --help        this text

Read-only by default is not an option here: this script always configures an
upload token, because the point of running this service instead of Nginx is
that relkit can publish to it directly.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--binary) BINARY="$2"; shift 2 ;;
	--dir) DIR="$2"; shift 2 ;;
	--config-dir) CONFIG_DIR="$2"; shift 2 ;;
	--addr) ADDR="$2"; shift 2 ;;
	--user) USER_NAME="$2"; shift 2 ;;
	--prefix) PREFIX="$2"; shift 2 ;;
	--rotate-token) ROTATE=1; shift ;;
	-h | --help) usage; exit 0 ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

fail() { echo "error: $*" >&2; exit 1; }
step() { echo; echo "==> $*"; }

[ -n "$BINARY" ] || { usage >&2; fail "--binary is required"; }
[ -f "$BINARY" ] || fail "$BINARY does not exist"
[ "$(id -u)" = 0 ] || fail "run with sudo"
command -v systemctl >/dev/null 2>&1 ||
	fail "systemctl not found; see AGENT-GUIDE.md section 3 for manual steps"

# Refuse a binary for the wrong architecture now rather than after the unit
# fails to start, where the error is an opaque "Exec format error".
case "$(uname -m)" in
x86_64) want=x86-64 ;;
aarch64 | arm64) want=aarch64 ;;
*) want="" ;;
esac
if [ -n "$want" ] && command -v file >/dev/null 2>&1; then
	file -L "$BINARY" | grep -q "$want" ||
		fail "$BINARY does not look like a $(uname -m) binary"
fi

step "Service account: $USER_NAME"
if id "$USER_NAME" >/dev/null 2>&1; then
	echo "already exists"
else
	useradd -r -s /usr/sbin/nologin "$USER_NAME" 2>/dev/null ||
		useradd -r -s /sbin/nologin "$USER_NAME"
	echo "created"
fi

step "Release directory: $DIR"
install -d -o "$USER_NAME" -g "$USER_NAME" -m 0755 "$DIR"
# A directory that already holds unrelated files is a data leak: this service
# has no listing, but every path in it can be fetched by name.
if [ -n "$(find "$DIR" -maxdepth 1 -mindepth 1 ! -name index ! -name manifest ! -name artifact ! -name fallback ! -name directory -print -quit 2>/dev/null)" ]; then
	echo "WARNING: $DIR holds files outside index/ manifest/ artifact/ fallback/ directory/."
	echo "         Everything here is downloadable by name. See AGENT-GUIDE.md 2.4."
fi

step "Binary: $PREFIX/relkit-serve"
install -d -m 0755 "$PREFIX"
install -m 0755 "$BINARY" "$PREFIX/relkit-serve"
"$PREFIX/relkit-serve" -version

step "Config: $CONFIG_DIR"
install -d -o "$USER_NAME" -g "$USER_NAME" -m 0750 "$CONFIG_DIR"
CONFIG="$CONFIG_DIR/relkit-serve.json"
TOKEN_FILE="$CONFIG_DIR/relkit-serve.token"

extract_token() {
	sed -n "s/.*RELKIT_UPLOAD_TOKEN='\\(.*\\)'.*/\\1/p"
}

extract_bootstrap() {
	sed -n "s/.*RELKIT_ADMIN_BOOTSTRAP='\\(.*\\)'.*/\\1/p"
}

chown_admin_state() {
	for f in "$DIR/.relkit-serve-admin.json" "$CONFIG_DIR/admin.json"; do
		if [ -f "$f" ]; then
			chown "$USER_NAME:$USER_NAME" "$f"
			chmod 600 "$f"
		fi
	done
}

NEW_TOKEN=""
NEW_BOOTSTRAP=""
if [ ! -f "$CONFIG" ] || [ ! -f "$TOKEN_FILE" ]; then
	INIT_OUT=$("$PREFIX/relkit-serve" init -dir "$DIR" -out "$CONFIG_DIR" -force)
	NEW_TOKEN=$(printf '%s\n' "$INIT_OUT" | extract_token)
	NEW_BOOTSTRAP=$(printf '%s\n' "$INIT_OUT" | extract_bootstrap)

	# init writes addr :8080; honour --addr without needing a JSON parser here.
	if [ "$ADDR" != ":8080" ]; then
		tmp="$CONFIG.tmp$$"
		sed "s|\"addr\": \".*\"|\"addr\": \"$ADDR\"|" "$CONFIG" >"$tmp"
		mv "$tmp" "$CONFIG"
	fi
elif [ "$ROTATE" = 1 ]; then
	echo "rotating the token; every publisher must be given the new value"
	# -token-only so that hand-edited settings survive. Regenerating the whole
	# config would quietly revert them, and a reverted cache prefix shows up
	# only as releases arriving minutes late. Panel accounts are left alone.
	NEW_TOKEN=$("$PREFIX/relkit-serve" init -out "$CONFIG_DIR" -token-only | extract_token)
else
	echo "keeping existing config and token"
	echo "(pass --rotate-token to replace the token)"
fi

if [ -n "$NEW_TOKEN" ]; then
	chown -R "$USER_NAME:$USER_NAME" "$CONFIG_DIR"
fi
chown_admin_state

step "systemd unit"
UNIT=/etc/systemd/system/relkit-serve.service
UNIT_SRC="$(dirname "$0")/relkit-serve.service"
[ -f "$UNIT_SRC" ] || fail "$UNIT_SRC not found; run this script from deploy/"

sed -e "s|^User=.*|User=$USER_NAME|" \
	-e "s|^Group=.*|Group=$USER_NAME|" \
	-e "s|^ReadWritePaths=.*|ReadWritePaths=$DIR|" \
	-e "s|^ExecStart=.*|ExecStart=$PREFIX/relkit-serve -config $CONFIG|" \
	"$UNIT_SRC" >"$UNIT"

# A port below 1024 cannot be bound by an unprivileged user without help. The
# alternatives are worse: running as root gives away everything to protect one
# bind, and moving to a high port puts ":8080" into every signed manifest, where
# it cannot be changed later without cutting a new release. One capability,
# granted only when the port actually needs it, is the smallest of the three.
UNIT_PORT=$(printf '%s' "$ADDR" | sed 's/.*://')
if [ -n "$UNIT_PORT" ] && [ "$UNIT_PORT" -lt 1024 ] 2>/dev/null; then
	# Inserted before [Install] rather than appended, because appending would
	# land these in [Install], where systemd ignores them -- and the service
	# would then fail to bind with no hint as to why.
	tmp="$UNIT.tmp$$"
	awk '/^\[Install\]/ && !done {
		print "# Required to bind a privileged port as an unprivileged user."
		print "# Added by install.sh because the configured port is below 1024."
		print "AmbientCapabilities=CAP_NET_BIND_SERVICE"
		print "CapabilityBoundingSet=CAP_NET_BIND_SERVICE"
		print ""
		done = 1
	} { print }' "$UNIT" >"$tmp"
	mv "$tmp" "$UNIT"
	echo "granted CAP_NET_BIND_SERVICE for port $UNIT_PORT"
fi

chmod 0644 "$UNIT"

systemctl daemon-reload
systemctl enable relkit-serve >/dev/null 2>&1 || true
systemctl restart relkit-serve

step "Self-check"
# The port may be a bare ":8080" or "1.2.3.4:8080"; probe over loopback either
# way, since the checks below are about the service, not about reachability.
PORT=$(printf '%s' "$ADDR" | sed 's/.*://')
BASE="http://127.0.0.1:$PORT"

ok=0
attempt=0
while [ "$attempt" -lt 10 ]; do
	if curl -fsS -o /dev/null "$BASE/-/health" 2>/dev/null; then ok=1; break; fi
	attempt=$((attempt + 1))
	# Whole seconds: busybox and dash builds of sleep do not all accept 0.5.
	sleep 1
done
[ "$ok" = 1 ] || {
	echo "service did not become healthy; recent log:" >&2
	journalctl -u relkit-serve -n 30 --no-pager >&2 || true
	exit 1
}
echo "1/5 health              ok"

code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT --data-binary 'x' "$BASE/.probe~" || true)
case "$code" in
401) echo "2/5 upload auth         ok (unauthenticated PUT rejected)" ;;
405) fail "upload endpoint is disabled; the service did not read the token" ;;
*) fail "unauthenticated PUT returned $code, expected 401" ;;
esac

if [ -n "$NEW_TOKEN" ]; then
	TOKEN="$NEW_TOKEN"
else
	TOKEN=$(cat "$TOKEN_FILE" 2>/dev/null || true)
fi
[ -n "$TOKEN" ] || fail "cannot read the token to finish the self-check"

curl -fsS -o /dev/null -X PUT -H "Authorization: Bearer $TOKEN" \
	--data-binary 'relkit-serve probe' "$BASE/.probe~" ||
	fail "authenticated PUT failed"
echo "3/5 upload              ok"

# Range is what makes concurrent download work. A proxy or a misconfiguration
# that drops it silently degrades every client to a single stream, so this is
# checked explicitly rather than assumed.
code=$(curl -s -o /dev/null -w '%{http_code}' -r 0-3 "$BASE/.probe~" || true)
[ "$code" = 206 ] || fail "ranged GET returned $code, expected 206"
echo "4/5 range requests      ok"

code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/" || true)
[ "$code" = 404 ] || fail "directory listing returned $code, expected 404"
echo "5/5 no directory listing ok"

rm -f "$DIR/.probe~"

step "Done"
echo "listening   $ADDR"
echo "serving     $DIR"
echo "config      $CONFIG"
echo "logs        journalctl -u relkit-serve -f"
echo "guide       relkit-serve agent-guide"

if [ -n "$NEW_TOKEN" ]; then
	cat <<EOF

The publisher needs this token. It is shown once; the server stores only its
sha256 and cannot recover it.

  export RELKIT_UPLOAD_TOKEN='$NEW_TOKEN'

And in the publishing project's relkit.json:

  "backends": {
    "dl": {
      "type": "http-put",
      "baseUrl": "http://$(hostname -f 2>/dev/null || hostname)$ADDR/",
      "tokenEnv": "RELKIT_UPLOAD_TOKEN"
    }
  }

baseUrl must be the address clients will actually use: it goes into the signed
manifest, so changing it later needs a new release.
EOF
fi

if [ -n "$NEW_BOOTSTRAP" ]; then
	cat <<EOF

Open http://127.0.0.1:${PORT:-8080}/-/admin and create the first operator with
this one-shot bootstrap. It is spent the moment that account exists; do not
store it.

  export RELKIT_ADMIN_BOOTSTRAP='$NEW_BOOTSTRAP'
EOF
fi
