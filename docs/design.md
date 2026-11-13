# print-dis v2: design and build plan

Status: accepted, 2026-11-11. Open decision 1 is overruled: friends can upload files (section 6a).

Progress: M0 to M8 are built and pass `make check`, and so is the JSON API (section 6b). Deployed 2026-11-11 at https://prints.cmdcentral.xyz (tf/authentik `fd6ad20`, k8s/generators `4d57a247`, bucket `print-dis` on versitygw).

This is a ground-up rewrite in the existing repository. The current code (Go, static HTML, SQLite/Postgres, local password accounts) is replaced, not extended; only `internal/spoolman/types.go` is worth carrying over.

## 1. What this is for

One person runs three FDM printers (a Bambu H2D and two Klipper Vorons: a Trident 350 and a Pandora's Box) and prints for friends as well as for themselves. Requests currently live in a shared Google Sheet, in open browser tabs, and in memory.

print-dis is the single queue for all of it. A friend signs in, files a request, and can see and edit it. The owner sees everything on a board, knows what each request needs (filament, deadline, which printers can do it), and moves it along.

Two off-the-shelf options were tried or considered and rejected, which is why this exists:

- **STL Quest** (trialled): no filament or material field, no due dates, and printer matching that only knows bounding boxes. Its automation did not help a small mixed fleet.
- **Generic tables (NocoDB, Grist)**: would hold the columns, but cannot offer a filament picker fed by Spoolman or printer-capability matching.

Those two things, plus one-click capture of a model link, are the reason to build. Everything else should stay as small as possible.

### Non-goals

- Controlling printers, slicing, queueing jobs on printers, or estimating print time. Live status is read-only (section 5a).
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
  material              text not null default '',   -- "PETG", "PLA + TPU", or empty for "any"
  colors                jsonb not null default '[]', -- ordered color choices; empty for "any" (section 5)
  needs_multi_material  boolean not null default false,
  needs_enclosure       boolean not null default false,
  size_x_mm             integer,
  size_y_mm             integer,
  size_z_mm             integer,

  printer_id            bigint references printers(id),
  project               text not null default '',  -- groups requests, e.g. the parts of one build; '' for none
  external_key          text unique,             -- set by the script that filed it (sheet import, API), for re-runs
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
  size_x_mm       integer,                -- the model's bounding box, when it could be measured (section 6a)
  size_y_mm       integer,
  size_z_mm       integer,
  created_at      timestamptz not null default now()
);

create table api_tokens (
  id            bigint generated always as identity primary key,
  user_id       bigint not null references users(id) on delete cascade,
  name          text not null,
  token_hash    bytea not null unique,   -- SHA-256 of the token; the token is shown once and never stored
  created_at    timestamptz not null default now(),
  last_used_at  timestamptz
);
```

Notes on the choices:

- **Statuses match the existing sheet** (Requested, Needs info, Printing, Ready for pickup, Delivered) plus `cancelled`. `delivered` and `cancelled` are terminal and hidden from the board.
- **Colors are a list of snapshots.** A request lists zero or more colors (none means any color), each one of: a filament picked from Spoolman, a rough color from a fixed palette, an exact hex color, or just a note. The Spoolman ID is kept for linking, but the label, material and color are copied at pick time so a request still reads correctly after that filament is deleted or Spoolman is down. The list is a jsonb column rather than a table because it is only ever read and written with its request (migration `00005` moved the old single-filament columns into it).
- **`printed_count` is the whole of progress tracking.** "3 of 10 printed" is enough; copies do not have individual states.
- **`request_events` is both the comment thread and the audit trail.** A "needs info" exchange happens there.
- **Projects are a plain text column**, not a table: a project is only ever a label that requests share. Only admins set or see it. It exists for builds like a Voron, whose hundred-odd parts would otherwise bury the board; the board and table filter to one project, to none, or to everything but one. The filter's value is the name, `-` for none, or `!` and the name to leave one out, so no project may be called `-` or start with `!`.

### Printer eligibility

A pure function, `Eligible(request, printers) []Printer`, with no database access. A printer is eligible when all of these hold:

1. It is active.
2. The request does not need multi-material, or the printer has it.
3. The request does not need an enclosure, or the printer has one.
4. The request has no size, or the size fits the bed. X and Y may be swapped (rotation about Z); Z may not.

The request form pre-ticks "needs enclosure" when the material, or a picked filament's material, is ABS, ASA, PC or PA, and pre-ticks "several colors or materials in one print" (needs multi-material) once two colors are picked. The user can untick either; two colors can also mean separate parts.

The request page lists eligible printers and says why each of the others was excluded. Once a printer is assigned, it is shown on its own and the others fold into a collapsed "Other printers" list with a count of how many are eligible. Assigning an ineligible printer is allowed with a visible warning, because the owner sometimes knows better.

## 4. Screens

Server-rendered HTML with `html/template`, htmx for partial updates, and SortableJS for dragging cards. Both scripts are vendored into the repo and embedded in the binary; there is no Node toolchain and no build step for the frontend. It must be usable on a phone.

The look borrows from printer UIs like Fluidd and Bambuddy: dark by default (light when the OS asks for it), a nav rail that becomes a bottom tab bar on phones, and panels with header strips. Board columns are lanes tinted with their status color, with cards as lighter raised tiles on them; a card is striped with its colors, top to bottom, when they are known. A firm date due within three days is a solid orange pill, the card's only alarm. Printer tiles above the board are a small dashboard of flat panels with header strips: color only in the state badge and progress bar, and progress in large type. No decorative gradients. The accent color comes from `ACCENT_COLOR`. Icons are an inline SVG sprite in `layout.html`; all styling is one hand-written `app.css`. Only admins can drag cards, so only their cards lift on hover and show a grab cursor. Avatars show a person's initials (first and last word of the name) on a hue chosen by a hash of their name, so people are told apart at a glance.

| Route | Purpose |
|---|---|
| `GET /` | Board. Columns: Requested, Needs info, Printing, Ready. Admin sees all requests and can drag between columns; requesters see their own, read-only. Cards show title, requester, quantity and progress, material and color swatches, needed-by date, assigned printer and, for the admin, project. A Ready card has a "Delivered" button for the admin, which takes it off the board in place (no confirmation: a delivered request is still in the table and can be moved back from its page). Firm dates that are overdue or within three days are highlighted. Filters: requester, printer, material, and for the admin project (section 3). Any column can be collapsed to a thin lane (it still takes drops); which ones is remembered per browser in the `board_collapsed` cookie, so the server renders it collapsed. The board keeps itself current: every 20 seconds, and when its tab comes back into view, it fetches `GET /board/version` with its own filters, and when that has changed it swaps in a freshly rendered board (not during a drag or a Delivered post, and not while a button on the board has focus; it waits for the next poll). Cards that arrived since are outlined in the accent color until the board has been in view for a few seconds, and while the tab is in the background its title counts them, `(2) Board · print-dis`. A lapsed session makes the poll reload the page, which signs in as usual. |
| `GET /board/version` | A short fingerprint of the cards the board would show this viewer under the given filters, polled by the board. |
| `GET /requests` | Table of everything, including delivered and cancelled. Sortable; the same filters. |
| `GET /requests/new` | New request form. Accepts `?url=` to prefill from a link (section 6), or `?from={id}` to clone a request (below). Admins get a "for" field to pick or create a person. |
| `GET /requests/{id}` | Detail: all fields, eligible printers, comment thread, history. Edit in place, subject to section 2. A Clone button opens the new-request form copied from it. |
| `POST /requests/{id}/status` | Status change (admin; requesters may only cancel). Records an event. |
| `POST /requests/{id}/move` | Reorder within or across columns from a drag. |
| `POST /requests/{id}/comments` | Add a comment. |
| `POST /requests/{id}/files` | Upload one or more files (section 6a). |
| `GET /files/{id}` | Download a file, subject to the same visibility as its request. |
| `POST /files/{id}/delete` | Remove a file. |
| `POST /files/{id}/size` | Set the request's size from a measured model file (section 6a). |
| `GET /printers`, `POST /printers...` | Admin: printer list and editing, including bed dimensions and the live status source. |
| `GET /printers/status` | Admin: the board's live status strip, polled by htmx (section 5a). |
| `POST /printers/{id}/clear` | Admin: mark a printer's plate cleared after its last job ended (section 5a). |
| `POST /printers/bambuddy` | Admin: save the Bambuddy connection (section 5a). |
| `GET /printers/{id}/camera`, `GET /printers/{id}/camera/stream` | Admin: a printer's live camera, and the relayed MJPEG stream behind it (section 5a). |
| `GET /people` | Admin: users and placeholders; create a placeholder; merge two rows. |
| `GET /help` | How to file a request, the bookmarklet, and for the admin, API tokens (section 6b). |
| `POST /tokens`, `POST /tokens/{id}/delete` | Admin: create an API token, shown once on the help page; revoke one. |
| `/api/v1/...` | The JSON API (section 6b). |
| `GET /auth/login`, `/auth/callback`, `POST /auth/logout` | OIDC code flow with PKCE. |
| `GET /healthz`, `GET /readyz` | Liveness, and readiness including a database ping. |

Every state-changing route requires a CSRF token, except the API's, which never read the session cookie (section 6b).

**Cloning and batches.** Friends often want the same thing in several colors, each with its own count, and the owner tracks each color separately. Two shortcuts file those as separate requests:

- **Clone** (`?from={id}`, open to anyone who can see that request): the form is prefilled with its title, link, material, colors, size, needs, needed-by, timing and notes; an admin's clone keeps the requester. Progress, status and printer start afresh. Its files are listed with checkboxes, all ticked, and the ticked ones are copied (a new object each, keeping their measured size). The thumbnail is copied while the link is unchanged. The new request records "cloned this from #N".
- **One request per color**: on the new-request form only, with two or more colors, a "File each color as its own request" checkbox shows a count per color. Filing creates one request per color, in one transaction, each with that color alone and its count (a blank count uses "How many?"); multi-material is off, and when no material was chosen each takes its own filament's. Uploads go to the first and are copied to the rest. The form then returns to the board.

## 5. Spoolman integration

Read-only, against the in-cluster service `http://spoolman.spoolman.svc.cluster.local:8000`.

- Fetch `GET /api/v1/filament` and `GET /api/v1/spool`, and cache both in memory for five minutes.
- The form asks for a material first, as chips: Any, the common ones (PLA, PETG, ABS, ASA, TPU), every material Spoolman has, and Other with a text box. Someone who only cares about material picks it and leaves color on Any.
- Then one or more colors, each with: a row of rough palette colors (`core.RoughColors`) plus Any and an exact color picker; a collapsible "filament on the shelf" grid of swatch tiles, grouped by material, in-stock first, with grams left, filtered by the chosen material and a search box; and a free-text note. "Add another color" adds a slot (up to eight); without script it re-renders the form with a blank slot instead of saving.
- Multi-color filaments (`multi_color_hexes`) show as a pie swatch.
- Picking a filament snapshots its ID, label, material and colors; the client's values for those are never read. When the request names no material, the picked filaments' material is used.
- On a request page, the owner sees "On the shelf": the in-stock filaments closest in color to each rough or exact color asked for, in the requested material. Inventory is often out of date, so rough colors are first-class and these are suggestions only.
- If Spoolman is unreachable, the form offers the material chips and rough colors and says the filament list is unavailable. The app must start and work without Spoolman.

print-dis never writes to Spoolman.

## 5a. Live printer status

Read-only status from Moonraker (the Vorons) and Bambuddy (the H2D), so the board shows what each printer is doing now. print-dis never starts, stops or queues a print: requests carry unsliced models, and choosing how to print one stays in the slicer.

- **Configuration, all in the app:** each printer has at most one source, chosen on the printers page with a None / Moonraker / Bambuddy switch that shows only the chosen source's field. Moonraker takes an address (`http://trident.lan:7125`). Bambuddy is a dropdown of the printers Bambuddy lists (`GET /api/v1/printers/`, cached for a minute), falling back to typing the id when Bambuddy cannot be reached. The Bambuddy connection (its address, and an API key when its sign-in is on) is a panel on the same page, stored in a one-row `settings` table. The key needs only Bambuddy's "read status" permission; untick the others. It is never sent back to the browser: a blank key field keeps the saved one, and a checkbox removes it. It sits in Postgres, and so in its backups, which is acceptable for a read-only key. Moonraker has no key here, so print-dis's pod addresses must be in Moonraker's `trusted_clients`.
- **Polling:** a background monitor (`internal/printerstatus`) reads every active printer with a source every 15 seconds, concurrently, with a 5-second timeout each, and keeps the latest reading in memory. It re-reads the printers and settings each round, so edits apply without a restart. Moonraker: `GET /printer/objects/query?print_stats&virtual_sdcard&display_status`. Bambuddy: `GET /api/v1/printers/{id}/status`.
- **States:** idle, printing (including preparing), paused, finished, failed, cancelled, offline, and unknown. A reading has the job name, progress, time left and layers when the source gives them. Moonraker's time left is extrapolated from progress and elapsed time; Bambuddy's comes from the printer.
- **When a job ended:** both sources keep reporting finished (or failed, cancelled) until the next job, so a tile says how long ago it ended ("Finished 3h ago") as a cue that a plate is waiting. The time is when the monitor saw the state change, kept in memory, and survives the printer dropping offline briefly. When print-dis did not see the change (the state already held at its first reading after a restart, or the job ended while the printer was offline), it shows no time rather than a guess.
- **Plate cleared:** a tile showing an ended job has a "Plate cleared" button. From then until the printer reports a different state, it reads as idle with no job. This is remembered in memory only and changes nothing on the printer; after a restart the tile shows the ended job again.
- **Where it shows:** a strip of printer tiles above the board, which refreshes itself and the cards every 15 seconds via htmx. Each card in Printing that has an assigned printer shows that printer's state and progress (the printer's, which may be another request's job if two share it). The request page shows each printer's state beside its eligibility, to help pick a free one. The printers page shows it on each printer.
- **Camera:** each printer tile has a camera button that opens a live view in a dialog (or, without JavaScript, the camera page). The stream is relayed by print-dis, so credentials stay on the server and the browser only talks to print-dis. Moonraker: the first enabled webcam in `/server/webcams/list`. Its stream address is tried as MJPEG first, whatever the service type says (Fluidd's "MJPEG Adaptive" is an ordinary mjpg-streamer); when that is not MJPEG (WebRTC, HLS), it becomes a one-frame-a-second stream from the snapshot address. The camera button shows only when the printer has a camera: Moonraker's webcam list is re-read every five minutes, and a Bambuddy printer has one while connected. Relative webcam addresses resolve against the printer's web server, the Moonraker host without Moonraker's port. The webcam's rotation and flips are applied in CSS. Bambuddy: a stream token from `POST /api/v1/printers/camera/stream-token` (reused for 50 minutes; the read-status key is enough), then `/api/v1/printers/{id}/camera/stream`. A relay ends when the viewer closes it, or after 30 minutes.
- **Admins only.** A job name can belong to someone else's request, and a camera shows the room, so requesters see none of it.
- **Failure:** an unreachable source or a printer Bambuddy is not connected to reads as offline; one failure is logged when a printer goes offline, not every poll. Nothing else depends on it, and the app starts without either service.
- **Not done:** counting a finished job towards `printed_count` automatically. Matching a file name to a request is guesswork; the owner still updates the count.

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
- **Through the app, not presigned:** uploads are streamed from the multipart body to the bucket in 8 MiB parts (the part size must be set: for a stream of unknown length minio-go otherwise buffers ~537 MiB per upload), hashing as they go; downloads are streamed back with `Content-Disposition: attachment`. This keeps every permission check in one place, needs no CORS on the gateway, and means the S3 endpoint never has to be reachable by friends.
- **Permissions:** anyone who can see a request can download its files. A requester can upload to and delete their own uploads from their own request while they may edit it (`requested` or `needs_info`); an admin can do either at any time. Uploads and deletions are recorded as `file` events.
- **Consistency:** the object is written first and the row inserted after; if the insert fails the object is deleted. Deleting a file deletes the row, then the object; a failed object delete is logged, not shown to the user.
- **On the new-request form too:** files can be attached before the request is first saved. The form is then multipart with the file input last; the handler reads the fields from the stream, creates the request, and streams the files after it, so nothing is spooled to disk. If the form has an error the files are dropped and the form says to choose them again.
- **Progress:** with JavaScript, a form with files chosen is sent by XHR with `HX-Request`, so the page shows a percentage while the bytes go up and then that the server is saving them (the write to versitygw is the slow part: seconds even for a tiny file, since it syncs to disk). The server's answer to `HX-Request` is an `HX-Redirect` on success; a re-rendered form with errors replaces the page, and a plain-text refusal shows in place of the progress. Without JavaScript the form posts normally.
- **Measuring:** each STL (binary or ASCII), OBJ or 3MF is read back after upload and its bounding box stored on the file (`internal/meshsize`), assuming millimetres where the format has no unit. A 3MF applies its build and component transforms, including the production extension's parts in other files, and measures each build item separately: its size is a box every part fits in, since parts laid out on one plate can be printed separately. A 3MF is read by random access from the blob store (ranged reads on S3), not buffered; decompression is capped at 1 GiB. STEP and ZIP are not measured. When a request has no size, uploading measurable models sets it (the box all of them fit in, X the longer side) and records an edit event; a size already typed in is never overwritten, but a file's "Use size" button replaces it. A file that cannot be measured is logged and otherwise ignored.
- **Older uploads:** files uploaded before measuring existed are sized by a one-off `print-dis measure-files`, run in the server's pod (`kubectl -n print-dis exec deploy/print-dis -- /print-dis measure-files`). It measures every stored model with no size, then sizes each of those requests that has none, as the upload would have, recording the edit as made by the uploader. It is safe to re-run; a model that cannot be measured is logged and retried next time. There is no button, since every new upload is measured.
- If the bucket is unreachable the app still starts; the upload control says files are unavailable, and `/readyz` does not depend on it.

## 6b. JSON API

So a script can file and track requests: the motivating case is walking the Voron repository's `STLs/` tree and filing one request per part, so none is forgotten. The reference, with every field and status code, is `docs/api.md`.

- **Admins only.** A token acts as the admin who made it, and stops working if they stop being an admin. Tokens are made, listed (with when each was last used) and revoked on the help page. A new token is shown once, carried to that page in the session so a reload cannot mint another, and only its SHA-256 is stored. Tokens look like `pd_` and 43 URL-safe characters, so a secret scanner can spot them.
- **Bearer tokens, never cookies.** The API is mounted outside the session middleware and authenticates only by `Authorization: Bearer`. A browser cannot be made to send that header cross-site, so there is no CSRF token; a signed-in admin's cookie gets a 401.
- **Routes**, all JSON, errors as `{"error": "a plain sentence"}`:
  - `GET /api/v1/requests`, filtered by `project`, `key` and repeated `status`.
  - `POST /api/v1/requests`: file one. It is checked by the same code as the request form, by encoding the JSON as the form's fields, so the rules cannot drift and a Spoolman filament's label and color still come from Spoolman. Unknown fields are refused, so a misspelling is an error. "Needs an enclosure" defaults from the material and "multi-material" from having two or more colors, as the form pre-ticks them; an explicit `false` wins.
  - `GET /api/v1/requests/{id}`, and `GET .../files`.
  - `POST .../files?name=part.stl` with the file as the raw body: the same type and size limits, and the same measuring and sizing, as a browser upload.
  - `POST .../status` with `{"status": ...}`, and `POST .../printed` with `{"printed": n}`. Setting what is already there answers 200 and records nothing.
- **Re-runs.** A request may carry a `key`, unique across requests (the sheet import's keys live in the same column). Filing with a key already in use answers 200 with the existing request instead of 201, so a script can be run again after adding parts and files only what is new.
- **Remote use.** Off the LAN, `prints.cmdcentral.xyz` is behind Cloudflare Access (section 8, step 5), which would answer a script with its login page. Scripts run from the LAN, where the host resolves to traefik; using it remotely would need an Access service token, which is not set up.

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
| `ACCENT_COLOR` | Optional hex color for the UI accent, e.g. `#3b82f6`. Default `#00ae42`. Text on it is picked black or white for contrast. |
| `FILES_BACKEND` | `s3`, `dir`, or empty to disable uploads. |
| `FILES_DIR` | Directory for the `dir` backend. |
| `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_REGION` | For the `s3` backend. Path-style addressing is always used. |
| `UPLOAD_MAX_BYTES` | Per-file limit. Default 209715200 (200 MiB). |
| `DEV_USER` | Local development only: skips OIDC and signs in as an admin with this email. The server refuses to start with it set unless `BASE_URL` is a localhost origin. |
| `LISTEN_ADDR` | Default `:8080`. |

```
cmd/print-dis/        main: serve, and the import-sheet and measure-files subcommands
internal/store/       SQL and migrations
internal/core/        types, eligibility, permission rules (pure, no IO)
internal/web/         handlers, templates, embedded static files
internal/auth/        OIDC and sessions
internal/spoolman/    client and cache
internal/preview/     link fetch and SSRF guard
internal/printerstatus/  Moonraker and Bambuddy polling
internal/files/       Blobs interface, s3 and dir backends
internal/meshsize/    bounding boxes of STL, OBJ and 3MF files
```

### Testing

- `internal/core` is pure and gets table-driven tests: eligibility, and every row of the permissions table in section 2.
- Store and handler tests run against a real Postgres named by `TEST_DATABASE_URL` and are skipped when it is unset. CI provides one as a service container.
- Spoolman, preview and printer status tests use `httptest` servers, including the failure paths.
- `make check` runs vet, `golangci-lint` and the tests. It must pass before any milestone is called done.

## 8. Deployment

Four repositories and one bucket are involved. Steps 2 to 6 change live infrastructure and need the owner's go-ahead at the time.

1. **This repo.** A multi-stage `Dockerfile` producing a static binary on a distroless non-root base. A GitHub Actions workflow tests, then builds and pushes `ghcr.io/bjschafer/print-dis:main` on every push to `main`. Mirror the workflow in `bjschafer/pfapi`, which is deployed the same way.
2. **`~/development/tf/authentik`.** Add an OAuth2 provider and application named `print-dis`, with redirect URI `https://prints.cmdcentral.xyz/auth/callback`. Follow that repo's `CLAUDE.md` exactly: set `grant_types` explicitly, use `local.default_scope_mappings`, and add the app to `local.kv_oauth2_clients` so the client secret is published to `kv/kubernetes/print-dis/authentik`. Add the admin group in `groups.tf`. Pushing triggers a CI apply against the live identity provider.
3. **The bucket.** Create a `print-dis` bucket and access key on versitygw, and store them in OpenBao at `kv/kubernetes/print-dis/s3`.
4. **`~/development/k8s/generators`.** Add `apps/print-dis/app.ts` using `AppPlus`, and a `printdis` entry in `apps/postgres/databases.ts` with a generated, symbol-free password (the URL-safe pattern used by `bambuddy`). Serve it at `prints.cmdcentral.xyz` only. Connect to the primary (`prod.postgres.svc.cluster.local`), not the pooler, because migrations run at boot. Run as non-root with a read-only root filesystem. There are no volumes; uploads go to the bucket via an `OpenBaoSecret` for the S3 credentials. Follow that repo's `AGENTS.md`: run `mise run pre-commit`, commit `dist/` with the source, commit to `main`.

5. **Remote access.** `prints.cmdcentral.xyz` is routed through the `cmdcentral` Cloudflare tunnel (`~/development/tf/cloudflare`, `tunnels.tf`) behind a Cloudflare Access application that admits any Authentik login, with 30-day sessions to match the app's so htmx requests do not meet an Access re-login. LAN clients resolve the host to traefik and skip Cloudflare. The ingress carries the `cmdcentral.xyz/external` label. Remote uploads pass through Cloudflare, whose request-body limit for the zone (`max_upload`, Enterprise) is 500 MB, above `UPLOAD_MAX_BYTES`. A single form with several files counts as one request against it.
6. **Who may sign in.** In `tf/authentik`, the print-dis application is bound to the `print-dis users` and `print-dis admins` groups; anyone else is refused by Authentik before the app sees them. Friends without an account are invited: Directory > Invitations > Create, flow `print-dis registration`, single use, with an expiry. That flow refuses to run without an invitation, verifies the email address, adds the new user to `print-dis users` and lands them on the board. Membership of `print-dis users` is seeded in Terraform and otherwise managed in the Authentik UI. Invite a friend who already has a placeholder with the placeholder's email, so their requests link on first sign-in.

## 9. Milestones

Each milestone ends with `make check` passing and the listed behaviour demonstrated. They are ordered so the app is deployed and in real use after milestone 3.

**M0. Reset and skeleton.** Remove the old code. Module layout from section 7, config loading, migrations runner with the full schema from section 3, `/healthz` and `/readyz`, Dockerfile, CI that tests and publishes the image, a rewritten `AGENTS.md` and `README.md`.
*Done when:* the container starts against an empty database, applies migrations, and answers both health routes.

**M1. Requests, board and printers, behind `DEV_USER`.** Everything in section 4 except auth and people, with the eligibility logic of section 3. Filament and color are free text for now.
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

**M8. Live printer status.** Section 5a.
*Done when:* a printer with a Moonraker address or Bambuddy id shows its state, job and progress on the board, and its Printing cards follow along without a reload; an unreachable source reads as offline without slowing pages; requesters see none of it.

### Later, not planned in detail

- Notifications: email the requester on `ready` and `needs_info`; notify the owner of new requests.
- A Prometheus metrics endpoint.

## 10. Open decisions

Each has a default so that work is not blocked. The owner may overrule any of them.

1. **File uploads.** Decided: yes, in an S3 bucket (section 6a). Still open: whether the versitygw bucket is itself backed up offsite, and whether files should be pruned some time after a request is delivered.
2. **Can requesters see each other's requests?** Default: no.
3. **Do friends already have Authentik accounts?** Decided: some do; the rest are invited (section 8, step 6).
4. **Repository and image name.** Default: keep `print-dis`.
