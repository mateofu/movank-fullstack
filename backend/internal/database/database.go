package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context) (*pgxpool.Pool, error) {
	for _, name := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGSSLMODE"} {
		if os.Getenv(name) == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
	}
	cfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "movank-api"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not initialize PostgreSQL pool")
	}
	return pool, nil
}
