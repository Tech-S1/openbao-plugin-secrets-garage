#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

BAO_ADDR="${BAO_ADDR:-https://openbao.zachtech.dev}"
export BAO_ADDR
printf 'OpenBao token: '
read -rs BAO_TOKEN
printf '\n'
export BAO_TOKEN

PLUGIN=openbao-plugin-secrets-garage
OPENBAO_SSH_HOST="${OPENBAO_SSH_HOST:-192.168.68.63}"
OPENBAO_SSH_USER="${OPENBAO_SSH_USER:-zach}"
REMOTE_PLUGIN_DIR=/var/lib/openbao/plugins

make build-linux

arch=$(ssh "$OPENBAO_SSH_USER@$OPENBAO_SSH_HOST" 'uname -m')
case "$arch" in
  x86_64) goarch=amd64 ;;
  aarch64|arm64) goarch=arm64 ;;
  *) echo "unsupported server arch: $arch" >&2; exit 1 ;;
esac

bin="$ROOT/bin/$PLUGIN-linux-$goarch"
scp "$bin" "$OPENBAO_SSH_USER@$OPENBAO_SSH_HOST:/tmp/$PLUGIN"
ssh -t "$OPENBAO_SSH_USER@$OPENBAO_SSH_HOST" "sudo install -o openbao -g openbao -m 0750 /tmp/$PLUGIN $REMOTE_PLUGIN_DIR/$PLUGIN && rm /tmp/$PLUGIN"

SUM=$(sha256sum "$bin" | awk '{print $1}')
bao plugin register -sha256="$SUM" secret "$PLUGIN"
bao secrets enable -path=garage "$PLUGIN" 2>/dev/null || true
ssh -t "$OPENBAO_SSH_USER@$OPENBAO_SSH_HOST" "sudo systemctl restart openbao"
sleep 2

GARAGE_ADDRESS="${GARAGE_ADDRESS:-http://127.0.0.1:3903}"
printf 'Garage admin token: '
read -rs GARAGE_ADMIN_TOKEN
printf '\n'

bao write garage/config address="$GARAGE_ADDRESS" token="$GARAGE_ADMIN_TOKEN"
bao write garage/roles/default bucket="${GARAGE_BUCKET:-homelab-openbao-terraform}" ttl=5m max_ttl=15m read=true write=true
