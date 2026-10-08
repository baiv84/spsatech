package store

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// RegisterDevice привязывает FCM-токен к пользователю. Если телефоном раньше
// пользовался другой сотрудник, токен переходит к новому.
func (s *Store) RegisterDevice(ctx context.Context, userID int64, token, platform string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_tokens (token, user_id, platform) VALUES ($1, $2, $3)
		ON CONFLICT (token) DO UPDATE SET user_id = $2, platform = $3, last_seen_at = now()`,
		token, userID, platform)
	return err
}

func (s *Store) UnregisterDevice(ctx context.Context, userID int64, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM device_tokens WHERE token = $1 AND user_id = $2`, token, userID)
	return err
}

func (s *Store) DeleteDeviceToken(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM device_tokens WHERE token = $1`, token)
	return err
}

func (s *Store) DeviceTokens(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT token FROM device_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

type PushJob struct {
	ID       int64
	UserID   int64
	TicketID int64
	Title    string
	Body     string
	Attempts int
}

// PendingPushes забирает порцию неотправленных уведомлений.
func (s *Store) PendingPushes(ctx context.Context, maxAttempts, limit int) ([]PushJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, ticket_id, title, body, attempts FROM push_outbox
		WHERE sent_at IS NULL AND attempts < $1
		ORDER BY id LIMIT $2`, maxAttempts, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[PushJob])
}

func (s *Store) MarkPushSent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE push_outbox SET sent_at = now(), attempts = attempts + 1 WHERE id = $1`, id)
	return err
}

func (s *Store) MarkPushFailed(ctx context.Context, id int64, cause string) error {
	_, err := s.pool.Exec(ctx, `UPDATE push_outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`, id, cause)
	return err
}

// CleanupPushes удаляет старые записи очереди.
func (s *Store) CleanupPushes(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_outbox WHERE created_at < now() - interval '30 days'`)
	return err
}

// enqueuePush ставит уведомление в очередь в рамках текущей транзакции.
// Самому себе уведомления не шлём.
func enqueuePush(ctx context.Context, tx pgx.Tx, toUserID *int64, actorID, ticketID int64, title, body string) error {
	if toUserID == nil || *toUserID == actorID {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO push_outbox (user_id, ticket_id, title, body) VALUES ($1, $2, $3, $4)`,
		*toUserID, ticketID, title, body)
	return err
}

func preview(body string, photos int) string {
	body = strings.Join(strings.Fields(body), " ")
	if utf8.RuneCountInString(body) > 200 {
		body = string([]rune(body)[:200]) + "…"
	}
	if body == "" && photos > 0 {
		return fmt.Sprintf("📷 Фото: %d", photos)
	}
	return body
}

var statusNames = map[Status]string{
	StatusNew: "новая", StatusInProgress: "в работе", StatusWaiting: "ожидает вашего ответа",
	StatusResolved: "решена", StatusClosed: "закрыта",
}
