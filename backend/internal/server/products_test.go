package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
)

type productStub struct {
	scope string
	err   error
}

func (s *productStub) Create(ctx context.Context, scope string, input product.Input) (product.Product, error) {
	s.scope = scope
	return product.Product{ID: "33333333-3333-4333-8333-333333333333", Input: input}, s.err
}

func (s *productStub) Get(ctx context.Context, scope, id string) (product.Product, error) {
	s.scope = scope
	return product.Product{ID: id}, s.err
}

func (s *productStub) List(ctx context.Context, scope, after string, limit int) (product.Page, error) {
	s.scope = scope
	return product.Page{Items: []product.Product{}}, s.err
}

func TestProductRequestValidation(t *testing.T) {
	const merchantID = "11111111-1111-4111-8111-111111111111"
	tokens, _ := auth.New(strings.Repeat("ab", 32))
	token, _ := tokens.Issue(merchantID, merchantID)
	for _, tc := range []struct {
		method string
		path   string
		body   string
		media  string
		status int
	}{
		{"POST", "/v1/products", `{`, "application/json", 400},
		{"POST", "/v1/products", `{} {}`, "application/json", 400},
		{"POST", "/v1/products", `{"merchant_id":"another"}`, "application/json", 400},
		{"POST", "/v1/products", `{"price_minor":1.25}`, "application/json", 400},
		{"POST", "/v1/products", `{"name":"` + strings.Repeat("a", 9000) + `"}`, "application/json", 413},
		{"POST", "/v1/products", `{}`, "text/plain", 415},
		{"POST", "/v1/products?merchant_id=other", `{}`, "application/json", 400},
		{"GET", "/v1/products?merchant_id=other", "", "", 400},
		{"GET", "/v1/products?limit=0", "", "", 400},
		{"GET", "/v1/products?limit=101", "", "", 400},
		{"GET", "/v1/products?limit=1&limit=2", "", "", 400},
		{"GET", "/v1/products?after=invalid", "", "", 400},
		{"GET", "/v1/products?after=", "", "", 400},
		{"GET", "/v1/products?after=%zz", "", "", 400},
		{"GET", "/v1/products/invalid", "", "", 400},
	} {
		stub := &productStub{}
		handler := Handler(func(context.Context) error { return nil }, tokens, func(context.Context, string) (merchant.Merchant, error) {
			return merchant.Merchant{ID: merchantID}, nil
		}, stub)
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", tc.media)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status || stub.scope != "" {
			t.Fatalf("%s %s: expected %d before repository, got %d", tc.method, tc.path, tc.status, w.Code)
		}
	}
}

func TestProductErrorsAndScope(t *testing.T) {
	const scope = "11111111-1111-4111-8111-111111111111"
	tokens, _ := auth.New(strings.Repeat("ab", 32))
	token, _ := tokens.Issue(scope, scope)
	for _, tc := range []struct {
		err    error
		status int
	}{{nil, 201}, {product.ErrConflict, 409}, {product.ErrInvalid, 400}, {merchant.ErrNotFound, 403}, {context.DeadlineExceeded, 503}} {
		stub := &productStub{err: tc.err}
		handler := Handler(func(context.Context) error { return nil }, tokens, func(context.Context, string) (merchant.Merchant, error) {
			return merchant.Merchant{ID: scope}, nil
		}, stub)
		r := httptest.NewRequest("POST", "/v1/products", strings.NewReader(`{"sku":"A","name":"Test","price_minor":100,"currency":"COP"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Merchant-ID", "another-merchant")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status || stub.scope != scope {
			t.Fatalf("unexpected response or scope: %d %s", w.Code, stub.scope)
		}
	}
}
