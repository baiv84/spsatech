#!/bin/sh
# Выкладывает сервер: бэкап, загрузка исходников, пересборка контейнера.
#   ./deploy/deploy.sh            — из папки server/
set -eu
. "$(dirname "$0")/common.sh"
SRC="$(cd "$(dirname "$0")/.." && pwd)"

remote "cd $REMOTE_DIR && ./backup.sh"
rsync -az --delete -e "ssh $SSH_OPTS" --exclude .env --exclude data --exclude bin --exclude deploy \
  "$SRC/" "$SSH_USER@$SSH_HOST:$REMOTE_DIR/src/"
rsync -az -e "ssh $SSH_OPTS" "$SRC/deploy/docker-compose.yml" "$SRC/deploy/backup.sh" "$SRC/deploy/.env.example" \
  "$SSH_USER@$SSH_HOST:$REMOTE_DIR/"
remote "cd $REMOTE_DIR && chown -R root:root src docker-compose.yml backup.sh .env.example \
  && docker compose up -d --build app && sleep 5 && docker compose logs app --since 1m | grep -v healthz | tail -5"
curl -fsS -o /dev/null "$PUBLIC_URL/healthz" && echo "Сервер отвечает: $PUBLIC_URL"
