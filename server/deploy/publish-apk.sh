#!/bin/sh
# Публикует APK на страницу <PUBLIC_URL>/app (сервер — в deploy/deploy.conf)
#   ./deploy/publish-apk.sh ../dist/helpdesk-1.1.apk            — на боевой сервер
#   ./deploy/publish-apk.sh ../dist/helpdesk-1.1.apk --min 2    — и обязать всех со
#                                                                  versionCode < 2 обновиться
#   ./deploy/publish-apk.sh ../dist/helpdesk-1.1.apk --local    — в ./data/apk (проверка)
# Без --min минимальная версия остаётся прежней (берётся с сервера).
set -eu
APK=${1:?укажите путь к APK}
shift
LOCAL=0
MIN=""
while [ $# -gt 0 ]; do
  case "$1" in
    --local) LOCAL=1 ;;
    --min) MIN=${2:?укажите versionCode}; shift ;;
    *) echo "Неизвестный параметр $1"; exit 1 ;;
  esac
  shift
done
BT=$(ls -d "$HOME/Library/Android/sdk/build-tools/"* | sort -V | tail -1)

"$BT/apksigner" verify "$APK" >/dev/null || { echo "APK не подписан"; exit 1; }
BADGING=$("$BT/aapt2" dump badging "$APK")
PKG=$(echo "$BADGING" | sed -n "s/^package: name='\([^']*\)'.*/\1/p")
[ "$PKG" = "com.spsatech.app" ] || { echo "Неожиданный пакет: $PKG"; exit 1; }
echo "$BADGING" | grep -q "application-debuggable" && { echo "Это debug-сборка, нужна release"; exit 1; }
VNAME=$(echo "$BADGING" | sed -n "s/.*versionName='\([^']*\)'.*/\1/p")
VCODE=$(echo "$BADGING" | sed -n "s/.*versionCode='\([^']*\)'.*/\1/p")
SIZE=$(stat -f %z "$APK")
SHA=$(shasum -a 256 "$APK" | cut -d' ' -f1)
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)

if [ -z "$MIN" ]; then
  if [ "$LOCAL" = 1 ]; then SRC="http://localhost:8091"; else . "$(dirname "$0")/common.sh"; SRC="$PUBLIC_URL"; fi
  MIN=$(curl -fs "$SRC/app/version.json" | sed -n 's/.*"minVersionCode":\([0-9]*\).*/\1/p')
  MIN=${MIN:-0}
fi
[ "$MIN" -le "$VCODE" ] || { echo "--min $MIN больше версии публикуемого APK ($VCODE)"; exit 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
printf '{"versionName":"%s","versionCode":%s,"minVersionCode":%s,"sizeBytes":%s,"sha256":"%s","publishedAt":"%s"}\n' \
  "$VNAME" "$VCODE" "$MIN" "$SIZE" "$SHA" "$NOW" > "$TMP/meta.json"
echo "Версия $VNAME ($VCODE), минимальная $MIN, $SIZE байт, sha256 $SHA"

if [ "$LOCAL" = 1 ]; then
  DIR="$(dirname "$0")/../data/apk"
  mkdir -p "$DIR" && cp "$APK" "$DIR/helpdesk.apk" && cp "$TMP/meta.json" "$DIR/meta.json"
  echo "Опубликовано локально: $DIR"
  exit 0
fi

. "$(dirname "$0")/common.sh"
# Сначала загружаем под временными именами, потом атомарно переименовываем:
# сотрудник никогда не скачает недокачанный файл.
remote "install -d -m 755 $REMOTE_DIR/apk"
upload "$APK" "$REMOTE_DIR/apk/.helpdesk.apk.new"
upload "$TMP/meta.json" "$REMOTE_DIR/apk/.meta.json.new"
remote "cd $REMOTE_DIR/apk && echo '$SHA  .helpdesk.apk.new' | sha256sum -c --quiet \
  && chmod 644 .helpdesk.apk.new .meta.json.new \
  && mv .helpdesk.apk.new helpdesk.apk && mv .meta.json.new meta.json && echo 'Опубликовано на сервере'"
