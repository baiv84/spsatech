# Общие настройки для скриптов выкладки: читает deploy.conf рядом.
CONF="$(dirname "$0")/deploy.conf"
[ -f "$CONF" ] || { echo "Нет $CONF — скопируйте deploy.conf.example и заполните"; exit 1; }
. "$CONF"
SSH_OPTS="-p $SSH_PORT -i $SSH_KEY"
remote() { ssh $SSH_OPTS "$SSH_USER@$SSH_HOST" "$@"; }
upload() { scp -q -P "$SSH_PORT" -i "$SSH_KEY" "$1" "$SSH_USER@$SSH_HOST:$2"; }
