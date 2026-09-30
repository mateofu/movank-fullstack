package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mateofu/movank-fullstack/backend/internal/auth"
	"github.com/mateofu/movank-fullstack/backend/internal/config"
	"github.com/mateofu/movank-fullstack/backend/internal/database"
	"github.com/mateofu/movank-fullstack/backend/internal/merchant"
	"github.com/mateofu/movank-fullstack/backend/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("process failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:" + cfg.HTTPPort + "/healthz")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("healthcheck status: %d", resp.StatusCode)
		}
		return nil
	}
	if len(os.Args) > 1 && (len(os.Args) != 4 || os.Args[1] != "token") {
		return fmt.Errorf("usage: api [healthcheck | token MERCHANT_UUID USER_UUID]")
	}
	tokens, err := auth.New(os.Getenv("AUTH_SIGNING_KEY"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	merchants := merchant.NewStore(pool)
	if len(os.Args) == 4 {
		lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if _, err := merchants.Get(lookupCtx, os.Args[2]); err != nil {
			return fmt.Errorf("merchant is not available")
		}
		token, err := tokens.Issue(os.Args[2], os.Args[3])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, token)
		return err
	}
	listener, err := net.Listen("tcp", ":"+cfg.HTTPPort)
	if err != nil {
		return err
	}
	return server.Serve(ctx, listener, server.Handler(pool.Ping, tokens, merchants.Get), logger)
}
