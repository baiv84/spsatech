CREATE TABLE users (
    id                   BIGSERIAL PRIMARY KEY,
    login                TEXT        NOT NULL UNIQUE,
    password_hash        TEXT        NOT NULL,
    full_name            TEXT        NOT NULL,
    department           TEXT        NOT NULL DEFAULT '',
    phone                TEXT        NOT NULL DEFAULT '',
    role                 TEXT        NOT NULL CHECK (role IN ('employee', 'support', 'admin')),
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    must_change_password BOOLEAN     NOT NULL DEFAULT FALSE,
    -- Увеличивается при смене пароля, чтобы отозвать ранее выданные токены.
    token_version        INT         NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tickets (
    id          BIGSERIAL PRIMARY KEY,
    author_id   BIGINT      NOT NULL REFERENCES users (id),
    assignee_id BIGINT      REFERENCES users (id),
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL,
    location    TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'new'
        CHECK (status IN ('new', 'in_progress', 'waiting', 'resolved', 'closed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tickets_author_idx ON tickets (author_id, updated_at DESC);
CREATE INDEX tickets_status_idx ON tickets (status, updated_at DESC);

CREATE TABLE ticket_messages (
    id         BIGSERIAL PRIMARY KEY,
    ticket_id  BIGINT      NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    author_id  BIGINT      NOT NULL REFERENCES users (id),
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ticket_messages_ticket_idx ON ticket_messages (ticket_id, created_at);

CREATE TABLE attachments (
    id           BIGSERIAL PRIMARY KEY,
    ticket_id    BIGINT      NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    message_id   BIGINT      REFERENCES ticket_messages (id) ON DELETE CASCADE,
    uploader_id  BIGINT      NOT NULL REFERENCES users (id),
    file_name    TEXT        NOT NULL,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    storage_key  TEXT        NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX attachments_ticket_idx ON attachments (ticket_id);
