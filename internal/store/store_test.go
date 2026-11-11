package store_test

import (
	"context"
	"testing"

	"github.com/bjschafer/print-dis/internal/store"
	"github.com/bjschafer/print-dis/internal/store/storetest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	s := storetest.New(t)
	if err := store.Migrate(context.Background(), s.Pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := s.Pool.QueryRow(context.Background(),
		`select count(*) from information_schema.tables where table_schema='public' and table_name in ('users','printers','requests','request_events','request_files','sessions')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("got %d tables, want 6", n)
	}
}
