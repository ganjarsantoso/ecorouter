#!/usr/bin/env bash
# EcoRouter OCI / Ubuntu provisioning script
# Usage: curl -fsSL ... | sudo bash   OR   sudo ./scripts/install-oci.sh
set -euo pipefail

ECO_USER="${ECO_USER:-ecorouter}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/etc/ecorouter}"
DATA_DIR="${DATA_DIR:-/var/lib/ecorouter}"
DOMAIN="${DOMAIN:-}"
BINARY_SRC="${BINARY_SRC:-}"

log()  { printf '==> %s\n' "$*"; }
die()  { printf '✗ %s\n' "$*" >&2; exit 1; }

if [[ "$(id -u)" -ne 0 ]]; then
  die "run as root (sudo)"
fi

log "Installing EcoRouter"

# binary
if [[ -n "$BINARY_SRC" && -x "$BINARY_SRC" ]]; then
  install -m 755 "$BINARY_SRC" "$INSTALL_DIR/eco"
elif [[ -x ./bin/eco ]]; then
  install -m 755 ./bin/eco "$INSTALL_DIR/eco"
elif command -v eco >/dev/null 2>&1; then
  log "eco already on PATH: $(command -v eco)"
else
  die "no eco binary found; build with 'make build' and re-run, or set BINARY_SRC="
fi

# system user
if ! id "$ECO_USER" >/dev/null 2>&1; then
  useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" "$ECO_USER"
  log "created user $ECO_USER"
fi

mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chown "$ECO_USER:$ECO_USER" "$DATA_DIR"
chmod 700 "$DATA_DIR"

# config seed
if [[ ! -f "$CONFIG_DIR/config.toml" ]]; then
  cat > "$CONFIG_DIR/config.toml" <<EOF
[server]
port       = 8080
host       = "127.0.0.1"
domain     = "${DOMAIN}"
timeout_ms = 30000

[security]
require_tls       = true
max_body_bytes    = 10485760
global_rate       = "600/min"
auth_fail_lockout = "5/1m -> 15m"
global_daily_cap  = 0

[access]
allow = []
deny  = []

[defaults]
active_route  = ""
saver_default = ""

[health]
window          = 20
error_threshold = 0.5
min_requests    = 5
cooldown_ms     = 60000
EOF
  chown root:"$ECO_USER" "$CONFIG_DIR/config.toml"
  chmod 640 "$CONFIG_DIR/config.toml"
  log "wrote $CONFIG_DIR/config.toml"
fi

# systemd unit
UNIT_SRC="$(dirname "$0")/../deploy/ecorouter.service"
if [[ -f "$UNIT_SRC" ]]; then
  cp "$UNIT_SRC" /etc/systemd/system/ecorouter.service
else
  cat > /etc/systemd/system/ecorouter.service <<'EOF'
[Unit]
Description=EcoRouter LLM router
After=network-online.target
Wants=network-online.target

[Service]
User=ecorouter
Group=ecorouter
Environment=ECO_HOME=/var/lib/ecorouter
Environment=ECO_CONFIG=/etc/ecorouter/config.toml
ExecStart=/usr/local/bin/eco start --config /etc/ecorouter/config.toml
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/ecorouter

[Install]
WantedBy=multi-user.target
EOF
fi
systemctl daemon-reload
systemctl enable ecorouter

# Caddy (optional)
if command -v caddy >/dev/null 2>&1; then
  if [[ -n "$DOMAIN" ]]; then
    cat > /etc/caddy/Caddyfile <<EOF
${DOMAIN} {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080
	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
		X-Content-Type-Options "nosniff"
		-Server
	}
	request_body {
		max_size 10MB
	}
}
EOF
    systemctl enable --now caddy || true
    systemctl reload caddy || true
    log "Caddy configured for $DOMAIN"
  else
    log "Caddy present; set DOMAIN=eco.you.dev to write Caddyfile"
  fi
else
  log "Caddy not installed — for TLS: apt install caddy  (or see deploy/Caddyfile)"
fi

# firewall hints
if command -v ufw >/dev/null 2>&1; then
  log "UFW tips: ufw allow 22/tcp; ufw allow 443/tcp; ufw enable  (never open 8080)"
fi

cat <<EOF

✓ EcoRouter installed.

  Next (as operator):
    sudo -u $ECO_USER env ECO_HOME=$DATA_DIR ECO_CONFIG=$CONFIG_DIR/config.toml \\
      eco init --yes --domain ${DOMAIN:-eco.you.dev} \\
        --provider-type openai --provider-key "\$OPENAI_API_KEY" \\
        --route-mode fallback --route-models gpt-4o,gpt-4o-mini

    sudo systemctl start ecorouter
    sudo -u $ECO_USER env ECO_HOME=$DATA_DIR ECO_CONFIG=$CONFIG_DIR/config.toml eco status
    sudo -u $ECO_USER env ECO_HOME=$DATA_DIR ECO_CONFIG=$CONFIG_DIR/config.toml eco doctor

  Client (nothing to install):
    export OPENAI_BASE_URL="https://${DOMAIN:-your-domain}/v1"
    export OPENAI_API_KEY="eco_live_…"

EOF
