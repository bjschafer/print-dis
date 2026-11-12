package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/files"
	"github.com/bjschafer/print-dis/internal/store"
	"github.com/bjschafer/print-dis/internal/web"
)

// measureFiles sizes the models uploaded before uploads were measured. It
// reads the server's configuration, so it runs in the server's own pod.
func measureFiles(getenv func(string) string, out io.Writer) error {
	cfg, err := config.FromEnv(getenv)
	if err != nil {
		return err
	}
	blobs, err := files.New(cfg)
	if err != nil {
		return err
	}
	if blobs == nil {
		return errors.New("FILES_BACKEND is not set, so there are no files to measure")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	measured, sized, err := web.MeasureOldFiles(ctx, st, blobs, log)
	_, _ = fmt.Fprintf(out, "measured %d files, sized %d requests\n", measured, sized)
	return err
}
