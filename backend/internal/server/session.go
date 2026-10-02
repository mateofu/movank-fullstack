package server

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

func registerSession(mux *http.ServeMux, tokens *auth.Authenticator, getMerchant func(context.Context, string) (merchant.Merchant, error)) {
	mux.HandleFunc("POST /v1/session", func(w http.ResponseWriter, r *http.Request) {
		if !tokens.SameOrigin(r) {
			writeJSON(w, 403, map[string]string{"error": "invalid_origin"})
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			writeJSON(w, 415, map[string]string{"error": "unsupported_media_type"})
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		var input struct {
			Token string `json:"token"`
		}
		if err := decoder.Decode(&input); err != nil {
			decodeError(w, err)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			decodeError(w, err)
			return
		}
		principal, err := tokens.Verify(input.Token)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		commerce, err := getMerchant(ctx, principal.MerchantID)
		if err != nil {
			productError(w, err)
			return
		}
		tokens.SetSession(w, r, input.Token, principal.ExpiresAt)
		writeJSON(w, 200, struct {
			UserID   string            `json:"user_id"`
			Merchant merchant.Merchant `json:"merchant"`
		}{principal.UserID, commerce})
	})
	mux.HandleFunc("DELETE /v1/session", func(w http.ResponseWriter, r *http.Request) {
		if !tokens.SameOrigin(r) {
			writeJSON(w, 403, map[string]string{"error": "invalid_origin"})
			return
		}
		tokens.SetSession(w, r, "", time.Unix(0, 0))
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(204)
	})
}
