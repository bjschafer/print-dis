package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bjschafer/print-dis/internal/core"
)

// requestCols excludes source_image, which is fetched separately.
const requestCols = `r.id, r.requester_id, r.created_by_id, r.title, r.quantity, r.printed_count, r.status, r.needed_by,
	r.timing, r.notes, r.source_url, r.source_title, r.source_image_type, r.material, r.spoolman_filament_id,
	r.filament_label, r.filament_color_hex, r.color_note, r.needs_multi_material, r.needs_enclosure,
	r.size_x_mm, r.size_y_mm, r.size_z_mm, r.printer_id, r.position, r.created_at, r.updated_at,
	u.name, coalesce(p.name, ''), (r.source_image is not null)`

const requestFrom = ` from requests r join users u on u.id = r.requester_id left join printers p on p.id = r.printer_id`

func scanRequest(row pgx.Row) (*core.Request, error) {
	var r core.Request
	var hasImage bool
	err := row.Scan(&r.ID, &r.RequesterID, &r.CreatedByID, &r.Title, &r.Quantity, &r.Printed, &r.Status, &r.NeededBy,
		&r.Timing, &r.Notes, &r.SourceURL, &r.SourceTitle, &r.SourceImageType, &r.Material, &r.SpoolmanFilamentID,
		&r.FilamentLabel, &r.FilamentColorHex, &r.ColorNote, &r.NeedsMultiMaterial, &r.NeedsEnclosure,
		&r.SizeX, &r.SizeY, &r.SizeZ, &r.PrinterID, &r.Position, &r.CreatedAt, &r.UpdatedAt,
		&r.RequesterName, &r.PrinterName, &hasImage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if hasImage && r.SourceImageType == nil {
		t := "application/octet-stream"
		r.SourceImageType = &t
	} else if !hasImage {
		r.SourceImageType = nil
	}
	return &r, err
}

// Filter narrows a request listing. Zero values mean "no constraint".
type Filter struct {
	RequesterID int64
	PrinterID   int64
	Material    string
	Statuses    []core.Status
	// Sort is a column key from SortColumns; empty means board order.
	Sort string
	Desc bool
}

// SortColumns maps the sort keys accepted from the UI to SQL.
var SortColumns = map[string]string{
	"title":     "lower(r.title)",
	"requester": "lower(u.name)",
	"status":    "r.status",
	"needed_by": "r.needed_by",
	"created":   "r.created_at",
	"updated":   "r.updated_at",
	"printer":   "lower(p.name)",
	"material":  "lower(r.material)",
}

func (s *Store) ListRequests(ctx context.Context, f Filter) ([]core.Request, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.RequesterID != 0 {
		add("r.requester_id = $%d", f.RequesterID)
	}
	if f.PrinterID != 0 {
		add("r.printer_id = $%d", f.PrinterID)
	}
	if f.Material != "" {
		add("lower(r.material) = lower($%d)", f.Material)
	}
	if len(f.Statuses) > 0 {
		ss := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			ss[i] = string(st)
		}
		add("r.status::text = any($%d)", ss)
	}
	q := `select ` + requestCols + requestFrom
	if len(where) > 0 {
		q += " where " + strings.Join(where, " and ")
	}
	if col, ok := SortColumns[f.Sort]; ok {
		dir := " asc nulls last"
		if f.Desc {
			dir = " desc nulls last"
		}
		q += " order by " + col + dir + ", r.id"
	} else {
		q += " order by r.position, r.id"
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Request
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// ListMaterials returns the distinct non-empty materials in use, for filters.
func (s *Store) ListMaterials(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `select distinct material from requests where material <> '' order by 1`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) GetRequest(ctx context.Context, id int64) (*core.Request, error) {
	return scanRequest(s.Pool.QueryRow(ctx, `select `+requestCols+requestFrom+` where r.id = $1`, id))
}

// GetRequestImage returns the stored thumbnail and its content type.
func (s *Store) GetRequestImage(ctx context.Context, id int64) ([]byte, string, error) {
	var img []byte
	var typ *string
	err := s.Pool.QueryRow(ctx, `select source_image, source_image_type from requests where id = $1`, id).Scan(&img, &typ)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && img == nil) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	t := "application/octet-stream"
	if typ != nil {
		t = *typ
	}
	return img, t, nil
}

// CreateRequest inserts r at the end of its column and fills in ID and timestamps.
func (s *Store) CreateRequest(ctx context.Context, r *core.Request) error {
	return s.Pool.QueryRow(ctx, `
		insert into requests (requester_id, created_by_id, title, quantity, printed_count, status, needed_by, timing, notes,
			source_url, source_title, source_image, source_image_type, material, spoolman_filament_id, filament_label,
			filament_color_hex, color_note, needs_multi_material, needs_enclosure, size_x_mm, size_y_mm, size_z_mm,
			printer_id, position)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,
			coalesce((select max(position) from requests where status = $6), 0) + 1)
		returning id, position, created_at, updated_at`,
		r.RequesterID, r.CreatedByID, r.Title, r.Quantity, r.Printed, r.Status, r.NeededBy, r.Timing, r.Notes,
		r.SourceURL, r.SourceTitle, r.SourceImage, r.SourceImageType, r.Material, r.SpoolmanFilamentID, r.FilamentLabel,
		r.FilamentColorHex, r.ColorNote, r.NeedsMultiMaterial, r.NeedsEnclosure, r.SizeX, r.SizeY, r.SizeZ,
		r.PrinterID,
	).Scan(&r.ID, &r.Position, &r.CreatedAt, &r.UpdatedAt)
}

// UpdateRequest writes every editable column (not status or position) and
// records an edit event with summary as its body, in one transaction.
func (s *Store) UpdateRequest(ctx context.Context, r *core.Request, byUser int64, summary string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			update requests set requester_id=$2, title=$3, quantity=$4, printed_count=$5, needed_by=$6, timing=$7, notes=$8,
				source_url=$9, source_title=$10, material=$11, spoolman_filament_id=$12, filament_label=$13,
				filament_color_hex=$14, color_note=$15, needs_multi_material=$16, needs_enclosure=$17,
				size_x_mm=$18, size_y_mm=$19, size_z_mm=$20, printer_id=$21, updated_at=now()
			where id=$1`,
			r.ID, r.RequesterID, r.Title, r.Quantity, r.Printed, r.NeededBy, r.Timing, r.Notes,
			r.SourceURL, r.SourceTitle, r.Material, r.SpoolmanFilamentID, r.FilamentLabel,
			r.FilamentColorHex, r.ColorNote, r.NeedsMultiMaterial, r.NeedsEnclosure,
			r.SizeX, r.SizeY, r.SizeZ, r.PrinterID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if summary == "" {
			return nil
		}
		return addEvent(ctx, tx, r.ID, byUser, core.EventEdit, summary)
	})
}

// SetStatus changes status, moves the request to the end of the new column,
// and records a status event.
func (s *Store) SetStatus(ctx context.Context, id int64, to core.Status, byUser int64) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		return setStatus(ctx, tx, id, to, byUser, nil)
	})
}

func setStatus(ctx context.Context, tx pgx.Tx, id int64, to core.Status, byUser int64, position *float64) error {
	var from core.Status
	if err := tx.QueryRow(ctx, `select status from requests where id = $1 for update`, id).Scan(&from); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if from == to {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		update requests set status = $2, updated_at = now(),
			position = coalesce($3, (select coalesce(max(position), 0) + 1 from requests where status = $2))
		where id = $1`, id, to, position); err != nil {
		return err
	}
	return addEvent(ctx, tx, id, byUser, core.EventStatus, core.StatusChangeBody(from, to))
}

// Move puts request id into column to, ordering that column as order (a list
// of request ids, which should include id). Ids not in the column are ignored.
func (s *Store) Move(ctx context.Context, id int64, to core.Status, order []int64, byUser int64) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := setStatus(ctx, tx, id, to, byUser, nil); err != nil {
			return err
		}
		for i, rid := range order {
			if _, err := tx.Exec(ctx, `update requests set position = $2 where id = $1 and status = $3`, rid, float64(i+1), to); err != nil {
				return err
			}
		}
		return nil
	})
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func addEvent(ctx context.Context, q execer, requestID, userID int64, kind core.EventKind, body string) error {
	_, err := q.Exec(ctx, `insert into request_events (request_id, user_id, kind, body) values ($1,$2,$3,$4)`,
		requestID, userID, string(kind), body)
	return err
}

// AddEvent records a comment, file or other event.
func (s *Store) AddEvent(ctx context.Context, requestID, userID int64, kind core.EventKind, body string) error {
	return addEvent(ctx, s.Pool, requestID, userID, kind, body)
}

func (s *Store) ListEvents(ctx context.Context, requestID int64) ([]core.Event, error) {
	rows, err := s.Pool.Query(ctx, `
		select e.id, e.request_id, e.user_id, u.name, e.kind, e.body, e.created_at
		from request_events e join users u on u.id = e.user_id
		where e.request_id = $1 order by e.created_at, e.id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Event
	for rows.Next() {
		var e core.Event
		if err := rows.Scan(&e.ID, &e.RequestID, &e.UserID, &e.UserName, &e.Kind, &e.Body, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
