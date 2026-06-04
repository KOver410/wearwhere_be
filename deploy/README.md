# WearWhere Deployment Runbook

Single GCP Compute Engine VM running `docker-compose.prod.yml`
(postgres + redis + api + caddy, plus a one-shot `migrate` service).
Infra in `deploy/terraform/`.

## First deploy
See `docs/superpowers/plans/2026-06-04-gcp-deployment.md` (Tasks 4–8).

## Update
```
ssh -i ~/.ssh/wearwhere <user>@<ip>
cd /opt/wearwhere && ./deploy/deploy.sh
```

## Restore a backup
```
cd /opt/wearwhere
# Load $BUCKET / $POSTGRES_USER / $POSTGRES_DB from the env file first
set -a; source /opt/wearwhere/.env; set +a
docker run --rm -v /tmp:/tmp google/cloud-sdk:slim \
  gcloud storage cp gs://$BUCKET/backups/<file>.sql.gz /tmp/restore.sql.gz
gunzip -c /tmp/restore.sql.gz | \
  docker compose -f docker-compose.prod.yml exec -T postgres psql -U $POSTGRES_USER -d $POSTGRES_DB
```

## Production cutover checklist (demo → prod)

Edit `/opt/wearwhere/.env` on the VM, then `./deploy/deploy.sh`:

- [ ] `APP_ENV=production`
- [ ] `JWT_SECRET` regenerated (`openssl rand -base64 64`)
- [ ] `CORS_ALLOWED_ORIGINS` set to real web origin(s) — NOT blank
- [ ] Real domain: update DNS A record → `api_ip`; set `SITE_ADDRESS`,
      `STORAGE_BASE_URL`, `PAYOS_RETURN_URL`, `PAYOS_CANCEL_URL`, `PAYOS_BASE_URL`
- [ ] PayOS: register individual account (CCCD + personal bank account in your name),
      set `PAYOS_MODE=production` + `PAYOS_CLIENT_ID` / `PAYOS_API_KEY` / `PAYOS_CHECKSUM_KEY`
- [ ] Goship: `SHIPPING_PROVIDER=goship`, `GOSHIP_MODE=production` (or sandbox) + `GOSHIP_TOKEN`
- [ ] OAuth: fill `GOOGLE_CLIENT_IDS` / `APPLE_CLIENT_IDS` (web + iOS + Android)
- [ ] SMTP: real Gmail app password
- [ ] Storage → GCS: `STORAGE_DRIVER=gcs`, `STORAGE_GCS_BUCKET=<bucket>`
      (VM service account already has objectAdmin)
- [ ] Backups verified (`./deploy/backup.sh` + cron installed)
- [ ] SSH firewall locked to your IP (already via `allowed_ssh_cidr`)

## Known follow-ups (out of scope of this plan)
- Mobile (Flutter) PayOS return needs a deep link, not the web return URL.
- React static site hosting (Cloudflare Pages / Vercel) is a separate task.
- Push notifications (FCM) not implemented.
- GitHub Actions CI/CD automation.
