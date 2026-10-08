// Package effcon синхронизирует пользователей из системы «Оценка эффективности НПР».
//
// Источник — её база (схема dbo), доступ только на чтение к четырём таблицам.
// Пароли там хранятся bcrypt-хешами, как и у нас, поэтому хеш переносится как
// есть и сотрудник входит со своим привычным паролем.
package effcon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Source = "effcon"

// Report — итог синхронизации; сохраняется в sync_runs и показывается в панели.
type Report struct {
	SourceTotal int       `json:"sourceTotal"` // учёток в effcon
	Eligible    int       `json:"eligible"`    // действующих: не закрыта, есть пароль и должность
	NoPosition  int       `json:"noPosition"`  // не закрыта, но нет действующей должности
	Created     int       `json:"created"`
	Updated     int       `json:"updated"`
	Deactivated int       `json:"deactivated"`
	Unchanged   int       `json:"unchanged"`
	Skipped     []Skipped `json:"skipped"`
	Duration    string    `json:"duration"`
	FinishedAt  time.Time `json:"finishedAt"`
}

type Skipped struct {
	Login  string `json:"login"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type sourceUser struct {
	ID         int64
	Login      string
	FullName   string
	Hash       string
	Cancelled  bool
	Position   string
	Department string
	Depts      []string
	HasPost    bool
}

// Действующее назначение — Date_to IS NULL (так считает и сам effcon).
const sourceQuery = `
SELECT u.id_user,
       lower(trim(coalesce(u.us_login, ''))),
       trim(concat_ws(' ', nullif(trim(u.surname), ''), nullif(trim(u.name), ''), nullif(trim(u.fathername), ''))),
       coalesce(u.password_hash, ''),
       u.cancel_sign <> 0,
       coalesce(a.posts, ''),
       coalesce(a.depts, ''),
       coalesce(a.dept_list, '{}'),
       a.id_user IS NOT NULL
FROM dbo.user_ u
LEFT JOIN (
    SELECT s.id_user,
           string_agg(DISTINCT trim(p.full_name_post), ', ') AS posts,
           string_agg(DISTINCT trim(d.full_name_department), ', ') AS depts,
           array_remove(array_agg(DISTINCT trim(d.full_name_department) ORDER BY trim(d.full_name_department)), NULL) AS dept_list
    FROM dbo.schedule_state s
    LEFT JOIN dbo.post p ON p.id_post = s.id_post
    LEFT JOIN dbo.department d ON d.id_department = s.id_department
    WHERE s.date_to IS NULL
    GROUP BY s.id_user
) a ON a.id_user = u.id_user`

func readSource(ctx context.Context, url string) ([]sourceUser, error) {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to effcon: %w", err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, sourceQuery)
	if err != nil {
		return nil, fmt.Errorf("query effcon: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (sourceUser, error) {
		var u sourceUser
		err := row.Scan(&u.ID, &u.Login, &u.FullName, &u.Hash, &u.Cancelled, &u.Position, &u.Department, &u.Depts, &u.HasPost)
		return u, err
	})
}

type localUser struct {
	ID         int64
	Login      string
	Hash       string
	FullName   string
	Position   string
	Department string
	Depts      []string
	IsActive   bool
	ExternalID *int64
}

// Sync переносит изменения из effcon в нашу базу одной транзакцией.
func Sync(ctx context.Context, pool *pgxpool.Pool, sourceURL string) (*Report, error) {
	started := time.Now()
	src, err := readSource(ctx, sourceURL)
	if err != nil {
		return nil, err
	}
	rep := &Report{SourceTotal: len(src), Skipped: []Skipped{}}

	// Отбираем действующих; одинаковые логины у нескольких действующих — не угадываем, пропускаем.
	byLogin := map[string][]sourceUser{}
	for _, u := range src {
		switch {
		case u.Cancelled:
			continue
		case !u.HasPost:
			rep.NoPosition++
			continue
		case u.Login == "":
			rep.Skipped = append(rep.Skipped, Skipped{Name: u.FullName, Reason: "нет логина"})
			continue
		case u.Hash == "":
			rep.Skipped = append(rep.Skipped, Skipped{Login: u.Login, Name: u.FullName, Reason: "не задан пароль"})
			continue
		}
		byLogin[u.Login] = append(byLogin[u.Login], u)
	}
	var eligible []sourceUser
	for login, us := range byLogin {
		if len(us) > 1 {
			names := make([]string, len(us))
			for i, u := range us {
				names[i] = u.FullName
			}
			rep.Skipped = append(rep.Skipped, Skipped{
				Login: login, Name: strings.Join(names, "; "),
				Reason: "один логин у нескольких действующих учёток в effcon",
			})
			continue
		}
		eligible = append(eligible, us[0])
	}
	rep.Eligible = len(eligible)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, lower(login), password_hash, full_name, position, department, departments, is_active,
		       CASE WHEN external_source = $1 THEN external_id END
		FROM users FOR UPDATE`, Source)
	if err != nil {
		return nil, err
	}
	locals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (localUser, error) {
		var u localUser
		err := row.Scan(&u.ID, &u.Login, &u.Hash, &u.FullName, &u.Position, &u.Department, &u.Depts, &u.IsActive, &u.ExternalID)
		return u, err
	})
	if err != nil {
		return nil, err
	}
	byExternal := map[int64]*localUser{}
	loginOwner := map[string]*localUser{}
	activeSynced := 0
	for i := range locals {
		u := &locals[i]
		loginOwner[u.Login] = u
		if u.ExternalID != nil {
			byExternal[*u.ExternalID] = u
			if u.IsActive {
				activeSynced++
			}
		}
	}

	// Защита: если источник внезапно «опустел» (сбой, пустая база),
	// не блокируем массово людей, а останавливаемся.
	if activeSynced > 20 && len(eligible) < activeSynced/2 {
		return nil, fmt.Errorf("effcon вернул %d действующих пользователей, а у нас их %d — "+
			"похоже на сбой источника, синхронизация остановлена", len(eligible), activeSynced)
	}

	seen := map[int64]bool{}
	for _, s := range eligible {
		seen[s.ID] = true
		if l, ok := byExternal[s.ID]; ok {
			if owner, taken := loginOwner[s.Login]; taken && owner.ID != l.ID {
				rep.Skipped = append(rep.Skipped, Skipped{Login: s.Login, Name: s.FullName,
					Reason: "логин в effcon сменился на занятый другим пользователем"})
				continue
			}
			if l.Login == s.Login && l.Hash == s.Hash && l.FullName == s.FullName &&
				l.Position == s.Position && l.Department == s.Department && slices.Equal(l.Depts, s.Depts) && l.IsActive {
				rep.Unchanged++
				continue
			}
			// Смена пароля или блокировка завершает выданные сессии.
			if _, err := tx.Exec(ctx, `
				UPDATE users SET login = $2, full_name = $3, position = $4, department = $5, departments = $7,
				       token_version = token_version + CASE WHEN password_hash <> $6 OR NOT is_active THEN 1 ELSE 0 END,
				       password_hash = $6, is_active = TRUE, must_change_password = FALSE
				WHERE id = $1`, l.ID, s.Login, s.FullName, s.Position, s.Department, s.Hash, s.Depts); err != nil {
				return nil, err
			}
			delete(loginOwner, l.Login)
			loginOwner[s.Login] = l
			rep.Updated++
			continue
		}
		if owner, taken := loginOwner[s.Login]; taken {
			reason := "логин занят пользователем, заведённым в панели вручную"
			if owner.ExternalID != nil {
				reason = "логин занят другим пользователем из effcon"
			}
			rep.Skipped = append(rep.Skipped, Skipped{Login: s.Login, Name: s.FullName, Reason: reason})
			continue
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (login, password_hash, full_name, position, department, departments, role,
			                   external_source, external_id)
			VALUES ($1, $2, $3, $4, $5, $6, 'employee', $7, $8) RETURNING id`,
			s.Login, s.Hash, s.FullName, s.Position, s.Department, s.Depts, Source, s.ID).Scan(&id); err != nil {
			return nil, err
		}
		loginOwner[s.Login] = &localUser{ID: id, Login: s.Login}
		rep.Created++
	}

	// Кого в effcon больше нет среди действующих — блокируем.
	for id, l := range byExternal {
		if seen[id] || !l.IsActive {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET is_active = FALSE, token_version = token_version + 1 WHERE id = $1`, l.ID); err != nil {
			return nil, err
		}
		rep.Deactivated++
	}

	rep.FinishedAt = time.Now()
	rep.Duration = rep.FinishedAt.Sub(started).Round(time.Millisecond).String()
	if _, err := tx.Exec(ctx, `
		INSERT INTO sync_runs (source, started_at, finished_at, ok, report) VALUES ($1, $2, $3, TRUE, $4)`,
		Source, started, rep.FinishedAt, rep); err != nil {
		return nil, err
	}
	return rep, tx.Commit(ctx)
}

// RecordFailure сохраняет неудачную попытку, чтобы её было видно в панели.
func RecordFailure(ctx context.Context, pool *pgxpool.Pool, cause error) {
	pool.Exec(ctx, `INSERT INTO sync_runs (source, finished_at, ok, report) VALUES ($1, now(), FALSE, $2)`,
		Source, map[string]string{"error": cause.Error()})
}

type LastRun struct {
	FinishedAt time.Time
	OK         bool
	Report     Report
	Error      string
}

func Last(ctx context.Context, pool *pgxpool.Pool) (*LastRun, error) {
	var r LastRun
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT finished_at, ok, report FROM sync_runs WHERE source = $1 ORDER BY id DESC LIMIT 1`, Source,
	).Scan(&r.FinishedAt, &r.OK, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !r.OK {
		var e struct{ Error string }
		json.Unmarshal(raw, &e)
		r.Error = e.Error
		return &r, nil
	}
	return &r, json.Unmarshal(raw, &r.Report)
}

// Scheduler запускает синхронизацию при старте и затем каждый день в 04:00.
// Одновременно выполняется не больше одной синхронизации.
type Scheduler struct {
	pool *pgxpool.Pool
	url  string
	mu   sync.Mutex
}

func NewScheduler(pool *pgxpool.Pool, url string) *Scheduler {
	return &Scheduler{pool: pool, url: url}
}

func (s *Scheduler) RunNow(ctx context.Context) (*Report, error) {
	if !s.mu.TryLock() {
		return nil, errors.New("синхронизация уже выполняется")
	}
	defer s.mu.Unlock()
	rep, err := Sync(ctx, s.pool, s.url)
	if err != nil {
		RecordFailure(context.WithoutCancel(ctx), s.pool, err)
		slog.Error("effcon sync failed", "err", err)
		return nil, err
	}
	slog.Info("effcon sync", "created", rep.Created, "updated", rep.Updated,
		"deactivated", rep.Deactivated, "skipped", len(rep.Skipped))
	return rep, nil
}

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		s.RunNow(ctx)
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, now.Location())
			if !next.After(now) {
				next = next.AddDate(0, 0, 1)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				s.RunNow(ctx)
			}
		}
	}()
}
