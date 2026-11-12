-- +goose Up
-- The bounding box of an uploaded model, when it could be measured
-- (docs/design.md section 6a).
alter table request_files
  add column size_x_mm integer,
  add column size_y_mm integer,
  add column size_z_mm integer;

-- +goose Down
alter table request_files drop column size_x_mm, drop column size_y_mm, drop column size_z_mm;
