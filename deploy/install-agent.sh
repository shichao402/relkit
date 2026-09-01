#!/usr/bin/env bash
# Install relkit-agent on a Linux publish host (run as root).
set -euo pipefail

BINARY="${1:-./relkit-agent}"
CONFIG_DIR=/etc/relkit-agent
STATE_DIR=/var/lib/relkit-agent
PRODUCT_ROOT=/srv/relkit/dec

install -d -m 0755 "$CONFIG_DIR" "$STATE_DIR" "$PRODUCT_ROOT"
id -u relkit >/dev/null 2>&1 || useradd --system --home /srv/relkit --shell /usr/sbin/nologin relkit

install -m 0755 "$BINARY" /usr/local/bin/relkit-agent
if [[ ! -f "$CONFIG_DIR/token" ]]; then
  umask 077
  openssl rand -hex 32 >"$CONFIG_DIR/token"
  echo "wrote new token to $CONFIG_DIR/token"
fi
# Agent runs as user relkit; token/env must be group-readable.
chown root:relkit "$CONFIG_DIR/token"
chmod 0640 "$CONFIG_DIR/token"
if [[ ! -f "$CONFIG_DIR/relkit-agent.json" ]]; then
  install -m 0644 "$(dirname "$0")/relkit-agent.example.json" "$CONFIG_DIR/relkit-agent.json"
fi
chown root:relkit "$CONFIG_DIR/relkit-agent.json"
chmod 0644 "$CONFIG_DIR/relkit-agent.json"
install -m 0644 "$(dirname "$0")/relkit-agent.service" /etc/systemd/system/relkit-agent.service
chown -R relkit:relkit "$STATE_DIR" "$PRODUCT_ROOT"
systemctl daemon-reload
systemctl enable --now relkit-agent
systemctl --no-pager status relkit-agent || true
echo "next: set EnvironmentFile=/etc/relkit-agent/env (RELKIT_PRIVATE_KEY, COS_SECRET_ID, COS_SECRET_KEY)"
echo "new product profile: write $CONFIG_DIR/products/<id>.json (product + signing.keyId + backends), then:"
echo "  relkit-agent init -config $CONFIG_DIR/relkit-agent.json -product <id>"
echo "migrate old product-root relkit.json: relkit-agent init -config $CONFIG_DIR/relkit-agent.json -product <id> -migrate-profile"
echo "add more products with: relkit-agent init -config $CONFIG_DIR/relkit-agent.json -product <id>"
echo "list: relkit-agent init -config $CONFIG_DIR/relkit-agent.json -list-products"
echo "intranet: copy relkit-agent.intranet.example.json; put the local backend in the product profile (see relkit-intranet-product.example.json), or migrate-profile from a legacy relkit.json"
