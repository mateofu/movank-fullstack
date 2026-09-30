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

	"github.com/mateofu/movank-fullstack/backend/internal/config"
	"github.com/mateofu/movank-fullstack/backend/internal/database"
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
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "healthcheck" {
			return fmt.Errorf("usage: api [healthcheck]")
		}
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	listener, err := net.Listen("tcp", ":"+cfg.HTTPPort)
	if err != nil {
		return err
	}
	return server.Serve(ctx, listener, server.Handler(pool.Ping), logger)
}
