package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

func TestSalesRejectInvalidHTTPBeforeRepository(t *testing.T) {
	const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tokens, _ := auth.New(strings.Repeat("ab", 32))
	token, _ := tokens.Issue(id, id)
	handler := Handler(nil, tokens, func(context.Context, string) (merchant.Merchant, error) {
		return merchant.Merchant{ID: id}, nil
	}, nil, nil)
	for _, tc := range []struct {
		body, media, key, token string
		status                  int
	}{
		{`{}`, "application/json", "key", "", 401},
		{`{}`, "application/json", "", token, 400},
		{`{}`, "text/plain", "key", token, 415},
		{`{`, "application/json", "key", token, 400},
		{`{} {}`, "application/json", "key", token, 400},
		{`{"merchant_id":"` + id + `","items":[]}`, "application/json", "key", token, 400},
		{`{"items":[{"product_id":"` + id + `","quantity":1.5}]}`, "application/json", "key", token, 400},
		{strings.Repeat(" ", 17000) + `{}`, "application/json", "key", token, 413},
	} {
		r := httptest.NewRequest("POST", "/v1/sales", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.media)
		if tc.key != "" {
			r.Header.Set("Idempotency-Key", tc.key)
		}
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
}
