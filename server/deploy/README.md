# Развёртывание

Адрес: https://helpdesk.app-sibpsa.ru. Параметры SSH-подключения — в `deploy/deploy.conf`
(не в git; образец — `deploy.conf.example`).

| Что | Где |
|---|---|
| Docker Compose (Postgres + приложение) | `/opt/helpdesk` |
| Секреты (пароль БД, JWT, пароль роли helpdesk_sync в базе effcon) | `/opt/helpdesk/.env` — только на сервере |
| Ключ Firebase для push | `/opt/helpdesk/secrets/fcm.json` (владелец 10001, права 400) — только на сервере |
| Фото | `/opt/helpdesk/uploads` |
| База | `/opt/helpdesk/pgdata` |
| Бэкапы (каждый день в 03:30, хранятся 14 дней) | `/opt/helpdesk/backups`, лог `/var/log/helpdesk-backup.log` |
| nginx | `/etc/nginx/sites-available/helpdesk.conf` (сертификат — certbot) |

Приложение слушает только `127.0.0.1:8090`, снаружи доступно через nginx.
Пользователи каждую ночь в 04:00 синхронизируются из базы effcon (роль `helpdesk_sync`, только чтение
`dbo.user_`, `dbo.schedule_state`, `dbo.post`, `dbo.department`); контейнер подключён к сети `effcon_default`.

Если ключ Firebase понадобится заменить: Firebase Console → Project settings → Service accounts →
Generate new private key, положить в `secrets/fcm.json` с теми же правами и `docker compose up -d app`.
Старый ключ после этого удалить там же, в Google Cloud Console → IAM → Service accounts.

## Обновление

Из папки `server/`:

```sh
./deploy/deploy.sh
```

Скрипт делает бэкап, загружает исходники и пересобирает контейнер приложения.
Миграции базы применяются автоматически при старте.

## Android-приложение

Страница скачивания: https://helpdesk.app-sibpsa.ru/app (APK лежит в `/opt/helpdesk/apk`).
Новая версия: увеличить `versionCode`/`versionName` в `app/build.gradle.kts`, затем из корня проекта

```sh
./gradlew assembleRelease
cp app/build/outputs/apk/release/app-release.apk dist/helpdesk-<версия>.apk
server/deploy/publish-apk.sh dist/helpdesk-<версия>.apk
```

Подпись — ключом из `keystore/` (не в git, обязательно храните резервную копию).

Приложение (начиная с 1.1) само проверяет `/app/version.json` при открытии и предлагает обновиться.
Если новая версия сервера несовместима со старыми приложениями, опубликуйте с `--min <versionCode>` —
версии старее не будут работать, пока их не обновят. Без `--min` минимальная версия остаётся прежней.

## Полезное

```sh
cd /opt/helpdesk
docker compose logs -f app                       # логи
docker compose exec app helpdesk create-user -h  # завести пользователя из консоли
# восстановление базы из бэкапа:
docker compose exec -T db pg_restore -U helpdesk -d helpdesk --clean < backups/db-YYYYMMDD-HHMM.dump
```
