package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Role string

const (
	RoleEmployee Role = "employee"
	RoleSupport  Role = "support"
	RoleAdmin    Role = "admin"
)

func (r Role) Valid() bool {
	return r == RoleEmployee || r == RoleSupport || r == RoleAdmin
}

// IsStaff — сотрудник техподдержки или администратор: видит все заявки.
func (r Role) IsStaff() bool {
	return r == RoleSupport || r == RoleAdmin
}

type User struct {
	ID                 int64
	Login              string
	PasswordHash       string
	FullName           string
	Department         string
	Phone              string
	Role               Role
	IsActive           bool
	MustChangePassword bool
	TokenVersion       int
	CreatedAt          time.Time
	Position           string
	ExternalSource     *string
	ExternalID         *int64
	Departments        []string
}

// IsExternal — пользователь синхронизируется из другой системы: его данные
// и пароль меняются там, а не у нас.
func (u *User) IsExternal() bool { return u.ExternalSource != nil }

const userColumns = `id, login, password_hash, full_name, department, phone, role,
	is_active, must_change_password, token_version, created_at, position, external_source, external_id, departments`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Login, &u.PasswordHash, &u.FullName, &u.Department, &u.Phone,
		&u.Role, &u.IsActive, &u.MustChangePassword, &u.TokenVersion, &u.CreatedAt,
		&u.Position, &u.ExternalSource, &u.ExternalID, &u.Departments)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// UserByLogin ищет пользователя без учёта регистра логина.
func (s *Store) UserByLogin(ctx context.Context, login string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(login) = lower($1)`, login))
}

type NewUser struct {
	Login              string
	PasswordHash       string
	FullName           string
	Department         string
	Phone              string
	Position           string
	Role               Role
	MustChangePassword bool
}

func (s *Store) CreateUser(ctx context.Context, n NewUser) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (login, password_hash, full_name, department, phone, position, role, must_change_password,
		                   departments)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CASE WHEN $4 = '' THEN '{}' ELSE ARRAY[$4] END)
		RETURNING `+userColumns,
		n.Login, n.PasswordHash, n.FullName, n.Department, n.Phone, n.Position, n.Role, n.MustChangePassword))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil, ErrDuplicate
	}
	return u, err
}

// SetPassword меняет пароль и отзывает все ранее выданные токены.
// Возвращает новую версию токена.
func (s *Store) SetPassword(ctx context.Context, userID int64, hash string, mustChange bool) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx, `
		UPDATE users
		SET password_hash = $2, must_change_password = $3, token_version = token_version + 1
		WHERE id = $1
		RETURNING token_version`, userID, hash, mustChange).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return version, err
}

// ListUsers возвращает пользователей, отсортированных по ФИО.
func (s *Store) ListUsers(ctx context.Context, query string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+userColumns+` FROM users
		WHERE $1 = '' OR login ILIKE '%' || $1 || '%' OR full_name ILIKE '%' || $1 || '%'
		   OR department ILIKE '%' || $1 || '%' OR phone ILIKE '%' || $1 || '%'
		ORDER BY is_active DESC, full_name
		LIMIT 1000`, escapeLike(query))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		u, err := scanUser(row)
		if err != nil {
			return User{}, err
		}
		return *u, nil
	})
}

// StaffUsers — активные сотрудники техподдержки и администраторы (для назначения исполнителя).
func (s *Store) StaffUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+userColumns+` FROM users
		WHERE role IN ('support', 'admin') AND is_active
		ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		u, err := scanUser(row)
		if err != nil {
			return User{}, err
		}
		return *u, nil
	})
}

type UserUpdate struct {
	FullName   string
	Department string
	Phone      string
	Position   string
	Role       Role
	IsActive   bool
}

// UpdateUser меняет данные пользователя. Блокировка отзывает его токены.
func (s *Store) UpdateUser(ctx context.Context, id int64, u UserUpdate) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET full_name = $2, department = $3, phone = $4, role = $5, is_active = $6, position = $7,
		    departments = CASE WHEN external_source IS NOT NULL THEN departments
		                       WHEN $3 = '' THEN '{}' ELSE ARRAY[$3] END,
		    token_version = token_version + CASE WHEN is_active AND NOT $6 THEN 1 ELSE 0 END
		WHERE id = $1`, id, u.FullName, u.Department, u.Phone, u.Role, u.IsActive, u.Position)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
