-- Аналитика: журнал событий заявки, подразделение на момент создания,
-- время первого ответа и решения.

-- Подразделения пользователя списком (у сотрудника их может быть несколько).
-- users.department остаётся строкой для показа.
ALTER TABLE users ADD COLUMN departments TEXT[] NOT NULL DEFAULT '{}';
UPDATE users SET departments = ARRAY[department] WHERE department <> '' AND external_source IS NULL;

-- Подразделение, к которому относится заявка. Фиксируется при создании,
-- чтобы отчёты не «переезжали» вместе с сотрудником.
ALTER TABLE tickets ADD COLUMN department TEXT NOT NULL DEFAULT '';
ALTER TABLE tickets ADD COLUMN first_response_at TIMESTAMPTZ;
-- Когда заявка последний раз стала «Решена»/«Закрыта»; NULL, если она снова открыта.
ALTER TABLE tickets ADD COLUMN resolved_at TIMESTAMPTZ;

CREATE TABLE ticket_events (
    id         BIGSERIAL PRIMARY KEY,
    ticket_id  BIGINT      NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    actor_id   BIGINT      REFERENCES users (id),
    -- created, status, assignee, message
    type       TEXT        NOT NULL,
    from_value TEXT,
    to_value   TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ticket_events_ticket_idx ON ticket_events (ticket_id, created_at);
CREATE INDEX tickets_created_idx ON tickets (created_at);
CREATE INDEX tickets_resolved_idx ON tickets (resolved_at) WHERE resolved_at IS NOT NULL;

-- Заполняем для уже существующих заявок то, что можно восстановить.
UPDATE tickets t SET department = u.department FROM users u WHERE u.id = t.author_id;
UPDATE tickets t SET first_response_at = (
    SELECT min(m.created_at) FROM ticket_messages m JOIN users u ON u.id = m.author_id
    WHERE m.ticket_id = t.id AND u.role IN ('support', 'admin'));
UPDATE tickets SET resolved_at = updated_at WHERE status IN ('resolved', 'closed');
INSERT INTO ticket_events (ticket_id, actor_id, type, to_value, created_at)
SELECT id, author_id, 'created', 'new', created_at FROM tickets;
