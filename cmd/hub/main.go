package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/HyeonTee/agent-control-plane/internal/adapter/httpapi"
	"github.com/HyeonTee/agent-control-plane/internal/adapter/postgres"
	"github.com/HyeonTee/agent-control-plane/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	var err error
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		err = runAdmin(os.Args[2:])
	} else if len(os.Args) == 1 {
		err = run()
	} else {
		err = errors.New("usage: hub [admin bootstrap|issue-token|revoke-token]")
	}
	if err != nil {
		logger.Error("hub stopped", "error", err)
		os.Exit(1)
	}
}

func runAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("admin command required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid HUB_DATABASE_URL")
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return errors.New("could not create database pool")
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	switch args[0] {
	case "bootstrap":
		flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
		space := flags.String("space", "personal", "initial space name")
		name := flags.String("token-name", "owner", "initial token name")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected arguments")
		}
		result, err := postgres.BootstrapOwner(ctx, pool, *space, *name)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "issue-token":
		flags := flag.NewFlagSet("issue-token", flag.ContinueOnError)
		spaceID := flags.String("space-id", "", "allowed space ID")
		name := flags.String("name", "", "integration name")
		scopes := flags.String("scopes", "context:read", "comma-separated scopes")
		days := flags.Int("days", 90, "expiry in days")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected arguments")
		}
		result, err := postgres.CreateToken(ctx, pool, *spaceID, *name, strings.Split(*scopes, ","), time.Duration(*days)*24*time.Hour)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "revoke-token":
		flags := flag.NewFlagSet("revoke-token", flag.ContinueOnError)
		clientID := flags.String("client-id", "", "token client ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected arguments")
		}
		if err := postgres.RevokeToken(ctx, pool, *clientID); err != nil {
			return err
		}
		_, err := fmt.Fprintln(os.Stdout, "revoked")
		return err
	default:
		return errors.New("unknown admin command")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid HUB_DATABASE_URL")
	}
	poolCfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return errors.New("could not create database pool")
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandler(pool, postgres.NewStore(pool)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	slog.Info("hub listening", "addr", cfg.HTTPAddr)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
