package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bjschafer/print-dis/internal/core"
)

// APIToken is a bearer token's record; the token itself is never stored.
type APIToken struct {
	ID         int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// CreateAPIToken records a token for userID by the SHA-256 of its value.
func (s *Store) CreateAPIToken(ctx context.Context, userID int64, name string, hash []byte) error {
	_, err := s.Pool.Exec(ctx, `insert into api_tokens (user_id, name, token_hash) values ($1, $2, $3)`, userID, name, hash)
	return err
}

// ListAPITokens returns userID's tokens, newest first.
func (s *Store) ListAPITokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.Pool.Query(ctx, `
		select id, name, created_at, last_used_at from api_tokens where user_id = $1 order by created_at desc, id desc`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (APIToken, error) {
		var t APIToken
		err := row.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt)
		return t, err
	})
}

// DeleteAPIToken revokes one of userID's tokens. It returns ErrNotFound when
// userID has no such token.
func (s *Store) DeleteAPIToken(ctx context.Context, userID, id int64) error {
	tag, err := s.Pool.Exec(ctx, `delete from api_tokens where id = $1 and user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UserByAPIToken returns the owner of the token with this hash, noting that
// it was used, or ErrNotFound.
func (s *Store) UserByAPIToken(ctx context.Context, hash []byte) (*core.User, error) {
	var uid int64
	err := s.Pool.QueryRow(ctx, `update api_tokens set last_used_at = now() where token_hash = $1 returning user_id`, hash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, uid)
}
