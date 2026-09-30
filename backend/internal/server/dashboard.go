package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/dashboard"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

func registerDashboard(mux *http.ServeMux, tokens *auth.Authenticator, getMerchant func(context.Context, string) (merchant.Merchant, error), features Features) {
	protect := func(next http.HandlerFunc) http.Handler {
		return tokens.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, _ := auth.FromContext(r.Context())
			if r.URL.RawQuery != "" {
				writeJSON(w, 400, map[string]string{"error": "invalid_query"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			_, err := getMerchant(ctx, principal.MerchantID)
			cancel()
			if err != nil {
				productError(w, err)
				return
			}
			next(w, r)
		}))
	}
	mux.Handle("GET /v1/dashboard/today", protect(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := auth.FromContext(r.Context())
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		result, err := features.Dashboard.Today(ctx, principal.MerchantID, false)
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		writeJSON(w, 200, result)
	}))
	mux.Handle("GET /v1/dashboard/stream", protect(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := auth.FromContext(r.Context())
		events, unsubscribe, ok := features.Hub.Subscribe(principal.MerchantID)
		if !ok {
			writeJSON(w, 429, map[string]string{"error": "too_many_streams"})
			return
		}
		defer unsubscribe()
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		initial, err := features.Dashboard.Today(ctx, principal.MerchantID, false)
		cancel()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		control := http.NewResponseController(w)
		send := func(snapshot dashboard.Snapshot) error {
			if err := control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return err
			}
			data, _ := json.Marshal(snapshot)
			if _, err := fmt.Fprintf(w, "id: %s:%d\nevent: dashboard\ndata: %s\n\n", snapshot.Date, snapshot.PaidSales, data); err != nil {
				return err
			}
			return control.Flush()
		}
		if err := send(initial); err != nil {
			return
		}
		last := initial
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		expiry := time.NewTimer(time.Until(principal.ExpiresAt))
		defer expiry.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-features.Shutdown.Done():
				return
			case <-expiry.C:
				return
			case snapshot := <-events:
				if snapshot.Date < last.Date || (snapshot.Date == last.Date && snapshot.PaidSales <= last.PaidSales) {
					continue
				}
				if err := send(snapshot); err != nil {
					return
				}
				last = snapshot
			case <-heartbeat.C:
				if err := control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
					return
				}
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				if err := control.Flush(); err != nil {
					return
				}
			}
		}
	}))
}
