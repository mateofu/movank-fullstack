package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/dashboard"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
	"github.com/mateofu/movank-fullstack/backend/internal/payment"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
	"github.com/mateofu/movank-fullstack/backend/internal/sale"
	"github.com/mateofu/movank-fullstack/backend/internal/server"
	"github.com/mateofu/movank-fullstack/backend/internal/worker"
)

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("MOVANK_INTEGRATION") != "1" {
		t.Skip("requires PostgreSQL and Redis")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Host = "postgres"
	cfg.ConnConfig.Port = 5432
	cfg.ConnConfig.Database = "movank"
	cfg.ConnConfig.User = "movank"
	cfg.ConnConfig.Password = os.Getenv("POSTGRES_PASSWORD")
	cfg.ConnConfig.TLSConfig = nil
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("movank_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg = cfg.Copy()
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, err := admin.Exec(ctx, "DROP DATABASE "+name)
		admin.Close()
		if err != nil {
			t.Error(err)
		}
	})
	paths, err := filepath.Glob("../migrations/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatal("missing migrations")
	}
	for _, path := range paths {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return pool
}

type interruptedProvider struct {
	payment.Provider
	cancel context.CancelFunc
	before bool
}

func (p interruptedProvider) Charge(ctx context.Context, value payment.Payment) (string, error) {
	if p.before {
		p.cancel()
		return "UNKNOWN", ctx.Err()
	}
	status, err := p.Provider.Charge(ctx, value)
	p.cancel()
	return status, err
}

type unavailableProvider struct{ payment.Provider }

func (p unavailableProvider) Lookup(context.Context, payment.Payment) (string, error) {
	return "UNKNOWN", errors.New("offline")
}

func TestBackend(t *testing.T) {
	pool := database(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var a, b string
	if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name) VALUES('A') RETURNING id::text`).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name) VALUES('B') RETURNING id::text`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	products := product.NewStore(pool)
	item, err := products.Create(ctx, a, product.Input{SKU: "COFFEE", Name: "Coffee", PriceMinor: 125050, Currency: "COP"})
	if err != nil {
		t.Fatal(err)
	}
	sales := sale.NewStore(pool)
	newSale := func(key string) sale.Sale {
		t.Helper()
		value, err := sales.Create(ctx, a, key, sale.Input{Items: []sale.Line{{ProductID: item.ID, Quantity: 2}}})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	simulator := payment.NewSimulator(pool)
	payments := payment.New(pool, simulator)
	dash := dashboard.New(pool, os.Getenv("REDIS_ADDR"))
	defer dash.Close()
	hub := dashboard.NewHub()
	work := worker.New(pool, payments, dash, hub)
	input := payment.Input{Method: "CARD", Scenario: "APPROVED"}
	count := func(table, id string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM public."+table+" WHERE merchant_id=$1 AND sale_id=$2", a, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	due := func() { exec(`UPDATE public.payments SET next_check_at=now() WHERE merchant_id=$1`, a) }

	t.Run("concurrent payments and isolation", func(t *testing.T) {
		value := newSale("concurrent")
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p, err := payments.Pay(ctx, a, value.ID, input)
				if err != nil {
					t.Error(err)
				} else if p.Status != "APPROVED" && p.Status != "PENDING" {
					t.Errorf("state %s", p.Status)
				}
			}()
		}
		wg.Wait()
		p, err := payments.Pay(ctx, a, value.ID, input)
		if err != nil || p.Status != "APPROVED" {
			t.Fatalf("replay: %+v %v", p, err)
		}
		if count("provider_operations", value.ID) != 1 || count("outbox", value.ID) != 1 {
			t.Fatal("duplicate payment or outbox")
		}
		if _, err := payments.Pay(ctx, a, value.ID, payment.Input{Method: "CARD", Scenario: "DECLINED"}); !errors.Is(err, payment.ErrConflict) {
			t.Fatalf("scenario changed: %v", err)
		}
		if _, err := payments.Pay(ctx, b, value.ID, input); !errors.Is(err, payment.ErrNotFound) {
			t.Fatal("cross merchant payment")
		}
	})

	t.Run("declined remains declined", func(t *testing.T) {
		value := newSale("declined")
		declined := payment.Input{Method: "CARD", Scenario: "DECLINED"}
		p, err := payments.Pay(ctx, a, value.ID, declined)
		if err != nil || p.Status != "DECLINED" {
			t.Fatalf("%+v %v", p, err)
		}
		p, err = payments.Pay(ctx, a, value.ID, declined)
		if err != nil || p.Status != "DECLINED" || count("outbox", value.ID) != 0 || count("provider_operations", value.ID) != 1 {
			t.Fatal("declined replay failed")
		}
	})

	t.Run("unknown survives failed reconciliation", func(t *testing.T) {
		value := newSale("unknown")
		timeout := payment.Input{Method: "CARD", Scenario: "TIMEOUT"}
		p, err := payments.Pay(ctx, a, value.ID, timeout)
		if err != nil || p.Status != "UNKNOWN" {
			t.Fatalf("timeout: %+v %v", p, err)
		}
		exec(`UPDATE public.provider_operations SET visible_at=now()+interval '1 day' WHERE reference=$1`, p.ID)
		replay, err := payments.Pay(ctx, a, value.ID, timeout)
		if err != nil || replay.ID != p.ID || replay.Status != "UNKNOWN" {
			t.Fatal("timeout reinterpreted")
		}
		due()
		failed := payment.New(pool, unavailableProvider{simulator})
		if err := failed.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		current, err := payments.Current(ctx, a, value.ID)
		if err != nil || current.Status != "UNKNOWN" || current.LastError == nil {
			t.Fatal("failed lookup was not auditable")
		}
		due()
		if err := payments.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		current, _ = payments.Current(ctx, a, value.ID)
		if current.Status != "UNKNOWN" {
			t.Fatal("unresolved lookup changed status")
		}
		exec(`UPDATE public.provider_operations SET visible_at=now() WHERE reference=$1`, p.ID)
		due()
		if err := payments.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		current, _ = payments.Current(ctx, a, value.ID)
		if current.Status != "APPROVED" || current.ID != p.ID || count("provider_operations", value.ID) != 1 || count("payment_checks", value.ID) != 4 {
			t.Fatal("reconciliation duplicated or lost audit")
		}
	})

	t.Run("outbox failure rolls back approval", func(t *testing.T) {
		value := newSale("rollback")
		exec(`CREATE FUNCTION reject_outbox() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''injected''; END'; CREATE TRIGGER reject_outbox BEFORE INSERT ON public.outbox FOR EACH ROW EXECUTE FUNCTION reject_outbox()`)
		_, err := payments.Pay(ctx, a, value.ID, input)
		if err == nil {
			t.Fatal("expected failure")
		}
		current, err := payments.Current(ctx, a, value.ID)
		if err != nil || current.Status != "PENDING" || count("outbox", value.ID) != 0 || count("provider_operations", value.ID) != 1 {
			t.Fatal("partial approval")
		}
		exec(`DROP TRIGGER reject_outbox ON public.outbox; DROP FUNCTION reject_outbox()`)
		due()
		if err := payments.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		current, _ = payments.Current(ctx, a, value.ID)
		if current.Status != "APPROVED" || count("provider_operations", value.ID) != 1 || count("outbox", value.ID) != 1 {
			t.Fatal("recovery failed")
		}
	})

	for _, before := range []bool{true, false} {
		t.Run(fmt.Sprintf("cancellation before provider %t", before), func(t *testing.T) {
			value := newSale(fmt.Sprintf("cancel-%t", before))
			callCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			service := payment.New(pool, interruptedProvider{simulator, cancel, before})
			if _, err := service.Pay(callCtx, a, value.ID, input); err == nil {
				t.Fatal("expected cancellation")
			}
			current, err := payments.Current(ctx, a, value.ID)
			if err != nil || current.Status != "PENDING" {
				t.Fatal("intent was lost")
			}
			due()
			if err := payments.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			current, _ = payments.Current(ctx, a, value.ID)
			if current.Status != "APPROVED" || count("provider_operations", value.ID) != 1 {
				t.Fatal("recovery duplicated payment")
			}
		})
	}

	t.Run("outbox replay cache rebuild and fanout", func(t *testing.T) {
		events, stop, _ := hub.Subscribe(a)
		defer stop()
		other, stopB, _ := hub.Subscribe(b)
		defer stopB()
		if err := work.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		snapshot := <-events
		if snapshot.PaidSales != 5 || snapshot.TotalMinor != 5*250100 {
			t.Fatalf("aggregate: %+v", snapshot)
		}
		select {
		case <-other:
			t.Fatal("cross merchant broadcast")
		default:
		}
		exec(`UPDATE public.outbox SET published_at=NULL WHERE merchant_id=$1`, a)
		if err := work.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		if again := <-events; again != snapshot {
			t.Fatal("replay double counted")
		}
		fallback := dashboard.New(pool, "127.0.0.1:1")
		defer fallback.Close()
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := fallback.Today(ctx, a, false)
				if err != nil || got != snapshot {
					t.Errorf("fallback: %+v %v", got, err)
				}
			}()
		}
		wg.Wait()
		got, err := dash.Today(ctx, b, false)
		if err != nil || got.PaidSales != 0 {
			t.Fatal("cross merchant cache")
		}
	})

	t.Run("http sse and worker recovery", func(t *testing.T) {
		tokens, _ := auth.New(strings.Repeat("ab", 32))
		tokenA, _ := tokens.Issue(a, a)
		tokenB, _ := tokens.Issue(b, b)
		runCtx, stop := context.WithCancel(ctx)
		handler := server.Handler(pool.Ping, tokens, merchant.NewStore(pool).Get, products, sales, server.Features{Payments: payments, Dashboard: dash, Hub: hub, Shutdown: runCtx})
		httpServer := httptest.NewServer(handler)
		defer httpServer.Close()
		defer stop()
		stream := func(token string) (<-chan dashboard.Snapshot, io.Closer) {
			t.Helper()
			req, _ := http.NewRequest("GET", httpServer.URL+"/v1/dashboard/stream", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 {
				response.Body.Close()
				t.Fatalf("stream %d", response.StatusCode)
			}
			values := make(chan dashboard.Snapshot, 10)
			go func() {
				defer close(values)
				scanner := bufio.NewScanner(response.Body)
				for scanner.Scan() {
					if strings.HasPrefix(scanner.Text(), "data: ") {
						var s dashboard.Snapshot
						if json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &s) == nil {
							values <- s
						}
					}
				}
			}()
			return values, response.Body
		}
		receive := func(values <-chan dashboard.Snapshot) dashboard.Snapshot {
			t.Helper()
			select {
			case s, ok := <-values:
				if !ok {
					t.Fatal("closed stream")
				}
				return s
			case <-time.After(8 * time.Second):
				t.Fatal("missing SSE")
				return dashboard.Snapshot{}
			}
		}
		events, body := stream(tokenA)
		defer body.Close()
		other, otherBody := stream(tokenB)
		defer otherBody.Close()
		initial := receive(events)
		if initial.PaidSales != 5 {
			t.Fatal("initial SSE")
		}
		if receive(other).PaidSales != 0 {
			t.Fatal("other initial SSE")
		}
		value := newSale("sse")
		req, _ := http.NewRequest("POST", httpServer.URL+"/v1/sales/"+value.ID+"/pay", strings.NewReader(`{"method":"CARD","scenario":"APPROVED"}`))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("payment HTTP %d", response.StatusCode)
		}
		finished := make(chan struct{})
		go func() { defer close(finished); work.Run(runCtx, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
		if s := receive(events); s.PaidSales != 6 {
			t.Fatalf("worker snapshot %+v", s)
		}
		select {
		case <-other:
			t.Fatal("other commerce received payment")
		case <-time.After(100 * time.Millisecond):
		}
		req, _ = http.NewRequest("GET", httpServer.URL+"/v1/sales/"+value.ID, nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		response, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatal("cross merchant HTTP")
		}
		stop()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("worker shutdown hung")
		}
	})
}
