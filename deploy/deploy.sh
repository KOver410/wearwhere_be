#!/usr/bin/env bash
# Pull latest code and (re)build the stack on the VM.
set -euo pipefail
cd /opt/wearwhere
git pull --ff-only
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
echo "deployed commit: $(git rev-parse --short HEAD)"
