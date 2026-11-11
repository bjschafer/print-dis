// Command print-dis serves the print request queue.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/spoolman"
	"github.com/bjschafer/print-dis/internal/store"
	"github.com/bjschafer/print-dis/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "print-dis:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return serve()
	case "import-sheet":
		return importSheet(args[1:], os.Getenv, os.Stdout)
	default:
		return fmt.Errorf("unknown command %q (want: serve, import-sheet)", cmd)
	}
}

func serve() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	deps := web.Deps{Config: cfg, Store: st, Log: log}
	if cfg.SpoolmanURL != "" {
		deps.Filaments = spoolman.New(cfg.SpoolmanURL)
	}
	h, err := web.New(ctx, deps)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr, "base_url", cfg.BaseURL.String(), "dev_user", cfg.DevUser != "")

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
