package product

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

var ErrInvalid = errors.New("invalid product")
var ErrConflict = errors.New("sku already exists")
var ErrNotFound = errors.New("product not found")
var skuPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,63}$`)

type Input struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	PriceMinor int64  `json:"price_minor"`
	Currency   string `json:"currency"`
}

func (in Input) Normalize() (Input, error) {
	in.SKU = strings.ToUpper(strings.TrimSpace(in.SKU))
	in.Name = strings.TrimSpace(in.Name)
	if !skuPattern.MatchString(in.SKU) || !utf8.ValidString(in.Name) || strings.ContainsRune(in.Name, 0) || utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 120 || in.PriceMinor < 1 || in.PriceMinor > 1_000_000_000_000 || in.Currency != "COP" {
		return Input{}, ErrInvalid
	}
	return in, nil
}

func ValidID(id string) bool {
	var value pgtype.UUID
	return len(id) == 36 && id[8] == '-' && id[13] == '-' && id[18] == '-' && id[23] == '-' && value.Scan(id) == nil
}

type Product struct {
	ID string `json:"id"`
	Input
	CreatedAt time.Time `json:"created_at"`
}

type Page struct {
	Items      []Product `json:"items"`
	NextCursor *string   `json:"next_cursor"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(ctx context.Context, merchantID string, input Input) (Product, error) {
	if !ValidID(merchantID) {
		return Product{}, merchant.ErrNotFound
	}
	input, err := input.Normalize()
	if err != nil {
		return Product{}, err
	}
	result, err := scan(s.pool.QueryRow(ctx, `INSERT INTO public.products (merchant_id, sku, name, price_minor, currency)
		VALUES ($1, $2, $3, $4, $5) RETURNING id::text, sku, name, price_minor, currency, created_at`,
		merchantID, input.SKU, input.Name, input.PriceMinor, input.Currency))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return Product{}, ErrConflict
		case "23503":
			return Product{}, merchant.ErrNotFound
		}
	}
	return result, err
}

func (s *Store) Get(ctx context.Context, merchantID, id string) (Product, error) {
	if !ValidID(merchantID) || !ValidID(id) {
		return Product{}, ErrNotFound
	}
	result, err := scan(s.pool.QueryRow(ctx, `SELECT id::text, sku, name, price_minor, currency, created_at
		FROM public.products WHERE merchant_id = $1 AND id = $2`, merchantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return result, err
}

func (s *Store) List(ctx context.Context, merchantID, after string, limit int) (Page, error) {
	if !ValidID(merchantID) || limit < 1 || limit > 100 || (after != "" && !ValidID(after)) {
		return Page{}, ErrInvalid
	}
	var cursor any
	if after != "" {
		cursor = after
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, sku, name, price_minor, currency, created_at
		FROM public.products WHERE merchant_id = $1 AND ($2::uuid IS NULL OR id > $2::uuid)
		ORDER BY id LIMIT $3`, merchantID, cursor, limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Items: make([]Product, 0)}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = &page.Items[limit-1].ID
	}
	return page, nil
}

func scan(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.SKU, &p.Name, &p.PriceMinor, &p.Currency, &p.CreatedAt)
	return p, err
}
