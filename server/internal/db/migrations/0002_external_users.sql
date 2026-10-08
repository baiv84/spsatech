-- Должность и пользователи, которые синхронизируются из внешней системы
-- («Оценка эффективности НПР», effcon). У таких пользователей ФИО, должность,
-- подразделение, пароль и активность ведутся во внешней системе.
ALTER TABLE users ADD COLUMN position TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN external_source TEXT;
ALTER TABLE users ADD COLUMN external_id BIGINT;
CREATE UNIQUE INDEX users_external_idx ON users (external_source, external_id)
    WHERE external_source IS NOT NULL;

CREATE TABLE sync_runs (
    id          BIGSERIAL PRIMARY KEY,
    source      TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    ok          BOOLEAN     NOT NULL DEFAULT FALSE,
    -- Сводка: сколько создано, обновлено, заблокировано, какие конфликты.
    report      JSONB       NOT NULL DEFAULT '{}'
);
