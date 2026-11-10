#!/usr/bin/env bash
set -euo pipefail

# AuthForge Automated PostgreSQL Backup Script
# Usage: ./scripts/backup-db.sh [backup_directory]

BACKUP_DIR="${1:-./backups}"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/authforge_backup_${TIMESTAMP}.sql.gz"
RETENTION_DAYS=7

mkdir -p "${BACKUP_DIR}"

echo "==> Starting AuthForge database backup at $(date)..."

# Detect running compose project or container
COMPOSE_CMD="docker compose"
if ! command -v docker &>/dev/null; then
    echo "Error: docker command not found." >&2
    exit 1
fi

# Run pg_dump from the postgres container and gzip the output
${COMPOSE_CMD} exec -T postgres pg_dump -U authforge -d authforge \
    --format=plain \
    --no-owner \
    --no-privileges \
    | gzip -9 > "${BACKUP_FILE}"

FILE_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
echo "==> Backup completed successfully: ${BACKUP_FILE} (${FILE_SIZE})"

# Prune backups older than RETENTION_DAYS
echo "==> Pruning backups older than ${RETENTION_DAYS} days..."
find "${BACKUP_DIR}" -name "authforge_backup_*.sql.gz" -type f -mtime +"${RETENTION_DAYS}" -delete || true

echo "==> Done."
