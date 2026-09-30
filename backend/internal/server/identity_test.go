package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

func TestIdentityUsesOnlyVerifiedMerchant(t *testing.T) {
	const merchantID = "11111111-1111-4111-8111-111111111111"
	const otherID = "22222222-2222-4222-8222-222222222222"
	const userID = "33333333-3333-4333-8333-333333333333"
	tokens, _ := auth.New(strings.Repeat("ab", 32))
	raw, _ := tokens.Issue(merchantID, userID)
	for _, tc := range []struct {
		name   string
		token  string
		err    error
		status int
	}{
		{"valid", raw, nil, 200},
		{"missing token", "", nil, 401},
		{"invalid token", "invalid", nil, 401},
		{"unknown merchant", raw, merchant.ErrNotFound, 403},
		{"database failure", raw, errors.New("private database details"), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := Handler(func(context.Context) error { return nil }, tokens, func(ctx context.Context, id string) (merchant.Merchant, error) {
				called = true
				if id != merchantID {
					t.Fatal("repository received untrusted merchant")
				}
				return merchant.Merchant{ID: id, Name: "Commerce A"}, tc.err
			})
			r := httptest.NewRequest("GET", "/v1/me?merchant_id="+otherID, strings.NewReader(`{"merchant_id":"`+otherID+`"}`))
			r.Header.Set("X-Merchant-ID", otherID)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
			if tc.status == 401 && called {
				t.Fatal("unauthorized request queried repository")
			}
			if tc.status == 200 {
				var body struct {
					UserID   string            `json:"user_id"`
					Merchant merchant.Merchant `json:"merchant"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Merchant.ID != merchantID || body.UserID != userID {
					t.Fatal("identity response does not match token")
				}
			}
		})
	}
}

func TestConcurrentIdentitiesRemainSeparate(t *testing.T) {
	tokens, _ := auth.New(strings.Repeat("ab", 32))
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	handler := Handler(func(context.Context) error { return nil }, tokens, func(ctx context.Context, id string) (merchant.Merchant, error) {
		return merchant.Merchant{ID: id, Name: id}, nil
	})
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		id := ids[i%2]
		raw, err := tokens.Issue(id, id)
		if err != nil {
			t.Fatal(err)
		}
		workers.Go(func() {
			r := httptest.NewRequest("GET", "/v1/me", nil)
			r.Header.Set("Authorization", "Bearer "+raw)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			var body struct {
				UserID   string            `json:"user_id"`
				Merchant merchant.Merchant `json:"merchant"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || body.UserID != id || body.Merchant.ID != id {
				t.Error("concurrent identity was mixed with another request")
			}
		})
	}
	workers.Wait()
}
