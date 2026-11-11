package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bjschafer/print-dis/internal/importer"
	"github.com/bjschafer/print-dis/internal/store"
)

type importArgs struct {
	file      string
	requester string
	dryRun    bool
}

// parseImportArgs accepts the file argument before, after or between flags.
func parseImportArgs(args []string, usageOut io.Writer) (*importArgs, error) {
	fs := flag.NewFlagSet("import-sheet", flag.ContinueOnError)
	fs.SetOutput(usageOut)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(usageOut, "usage: print-dis import-sheet <file.csv> --requester <email> [--dry-run]")
		fs.PrintDefaults()
	}
	a := &importArgs{}
	fs.StringVar(&a.requester, "requester", "", "email of the user who owns every imported request (required)")
	fs.BoolVar(&a.dryRun, "dry-run", false, "map and report, but write nothing")

	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	if len(pos) != 1 {
		return nil, errors.New("want exactly one CSV file argument")
	}
	if a.requester == "" {
		return nil, errors.New("--requester is required")
	}
	a.file = pos[0]
	return a, nil
}

// importSheet needs only DATABASE_URL, not the rest of the server configuration.
func importSheet(args []string, getenv func(string) string, out io.Writer) error {
	a, err := parseImportArgs(args, os.Stderr)
	if err != nil {
		return err
	}
	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	f, err := os.Open(a.file)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	records, err := importer.ReadCSV(f)
	if err != nil {
		return err
	}
	res, err := importer.Map(records, time.Now())
	if err != nil {
		return fmt.Errorf("%s: %w", a.file, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close()

	sum, infos, err := importer.Run(ctx, st, res, a.requester, a.dryRun)
	if err != nil {
		return err
	}
	importer.Report(out, res, sum, infos, a.dryRun)
	return nil
}
