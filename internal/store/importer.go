package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bjschafer/print-dis/internal/core"
)

// ImportItem is a request to insert with the key that identifies it across re-runs.
type ImportItem struct {
	Key string
	Req core.Request
}

// ExistingImportKeys returns the subset of keys already used by an imported request.
func (s *Store) ExistingImportKeys(ctx context.Context, keys []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx, `select external_key from requests where external_key = any($1)`, keys)
	if err != nil {
		return nil, err
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, k := range got {
		out[k] = true
	}
	return out, nil
}

// ImportRequests inserts items for the user with the given email in a single
// transaction, creating a placeholder named name if there is no such user.
// Items whose key already exists are skipped. It returns the keys inserted.
func (s *Store) ImportRequests(ctx context.Context, email, name string, items []ImportItem) ([]string, error) {
	var inserted []string
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		inserted = nil
		e := normEmail(email)
		if e == nil {
			return errors.New("requester email is empty")
		}
		var uid int64
		err := tx.QueryRow(ctx, `select id from users where email = $1`, *e).Scan(&uid)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `insert into users (name, email) values ($1, $2) returning id`,
				strings.TrimSpace(name), *e).Scan(&uid)
		}
		if err != nil {
			return err
		}
		for _, it := range items {
			r := it.Req
			tag, err := tx.Exec(ctx, `
				insert into requests (requester_id, created_by_id, title, quantity, printed_count, status, needed_by, timing,
					notes, source_url, material, colors, needs_enclosure, external_key, position)
				values ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
					coalesce((select max(position) from requests where status = $5), 0) + 1)
				on conflict (external_key) do nothing`,
				uid, r.Title, r.Quantity, r.Printed, r.Status, r.NeededBy, r.Timing,
				r.Notes, r.SourceURL, r.Material, colorsParam(r.Colors), r.NeedsEnclosure, it.Key)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				inserted = append(inserted, it.Key)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inserted, nil
}
