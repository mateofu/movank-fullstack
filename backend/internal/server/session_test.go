package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

func TestBrowserSession(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	tokens, _ := auth.New(strings.Repeat("ab", 32), "https://shop.example")
	token, _ := tokens.Issue(id, id)
	handler := Handler(nil, tokens, func(context.Context, string) (merchant.Merchant, error) {
		return merchant.Merchant{ID: id, Name: "Shop"}, nil
	}, nil, nil)
	login := httptest.NewRequest("POST", "https://shop.example/v1/session", strings.NewReader(`{"token":"`+token+`"}`))
	login.Header.Set("Origin", "https://shop.example")
	login.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	cookies := response.Result().Cookies()
	if response.Code != 200 || len(cookies) != 1 {
		t.Fatalf("login: %d %s", response.Code, response.Body.String())
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/v1" || cookie.MaxAge <= 0 {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	if strings.Contains(response.Body.String(), token) {
		t.Fatal("token in JSON response")
	}
	for _, tc := range []struct {
		method, path, origin, workspace string
		status                          int
	}{
		{"GET", "/v1/me", "", "", 200},
		{"GET", "/v1/me", "", id + ":" + id, 200},
		{"GET", "/v1/me", "", "another", 409},
		{"POST", "/v1/products", "https://evil.example", "", 403},
		{"POST", "/v1/products", "", "", 403},
		{"DELETE", "/v1/session", "https://evil.example", "", 403},
		{"DELETE", "/v1/session", "https://shop.example", "", 204},
	} {
		r := httptest.NewRequest(tc.method, "https://shop.example"+tc.path, nil)
		r.AddCookie(cookie)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Workspace", tc.workspace)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if tc.status == 204 && w.Result().Cookies()[0].MaxAge != -1 {
			t.Fatal("logout did not clear cookie")
		}
	}
	bad := httptest.NewRequest("POST", "https://shop.example/v1/session", strings.NewReader(`{"token":"`+token+`"}`))
	bad.Header.Set("Origin", "https://evil.example")
	bad.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, bad)
	if response.Code != 403 || len(response.Result().Cookies()) != 0 {
		t.Fatal("cross origin login accepted")
	}
}
