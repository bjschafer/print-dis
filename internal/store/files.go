package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/bjschafer/print-dis/internal/core"
)

const fileCols = `f.id, f.request_id, f.uploaded_by_id, u.name, f.object_key, f.filename, f.content_type, f.size_bytes, f.sha256, f.created_at`

const fileFrom = ` from request_files f join users u on u.id = f.uploaded_by_id`

func scanFile(row pgx.Row) (*core.File, error) {
	var f core.File
	err := row.Scan(&f.ID, &f.RequestID, &f.UploadedByID, &f.UploaderName, &f.ObjectKey, &f.Filename,
		&f.ContentType, &f.SizeBytes, &f.SHA256, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// InsertFile records an uploaded object's metadata and an event, setting f.ID
// and f.CreatedAt. The caller has already written the object.
func (s *Store) InsertFile(ctx context.Context, f *core.File) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			insert into request_files (request_id, uploaded_by_id, object_key, filename, content_type, size_bytes, sha256)
			values ($1,$2,$3,$4,$5,$6,$7) returning id, created_at`,
			f.RequestID, f.UploadedByID, f.ObjectKey, f.Filename, f.ContentType, f.SizeBytes, f.SHA256).Scan(&f.ID, &f.CreatedAt)
		if err != nil {
			return err
		}
		return addEvent(ctx, tx, f.RequestID, f.UploadedByID, core.EventFile, "uploaded "+f.Filename)
	})
}

// GetFile returns one file's metadata, or ErrNotFound.
func (s *Store) GetFile(ctx context.Context, id int64) (*core.File, error) {
	return scanFile(s.Pool.QueryRow(ctx, `select `+fileCols+fileFrom+` where f.id = $1`, id))
}

// ListFiles returns a request's files, oldest first.
func (s *Store) ListFiles(ctx context.Context, requestID int64) ([]core.File, error) {
	rows, err := s.Pool.Query(ctx, `select `+fileCols+fileFrom+` where f.request_id = $1 order by f.created_at, f.id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// CountFiles returns how many files a request has.
func (s *Store) CountFiles(ctx context.Context, requestID int64) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `select count(*) from request_files where request_id = $1`, requestID).Scan(&n)
	return n, err
}

// DeleteFile removes the row and records who removed it. The caller deletes
// the object afterwards.
func (s *Store) DeleteFile(ctx context.Context, f *core.File, byUser int64) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `delete from request_files where id = $1`, f.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return addEvent(ctx, tx, f.RequestID, byUser, core.EventFile, "removed "+f.Filename)
	})
}
