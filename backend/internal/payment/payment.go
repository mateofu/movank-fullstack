package payment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
)

var ErrInvalid = errors.New("invalid payment")
var ErrConflict = errors.New("payment already has another method or scenario")
var ErrNotFound = errors.New("sale not found")

type Input struct {
	Method   string `json:"method"`
	Scenario string `json:"scenario"`
}

type Payment struct {
	ID         string `json:"id"`
	SaleID     string `json:"sale_id"`
	MerchantID string `json:"-"`
	Input
	Status      string     `json:"status"`
	AmountMinor int64      `json:"amount_minor"`
	Attempts    int        `json:"checks"`
	LastError   *string    `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type Provider interface {
	Charge(context.Context, Payment) (string, error)
	Lookup(context.Context, Payment) (string, error)
}

type Service struct {
	pool     *pgxpool.Pool
	provider Provider
}

func New(pool *pgxpool.Pool, provider Provider) *Service {
	return &Service{pool: pool, provider: provider}
}

const columns = `id::text,sale_id::text,merchant_id::text,method,scenario,status,amount_minor,attempts,last_error,created_at,updated_at,completed_at`

func scan(row pgx.Row) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.SaleID, &p.MerchantID, &p.Method, &p.Scenario, &p.Status, &p.AmountMinor, &p.Attempts, &p.LastError, &p.CreatedAt, &p.UpdatedAt, &p.CompletedAt)
	return p, err
}

func (s *Service) Current(ctx context.Context, merchantID, saleID string) (*Payment, error) {
	if !product.ValidID(merchantID) || !product.ValidID(saleID) {
		return nil, ErrInvalid
	}
	p, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM public.payments WHERE merchant_id=$1 AND sale_id=$2`, merchantID, saleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) Pay(ctx context.Context, merchantID, saleID string, input Input) (Payment, error) {
	if !product.ValidID(merchantID) || !product.ValidID(saleID) || input.Method != "CARD" || (input.Scenario != "APPROVED" && input.Scenario != "DECLINED" && input.Scenario != "TIMEOUT") {
		return Payment{}, ErrInvalid
	}
	p, created, err := s.prepare(ctx, merchantID, saleID, input)
	if err != nil || !created {
		return p, err
	}
	return s.resolve(ctx, p)
}

func (s *Service) prepare(ctx context.Context, merchantID, saleID string, input Input) (Payment, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Payment{}, false, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('pay:' || $1::uuid::text || ':' || $2::uuid::text,0))`, merchantID, saleID); err != nil {
		return Payment{}, false, err
	}
	var amount int64
	err = tx.QueryRow(ctx, `SELECT total_minor FROM public.sales WHERE merchant_id=$1 AND id=$2`, merchantID, saleID).Scan(&amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, ErrNotFound
	}
	if err != nil {
		return Payment{}, false, err
	}
	p, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM public.payments WHERE merchant_id=$1 AND sale_id=$2`, merchantID, saleID))
	if err == nil {
		if p.Input != input {
			return Payment{}, false, ErrConflict
		}
		return p, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, err
	}
	p, err = scan(tx.QueryRow(ctx, `INSERT INTO public.payments(merchant_id,sale_id,method,scenario,amount_minor) VALUES($1,$2,$3,$4,$5) RETURNING `+columns, merchantID, saleID, input.Method, input.Scenario, amount))
	if err != nil {
		return Payment{}, false, err
	}
	return p, true, tx.Commit(ctx)
}

func (s *Service) resolve(ctx context.Context, p Payment) (Payment, error) {
	source := "lookup"
	var status string
	var err error
	if p.Status == "PENDING" {
		source = "charge"
		status, err = s.provider.Charge(ctx, p)
	} else {
		status, err = s.provider.Lookup(ctx, p)
	}
	var failure *string
	if err != nil {
		message := "provider_unavailable"
		failure = &message
		status = "UNKNOWN"
	}
	if status != "APPROVED" && status != "DECLINED" {
		status = "UNKNOWN"
	}
	return s.finish(ctx, p, status, source, failure)
}

func (s *Service) finish(ctx context.Context, p Payment, status, source string, failure *string) (Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Payment{}, err
	}
	defer tx.Rollback(context.Background())
	current, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM public.payments WHERE merchant_id=$1 AND sale_id=$2 FOR UPDATE`, p.MerchantID, p.SaleID))
	if err != nil {
		return Payment{}, err
	}
	if current.Status == "APPROVED" || current.Status == "DECLINED" {
		return current, nil
	}
	current, err = scan(tx.QueryRow(ctx, `UPDATE public.payments SET status=$3, attempts=attempts+1,last_error=$4,updated_at=clock_timestamp(),
  completed_at=CASE WHEN $3 IN ('APPROVED','DECLINED') THEN clock_timestamp() END,
  next_check_at=clock_timestamp()+make_interval(secs=>LEAST(60,5*power(2,LEAST(attempts,4)))::double precision)
  WHERE merchant_id=$1 AND sale_id=$2 RETURNING `+columns, p.MerchantID, p.SaleID, status, failure))
	if err != nil {
		return Payment{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.payment_checks(merchant_id,sale_id,status,source,error) VALUES($1,$2,$3,$4,$5)`, p.MerchantID, p.SaleID, status, source, failure); err != nil {
		return Payment{}, err
	}
	if status == "APPROVED" {
		if _, err = tx.Exec(ctx, `INSERT INTO public.outbox(merchant_id,sale_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.MerchantID, p.SaleID); err != nil {
			return Payment{}, err
		}
		if _, err = tx.Exec(ctx, `SELECT pg_notify('movank_outbox',$1)`, p.MerchantID); err != nil {
			return Payment{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Payment{}, err
	}
	return current, nil
}

func (s *Service) Reconcile(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM public.payments WHERE status IN ('PENDING','UNKNOWN') AND next_check_at<=now() ORDER BY next_check_at LIMIT 20`)
	if err != nil {
		return err
	}
	var pending []Payment
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range pending {
		if _, err = s.resolve(ctx, p); err != nil {
			return err
		}
	}
	return nil
}
