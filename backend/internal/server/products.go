package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
)

type ProductRepository interface {
	Create(context.Context, string, product.Input) (product.Product, error)
	Get(context.Context, string, string) (product.Product, error)
	List(context.Context, string, string, int) (product.Page, error)
}

func registerProducts(mux *http.ServeMux, tokens *auth.Authenticator, getMerchant func(context.Context, string) (merchant.Merchant, error), products ProductRepository) {
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
			next(w, r.WithContext(ctx), principal.MerchantID)
		}))
	}
	mux.Handle("POST /v1/products", protect(func(w http.ResponseWriter, r *http.Request, merchantID string) {
		if r.URL.RawQuery != "" {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeJSON(w, 415, map[string]string{"error": "unsupported_media_type"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input product.Input
		if err := decoder.Decode(&input); err != nil {
			decodeError(w, err)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			decodeError(w, err)
			return
		}
		item, err := products.Create(r.Context(), merchantID, input)
		if err != nil {
			productError(w, err)
			return
		}
		w.Header().Set("Location", "/v1/products/"+item.ID)
		writeJSON(w, 201, item)
	}))
	mux.Handle("GET /v1/products", protect(func(w http.ResponseWriter, r *http.Request, merchantID string) {
		query, parseErr := url.ParseQuery(r.URL.RawQuery)
		if parseErr != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
		for key, values := range query {
			if (key != "limit" && key != "after") || len(values) != 1 {
				writeJSON(w, 400, map[string]string{"error": "invalid_query"})
				return
			}
		}
		limit := 50
		var err error
		if query.Has("limit") {
			limit, err = strconv.Atoi(query.Get("limit"))
		}
		after := query.Get("after")
		if err != nil || limit < 1 || limit > 100 || (query.Has("after") && !product.ValidID(after)) {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
		page, err := products.List(r.Context(), merchantID, after, limit)
		if err != nil {
			productError(w, err)
			return
		}
		writeJSON(w, 200, page)
	}))
	mux.Handle("GET /v1/products/{id}", protect(func(w http.ResponseWriter, r *http.Request, merchantID string) {
		if !product.ValidID(r.PathValue("id")) || r.URL.RawQuery != "" {
			writeJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		item, err := products.Get(r.Context(), merchantID, r.PathValue("id"))
		if err != nil {
			productError(w, err)
			return
		}
		writeJSON(w, 200, item)
	}))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeJSON(w, 413, map[string]string{"error": "body_too_large"})
		return
	}
	writeJSON(w, 400, map[string]string{"error": "invalid_json"})
}

func productError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, product.ErrInvalid):
		writeJSON(w, 400, map[string]string{"error": "invalid_product"})
	case errors.Is(err, product.ErrConflict):
		writeJSON(w, 409, map[string]string{"error": "sku_conflict"})
	case errors.Is(err, product.ErrNotFound):
		writeJSON(w, 404, map[string]string{"error": "not_found"})
	case errors.Is(err, merchant.ErrNotFound):
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
	default:
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
	}
}
