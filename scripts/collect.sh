#!/usr/bin/env bash
# Daily catalog read on the server, from the ubuntu user's crontab:
#
#   0 9 * * * /home/ubuntu/cupproof/scripts/collect.sh
#
# The collector is the same image as the site ("cupproof scrape"), run once
# and removed. It joins the sister site's compose network because its
# database lives in that Postgres (database "cupproof", own role; the
# connection string is in .env next to this checkout, mode 600).
#
# 09:00 UTC is in the daytime window where the shared IP met no Shopify 429
# (13 Oct 2026, seven WheyProof cycles), away from the sister sites' night
# rounds. The run itself stops after three stores in a row refuse it.
set -uo pipefail

DIR=/home/ubuntu/cupproof
LOGS=/home/ubuntu/cupproof-logs
RETENTION_DAYS=30

mkdir -p "$LOGS"
LOG="$LOGS/collect-$(date -u +%Y%m%d).log"

# One run at a time: a slow run must not overlap the next one.
exec 9>"$LOGS/.lock"
if ! flock -n 9; then
  echo "$(date -u +%FT%TZ) a run is still going; this one skipped" >> "$LOG"
  exit 0
fi

{
  echo "=== $(date -u +%FT%TZ) start"
  docker run --rm --name cupproof-collect --network wheyproof_default \
    --env-file "$DIR/.env" --memory 256m cupproof scrape
  rc=$?
  echo "=== $(date -u +%FT%TZ) exit $rc"
} >> "$LOG" 2>&1

find "$LOGS" -name 'collect-*.log' -mtime +$RETENTION_DAYS -delete
exit "${rc:-1}"
