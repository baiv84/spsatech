// Package store содержит запросы к базе данных.
package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}
