package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
	"github.com/mateofu/movank-fullstack/backend/internal/sale"
)

type SaleRepository interface {
	Create(context.Context, string, string, sale.Input) (sale.Sale, error)
	Get(context.Context, string, string) (sale.Sale, error)
}

func registerSales(mux *http.ServeMux, tokens *auth.Authenticator, getMerchant func(context.Context, string) (merchant.Merchant, error), sales SaleRepository) {
	protect := func(next func(http.ResponseWriter, *http.Request, string)) http.Handler {
		return tokens.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.FromContext(r.Context())
			if !ok {
				writeJSON(w, 401, map[string]string{"error": "unauthorized"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if _, err := getMerchant(ctx, principal.MerchantID); err != nil {
				productError(w, err)
				return
			}
			if r.URL.RawQuery != "" {
				writeJSON(w, 400, map[string]string{"error": "invalid_query"})
				return
			}
			next(w, r.WithContext(ctx), principal.MerchantID)
		}))
	}
	mux.Handle("POST /v1/sales", protect(func(w http.ResponseWriter, r *http.Request, merchantID string) {
		if len(r.Header.Values("Idempotency-Key")) != 1 {
			saleError(w, sale.ErrInvalid)
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			writeJSON(w, 415, map[string]string{"error": "unsupported_media_type"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input sale.Input
		if err := decoder.Decode(&input); err != nil {
			decodeError(w, err)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			decodeError(w, err)
			return
		}
		result, err := sales.Create(r.Context(), merchantID, r.Header.Get("Idempotency-Key"), input)
		if err != nil {
			saleError(w, err)
			return
		}
		w.Header().Set("Location", "/v1/sales/"+result.ID)
		writeJSON(w, 201, result)
	}))
	mux.Handle("GET /v1/sales/{id}", protect(func(w http.ResponseWriter, r *http.Request, merchantID string) {
		if !product.ValidID(r.PathValue("id")) {
			saleError(w, sale.ErrInvalid)
			return
		}
		result, err := sales.Get(r.Context(), merchantID, r.PathValue("id"))
		if err != nil {
			saleError(w, err)
			return
		}
		writeJSON(w, 200, result)
	}))
}

func saleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sale.ErrInvalid):
		writeJSON(w, 400, map[string]string{"error": "invalid_sale"})
	case errors.Is(err, sale.ErrConflict):
		writeJSON(w, 409, map[string]string{"error": "idempotency_conflict"})
	case errors.Is(err, sale.ErrNotFound):
		writeJSON(w, 404, map[string]string{"error": "not_found"})
	default:
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
	}
}
