# API

JSON over HTTP under `/api/v1`. Authenticate with the session cookie, a personal token (`Authorization: Bearer pdw_…`, Settings → API & tokens), a Pocket Core token, or a Pocket Core app link (`pcx_…`, scopes below). Errors are `{"error": "…"}`. Everything is per person: another person's ids answer 404.

## Drawings

A drawing has a `kind`: `sketch` (an Excalidraw scene, JSON), `mermaid` (Mermaid source text) or `drawio` (draw.io XML). `data` is that content; lists leave it out.

| | |
|---|---|
| `GET /drawings?view=recent\|starred\|trash&kind&folder&q&sort=edited\|created\|title&limit&offset` | `{drawings, total}`. `folder=none` = not in a folder. `q` matches the name, the folder and the words on the drawing |
| `POST /drawings` | `{kind, title?, data?, preview?, folder_id? \| folder? (a name, made when missing), starred?}` → 201 |
| `GET /drawings/{id}` | One drawing with `data`. `?open=1` counts as opening it (Recent) |
| `PUT /drawings/{id}` | Any of `{title, folder_id, starred, data, preview, preview_rev}`. With `data` send `rev` (the rev it was made on): an older one answers 409 `{error, rev}` and nothing is lost |
| `DELETE /drawings/{id}` | To the trash; `?forever=1` removes a trashed one for good |
| `POST /drawings/{id}/restore` · `/duplicate` | Out of the trash · a copy |
| `GET /drawings/{id}/preview.svg` | The picture (made by the browser on save; served with a CSP that runs nothing) |
| `GET /drawings/{id}/file` | The file other tools open: `.excalidraw`, `.mmd`, `.drawio` |
| `POST /drawings/{id}/files` | Copy it to Pocket Files now → `{ok, path, open_url}` or `{ok:false, error}` |
| `GET /drawings/{id}/versions` | Kept versions, newest first (one per ten minutes of editing, and before a restore) |
| `GET /drawings/{id}/versions/{vid}` · `/preview.svg` · `POST …/restore` | One version · its picture · put it back (the current state is kept as a version) |
| `POST /trash/empty` | `{deleted}` |

## Folders, library, overview

| | |
|---|---|
| `GET /folders` · `POST /folders {name}` · `PUT /folders/{id} {name?, sort?}` · `DELETE /folders/{id}` | Deleting a folder keeps its drawings |
| `GET /library` · `PUT /library {items}` | The person's Excalidraw library (shapes) |
| `GET /dashboard` | `{counts: {all, starred, recent, trash, no_folder, bytes, kinds}, folders, recent}` — for Homepage |
| `GET /day/YYYY-MM-DD` | `{made, changed}` that day (what Pocket Day reads) |

## Import and export

| | |
|---|---|
| `POST /import?name=…&folder_id=…` | The file as the body (or multipart `file`): a Pocket Draw export, `.excalidraw`, `.excalidrawlib`, `.drawio`/`.xml`, `.mmd`, Markdown with ```` ```mermaid ```` blocks, or a Mermaid Live link → `{kind, counts, drawings}` |
| `GET /export.json` · `GET /export.csv` | Everything (re-importable, by `uid`) · a list |

## AI

`POST /ai/diagram {prompt, code?, type?}` → `{ok, title, code}` (drafts a text diagram, or changes `code` as asked) · `POST /ai/fix {code, error}` → `{ok, code, change}` · `GET /ai/status`.

## Pocket Core app links

| Scope | Allows |
|---|---|
| `drawings:read` | `GET /dashboard`, `/drawings`, `/folders`, `/drawings/{id}` (+ `/preview.svg`, `/file`), `/day/{date}` |
| `drawings:write` | `POST /drawings`; `PUT`/`DELETE /drawings/{id}` only for drawings that app added (`source` = its slug) |

Events on the bus: `draw.created`, `draw.edited` (at most once per ten minutes of editing, and on restore), `draw.deleted`; each carries `user_email`.
