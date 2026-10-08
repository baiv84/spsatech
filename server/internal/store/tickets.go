package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusWaiting    Status = "waiting"
	StatusResolved   Status = "resolved"
	StatusClosed     Status = "closed"
)

type Person struct {
	ID       int64  `json:"id"`
	FullName string `json:"fullName"`
	Role     Role   `json:"role"`
}

type TicketSummary struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Status       Status    `json:"status"`
	Location     string    `json:"location"`
	Department   string    `json:"department"`
	Author       Person    `json:"author"`
	MessageCount int       `json:"messageCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Ticket struct {
	ID          int64        `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Location    string       `json:"location"`
	Department  string       `json:"department"`
	Status      Status       `json:"status"`
	Author      Person       `json:"author"`
	Assignee    *Person      `json:"assignee"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Attachments []Attachment `json:"attachments"`
	Messages    []Message    `json:"messages"`
}

type Message struct {
	ID          int64        `json:"id"`
	Author      Person       `json:"author"`
	Body        string       `json:"body"`
	CreatedAt   time.Time    `json:"createdAt"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	ID          int64     `json:"id"`
	MessageID   *int64    `json:"-"`
	FileName    string    `json:"fileName"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	StorageKey  string    `json:"-"`
	TicketID    int64     `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ListTickets возвращает заявки автора или, если authorID == 0, все заявки.
func (s *Store) ListTickets(ctx context.Context, authorID int64) ([]TicketSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.title, t.status, t.location, t.department, a.id, a.full_name, a.role,
		       (SELECT count(*) FROM ticket_messages m WHERE m.ticket_id = t.id),
		       t.created_at, t.updated_at
		FROM tickets t
		JOIN users a ON a.id = t.author_id
		WHERE $1 = 0 OR t.author_id = $1
		ORDER BY t.updated_at DESC
		LIMIT 500`, authorID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TicketSummary, error) {
		var t TicketSummary
		err := row.Scan(&t.ID, &t.Title, &t.Status, &t.Location, &t.Department,
			&t.Author.ID, &t.Author.FullName, &t.Author.Role,
			&t.MessageCount, &t.CreatedAt, &t.UpdatedAt)
		return t, err
	})
}

type NewTicket struct {
	AuthorID    int64
	Title       string
	Description string
	Location    string
	Department  string
}

type NewAttachment struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
}

// CreateTicket создаёт заявку вместе с вложениями в одной транзакции.
func (s *Store) CreateTicket(ctx context.Context, n NewTicket, files []NewAttachment) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO tickets (author_id, title, description, location, department)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		n.AuthorID, n.Title, n.Description, n.Location, n.Department).Scan(&id); err != nil {
		return 0, err
	}
	if err := insertAttachments(ctx, tx, id, nil, n.AuthorID, files); err != nil {
		return 0, err
	}
	if err := logEvent(ctx, tx, id, n.AuthorID, EventCreated, "", string(StatusNew)); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func insertAttachments(ctx context.Context, tx pgx.Tx, ticketID int64, messageID *int64, uploaderID int64, files []NewAttachment) error {
	for _, f := range files {
		if _, err := tx.Exec(ctx, `
			INSERT INTO attachments (ticket_id, message_id, uploader_id, file_name, content_type, size_bytes, storage_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			ticketID, messageID, uploaderID, f.FileName, f.ContentType, f.SizeBytes, f.StorageKey); err != nil {
			return err
		}
	}
	return nil
}

// TicketAuthor возвращает id автора заявки — для проверки прав доступа.
func (s *Store) TicketAuthor(ctx context.Context, ticketID int64) (int64, error) {
	var authorID int64
	err := s.pool.QueryRow(ctx, `SELECT author_id FROM tickets WHERE id = $1`, ticketID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return authorID, err
}

func (s *Store) TicketByID(ctx context.Context, id int64) (*Ticket, error) {
	var t Ticket
	var assigneeID *int64
	var assigneeName *string
	var assigneeRole *Role
	err := s.pool.QueryRow(ctx, `
		SELECT t.id, t.title, t.description, t.location, t.department, t.status,
		       a.id, a.full_name, a.role,
		       s.id, s.full_name, s.role,
		       t.created_at, t.updated_at
		FROM tickets t
		JOIN users a ON a.id = t.author_id
		LEFT JOIN users s ON s.id = t.assignee_id
		WHERE t.id = $1`, id).Scan(
		&t.ID, &t.Title, &t.Description, &t.Location, &t.Department, &t.Status,
		&t.Author.ID, &t.Author.FullName, &t.Author.Role,
		&assigneeID, &assigneeName, &assigneeRole,
		&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if assigneeID != nil {
		t.Assignee = &Person{ID: *assigneeID, FullName: *assigneeName, Role: *assigneeRole}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT m.id, u.id, u.full_name, u.role, m.body, m.created_at
		FROM ticket_messages m JOIN users u ON u.id = m.author_id
		WHERE m.ticket_id = $1 ORDER BY m.created_at, m.id`, id)
	if err != nil {
		return nil, err
	}
	t.Messages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		var m Message
		err := row.Scan(&m.ID, &m.Author.ID, &m.Author.FullName, &m.Author.Role, &m.Body, &m.CreatedAt)
		m.Attachments = []Attachment{}
		return m, err
	})
	if err != nil {
		return nil, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT id, message_id, file_name, content_type, size_bytes, created_at
		FROM attachments WHERE ticket_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Attachment, error) {
		var a Attachment
		err := row.Scan(&a.ID, &a.MessageID, &a.FileName, &a.ContentType, &a.SizeBytes, &a.CreatedAt)
		return a, err
	})
	if err != nil {
		return nil, err
	}

	byMessage := make(map[int64]int, len(t.Messages))
	for i, m := range t.Messages {
		byMessage[m.ID] = i
	}
	t.Attachments = []Attachment{}
	for _, a := range all {
		if a.MessageID == nil {
			t.Attachments = append(t.Attachments, a)
		} else if i, ok := byMessage[*a.MessageID]; ok {
			t.Messages[i].Attachments = append(t.Messages[i].Attachments, a)
		}
	}
	return &t, nil
}

// AddMessage добавляет сообщение в переписку и применяет автоматические правила:
// первый ответ техподдержки берёт новую заявку в работу и назначает исполнителя,
// ответ автора возвращает в работу ожидающую или решённую заявку.
func (s *Store) AddMessage(ctx context.Context, ticketID int64, author *User, body string, files []NewAttachment) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO ticket_messages (ticket_id, author_id, body) VALUES ($1, $2, $3) RETURNING id`,
		ticketID, author.ID, body).Scan(&id); err != nil {
		return 0, err
	}
	if err := insertAttachments(ctx, tx, ticketID, &id, author.ID, files); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tickets SET updated_at = now(),
		       first_response_at = CASE WHEN $2 THEN COALESCE(first_response_at, now()) ELSE first_response_at END
		WHERE id = $1`, ticketID, author.Role.IsStaff()); err != nil {
		return 0, err
	}
	if err := logEvent(ctx, tx, ticketID, author.ID, EventMessage, "", ""); err != nil {
		return 0, err
	}

	cur, err := lockTicket(ctx, tx, ticketID)
	if err != nil {
		return 0, err
	}
	// Ответ техподдержки — автору заявки; ответ автора — исполнителю.
	text := preview(body, len(files))
	if author.Role.IsStaff() && author.ID != cur.authorID {
		err = enqueuePush(ctx, tx, &cur.authorID, author.ID, ticketID,
			fmt.Sprintf("Ответ по заявке №%d", ticketID), text)
	} else if author.ID == cur.authorID {
		err = enqueuePush(ctx, tx, cur.assigneeID, author.ID, ticketID,
			fmt.Sprintf("Новое сообщение по заявке №%d", ticketID), author.FullName+": "+text)
	}
	if err != nil {
		return 0, err
	}
	if author.Role.IsStaff() {
		if cur.status == StatusNew {
			if err := changeStatus(ctx, tx, ticketID, author.ID, StatusInProgress, false); err != nil {
				return 0, err
			}
		}
		if cur.assigneeID == nil {
			if err := changeAssignee(ctx, tx, ticketID, author.ID, &author.ID); err != nil {
				return 0, err
			}
		}
	} else if cur.status == StatusWaiting || cur.status == StatusResolved {
		if err := changeStatus(ctx, tx, ticketID, author.ID, StatusInProgress, false); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit(ctx)
}

func (s *Store) AttachmentByID(ctx context.Context, id int64) (*Attachment, error) {
	var a Attachment
	err := s.pool.QueryRow(ctx, `
		SELECT id, ticket_id, message_id, file_name, content_type, size_bytes, storage_key, created_at
		FROM attachments WHERE id = $1`, id).Scan(
		&a.ID, &a.TicketID, &a.MessageID, &a.FileName, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

type TicketFilter struct {
	Statuses   []Status
	AssigneeID int64  // 0 — любой исполнитель
	Query      string // поиск по номеру, теме, описанию, месту и автору
}

type StaffTicketSummary struct {
	TicketSummary
	Assignee *Person
}

// ListTicketsFiltered — очередь заявок для веб-панели.
func (s *Store) ListTicketsFiltered(ctx context.Context, f TicketFilter) ([]StaffTicketSummary, error) {
	statuses := make([]string, len(f.Statuses))
	for i, st := range f.Statuses {
		statuses[i] = string(st)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.title, t.status, t.location, t.department, a.id, a.full_name, a.role,
		       (SELECT count(*) FROM ticket_messages m WHERE m.ticket_id = t.id),
		       t.created_at, t.updated_at, s.id, s.full_name, s.role
		FROM tickets t
		JOIN users a ON a.id = t.author_id
		LEFT JOIN users s ON s.id = t.assignee_id
		WHERE (cardinality($1::text[]) = 0 OR t.status = ANY($1))
		  AND ($2 = 0 OR t.assignee_id = $2)
		  AND ($3 = '' OR t.id::text = $3
		       OR t.title ILIKE '%' || $3 || '%' OR t.description ILIKE '%' || $3 || '%'
		       OR t.location ILIKE '%' || $3 || '%' OR a.full_name ILIKE '%' || $3 || '%'
		       OR t.department ILIKE '%' || $3 || '%')
		ORDER BY t.updated_at DESC
		LIMIT 500`, statuses, f.AssigneeID, escapeLike(f.Query))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (StaffTicketSummary, error) {
		var t StaffTicketSummary
		var sID *int64
		var sName *string
		var sRole *Role
		err := row.Scan(&t.ID, &t.Title, &t.Status, &t.Location, &t.Department,
			&t.Author.ID, &t.Author.FullName, &t.Author.Role,
			&t.MessageCount, &t.CreatedAt, &t.UpdatedAt, &sID, &sName, &sRole)
		if sID != nil {
			t.Assignee = &Person{ID: *sID, FullName: *sName, Role: *sRole}
		}
		return t, err
	})
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// StatusCounts — число заявок в каждом статусе.
func (s *Store) StatusCounts(ctx context.Context) (map[Status]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT status, count(*) FROM tickets GROUP BY status`)
	if err != nil {
		return nil, err
	}
	counts := map[Status]int{}
	defer rows.Close()
	for rows.Next() {
		var st Status
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		counts[st] = n
	}
	return counts, rows.Err()
}

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusWaiting, StatusResolved, StatusClosed:
		return true
	}
	return false
}

// SetStatus меняет статус заявки и записывает это в журнал.
// notify — сообщить автору push-уведомлением (не нужно, если только что ушёл ответ).
func (s *Store) SetStatus(ctx context.Context, ticketID, actorID int64, status Status, notify bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return changeStatus(ctx, tx, ticketID, actorID, status, notify)
	})
}

// SetAssignee назначает исполнителя (nil — снять назначение) и записывает это в журнал.
func (s *Store) SetAssignee(ctx context.Context, ticketID, actorID int64, assigneeID *int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return changeAssignee(ctx, tx, ticketID, actorID, assigneeID)
	})
}

func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type ticketState struct {
	status     Status
	assigneeID *int64
	authorID   int64
	title      string
}

func lockTicket(ctx context.Context, tx pgx.Tx, id int64) (ticketState, error) {
	var st ticketState
	err := tx.QueryRow(ctx, `SELECT status, assignee_id, author_id, title FROM tickets WHERE id = $1 FOR UPDATE`, id).
		Scan(&st.status, &st.assigneeID, &st.authorID, &st.title)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, ErrNotFound
	}
	return st, err
}

func isDone(s Status) bool { return s == StatusResolved || s == StatusClosed }

// changeStatus меняет статус и время решения: resolved_at ставится при переходе
// в «Решена»/«Закрыта» (переход между ними его не сдвигает) и сбрасывается при повторном открытии.
func changeStatus(ctx context.Context, tx pgx.Tx, ticketID, actorID int64, to Status, notify bool) error {
	cur, err := lockTicket(ctx, tx, ticketID)
	if err != nil || cur.status == to {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tickets SET status = $2, updated_at = now(),
		       resolved_at = CASE WHEN NOT $3 THEN NULL WHEN $4 THEN resolved_at ELSE now() END
		WHERE id = $1`, ticketID, to, isDone(to), isDone(cur.status)); err != nil {
		return err
	}
	if notify {
		if err := enqueuePush(ctx, tx, &cur.authorID, actorID, ticketID,
			fmt.Sprintf("Заявка №%d %s", ticketID, statusNames[to]), cur.title); err != nil {
			return err
		}
	}
	return logEvent(ctx, tx, ticketID, actorID, EventStatus, string(cur.status), string(to))
}

func changeAssignee(ctx context.Context, tx pgx.Tx, ticketID, actorID int64, to *int64) error {
	cur, err := lockTicket(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	if (cur.assigneeID == nil && to == nil) || (cur.assigneeID != nil && to != nil && *cur.assigneeID == *to) {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE tickets SET assignee_id = $2, updated_at = now() WHERE id = $1`,
		ticketID, to); err != nil {
		return err
	}
	// В журнал пишем ФИО, чтобы история читалась и после переименований.
	name := func(id *int64) (string, error) {
		if id == nil {
			return "", nil
		}
		var n string
		err := tx.QueryRow(ctx, `SELECT full_name FROM users WHERE id = $1`, *id).Scan(&n)
		return n, err
	}
	from, err := name(cur.assigneeID)
	if err != nil {
		return err
	}
	toName, err := name(to)
	if err != nil {
		return err
	}
	return logEvent(ctx, tx, ticketID, actorID, EventAssignee, from, toName)
}

const (
	EventCreated  = "created"
	EventStatus   = "status"
	EventAssignee = "assignee"
	EventMessage  = "message"
)

func logEvent(ctx context.Context, tx pgx.Tx, ticketID, actorID int64, typ, from, to string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ticket_events (ticket_id, actor_id, type, from_value, to_value)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''))`, ticketID, actorID, typ, from, to)
	return err
}

type Event struct {
	Type      string
	From      string
	To        string
	Actor     string
	CreatedAt time.Time
}

// TicketEvents — история заявки для карточки в панели (без отдельных сообщений:
// они и так видны в переписке).
func (s *Store) TicketEvents(ctx context.Context, ticketID int64) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.type, coalesce(e.from_value, ''), coalesce(e.to_value, ''), coalesce(u.full_name, ''), e.created_at
		FROM ticket_events e LEFT JOIN users u ON u.id = e.actor_id
		WHERE e.ticket_id = $1 AND e.type <> 'message'
		ORDER BY e.created_at, e.id`, ticketID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Event])
}
