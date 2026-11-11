-- +goose Up
-- import_key identifies a request created by `print-dis import-sheet`, so that
-- re-running an import does not duplicate it. Null for every other request.
alter table requests add column import_key text unique;

-- +goose Down
alter table requests drop column import_key;
