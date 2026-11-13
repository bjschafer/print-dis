# print-dis JSON API

A small API for scripts that file and track requests. The design notes are in `design.md` section 6b; this page is the reference.

## Authentication

Every call needs an admin's token:

```sh
export PD=https://prints.cmdcentral.xyz
export PD_TOKEN=pd_...   # Help page > API > Create token. It is shown once.
curl -H "Authorization: Bearer $PD_TOKEN" "$PD/api/v1/requests"
```

- A token acts as the admin who created it. It stops working when it is revoked on the help page, or when its owner stops being an admin.
- The session cookie is ignored. A signed-in browser gets a 401 like anyone else without a token.
- From outside the LAN the host is behind Cloudflare Access, which answers with a login page rather than JSON. Run scripts from the LAN.

## Conventions

- Bodies are JSON, with one exception: file uploads send the file itself as the body.
- Unknown fields in a body are refused with a 400, so a misspelt field never goes unnoticed.
- Errors are `{"error": "A plain sentence."}`, with these status codes:

| Code | Meaning |
|---|---|
| 400 | The body is not valid JSON for the route, or a query parameter is wrong. |
| 401 | The token is missing, unknown or revoked. |
| 403 | The token's owner is not an admin. |
| 404 | No such request or route. |
| 409 | The request already has 20 files. |
| 413 | The file is larger than the server's limit (200 MiB by default). |
| 415 | That file type is not allowed. |
| 422 | The body is understood but breaks a rule, such as a missing title. Every problem is listed in the one sentence. |
| 503 | File uploads are turned off on this server. |

## Objects

### Request

```json
{
  "id": 42,
  "url": "https://prints.cmdcentral.xyz/requests/42",
  "key": "voron-trident:STLs/Gantry/Front_Idlers/[a]_idler_carrier_a_x2.stl",
  "project": "Voron Trident",
  "title": "idler_carrier_a",
  "quantity": 2,
  "printed": 0,
  "status": "requested",
  "requester": {"id": 1, "name": "Ben"},
  "needed_by": null,
  "timing": "whenever",
  "notes": "",
  "source_url": null,
  "material": "ABS",
  "colors": [{"rough": "red", "label": "Red", "hexes": ["#d32f2f"], "note": "accent"}],
  "needs_multi_material": false,
  "needs_enclosure": true,
  "size_mm": {"x": 48, "y": 22, "z": 15},
  "printer": null,
  "created_at": "2026-11-12T21:30:00Z",
  "updated_at": "2026-11-12T21:30:00Z"
}
```

- `status` is one of `requested`, `needs_info`, `printing`, `ready`, `delivered`, `cancelled`.
- `timing` is one of `firm`, `flexible`, `whenever`. `needed_by` is a date, `YYYY-MM-DD`.
- `colors` is empty for any color. Each entry is a snapshot. It has `filament_id` (from Spoolman), `rough` (from the palette below) or `hexes` (an exact color), plus an optional `note`. `label`, `material` and `hexes` are filled in by the server.
- `size_mm` is null until it is known. When it was measured from uploads, `x` is the longer side.
- `printer` is `{"id": …, "name": …}` once one is assigned.

### File

```json
{
  "id": 7,
  "url": "https://prints.cmdcentral.xyz/files/7",
  "filename": "idler_carrier_a_x2.stl",
  "content_type": "model/stl",
  "size_bytes": 184284,
  "sha256": "9f2c…",
  "size_mm": {"x": 22, "y": 48, "z": 15},
  "created_at": "2026-11-12T21:30:01Z"
}
```

`size_mm` is the measured bounding box, as the model lies. It is null for files that are not STL, OBJ or 3MF, and for models that could not be measured. `url` downloads the file in a signed-in browser; the API does not serve file contents.

## Routes

### `GET /api/v1/requests`

Lists requests, oldest first, including delivered and cancelled ones. All parameters are optional:

| Parameter | |
|---|---|
| `project` | Only this project. |
| `key` | Only the request filed with this key. |
| `status` | Only these statuses. Repeat it for several: `?status=requested&status=printing`. |

```sh
curl -H "Authorization: Bearer $PD_TOKEN" "$PD/api/v1/requests?project=Voron%20Trident&status=requested"
```

### `POST /api/v1/requests`

Files a request. Only `title` is required.

| Field | Type | |
|---|---|---|
| `title` | string | At most 200 characters. |
| `key` | string | Optional. Your own identifier for the request, at most 300 bytes. If a request already has this key, the reply is **200 with that request** instead of 201, and nothing changes. This lets you re-run a script. |
| `project` | string | Optional. At most 80 characters. Must not be `-` and must not start with `!`. |
| `quantity` | integer | 1 to 10000. Defaults to 1. |
| `requester_id` | integer | Optional. Who it is for. Defaults to the token's owner. |
| `needed_by` | string | Optional, `YYYY-MM-DD`. |
| `timing` | string | `firm`, `flexible` or `whenever` (the default). |
| `notes` | string | Optional. |
| `source_url` | string | Optional. An `http` or `https` link to the model. |
| `material` | string | Optional, such as `ABS` or `PETG`. Upper-cased. Empty means any material. When it is empty and colors name Spoolman filaments, it is taken from them. |
| `colors` | array | Optional, at most 8. Each entry is `{"rough": "red"}`, `{"hex": "#1e66d0"}` or `{"filament_id": 12}`, optionally with `"note"`, or just `{"note": "glow in the dark"}`. A filament's name and color come from Spoolman. |
| `needs_enclosure` | boolean | Defaults to true when the material is ABS, ASA, PC or PA (including variants like `ABS-GF`). |
| `needs_multi_material` | boolean | Defaults to true when two or more colors are given. |
| `size_mm` | object | Optional `{"x": …, "y": …, "z": …}`. Leave it out and upload models instead, and it is measured. |

Rough colors: `red`, `orange`, `yellow`, `green`, `teal`, `blue`, `purple`, `pink`, `brown`, `black`, `grey`, `white`, `silver`, `gold`, `clear`.

```sh
curl -H "Authorization: Bearer $PD_TOKEN" "$PD/api/v1/requests" -d '{
  "key": "voron-trident:STLs/Gantry/Front_Idlers/[a]_idler_carrier_a_x2.stl",
  "project": "Voron Trident",
  "title": "idler_carrier_a",
  "quantity": 2,
  "material": "ABS",
  "colors": [{"rough": "red", "note": "accent"}],
  "notes": "STLs/Gantry/Front_Idlers"
}'
```

Replies 201 with the request and a `Location` header, or 200 when the key was already used.

### `GET /api/v1/requests/{id}`

Replies with one request.

### `GET /api/v1/requests/{id}/files`

Replies with the request's files, oldest first. You can compare `sha256` against a local file before uploading it again.

### `POST /api/v1/requests/{id}/files?name={filename}`

Uploads one file. Send the raw file as the body. `name` gives the filename, and its extension decides the type. Allowed: `.stl`, `.3mf`, `.step`, `.stp`, `.obj`, `.zip`, `.png`, `.jpg`, `.jpeg`, `.webp`. STL, OBJ and 3MF files are measured. If the request has no size yet, it is set from the model. A request holds at most 20 files.

```sh
curl -H "Authorization: Bearer $PD_TOKEN" --data-binary @idler_carrier_a_x2.stl \
  "$PD/api/v1/requests/42/files?name=idler_carrier_a_x2.stl"
```

Replies 201 with the file. Uploading the same file twice stores it twice. Check `GET …/files` first if your script may re-run.

### `POST /api/v1/requests/{id}/status`

```sh
curl -H "Authorization: Bearer $PD_TOKEN" "$PD/api/v1/requests/42/status" -d '{"status": "printing"}'
```

Replies with the request. The change is recorded in the request's history, the same as a change in the browser. Setting the status it already has does nothing and replies 200.

### `POST /api/v1/requests/{id}/printed`

```sh
curl -H "Authorization: Bearer $PD_TOKEN" "$PD/api/v1/requests/42/printed" -d '{"printed": 2}'
```

Sets how many copies are printed (zero or more), and replies with the request. The status does not change on its own; move it to `ready` with the status route.

## Example: queueing a Voron build

The Voron repositories name parts in a consistent way. Under `STLs/`, the folders are the subassemblies. A name starting with `[a]_` is an accent-color part, and `_xN` before the extension is the quantity. A script that walks a checkout can file one request per part:

- Use the path as the `key`, prefixed with the printer (`voron-trident:STLs/…`). Re-running after a `git pull` then files only new parts.
- Use one `project` for the whole build, so the board can show only it, or everything but it.
- Set `title` from the filename without `[a]_` and `_xN`, `quantity` from `_xN`, and the color from `[a]_`: `{"rough": "red", "note": "accent"}` versus `{"rough": "black", "note": "primary"}`, or `filament_id`s from Spoolman.
- Use `material: "ABS"` (or ASA), which turns on `needs_enclosure`.
- Skip `superseded_parts/`, and leave out whichever of each alternative pair you are not printing (such as `xy_right.stl` versus `xy_right_d2f.stl`).
- Upload each STL only when the reply was 201. Its measured size then drives printer eligibility.
