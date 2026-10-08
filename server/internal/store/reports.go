package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Отчёты считаются за полуинтервал [From, To).
// «Поступило» — по дате создания, «Решено» — по дате перехода в «Решена»/«Закрыта».

type ReportSummary struct {
	Created        int
	Resolved       int
	StillOpen      int      // из поступивших в периоде ещё не решены
	OpenNow        int      // открыто сейчас всего, независимо от периода
	MedianResponse *float64 // часы до первого ответа, по поступившим в периоде
	MedianResolve  *float64 // часы от создания до решения, по решённым в периоде
}

type ReportRow struct {
	Name          string
	Created       int
	Resolved      int
	StillOpen     int
	InWork        int // только для исполнителей: открытых заявок на нём сейчас
	MedianResolve *float64
}

const done = `('resolved', 'closed')`

func (s *Store) ReportSummary(ctx context.Context, from, to time.Time) (ReportSummary, error) {
	var r ReportSummary
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM tickets WHERE created_at >= $1 AND created_at < $2),
		  (SELECT count(*) FROM tickets WHERE resolved_at >= $1 AND resolved_at < $2),
		  (SELECT count(*) FROM tickets WHERE created_at >= $1 AND created_at < $2 AND status NOT IN `+done+`),
		  (SELECT count(*) FROM tickets WHERE status NOT IN `+done+`),
		  (SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM first_response_at - created_at) / 3600)
		     FROM tickets WHERE created_at >= $1 AND created_at < $2 AND first_response_at IS NOT NULL),
		  (SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM resolved_at - created_at) / 3600)
		     FROM tickets WHERE resolved_at >= $1 AND resolved_at < $2)`, from, to,
	).Scan(&r.Created, &r.Resolved, &r.StillOpen, &r.OpenNow, &r.MedianResponse, &r.MedianResolve)
	return r, err
}

func (s *Store) ReportByDepartment(ctx context.Context, from, to time.Time) ([]ReportRow, error) {
	rows, err := s.pool.Query(ctx, `
		WITH c AS (
		  SELECT department, count(*) AS created,
		         count(*) FILTER (WHERE status NOT IN `+done+`) AS still_open
		  FROM tickets WHERE created_at >= $1 AND created_at < $2 GROUP BY department),
		r AS (
		  SELECT department, count(*) AS resolved,
		         percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM resolved_at - created_at) / 3600) AS med
		  FROM tickets WHERE resolved_at >= $1 AND resolved_at < $2 GROUP BY department)
		SELECT coalesce(nullif(coalesce(c.department, r.department), ''), 'Не указано'),
		       coalesce(c.created, 0), coalesce(r.resolved, 0), coalesce(c.still_open, 0), 0, r.med
		FROM c FULL JOIN r ON r.department = c.department
		ORDER BY 2 DESC, 3 DESC, 1`, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ReportRow])
}

func (s *Store) ReportByAssignee(ctx context.Context, from, to time.Time) ([]ReportRow, error) {
	rows, err := s.pool.Query(ctx, `
		WITH r AS (
		  SELECT coalesce(assignee_id, 0) AS uid, count(*) AS resolved,
		         percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM resolved_at - created_at) / 3600) AS med
		  FROM tickets WHERE resolved_at >= $1 AND resolved_at < $2 GROUP BY 1),
		w AS (
		  SELECT coalesce(assignee_id, 0) AS uid, count(*) AS in_work
		  FROM tickets WHERE status NOT IN `+done+` GROUP BY 1)
		SELECT coalesce(u.full_name, 'Не назначен'), 0, coalesce(r.resolved, 0), 0, coalesce(w.in_work, 0), r.med
		FROM r FULL JOIN w ON w.uid = r.uid
		LEFT JOIN users u ON u.id = coalesce(r.uid, w.uid)
		ORDER BY 3 DESC, 5 DESC, 1`, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ReportRow])
}

type ExportRow struct {
	ID              int64
	CreatedAt       time.Time
	Title           string
	Location        string
	Author          string
	Department      string
	Status          Status
	Assignee        string
	FirstResponseAt *time.Time
	ResolvedAt      *time.Time
	Messages        int
}

// ExportTickets — заявки, поступившие или решённые в периоде, для выгрузки в Excel.
func (s *Store) ExportTickets(ctx context.Context, from, to time.Time) ([]ExportRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.created_at, t.title, t.location, a.full_name, t.department, t.status,
		       coalesce(s.full_name, ''), t.first_response_at, t.resolved_at,
		       (SELECT count(*) FROM ticket_messages m WHERE m.ticket_id = t.id)
		FROM tickets t
		JOIN users a ON a.id = t.author_id
		LEFT JOIN users s ON s.id = t.assignee_id
		WHERE (t.created_at >= $1 AND t.created_at < $2) OR (t.resolved_at >= $1 AND t.resolved_at < $2)
		ORDER BY t.id`, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ExportRow])
}
