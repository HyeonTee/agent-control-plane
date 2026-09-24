#!/usr/bin/env bash
# Host-side backup avoids relying on container access to EC2 instance metadata.
set -euo pipefail
DIR=/opt/agent-control-plane
exec 9>"$DIR/backup.lock"
flock -n 9
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
BUCKET="$(aws ssm get-parameter --name /agent-control-plane/backup_bucket --query Parameter.Value --output text)"
FILE="hub-$(date -u +%Y%m%dT%H%M%SZ).dump"
docker compose --env-file "$DIR/.env" -f "$DIR/compose.yaml" exec -T db \
  pg_dump -U hub -Fc hub > "$WORK/$FILE"
test -s "$WORK/$FILE"
aws s3 cp "$WORK/$FILE" "s3://$BUCKET/backups/$FILE" --only-show-errors
BYTES="$(aws s3api head-object --bucket "$BUCKET" --key "backups/$FILE" --query ContentLength --output text)"
test "$BYTES" = "$(wc -c < "$WORK/$FILE" | tr -d ' ')"
echo "backup uploaded: $FILE ($BYTES bytes)"
