-- +goose Up
-- Where print-dis reads a printer's live status from (docs/design.md section
-- 5a). At most one source per printer; neither means no status.
alter table printers
  add column moonraker_url text not null default '',
  add column bambuddy_printer_id integer check (bambuddy_printer_id > 0),
  add constraint printers_one_status_source check (moonraker_url = '' or bambuddy_printer_id is null);

-- +goose Down
alter table printers
  drop constraint printers_one_status_source,
  drop column bambuddy_printer_id,
  drop column moonraker_url;
