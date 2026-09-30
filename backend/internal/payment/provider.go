package payment

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Simulator struct{ pool *pgxpool.Pool }

func NewSimulator(pool *pgxpool.Pool) *Simulator { return &Simulator{pool: pool} }

func (s *Simulator) Charge(ctx context.Context, p Payment) (string, error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO public.provider_operations(reference,merchant_id,sale_id,amount_minor,scenario,outcome,visible_at)
  VALUES($1,$2,$3,$4,$5,CASE WHEN $5='DECLINED' THEN 'DECLINED' ELSE 'APPROVED' END,
  now()+CASE WHEN $5='TIMEOUT' THEN interval '3 seconds' ELSE interval '0 seconds' END) ON CONFLICT(reference) DO NOTHING`, p.ID, p.MerchantID, p.SaleID, p.AmountMinor, p.Scenario)
	if err != nil {
		return "UNKNOWN", err
	}
	var matches bool
	err = s.pool.QueryRow(ctx, `SELECT merchant_id=$2 AND sale_id=$3 AND amount_minor=$4 AND scenario=$5 FROM public.provider_operations WHERE reference=$1`, p.ID, p.MerchantID, p.SaleID, p.AmountMinor, p.Scenario).Scan(&matches)
	if err != nil {
		return "UNKNOWN", err
	}
	if !matches {
		return "UNKNOWN", ErrConflict
	}
	if p.Scenario == "TIMEOUT" {
		return "UNKNOWN", nil
	}
	return s.Lookup(ctx, p)
}

func (s *Simulator) Lookup(ctx context.Context, p Payment) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT CASE WHEN visible_at<=now() THEN outcome ELSE 'UNKNOWN' END FROM public.provider_operations
  WHERE reference=$1 AND merchant_id=$2 AND sale_id=$3 AND amount_minor=$4`, p.ID, p.MerchantID, p.SaleID, p.AmountMinor).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "UNKNOWN", nil
	}
	return status, err
}
