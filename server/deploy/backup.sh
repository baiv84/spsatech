#!/bin/sh
# Ежедневный бэкап: дамп базы и архив фото, хранятся 14 дней.
set -eu
cd /opt/helpdesk
mkdir -p backups
stamp=$(date +%Y%m%d-%H%M)
docker compose exec -T db pg_dump -U helpdesk -Fc helpdesk > "backups/db-$stamp.dump"
tar -czf "backups/uploads-$stamp.tar.gz" uploads
find backups -type f -mtime +14 -delete
