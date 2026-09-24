#!/usr/bin/env bash
# Restores the latest S3 archive into a disposable database on the same cluster.
set -euo pipefail
DIR=/opt/agent-control-plane
exec 9>"$DIR/restore-check.lock"
flock -n 9
WORK="$(mktemp -d)"
CREATED=0
compose() { docker compose --env-file "$DIR/.env" -f "$DIR/compose.yaml" "$@"; }
cleanup() {
  if [ "$CREATED" = 1 ]; then compose exec -T db dropdb -U hub hub_restore_check || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

BUCKET="$(aws ssm get-parameter --name /agent-control-plane/backup_bucket --query Parameter.Value --output text)"
KEY="$(aws s3api list-objects-v2 --bucket "$BUCKET" --prefix backups/ \
  --query 'sort_by(Contents,&LastModified)[-1].Key' --output text)"
test -n "$KEY" && test "$KEY" != None
aws s3 cp "s3://$BUCKET/$KEY" "$WORK/restore.dump" --only-show-errors
test -s "$WORK/restore.dump"
compose exec -T db createdb -U hub hub_restore_check
CREATED=1
compose exec -T db pg_restore -U hub -d hub_restore_check --exit-on-error < "$WORK/restore.dump"
COUNTS="$(compose exec -T db psql -U hub -d hub_restore_check -Atc \
  'SELECT (SELECT count(*) FROM schema_migrations), (SELECT count(*) FROM tasks), (SELECT count(*) FROM checkpoints)')"
test -n "$COUNTS"
echo "restore verified: $KEY; migrations,tasks,checkpoints=$COUNTS"
