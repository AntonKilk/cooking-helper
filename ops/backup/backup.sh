#!/bin/sh
# Daily SQLite backup for Cooking Helper (CH-21 / tech-design §7 Operations).
#
# Runs SQLite's online ".backup" inside the running container (WAL-safe; does
# NOT require stopping the server) and writes a dated dump to /backups, which is
# bind-mounted to a host directory (see docker-compose.yml). Then prunes dumps
# older than 14 days.
#
# Invoked by launchd daily at 03:00 — see com.cookinghelper.backup.plist.
# launchd does not run a shell, so this script is wrapped in `/bin/sh -lc` by
# the plist; that is what lets $(date) expand and the prune step chain.
#
# Idempotent: re-running on the same day overwrites that day's dump.

set -eu

CONTAINER="${CONTAINER:-cooking-helper}"
DB_PATH="${DB_PATH:-/data/cooking.db}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

stamp() { date '+%Y-%m-%dT%H:%M:%S%z'; }
day="$(date +%F)"
target="${BACKUP_DIR}/${day}.db"

echo "[$(stamp)] backup start: container=${CONTAINER} db=${DB_PATH} -> ${target}"

# Online backup via the sqlite3 CLI inside the container. The CLI is added to
# the runtime image in the Dockerfile run stage (apk add sqlite). The container
# writes to its own /backups, which is the same host dir we prune below.
docker exec "${CONTAINER}" sqlite3 "${DB_PATH}" ".backup '${BACKUP_DIR}/${day}.db'"

echo "[$(stamp)] backup ok: ${target}"

# Retention: drop dumps older than RETENTION_DAYS. Run on the host so macOS
# `find` prunes the bind-mounted files directly.
find "${BACKUP_DIR}" -name '*.db' -mtime "+${RETENTION_DAYS}" -delete

echo "[$(stamp)] retention pruned: kept last ${RETENTION_DAYS} days in ${BACKUP_DIR}"
