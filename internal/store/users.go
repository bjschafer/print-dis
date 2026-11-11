package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bjschafer/print-dis/internal/core"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

const userCols = `id, oidc_sub, email, name, is_admin, created_at`

func scanUser(row pgx.Row) (*core.User, error) {
	var u core.User
	err := row.Scan(&u.ID, &u.OIDCSub, &u.Email, &u.Name, &u.IsAdmin, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func normEmail(e string) *string {
	e = strings.ToLower(strings.TrimSpace(e))
	if e == "" {
		return nil
	}
	return &e
}

func (s *Store) GetUser(ctx context.Context, id int64) (*core.User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `select `+userCols+` from users where id = $1`, id))
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*core.User, error) {
	e := normEmail(email)
	if e == nil {
		return nil, ErrNotFound
	}
	return scanUser(s.Pool.QueryRow(ctx, `select `+userCols+` from users where email = $1`, *e))
}

// ListUsers returns every user and placeholder, by name.
func (s *Store) ListUsers(ctx context.Context) ([]core.User, error) {
	rows, err := s.Pool.Query(ctx, `select `+userCols+` from users order by lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// EnsureDevUser finds or creates the DEV_USER admin.
func (s *Store) EnsureDevUser(ctx context.Context, email string) (*core.User, error) {
	e := normEmail(email)
	if e == nil {
		return nil, errors.New("dev user email is empty")
	}
	sub := "dev:" + *e
	return scanUser(s.Pool.QueryRow(ctx, `
		insert into users (oidc_sub, email, name, is_admin) values ($1, $2, $3, true)
		on conflict (email) do update set oidc_sub = coalesce(users.oidc_sub, excluded.oidc_sub), is_admin = true
		returning `+userCols, sub, *e, *e))
}

// CreatePlaceholder makes a user that has never signed in.
func (s *Store) CreatePlaceholder(ctx context.Context, name, email string) (*core.User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	u, err := scanUser(s.Pool.QueryRow(ctx,
		`insert into users (name, email) values ($1, $2) returning `+userCols, name, normEmail(email)))
	if err != nil && isUniqueViolation(err) {
		return nil, fmt.Errorf("someone with that email already exists")
	}
	return u, err
}
