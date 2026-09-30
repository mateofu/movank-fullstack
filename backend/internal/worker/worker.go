package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateofu/movank-fullstack/backend/internal/dashboard"
	"github.com/mateofu/movank-fullstack/backend/internal/payment"
)

type Worker struct {
	pool      *pgxpool.Pool
	payments  *payment.Service
	dashboard *dashboard.Service
	hub       *dashboard.Hub
}

func New(pool *pgxpool.Pool, payments *payment.Service, dash *dashboard.Service, hub *dashboard.Hub) *Worker {
	return &Worker{pool, payments, dash, hub}
}

func (w *Worker) Drain(ctx context.Context) error {
	for i := 0; i < 100; i++ {
		found, err := w.publishOne(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
	}
	return nil
}

func (w *Worker) publishOne(ctx context.Context) (bool, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	var id int64
	var merchantID string
	err = tx.QueryRow(ctx, `SELECT id,merchant_id::text FROM public.outbox WHERE published_at IS NULL ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &merchantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	snapshot, err := w.dashboard.Today(ctx, merchantID, true)
	if err != nil {
		return false, err
	}
	w.hub.Publish(merchantID, snapshot)
	if _, err = tx.Exec(ctx, `UPDATE public.outbox SET published_at=clock_timestamp() WHERE id=$1 AND merchant_id=$2 AND published_at IS NULL`, id, merchantID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (w *Worker) Run(ctx context.Context, logger *slog.Logger) {
	var conn *pgx.Conn
	defer func() {
		if conn != nil {
			_ = conn.Close(context.Background())
		}
	}()
	lastDay := time.Now().UTC().Format("2006-01-02")
	for ctx.Err() == nil {
		if conn == nil {
			connectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			var err error
			conn, err = pgx.ConnectConfig(connectCtx, w.pool.Config().ConnConfig.Copy())
			if err == nil {
				_, err = conn.Exec(connectCtx, "LISTEN movank_outbox")
			}
			cancel()
			if err != nil {
				if conn != nil {
					_ = conn.Close(context.Background())
					conn = nil
				}
				if !pause(ctx) {
					return
				}
				continue
			}
		}
		workCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := w.payments.Reconcile(workCtx); err != nil && ctx.Err() == nil {
			logger.Warn("payment reconciliation will retry")
		}
		if err := w.Drain(workCtx); err != nil && ctx.Err() == nil {
			logger.Warn("outbox will retry")
		}
		day := time.Now().UTC().Format("2006-01-02")
		if day != lastDay {
			succeeded := true
			for _, merchantID := range w.hub.Merchants() {
				snapshot, err := w.dashboard.Today(workCtx, merchantID, true)
				if err == nil {
					w.hub.Publish(merchantID, snapshot)
				} else {
					succeeded = false
				}
			}
			if succeeded {
				lastDay = day
			}
		}
		cancel()
		waitCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		_, err := conn.WaitForNotification(waitCtx)
		stop()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			_ = conn.Close(context.Background())
			conn = nil
		}
	}
}

func pause(ctx context.Context) bool {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
