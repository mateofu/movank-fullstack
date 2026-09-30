package merchant

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("merchant not found")

type Merchant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Get(ctx context.Context, merchantID string) (Merchant, error) {
	var result Merchant
	err := s.pool.QueryRow(ctx, "SELECT id::text, name FROM public.merchants WHERE id = $1", merchantID).Scan(&result.ID, &result.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Merchant{}, ErrNotFound
	}
	return result, err
}
