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

// OIDCIdentity is what a verified ID token tells us about a person.
type OIDCIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	IsAdmin       bool
}

// SignInOIDC finds or creates the user for id, in this order:
//  1. the user with this subject: refresh name, admin flag and (verified) email;
//  2. a placeholder with this verified email: link it, so its requests carry over;
//  3. a new user. The email is stored only when verified and not already taken.
//
// Unverified emails never link or claim anything.
func (s *Store) SignInOIDC(ctx context.Context, id OIDCIdentity) (*core.User, error) {
	if id.Subject == "" {
		return nil, errors.New("identity has no subject")
	}
	var email *string
	if id.EmailVerified {
		email = normEmail(id.Email)
	}
	var out *core.User
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		// Serialise sign-ins for the same email so two first logins can't race.
		if email != nil {
			if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext($1))`, *email); err != nil {
				return err
			}
		}
		u, err := scanUser(tx.QueryRow(ctx, `select `+userCols+` from users where oidc_sub = $1 for update`, id.Subject))
		switch {
		case err == nil:
			// Keep the stored email unless the new one is verified and free.
			newEmail := u.Email
			if email != nil && (u.Email == nil || *u.Email != *email) {
				var taken bool
				if err := tx.QueryRow(ctx, `select exists(select 1 from users where email = $1 and id <> $2)`, *email, u.ID).Scan(&taken); err != nil {
					return err
				}
				if !taken {
					newEmail = email
				}
			}
			out, err = scanUser(tx.QueryRow(ctx,
				`update users set name = $2, is_admin = $3, email = $4 where id = $1 returning `+userCols,
				u.ID, id.Name, id.IsAdmin, newEmail))
			return err
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if email != nil {
			existing, err := scanUser(tx.QueryRow(ctx, `select `+userCols+` from users where email = $1 for update`, *email))
			switch {
			case err == nil && existing.Placeholder():
				out, err = scanUser(tx.QueryRow(ctx,
					`update users set oidc_sub = $2, name = $3, is_admin = $4 where id = $1 returning `+userCols,
					existing.ID, id.Subject, id.Name, id.IsAdmin))
				return err
			case err == nil:
				// The email belongs to a different signed-in identity; don't take it.
				email = nil
			case !errors.Is(err, ErrNotFound):
				return err
			}
		}
		out, err = scanUser(tx.QueryRow(ctx,
			`insert into users (oidc_sub, email, name, is_admin) values ($1, $2, $3, $4) returning `+userCols,
			id.Subject, email, id.Name, id.IsAdmin))
		return err
	})
	return out, err
}

// UserSummary is a user with how many requests they have.
type UserSummary struct {
	core.User
	Requests int
}

// ListUserSummaries returns every user with their request count, by name.
func (s *Store) ListUserSummaries(ctx context.Context) ([]UserSummary, error) {
	rows, err := s.Pool.Query(ctx, `
		select u.id, u.oidc_sub, u.email, u.name, u.is_admin, u.created_at,
			(select count(*) from requests r where r.requester_id = u.id)
		from users u order by lower(u.name), u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserSummary
	for rows.Next() {
		var x UserSummary
		if err := rows.Scan(&x.ID, &x.OIDCSub, &x.Email, &x.Name, &x.IsAdmin, &x.CreatedAt, &x.Requests); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ErrMergeConflict is returned when both users have signed in with different identities.
var ErrMergeConflict = errors.New("both people have signed in; they are different accounts")

// MergeUsers moves everything belonging to from onto into and deletes from.
// If into is a placeholder and from has signed in, into takes over from's
// identity and email. Two signed-in identities cannot be merged.
func (s *Store) MergeUsers(ctx context.Context, fromID, intoID int64) error {
	if fromID == intoID {
		return errors.New("cannot merge someone into themselves")
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		from, err := scanUser(tx.QueryRow(ctx, `select `+userCols+` from users where id = $1 for update`, fromID))
		if err != nil {
			return err
		}
		into, err := scanUser(tx.QueryRow(ctx, `select `+userCols+` from users where id = $1 for update`, intoID))
		if err != nil {
			return err
		}
		if !from.Placeholder() && !into.Placeholder() {
			return ErrMergeConflict
		}
		for _, q := range []string{
			`update requests set requester_id = $2 where requester_id = $1`,
			`update requests set created_by_id = $2 where created_by_id = $1`,
			`update request_events set user_id = $2 where user_id = $1`,
			`update request_files set uploaded_by_id = $2 where uploaded_by_id = $1`,
			`update api_tokens set user_id = $2 where user_id = $1`,
		} {
			if _, err := tx.Exec(ctx, q, fromID, intoID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `delete from users where id = $1`, fromID); err != nil {
			return err
		}
		sub, email, admin := into.OIDCSub, into.Email, into.IsAdmin
		if into.Placeholder() && !from.Placeholder() {
			sub, admin = from.OIDCSub, from.IsAdmin
		}
		if email == nil {
			email = from.Email
		}
		_, err = tx.Exec(ctx, `update users set oidc_sub = $2, email = $3, is_admin = $4 where id = $1`, intoID, sub, email, admin)
		return err
	})
}
