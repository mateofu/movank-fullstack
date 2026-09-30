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
	"github.com/mateofu/movank-fullstack/backend/internal/dashboard"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
	"github.com/mateofu/movank-fullstack/backend/internal/payment"
)

type Features struct {
	Payments  *payment.Service
	Dashboard *dashboard.Service
	Hub       *dashboard.Hub
	Shutdown  context.Context
}

func registerPayments(mux *http.ServeMux, tokens *auth.Authenticator, getMerchant func(context.Context, string) (merchant.Merchant, error), features Features) {
	mux.Handle("POST /v1/sales/{id}/pay", tokens.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := auth.FromContext(r.Context())
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, err := getMerchant(ctx, principal.MerchantID); err != nil {
			productError(w, err)
			return
		}
		if r.URL.RawQuery != "" {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			writeJSON(w, 415, map[string]string{"error": "unsupported_media_type"})
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		var input payment.Input
		if err := decoder.Decode(&input); err != nil {
			decodeError(w, err)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			decodeError(w, err)
			return
		}
		result, err := features.Payments.Pay(ctx, principal.MerchantID, r.PathValue("id"), input)
		if err != nil {
			status, code := 503, "unavailable"
			switch {
			case errors.Is(err, payment.ErrInvalid):
				status, code = 400, "invalid_payment"
			case errors.Is(err, payment.ErrConflict):
				status, code = 409, "payment_conflict"
			case errors.Is(err, payment.ErrNotFound):
				status, code = 404, "not_found"
			}
			writeJSON(w, status, map[string]string{"error": code})
			return
		}
		code := 200
		if result.Status == "UNKNOWN" || result.Status == "PENDING" {
			code = 202
		}
		writeJSON(w, code, result)
	})))
}
