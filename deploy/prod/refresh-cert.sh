#!/usr/bin/env bash
# Export ACM's current origin certificate. A daily systemd timer picks up
# renewals; the private key never leaves this instance or enters a log.
set -euo pipefail

DIR=/opt/agent-control-plane
TLS_DIR="$DIR/tls"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
umask 077
mkdir -p "$TLS_DIR"

param() { aws ssm get-parameter --name "/agent-control-plane/$1" --with-decryption --query Parameter.Value --output text; }
CERT_ARN="$(param origin_certificate_arn)"
printf %s "$(param acm_export_passphrase)" > "$WORK/passphrase"
aws acm export-certificate --region ap-northeast-2 \
  --certificate-arn "$CERT_ARN" --passphrase "fileb://$WORK/passphrase" \
  --output json > "$WORK/export.json"
python3 - "$WORK/export.json" "$WORK/fullchain.pem" "$WORK/encrypted-key.pem" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as source:
    exported = json.load(source)
with open(sys.argv[2], "w", encoding="utf-8") as certificate:
    certificate.write(exported["Certificate"])
    certificate.write(exported["CertificateChain"])
with open(sys.argv[3], "w", encoding="utf-8") as key:
    key.write(exported["PrivateKey"])
PY
openssl pkey -in "$WORK/encrypted-key.pem" -passin "file:$WORK/passphrase" -out "$WORK/key.pem" >/dev/null 2>&1
openssl x509 -in "$WORK/fullchain.pem" -noout -checkend 1209600 >/dev/null
openssl x509 -in "$WORK/fullchain.pem" -pubkey -noout > "$WORK/cert-pub.pem"
openssl pkey -in "$WORK/key.pem" -pubout > "$WORK/key-pub.pem"
cmp -s "$WORK/cert-pub.pem" "$WORK/key-pub.pem"

if [ -f "$TLS_DIR/fullchain.pem" ] && [ -f "$TLS_DIR/key.pem" ] \
  && cmp -s "$WORK/fullchain.pem" "$TLS_DIR/fullchain.pem" \
  && cmp -s "$WORK/key.pem" "$TLS_DIR/key.pem"; then
  echo 'origin certificate unchanged'
  exit 0
fi

install -m 600 "$WORK/fullchain.pem" "$TLS_DIR/fullchain.pem.new"
install -m 600 "$WORK/key.pem" "$TLS_DIR/key.pem.new"
mv -f "$TLS_DIR/fullchain.pem.new" "$TLS_DIR/fullchain.pem"
mv -f "$TLS_DIR/key.pem.new" "$TLS_DIR/key.pem"

if [ -f "$DIR/.env" ] && docker compose --env-file "$DIR/.env" -f "$DIR/compose.yaml" ps -q proxy | grep -q .; then
  docker compose --env-file "$DIR/.env" -f "$DIR/compose.yaml" restart proxy >/dev/null
fi
echo 'origin certificate refreshed'
