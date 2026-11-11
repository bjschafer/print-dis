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
	"github.com/bjschafer/print-dis/internal/files"
	"github.com/bjschafer/print-dis/internal/preview"
	"github.com/bjschafer/print-dis/internal/printerstatus"
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

	blobs, err := files.New(cfg)
	if err != nil {
		return err
	}
	deps := web.Deps{Config: cfg, Store: st, Log: log, Files: blobs}
	if cfg.SpoolmanURL != "" {
		deps.Filaments = spoolman.New(cfg.SpoolmanURL)
	}
	deps.Previewer = preview.New(preview.WithAllowHosts(cfg.PreviewAllowHosts...))
	mon := printerstatus.New(printerstatus.Options{
		Targets: statusTargets(st),
		Log:     log,
	})
	go mon.Run(ctx)
	deps.PrinterStatus = mon
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

// statusTargets lists the active printers that have a live status source,
// with the Bambuddy settings as they are now.
func statusTargets(st *store.Store) func(context.Context) ([]printerstatus.Target, error) {
	return func(ctx context.Context) ([]printerstatus.Target, error) {
		ps, err := st.ListPrinters(ctx)
		if err != nil {
			return nil, err
		}
		set, err := st.GetSettings(ctx)
		if err != nil {
			return nil, err
		}
		var out []printerstatus.Target
		for _, p := range ps {
			if !p.Active || !p.HasStatusSource() {
				continue
			}
			t := printerstatus.Target{PrinterID: p.ID, MoonrakerURL: p.MoonrakerURL}
			if p.BambuddyPrinterID != nil {
				t.BambuddyID = *p.BambuddyPrinterID
				t.BambuddyURL, t.BambuddyAPIKey = set.BambuddyURL, set.BambuddyAPIKey
			}
			out = append(out, t)
		}
		return out, nil
	}
}
