#!/usr/bin/env bash
# Server-side deploy: pull, build, migrate, then (re)start the site.
#
#   /home/ubuntu/cupproof/scripts/deploy.sh
#
# The image is shared by the site ("serve", a long-running container) and the
# daily collector ("scrape", see collect.sh), so a deploy updates both.
#
# The site container joins two networks: wheyproof_default, where its
# database lives (the sister site's Postgres, database "cupproof"), and
# web-edge, where the Turkish stack's Caddy reaches the sites it serves. No
# port is published on the host: until Caddy routes a domain to it, the site
# is reachable only from inside those networks.
set -euo pipefail

DIR=/home/ubuntu/cupproof
SITE_URL=${SITE_URL:-https://www.cupproof.com}

cd "$DIR"
git pull --ff-only
docker build -q -t cupproof . >/dev/null
docker run --rm --network wheyproof_default --env-file .env --memory 256m cupproof migrate

docker rm -f cupproof-web >/dev/null 2>&1 || true
docker run -d --name cupproof-web --restart unless-stopped \
  --network wheyproof_default --env-file .env \
  -e ADDR=0.0.0.0:8080 -e SITE_URL="$SITE_URL" \
  --memory 128m cupproof serve >/dev/null
docker network connect --alias cupproof-web web-edge cupproof-web

# The site answers, through the network Caddy uses (Caddy's image has wget).
for i in 1 2 3 4 5; do
  if docker exec protein-avcisi-caddy-1 wget -q -O /dev/null http://cupproof-web:8080/en-us/; then
    echo "deployed $(git log --oneline -1)"
    exit 0
  fi
  sleep 2
done
echo "ERROR: cupproof-web does not answer on web-edge" >&2
docker logs --tail 20 cupproof-web >&2
exit 1
