package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bjschafer/print-dis/internal/core"
)

const printerCols = `id, name, bed_x_mm, bed_y_mm, bed_z_mm, multi_material, enclosed, notes, active`

func scanPrinter(row pgx.Row) (*core.Printer, error) {
	var p core.Printer
	err := row.Scan(&p.ID, &p.Name, &p.BedX, &p.BedY, &p.BedZ, &p.MultiMaterial, &p.Enclosed, &p.Notes, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// ErrDuplicate is returned when a unique column would collide.
var ErrDuplicate = errors.New("already exists")

// ListPrinters returns all printers, active first, then by name.
func (s *Store) ListPrinters(ctx context.Context) ([]core.Printer, error) {
	rows, err := s.Pool.Query(ctx, `select `+printerCols+` from printers order by active desc, lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Printer
	for rows.Next() {
		p, err := scanPrinter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) GetPrinter(ctx context.Context, id int64) (*core.Printer, error) {
	return scanPrinter(s.Pool.QueryRow(ctx, `select `+printerCols+` from printers where id = $1`, id))
}

// SavePrinter inserts when p.ID is zero, otherwise updates.
func (s *Store) SavePrinter(ctx context.Context, p *core.Printer) error {
	var err error
	if p.ID == 0 {
		err = s.Pool.QueryRow(ctx, `
			insert into printers (name, bed_x_mm, bed_y_mm, bed_z_mm, multi_material, enclosed, notes, active)
			values ($1,$2,$3,$4,$5,$6,$7,$8) returning id`,
			p.Name, p.BedX, p.BedY, p.BedZ, p.MultiMaterial, p.Enclosed, p.Notes, p.Active).Scan(&p.ID)
	} else {
		var tag pgconn.CommandTag
		tag, err = s.Pool.Exec(ctx, `
			update printers set name=$2, bed_x_mm=$3, bed_y_mm=$4, bed_z_mm=$5, multi_material=$6, enclosed=$7, notes=$8, active=$9
			where id=$1`,
			p.ID, p.Name, p.BedX, p.BedY, p.BedZ, p.MultiMaterial, p.Enclosed, p.Notes, p.Active)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
	}
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}
