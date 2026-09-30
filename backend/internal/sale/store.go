package sale

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
)

var ErrInvalid = errors.New("invalid sale")
var ErrConflict = errors.New("idempotency conflict")
var ErrNotFound = errors.New("sale not found")
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

const maxAmount int64 = 1_000_000_000_000

type Line struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

type Input struct {
	Items []Line `json:"items"`
}

func (in Input) normalize() (Input, [32]byte, error) {
	if len(in.Items) < 1 || len(in.Items) > 100 {
		return Input{}, [32]byte{}, ErrInvalid
	}
	items := append([]Line(nil), in.Items...)
	for i := range items {
		items[i].ProductID = strings.ToLower(items[i].ProductID)
		if !product.ValidID(items[i].ProductID) || items[i].Quantity < 1 || items[i].Quantity > 10000 {
			return Input{}, [32]byte{}, ErrInvalid
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })
	for i := 1; i < len(items); i++ {
		if items[i].ProductID == items[i-1].ProductID {
			return Input{}, [32]byte{}, ErrInvalid
		}
	}
	normalized := Input{Items: items}
	data, err := json.Marshal(normalized)
	return normalized, sha256.Sum256(data), err
}

type Item struct {
	Line
	ProductName    string `json:"product_name"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
	SubtotalMinor  int64  `json:"subtotal_minor"`
}

type Sale struct {
	ID         string    `json:"id"`
	TotalMinor int64     `json:"total_minor"`
	Currency   string    `json:"currency"`
	CreatedAt  time.Time `json:"created_at"`
	Items      []Item    `json:"items"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, merchantID, key string, input Input) (Sale, error) {
	if !product.ValidID(merchantID) || !keyPattern.MatchString(key) {
		return Sale{}, ErrInvalid
	}
	input, hash, err := input.normalize()
	if err != nil {
		return Sale{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Sale{}, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, strings.ToLower(merchantID)+":"+key); err != nil {
		return Sale{}, err
	}
	var id string
	var storedHash []byte
	err = tx.QueryRow(ctx, `SELECT id::text, request_hash FROM public.sales WHERE merchant_id=$1 AND idempotency_key=$2`, merchantID, key).Scan(&id, &storedHash)
	if err == nil {
		if !bytes.Equal(storedHash, hash[:]) {
			return Sale{}, ErrConflict
		}
		return read(ctx, tx, merchantID, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, err
	}
	result := Sale{Currency: "COP", Items: make([]Item, 0, len(input.Items))}
	for _, line := range input.Items {
		item := Item{Line: line}
		err = tx.QueryRow(ctx, `SELECT name, price_minor FROM public.products WHERE merchant_id=$1 AND id=$2`, merchantID, line.ProductID).Scan(&item.ProductName, &item.UnitPriceMinor)
		if errors.Is(err, pgx.ErrNoRows) {
			return Sale{}, ErrInvalid
		}
		if err != nil {
			return Sale{}, err
		}
		item.SubtotalMinor = item.UnitPriceMinor * line.Quantity
		if item.SubtotalMinor > maxAmount-result.TotalMinor {
			return Sale{}, ErrInvalid
		}
		result.TotalMinor += item.SubtotalMinor
		result.Items = append(result.Items, item)
	}
	err = tx.QueryRow(ctx, `INSERT INTO public.sales (merchant_id, idempotency_key, request_hash, total_minor, currency)
  VALUES ($1,$2,$3,$4,'COP') RETURNING id::text, created_at`, merchantID, key, hash[:], result.TotalMinor).Scan(&result.ID, &result.CreatedAt)
	if err != nil {
		return Sale{}, err
	}
	for _, item := range result.Items {
		_, err = tx.Exec(ctx, `INSERT INTO public.sale_items (merchant_id,sale_id,product_id,product_name,quantity,unit_price_minor)
   VALUES ($1,$2,$3,$4,$5,$6)`, merchantID, result.ID, item.ProductID, item.ProductName, item.Quantity, item.UnitPriceMinor)
		if err != nil {
			return Sale{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Sale{}, err
	}
	return result, nil
}

func (s *Store) Get(ctx context.Context, merchantID, id string) (Sale, error) {
	if !product.ValidID(merchantID) || !product.ValidID(id) {
		return Sale{}, ErrNotFound
	}
	return read(ctx, s.pool, merchantID, id)
}

type reader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func read(ctx context.Context, db reader, merchantID, id string) (Sale, error) {
	result := Sale{Items: []Item{}}
	err := db.QueryRow(ctx, `SELECT id::text,total_minor,currency,created_at FROM public.sales WHERE merchant_id=$1 AND id=$2`, merchantID, id).Scan(&result.ID, &result.TotalMinor, &result.Currency, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, ErrNotFound
	}
	if err != nil {
		return Sale{}, err
	}
	rows, err := db.Query(ctx, `SELECT product_id::text,quantity,product_name,unit_price_minor,subtotal_minor
  FROM public.sale_items WHERE merchant_id=$1 AND sale_id=$2 ORDER BY product_id`, merchantID, id)
	if err != nil {
		return Sale{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.ProductName, &item.UnitPriceMinor, &item.SubtotalMinor); err != nil {
			return Sale{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
