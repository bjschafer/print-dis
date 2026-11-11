# print-dis v2: design and build plan

Status: accepted, 2026-11-11. Open decision 1 is overruled: friends can upload files (section 6a).

Progress: M0 to M7 are built and pass `make check`. Deployed 2026-11-11 at https://prints.cmdcentral.xyz (tf/authentik `fd6ad20`, k8s/generators `4d57a247`, bucket `print-dis` on versitygw).

This is a ground-up rewrite in the existing repository. The current code (Go, static HTML, SQLite/Postgres, local password accounts) is replaced, not extended; only `internal/spoolman/types.go` is worth carrying over.

## 1. What this is for

One person runs three FDM printers (a Bambu H2D and two Klipper Vorons: a Trident 350 and a Pandora's Box) and prints for friends as well as for themselves. Requests currently live in a shared Google Sheet, in open browser tabs, and in memory.

print-dis is the single queue for all of it. A friend signs in, files a request, and can see and edit it. The owner sees everything on a board, knows what each request needs (filament, deadline, which printers can do it), and moves it along.

Two off-the-shelf options were tried or considered and rejected, which is why this exists:

- **STL Quest** (trialled): no filament or material field, no due dates, and printer matching that only knows bounding boxes. Its automation did not help a small mixed fleet.
- **Generic tables (NocoDB, Grist)**: would hold the columns, but cannot offer a filament picker fed by Spoolman or printer-capability matching.

Those two things, plus one-click capture of a model link, are the reason to build. Everything else should stay as small as possible.

### Non-goals

- Controlling printers, slicing, or estimating print time.
- Payments, quotes, or cost calculation.
- Being a model library. Friends can attach files to a request (section 6a), but browsing, versioning and organising models stays in Manyfold.
- Multi-tenancy. There is one queue and one owner.
- Automatic printer assignment. The app shows which printers are eligible; a person chooses.

## 2. Users and permissions

Sign-in is OIDC only, against Authentik at `https://login.cmdcentral.xyz`. There are no local passwords.

| Role | Who | Can do |
|---|---|---|
| Admin | The owner. Determined by membership of an Authentik group, read from the `groups` claim. | Everything: all requests, status changes, printers, people. |
| Requester | Any other signed-in user. | Create requests. See only their own. Edit their own while the status is `requested` or `needs_info`. Comment on their own at any time. Cancel their own before `printing`. |

A user row is created on first sign-in from the `sub`, `email` and `name` claims.

**Filing on someone's behalf.** An admin can create a request for a person who has never signed in. That creates a placeholder user with a name and, optionally, an email. When someone later signs in with a matching verified email, the placeholder is linked to their identity and they inherit its requests.

## 3. Data model

PostgreSQL only. Money and per-copy state are deliberately absent. File contents live in object storage (section 6a); only their metadata is here.

```sql
create table users (
  id          bigint generated always as identity primary key,
  oidc_sub    text unique,              -- null for placeholders
  email       text unique,              -- lower-cased; null allowed
  name        text not null,
  is_admin    boolean not null default false,  -- refreshed from the groups claim at each sign-in
  created_at  timestamptz not null default now()
);

create table printers (
  id              bigint generated always as identity primary key,
  name            text not null unique,
  bed_x_mm        integer not null,
  bed_y_mm        integer not null,
  bed_z_mm        integer not null,
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

  -- where the model comes from
  source_url            text,
  source_title          text,
  source_image          bytea,            -- small thumbnail, capped at 512 KiB
  source_image_type     text,

  -- what it needs
  material              text not null default '',   -- "PETG", "ABS", or empty for "any"
  spoolman_filament_id  integer,                    -- set when picked from Spoolman
  filament_label        text not null default '',   -- snapshot: "Tinmorry PETG-GF Blue"
  filament_color_hex    text,                       -- snapshot
  color_note            text not null default '',   -- free text: "any", "matte orange"
  needs_multi_material  boolean not null default false,
  needs_enclosure       boolean not null default false,
  size_x_mm             integer,
  size_y_mm             integer,
  size_z_mm             integer,

  printer_id            bigint references printers(id),
  position              double precision not null default 0,  -- manual order within a column
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now()
);

create table request_events (
  id          bigint generated always as identity primary key,
  request_id  bigint not null references requests(id) on delete cascade,
  user_id     bigint not null references users(id),
  kind        text not null check (kind in ('comment', 'status', 'edit', 'file')),
  body        text not null default '',   -- comment text, or "requested -> printing"
  created_at  timestamptz not null default now()
);

create table request_files (
  id              bigint generated always as identity primary key,
  request_id      bigint not null references requests(id) on delete cascade,
  uploaded_by_id  bigint not null references users(id),
  object_key      text not null unique,   -- "requests/{request_id}/{random}"; never derived from the filename
  filename        text not null,          -- original name, shown and used for downloads
  content_type    text not null,
  size_bytes      bigint not null check (size_bytes >= 0),
  sha256          text not null,
  created_at      timestamptz not null default now()
);
```

Notes on the choices:

- **Statuses match the existing sheet** (Requested, Needs info, Printing, Ready for pickup, Delivered) plus `cancelled`. `delivered` and `cancelled` are terminal and hidden from the board.
- **Filament fields are snapshots.** The Spoolman ID is kept for linking, but the label and colour are copied at pick time so a request still reads correctly after that filament is deleted or Spoolman is down.
- **`printed_count` is the whole of progress tracking.** "3 of 10 printed" is enough; copies do not have individual states.
- **`request_events` is both the comment thread and the audit trail.** A "needs info" exchange happens there.

### Printer eligibility

A pure function, `Eligible(request, printers) []Printer`, with no database access. A printer is eligible when all of these hold:

1. It is active.
2. The request does not need multi-material, or the printer has it.
3. The request does not need an enclosure, or the printer has one.
4. The request has no size, or the size fits the bed. X and Y may be swapped (rotation about Z); Z may not.

The request form pre-ticks "needs enclosure" when the material is ABS, ASA, PC or PA. The user can untick it.

The request page lists eligible printers and says why each of the others was excluded. Assigning an ineligible printer is allowed with a visible warning, because the owner sometimes knows better.

## 4. Screens

Server-rendered HTML with `html/template`, htmx for partial updates, and SortableJS for dragging cards. Both scripts are vendored into the repo and embedded in the binary; there is no Node toolchain and no build step for the frontend. It must be usable on a phone.

| Route | Purpose |
|---|---|
| `GET /` | Board. Columns: Requested, Needs info, Printing, Ready. Admin sees all requests and can drag between columns; requesters see their own, read-only. Cards show title, requester, quantity and progress, filament swatch, needed-by date, and assigned printer. Firm dates that are overdue or within three days are highlighted. Filters: requester, printer, material. |
| `GET /requests` | Table of everything, including delivered and cancelled. Sortable; the same filters. |
| `GET /requests/new` | New request form. Accepts `?url=` to prefill from a link (section 6). Admins get a "for" field to pick or create a person. |
| `GET /requests/{id}` | Detail: all fields, eligible printers, comment thread, history. Edit in place, subject to section 2. |
| `POST /requests/{id}/status` | Status change (admin; requesters may only cancel). Records an event. |
| `POST /requests/{id}/move` | Reorder within or across columns from a drag. |
| `POST /requests/{id}/comments` | Add a comment. |
| `POST /requests/{id}/files` | Upload one or more files (section 6a). |
| `GET /files/{id}` | Download a file, subject to the same visibility as its request. |
| `POST /files/{id}/delete` | Remove a file. |
| `GET /printers`, `POST /printers...` | Admin: printer list and editing, including bed dimensions. |
| `GET /people` | Admin: users and placeholders; create a placeholder; merge two rows. |
| `GET /auth/login`, `/auth/callback`, `POST /auth/logout` | OIDC code flow with PKCE. |
| `GET /healthz`, `GET /readyz` | Liveness, and readiness including a database ping. |

Every state-changing route requires a CSRF token.

## 5. Spoolman integration

Read-only, against the in-cluster service `http://spoolman.spoolman.svc.cluster.local:8000`.

- Fetch `GET /api/v1/filament` and `GET /api/v1/spool`, and cache both in memory for five minutes.
- The picker lists filaments grouped by material, each with vendor, name, a colour swatch, and whether any unarchived spool has stock left.
- Picking one fills `spoolman_filament_id`, `filament_label`, `filament_color_hex` and `material`.
- The picker always offers "any" and a free-text colour note alongside it.
- If Spoolman is unreachable, the form falls back to free-text material and colour and says so. The app must start and work without Spoolman.

print-dis never writes to Spoolman.

## 6. Capturing a link

The goal is to replace "leave the tab open" with one click.

- **Bookmarklet:** opens `/requests/new?url=<current page>` in a new tab. It rides the existing session, so there are no API tokens.
- **Preview fetch:** the server requests the URL, reads the Open Graph title and image, and prefills the form. The thumbnail is downloaded, capped at 512 KiB, and stored on the request.
- **Failure is normal.** Some model sites block non-browser clients. When the fetch fails, the form opens with just the URL and the user types a title.
- **SSRF guard:** only `http` and `https`; resolve the host and refuse loopback, link-local and private addresses, checking again after redirects. A configured allowlist (`PREVIEW_ALLOW_HOSTS`) exempts named internal hosts such as the Manyfold instance.

## 6a. File uploads

Friends often have a model file and nowhere to host it, so a request can carry attachments alongside, or instead of, a link.

- **What:** model files (`.stl`, `.3mf`, `.step`, `.stp`, `.obj`, `.zip`) and reference images (`.png`, `.jpg`, `.jpeg`, `.webp`). The extension list is checked server-side; the stored content type comes from the extension, not from the client. At most `UPLOAD_MAX_BYTES` per file (default 200 MiB) and 20 files per request.
- **Where:** an S3-compatible bucket. In production that is a `print-dis` bucket on `https://s3.cmdcentral.xyz`, the versitygw endpoint that CNPG and Velero already write to, addressed path-style. Credentials come from OpenBao.
- **Why not ceph-rbd:** a volume makes the pod stateful (ReadWriteOnce, one replica, `Recreate` rollouts), pulls it into Velero, and breaks the "no volumes" deployment in section 8. **Why not Postgres:** 100 MB STLs in `bytea` would bloat WAL and every barman backup.
- **Backends:** a small `Blobs` interface (`Put`, `Get`, `Delete`) with two implementations, `s3` (minio-go) and `dir` (a local directory). `dir` is for local development and tests; it would also serve a PVC if that is ever wanted.
- **Through the app, not presigned:** uploads are streamed from the multipart body to the bucket, hashing as they go; downloads are streamed back with `Content-Disposition: attachment`. This keeps every permission check in one place, needs no CORS on the gateway, and means the S3 endpoint never has to be reachable by friends.
- **Permissions:** anyone who can see a request can download its files. A requester can upload to and delete their own uploads from their own request while they may edit it (`requested` or `needs_info`); an admin can do either at any time. Uploads and deletions are recorded as `file` events.
- **Consistency:** the object is written first and the row inserted after; if the insert fails the object is deleted. Deleting a file deletes the row, then the object; a failed object delete is logged, not shown to the user.
- If the bucket is unreachable the app still starts; the upload control says files are unavailable, and `/readyz` does not depend on it.

## 7. Stack and layout

- **Go**, current stable. `net/http` with the standard library router.
- **Database:** `pgx/v5`, hand-written SQL, migrations embedded and applied at startup with `goose`.
- **Sessions:** `alexedwards/scs` with its pgx store. Session tokens are random and stored server-side, so there is no signing secret to manage.
- **OIDC:** `coreos/go-oidc` and `golang.org/x/oauth2`.
- **Config:** environment variables only.

| Variable | Meaning |
|---|---|
| `DATABASE_URL` | Postgres URL. Required. |
| `BASE_URL` | Public origin, e.g. `https://prints.cmdcentral.xyz`. Required. |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | Required unless `DEV_USER` is set. |
| `ADMIN_GROUP` | Authentik group whose members are admins. |
| `SPOOLMAN_URL` | Optional. Integration is off when unset. |
| `PREVIEW_ALLOW_HOSTS` | Optional, comma-separated internal hosts the preview fetch may reach. |
| `FILES_BACKEND` | `s3`, `dir`, or empty to disable uploads. |
| `FILES_DIR` | Directory for the `dir` backend. |
| `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_REGION` | For the `s3` backend. Path-style addressing is always used. |
| `UPLOAD_MAX_BYTES` | Per-file limit. Default 209715200 (200 MiB). |
| `DEV_USER` | Local development only: skips OIDC and signs in as an admin with this email. The server refuses to start with it set unless `BASE_URL` is a localhost origin. |
| `LISTEN_ADDR` | Default `:8080`. |

```
cmd/print-dis/        main: serve, and the import-sheet subcommand
internal/store/       SQL and migrations
internal/core/        types, eligibility, permission rules (pure, no IO)
internal/web/         handlers, templates, embedded static files
internal/auth/        OIDC and sessions
internal/spoolman/    client and cache
internal/preview/     link fetch and SSRF guard
internal/files/       Blobs interface, s3 and dir backends
```

### Testing

- `internal/core` is pure and gets table-driven tests: eligibility, and every row of the permissions table in section 2.
- Store and handler tests run against a real Postgres named by `TEST_DATABASE_URL` and are skipped when it is unset. CI provides one as a service container.
- Spoolman and preview tests use `httptest` servers, including the failure paths.
- `make check` runs vet, `golangci-lint` and the tests. It must pass before any milestone is called done.

## 8. Deployment

Three repositories and one bucket are involved. Steps 2 to 4 change live infrastructure and need the owner's go-ahead at the time.

1. **This repo.** A multi-stage `Dockerfile` producing a static binary on a distroless non-root base. A GitHub Actions workflow tests, then builds and pushes `ghcr.io/bjschafer/print-dis:main` on every push to `main`. Mirror the workflow in `bjschafer/pfapi`, which is deployed the same way.
2. **`~/development/tf/authentik`.** Add an OAuth2 provider and application named `print-dis`, with redirect URI `https://prints.cmdcentral.xyz/auth/callback`. Follow that repo's `CLAUDE.md` exactly: set `grant_types` explicitly, use `local.default_scope_mappings`, and add the app to `local.kv_oauth2_clients` so the client secret is published to `kv/kubernetes/print-dis/authentik`. Add the admin group in `groups.tf`. Pushing triggers a CI apply against the live identity provider.
3. **The bucket.** Create a `print-dis` bucket and access key on versitygw, and store them in OpenBao at `kv/kubernetes/print-dis/s3`.
4. **`~/development/k8s/generators`.** Add `apps/print-dis/app.ts` using `AppPlus`, and a `printdis` entry in `apps/postgres/databases.ts` with a generated, symbol-free password (the URL-safe pattern used by `bambuddy`). Serve it at `prints.cmdcentral.xyz` only. Connect to the primary (`prod.postgres.svc.cluster.local`), not the pooler, because migrations run at boot. Run as non-root with a read-only root filesystem. There are no volumes; uploads go to the bucket via an `OpenBaoSecret` for the S3 credentials. Follow that repo's `AGENTS.md`: run `mise run pre-commit`, commit `dist/` with the source, commit to `main`.

The app is internal-only. Do not add the `cmdcentral.xyz/external` ingress label without asking.

## 9. Milestones

Each milestone ends with `make check` passing and the listed behaviour demonstrated. They are ordered so the app is deployed and in real use after milestone 3.

**M0. Reset and skeleton.** Remove the old code. Module layout from section 7, config loading, migrations runner with the full schema from section 3, `/healthz` and `/readyz`, Dockerfile, CI that tests and publishes the image, a rewritten `AGENTS.md` and `README.md`.
*Done when:* the container starts against an empty database, applies migrations, and answers both health routes.

**M1. Requests, board and printers, behind `DEV_USER`.** Everything in section 4 except auth and people, with the eligibility logic of section 3. Filament and colour are free text for now.
*Done when:* an admin can create, edit, comment on, drag and deliver a request; printers can be created with bed sizes; a request needing multi-material lists only printers that have it and explains the exclusions.

**M2. Sign-in, roles and people.** OIDC login, user creation on first sign-in, admin from the `groups` claim, the permission rules of section 2 enforced in handlers and reflected in the UI, placeholders, linking by verified email, and merge.
*Done when:* a non-admin sees and edits only their own requests and receives a 403 for anyone else's; a placeholder's requests appear for the real user after their first sign-in. Confirm that Authentik actually sends `groups` with the scopes requested, and that `email_verified` is true.

**M3. Deploy.** Section 8, steps 2 to 4.
*Done when:* the owner signs in at `https://prints.cmdcentral.xyz` through Authentik and is an admin.

**M4. Spoolman picker.** Section 5.
*Done when:* the picker shows live filaments with swatches and stock; stopping access to Spoolman degrades the form to free text without errors; an old request still displays its filament.

**M5. Link capture.** Section 6, including the bookmarklet shown on a help page.
*Done when:* a Printables link prefills title and thumbnail; a blocked or failing site still yields a usable form; requests to loopback and private addresses are refused unless allowlisted.

**M6. Sheet import.** `print-dis import-sheet <file.csv> --requester <email>`: a one-off loader for a CSV export of the existing sheet. Columns: Status, Item / model, Qty, Printed, Method, Material / color, Model / file link, Needed by, Timing, Request details. Non-numeric quantities ("2 plates", "1 of Ea") become quantity 1 with the original text prepended to the notes. Rows whose Method is "Resin" are imported with that noted. It supports `--dry-run` and is safe to re-run.
*Done when:* a dry run of the real export reports every row as mapped or explains why not.

**M7. File uploads.** Section 6a. The table and the permission rules already exist from M0 and M2; this adds the `files` package, the routes and the UI on the request page and the new-request form.
*Done when:* a requester uploads an STL to their own request and downloads it again with the same SHA-256; another requester gets a 403 for it; the 200 MiB limit and the extension list are enforced; the `s3` backend passes the same tests as `dir` against a real S3 endpoint when `TEST_S3_ENDPOINT` is set.

### Later, not planned in detail

- Notifications: email the requester on `ready` and `needs_info`; notify the owner of new requests.
- Read-only printer status from Bambuddy and Moonraker, to show what is printing now.
- A JSON API with tokens.
- A Prometheus metrics endpoint.

## 10. Open decisions

Each has a default so that work is not blocked. The owner may overrule any of them.

1. **File uploads.** Decided: yes, in an S3 bucket (section 6a). Still open: whether the versitygw bucket is itself backed up offsite, and whether files should be pruned some time after a request is delivered.
2. **Can requesters see each other's requests?** Default: no.
3. **Do friends already have Authentik accounts?** Sign-in depends on it. If not, enrollment needs sorting out in `tf/authentik` before M3 is useful to anyone but the owner.
4. **Repository and image name.** Default: keep `print-dis`.
