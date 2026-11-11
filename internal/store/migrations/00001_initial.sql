-- +goose Up
create table users (
  id          bigint generated always as identity primary key,
  oidc_sub    text unique,
  email       text unique check (email is null or email = lower(email)),
  name        text not null,
  is_admin    boolean not null default false,
  created_at  timestamptz not null default now()
);

create table printers (
  id              bigint generated always as identity primary key,
  name            text not null unique,
  bed_x_mm        integer not null check (bed_x_mm > 0),
  bed_y_mm        integer not null check (bed_y_mm > 0),
  bed_z_mm        integer not null check (bed_z_mm > 0),
  multi_material  boolean not null default false,
  enclosed        boolean not null default false,
  notes           text not null default '',
  active          boolean not null default true
);

create type request_status as enum
  ('requested', 'needs_info', 'printing', 'ready', 'delivered', 'cancelled');
create type request_timing as enum ('firm', 'flexible', 'whenever');

create table requests (
  id                    bigint generated always as identity primary key,
  requester_id          bigint not null references users(id),
  created_by_id         bigint not null references users(id),
  title                 text not null,
  quantity              integer not null default 1 check (quantity > 0),
  printed_count         integer not null default 0 check (printed_count >= 0),
  status                request_status not null default 'requested',
  needed_by             date,
  timing                request_timing not null default 'whenever',
  notes                 text not null default '',

  source_url            text,
  source_title          text,
  source_image          bytea check (source_image is null or octet_length(source_image) <= 524288),
  source_image_type     text,

  material              text not null default '',
  spoolman_filament_id  integer,
  filament_label        text not null default '',
  filament_color_hex    text,
  color_note            text not null default '',
  needs_multi_material  boolean not null default false,
  needs_enclosure       boolean not null default false,
  size_x_mm             integer check (size_x_mm > 0),
  size_y_mm             integer check (size_y_mm > 0),
  size_z_mm             integer check (size_z_mm > 0),

  printer_id            bigint references printers(id),
  position              double precision not null default 0,
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now()
);
create index requests_requester_idx on requests (requester_id);
create index requests_status_position_idx on requests (status, position);

create table request_events (
  id          bigint generated always as identity primary key,
  request_id  bigint not null references requests(id) on delete cascade,
  user_id     bigint not null references users(id),
  kind        text not null check (kind in ('comment', 'status', 'edit', 'file')),
  body        text not null default '',
  created_at  timestamptz not null default now()
);
create index request_events_request_idx on request_events (request_id, created_at);

create table request_files (
  id              bigint generated always as identity primary key,
  request_id      bigint not null references requests(id) on delete cascade,
  uploaded_by_id  bigint not null references users(id),
  object_key      text not null unique,
  filename        text not null,
  content_type    text not null,
  size_bytes      bigint not null check (size_bytes >= 0),
  sha256          text not null,
  created_at      timestamptz not null default now()
);
create index request_files_request_idx on request_files (request_id);

-- alexedwards/scs pgxstore
create table sessions (
  token   text primary key,
  data    bytea not null,
  expiry  timestamptz not null
);
create index sessions_expiry_idx on sessions (expiry);

-- +goose Down
drop table sessions;
drop table request_files;
drop table request_events;
drop table requests;
drop type request_timing;
drop type request_status;
drop table printers;
drop table users;
