#!/usr/bin/env bash
set -euo pipefail

# AuthForge PostgreSQL Restore Script
# Usage: ./scripts/restore-db.sh <backup_file.sql.gz>

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 <path_to_backup_file.sql.gz>" >&2
    exit 1
fi

BACKUP_FILE="$1"

if [ ! -f "${BACKUP_FILE}" ]; then
    echo "Error: Backup file '${BACKUP_FILE}' does not exist." >&2
    exit 1
fi

echo "WARNING: This will overwrite data in the AuthForge database!"
read -r -p "Are you sure you want to restore from ${BACKUP_FILE}? [y/N] " confirmation
if [[ ! "${confirmation}" =~ ^[yY](es)?$ ]]; then
    echo "Restore cancelled."
    exit 0
fi

echo "==> Restoring AuthForge database from ${BACKUP_FILE}..."

if [[ "${BACKUP_FILE}" == *.gz ]]; then
    gunzip -c "${BACKUP_FILE}" | docker compose exec -T postgres psql -U authforge -d authforge
else
    cat "${BACKUP_FILE}" | docker compose exec -T postgres psql -U authforge -d authforge
fi

echo "==> Database restore completed successfully."
