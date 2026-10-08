-- Push-уведомления: устройства пользователей и очередь отправки.

CREATE TABLE device_tokens (
    token        TEXT PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform     TEXT        NOT NULL DEFAULT 'android',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX device_tokens_user_idx ON device_tokens (user_id);

-- Уведомление кладётся в очередь в той же транзакции, что и изменение заявки,
-- и отправляется фоновым обработчиком: ничего не теряется при сбое связи с FCM.
CREATE TABLE push_outbox (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ticket_id  BIGINT      NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts   INT         NOT NULL DEFAULT 0,
    sent_at    TIMESTAMPTZ,
    last_error TEXT
);
CREATE INDEX push_outbox_pending_idx ON push_outbox (id) WHERE sent_at IS NULL;
