#!/usr/bin/env bash
# Runs on EC2 via SSM with the image built from a reviewed main commit.
set -euo pipefail
test "$(id -u)" = 0
IMAGE="${1:?ECR image URI required}"
DIR=/opt/agent-control-plane
cd "$DIR"
umask 077

param() { aws ssm get-parameter --name "/agent-control-plane/$1" --with-decryption --query Parameter.Value --output text; }
DB_PASSWORD="$(param db_password)"
ORIGIN_SECRET="$(param origin_secret)"
BACKUP_BUCKET="$(param backup_bucket)"
cat > .env <<EOF
HUB_IMAGE=$IMAGE
DB_PASSWORD=$DB_PASSWORD
ORIGIN_SECRET=$ORIGIN_SECRET
BACKUP_BUCKET=$BACKUP_BUCKET
EOF
chmod 600 .env
mkdir -p pgdata tls
chmod 700 pgdata tls
chmod 700 refresh-cert.sh backup.sh restore-check.sh

./refresh-cert.sh
aws ecr get-login-password --region ap-northeast-2 \
  | docker login --username AWS --password-stdin "${IMAGE%%/*}" >/dev/null
docker compose --env-file .env -f compose.yaml pull --quiet
docker compose --env-file .env -f compose.yaml up -d

ready=0
for _ in $(seq 1 30); do
  if printf 'header = "X-Origin-Secret: %s"\n' "$ORIGIN_SECRET" \
    | curl --config - --silent --show-error --fail --output /dev/null \
      --resolve origin.agent.gwinam.com:443:127.0.0.1 \
      https://origin.agent.gwinam.com/ready; then
    ready=1
    break
  fi
  sleep 2
done
if [ "$ready" != 1 ]; then
  docker compose --env-file .env -f compose.yaml logs --tail 60 hub proxy >&2
  exit 1
fi

# First deployment must prove that the S3 archive can be restored before a
# client token is issued. Later deployments repeat the check on current data.
./backup.sh
./restore-check.sh

install -m 644 systemd/*.service systemd/*.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now agent-hub-backup.timer agent-hub-restore-check.timer agent-hub-cert-refresh.timer
echo 'agent hub origin ready'
