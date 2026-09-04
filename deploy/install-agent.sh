#!/usr/bin/env bash
# Install relkit-agent on a Linux publish host (run as root).
set -euo pipefail

BINARY="${1:-./relkit-agent}"
CONFIG_DIR=/etc/relkit-agent
STATE_DIR=/var/lib/relkit-agent
PRODUCT_ROOT=/srv/relkit

install -d -m 0755 "$CONFIG_DIR" "$STATE_DIR" "$PRODUCT_ROOT"
id -u relkit >/dev/null 2>&1 || useradd --system --home /srv/relkit --shell /usr/sbin/nologin relkit

install -m 0755 "$BINARY" /usr/local/bin/relkit-agent
# Do not create /etc/relkit-agent/token. Instance-wide Bearers are refused at
# startup. Each product gets tokens/<id>.token via `relkit-agent init -product`.
if [[ -f "$CONFIG_DIR/token" ]]; then
  echo "WARNING: $CONFIG_DIR/token is leftover instance-wide credential; delete after issuing per-product tokens"
fi
if [[ ! -f "$CONFIG_DIR/relkit-agent.json" ]]; then
  install -m 0644 "$(dirname "$0")/relkit-agent.example.json" "$CONFIG_DIR/relkit-agent.json"
fi
chown root:relkit "$CONFIG_DIR/relkit-agent.json"
chmod 0644 "$CONFIG_DIR/relkit-agent.json"
install -d -m 0750 -o root -g relkit "$CONFIG_DIR/tokens"
install -m 0644 "$(dirname "$0")/relkit-agent.service" /etc/systemd/system/relkit-agent.service
chown -R relkit:relkit "$STATE_DIR" "$PRODUCT_ROOT"
systemctl daemon-reload
systemctl enable --now relkit-agent
systemctl --no-pager status relkit-agent || true
echo "next: set EnvironmentFile=/etc/relkit-agent/env (COS_SECRET_ID, COS_SECRET_KEY only)"
echo "new product: write $CONFIG_DIR/products/<id>.json, then:"
echo "  relkit-agent init -config $CONFIG_DIR/relkit-agent.json -product <id>"
echo "  # prints export RELKIT_UPLOAD_TOKEN=... once; put that in the product CI, never in a sibling product"
echo "migrate old product-root relkit.json: relkit-agent init -config $CONFIG_DIR/relkit-agent.json -product <id> -migrate-profile"
echo "rotate: relkit-agent init -config $CONFIG_DIR/relkit-agent.json -product <id> -token-only"
echo "list: relkit-agent init -config $CONFIG_DIR/relkit-agent.json -list-products"
echo "onboard: relkit-agent onboard check -config $CONFIG_DIR/relkit-agent.json -product <id> -json"
echo "COS cert timer (separate process): install deploy/relkit-cos-cert-renew.service and .timer"
echo "intranet: copy relkit-agent.intranet.example.json; put the local backend in the product profile"
