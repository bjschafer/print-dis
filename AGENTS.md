# print-dis

A print-request queue for one owner and their friends. `docs/design.md` is the spec; read it before changing behaviour, and update it when a decision changes.

## Commands

- `make db`: start a throwaway Postgres 17 on port 55432 (podman).
- `make run`: build and serve on :8080 with `DEV_USER` sign-in and files in `./.files`.
- `make check`: vet, golangci-lint, tests. Must pass before any work is called done.
- Tests that need Postgres read `TEST_DATABASE_URL` and skip without it. Locally:
  `TEST_DATABASE_URL='postgres://postgres:pg@localhost:55432/printdis?sslmode=disable' make check`

## Done

Work in this repo is done when `make check` passes, it is committed to `main`, and it is pushed. Pushing is part of finishing, not a separate request: do it without asking. A push to `main` builds and publishes the image through CI, so watch the run (`gh run watch`) and report if it fails.

## Layout

```
cmd/print-dis/        main: serve, import-sheet
internal/config/      environment variables -> Config
internal/core/        types, eligibility, permission rules (pure, no IO)
internal/store/       SQL (pgx, hand-written) and goose migrations
internal/store/storetest/  New(t): a fresh migrated database per test
internal/web/         handlers, html/template templates, embedded static files
internal/auth/        OIDC and sessions
internal/spoolman/    client and cache
internal/preview/     link fetch and SSRF guard
internal/printerstatus/  Moonraker and Bambuddy status polling
internal/files/       Blobs interface, s3 and dir backends
```

## Conventions

- Standard library first. `net/http` ServeMux patterns (`GET /requests/{id}`), `log/slog`, `html/template`. No web framework, no ORM, no testify.
- Permission decisions live in `internal/core` as pure functions and are tested there table-driven. Handlers call them; they do not re-derive rules.
- New schema goes in a new numbered file under `internal/store/migrations`; never edit an applied migration.
- Frontend: server-rendered HTML, htmx and SortableJS vendored under `internal/web/static` and embedded. No Node, no build step. Must work on a phone.
- Every state-changing route is POST and checks the CSRF token.
- Outbound HTTP (Spoolman, link preview, printer status) always has a timeout and a tested failure path; the app must start and work when any of them is down.
- Errors shown to users are plain sentences; details go to the log.
