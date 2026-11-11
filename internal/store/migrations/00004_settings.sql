-- +goose Up
-- Settings an admin changes in the app (docs/design.md section 5a). One row.
create table settings (
  id                boolean primary key default true check (id),
  bambuddy_url      text not null default '',
  bambuddy_api_key  text not null default ''
);
insert into settings default values;

-- +goose Down
drop table settings;
