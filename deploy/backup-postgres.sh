#!/bin/sh
set -eu
set +x
umask 077
: "${POSTGRES_USER:?set POSTGRES_USER}"
: "${POSTGRES_DB:?set POSTGRES_DB}"
: "${BACKUP_DIR:?set BACKUP_DIR}"
COMPOSE_FILE=${COMPOSE_FILE:-/opt/hackathon/docker-compose.yml}
ENV_FILE=${ENV_FILE:-/etc/hackathon/main.env}
mkdir -p "$BACKUP_DIR"
chmod 0700 "$BACKUP_DIR"
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
NAME="manual-$STAMP-$$"
TARGET="$BACKUP_DIR/$NAME"
mkdir -m 0700 "$TARGET"
docker compose --project-name hackathon --env-file "$ENV_FILE" -f "$COMPOSE_FILE" \
    exec -T postgres pg_dump --format=custom --no-owner --no-privileges \
    -U "$POSTGRES_USER" -d "$POSTGRES_DB" > "$TARGET/postgres.dump"
sha256sum "$TARGET/postgres.dump" > "$TARGET/postgres.dump.sha256"
printf '%s\n' "$NAME"
