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
	name      string
	dryRun    bool
}

// parseImportArgs accepts the file argument before, after or between flags.
func parseImportArgs(args []string, usageOut io.Writer) (*importArgs, error) {
	fs := flag.NewFlagSet("import-sheet", flag.ContinueOnError)
	fs.SetOutput(usageOut)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(usageOut, "usage: print-dis import-sheet <file.csv|-> --requester <email> [--name <name>] [--dry-run]")
		fs.PrintDefaults()
	}
	a := &importArgs{}
	fs.StringVar(&a.requester, "requester", "", "email of the user who owns every imported request (required)")
	fs.StringVar(&a.name, "name", "", "name for the requester if they must be created as a placeholder (default: the part of the email before @)")
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
	// "-" reads stdin, so the import can run in a one-off pod with the CSV
	// piped in.
	in := io.Reader(os.Stdin)
	if a.file != "-" {
		f, err := os.Open(a.file)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	records, err := importer.ReadCSV(in)
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

	sum, infos, err := importer.Run(ctx, st, res, a.requester, a.name, a.dryRun)
	if err != nil {
		return err
	}
	importer.Report(out, res, sum, infos, a.dryRun)
	return nil
}
