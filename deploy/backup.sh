#!/usr/bin/env bash
#
# Nightly SQLite backup with retention for vocab-trainer.
#
# Uses the SQLite online-backup API (`sqlite3 .backup`), which takes a
# consistent snapshot while the server is running (safe with WAL mode).
#
# Configuration via environment (sensible defaults for the systemd unit):
#   DB_PATH      path to the live database (default: /opt/vocab-trainer/data/vocab.db)
#   BACKUP_DIR   directory to write backups into (default: <db dir>/backups)
#   RETAIN_DAYS  delete backups older than this many days (default: 14)
#
# Restore:
#   systemctl stop vocab-trainer
#   cp /opt/vocab-trainer/data/backups/vocab-YYYY-MM-DD_HHMMSS.sq3 \
#      /opt/vocab-trainer/data/vocab.db
#   systemctl start vocab-trainer
set -euo pipefail

DB_PATH="${DB_PATH:-/opt/vocab-trainer/data/vocab.db}"
BACKUP_DIR="${BACKUP_DIR:-$(dirname "$DB_PATH")/backups}"
RETAIN_DAYS="${RETAIN_DAYS:-14}"

if [[ ! -f "$DB_PATH" ]]; then
  echo "backup: database not found at $DB_PATH" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"
stamp="$(date +%Y-%m-%d_%H%M%S)"
dest="$BACKUP_DIR/vocab-$stamp.sq3"

# .backup is atomic and consistent even while the server is writing.
sqlite3 "$DB_PATH" ".backup '$dest'"
echo "backup: wrote $dest"

# Retention: prune backups older than RETAIN_DAYS.
find "$BACKUP_DIR" -name 'vocab-*.sq3' -type f -mtime "+$RETAIN_DAYS" -print -delete
