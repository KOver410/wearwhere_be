#!/usr/bin/env bash
# Dump Postgres and upload to gs://$BUCKET/backups/. Run via cron.
# Uses the VM service account (Application Default Credentials on GCE)
# through the google/cloud-sdk container — no gcloud install on host needed.
set -euo pipefail
set -a
source /opt/wearwhere/.env
set +a
cd /opt/wearwhere

TS=$(date +%Y%m%d-%H%M%S)
FILE=/tmp/wearwhere-$TS.sql.gz

docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip > "$FILE"

docker run --rm -v /tmp:/tmp google/cloud-sdk:slim \
  gcloud storage cp "$FILE" "gs://$BUCKET/backups/"

rm -f "$FILE"
echo "backup uploaded: gs://$BUCKET/backups/$(basename "$FILE")"
