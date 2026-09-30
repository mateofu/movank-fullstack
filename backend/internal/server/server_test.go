package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
)

func testHandler(check func(context.Context) error) http.Handler {
	tokens, _ := auth.New(strings.Repeat("ab", 32))
	return Handler(check, tokens, func(context.Context, string) (merchant.Merchant, error) {
		return merchant.Merchant{}, merchant.ErrNotFound
	}, nil, nil)
}

func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/healthz", 200},
		{"POST", "/healthz", 405},
		{"GET", "/v1/sales", 405},
	} {
		rr := httptest.NewRecorder()
		testHandler(func(context.Context) error { return nil }).ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
		if rr.Code != tc.status {
			t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, rr.Code, tc.status)
		}
		if tc.status == 200 && rr.Body.String() != "{\"status\":\"ok\"}\n" {
			t.Fatalf("unexpected health response: %s", rr.Body.String())
		}
	}
}

func TestReadiness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		check  func(context.Context) error
		status int
		body   string
	}{
		{"available", func(context.Context) error { return nil }, 200, "{\"status\":\"ok\"}\n"},
		{"unavailable", func(context.Context) error { return errors.New("private connection details") }, 503, "{\"status\":\"unavailable\"}\n"},
		{"timeout", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 503, "{\"status\":\"unavailable\"}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			testHandler(tc.check).ServeHTTP(rr, httptest.NewRequest("GET", "/readyz", nil))
			if rr.Code != tc.status || rr.Body.String() != tc.body {
				t.Fatalf("unexpected readiness response: %d %s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("readiness must not be cached")
			}
		})
	}
}

func TestLivenessDoesNotQueryDatabase(t *testing.T) {
	handler := testHandler(func(context.Context) error {
		t.Fatal("liveness called database")
		return nil
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected liveness status: %d", rr.Code)
	}
}

func TestShutdownDrainsActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, listener, handler, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				err = io.ErrUnexpectedEOF
			}
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter handler")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("server exited before completing request: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-response:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not complete")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
}
