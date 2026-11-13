-- +goose Up
-- The JSON API (docs/design.md section 6b).

-- external_key identifies a request filed by a script (the sheet import, or a
-- client of the API), so that re-running it does not file it twice.
alter table requests rename column import_key to external_key;

-- project groups requests that belong together, such as every part of one
-- printer build; empty for none.
alter table requests add column project text not null default '';

-- api_tokens are bearer tokens for the API. Only the SHA-256 of a token is
-- kept; the token itself is shown once, when it is created.
create table api_tokens (
  id            bigint generated always as identity primary key,
  user_id       bigint not null references users(id) on delete cascade,
  name          text not null,
  token_hash    bytea not null unique,
  created_at    timestamptz not null default now(),
  last_used_at  timestamptz
);

-- +goose Down
drop table api_tokens;
alter table requests drop column project;
alter table requests rename column external_key to import_key;
