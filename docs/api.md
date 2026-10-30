# API

JSON over HTTP under `/api/v1`. Authenticate with the session cookie, a personal token (`Authorization: Bearer pdw_…`, Settings → API & tokens), a Pocket Core token, or a Pocket Core app link (`pcx_…`, scopes below). Errors are `{"error": "…"}`. Everything is per person: another person's ids answer 404, unless that drawing is shared with you (below).

## Drawings

A drawing has a `kind`: `sketch` (an Excalidraw scene, JSON), `mermaid` (Mermaid source text) or `drawio` (draw.io XML). `data` is that content; lists leave it out.

| | |
|---|---|
| `GET /drawings?view=recent\|starred\|shared\|trash&kind&folder&q&sort=edited\|created\|title&limit&offset` | `{drawings, total}`. `folder=none` = not in a folder. `q` matches the name, the folder and the words on the drawing |
| `POST /drawings` | `{kind, title?, data?, preview?, folder_id? \| folder? (a name, made when missing), starred?}` → 201 |
| `GET /drawings/{id}` | One drawing with `data`. `?open=1` counts as opening it (Recent). `access` is `owner`, `edit` or `view`; a shared one also has `owner_name`, your own one `shares` (how many) |
| `PUT /drawings/{id}` | Any of `{title, folder_id, starred, data, preview, preview_rev}`. With `data` send `rev` (the rev it was made on): an older one answers 409 `{error, rev}` and nothing is lost |
| `DELETE /drawings/{id}` | To the trash; `?forever=1` removes a trashed one for good |
| `POST /drawings/{id}/restore` · `/duplicate` | Out of the trash · a copy |
| `GET /drawings/{id}/preview.svg` | The picture (made by the browser on save; served with a CSP that runs nothing) |
| `GET /drawings/{id}/file` | The file other tools open: `.excalidraw`, `.mmd`, `.drawio` |
| `GET /drawings/{id}/used` | Where it is shown: `{page_url, picture_url, markdown, sources: [{app, name, connected, detail, items: [{kind, title, detail, url, modified}], open_url}]}` — asked of Pocket Notes and Pocket Projects as the person (their `/mentions`) |
| `POST /drawings/{id}/files` | Copy it to Pocket Files now → `{ok, path, open_url}` or `{ok:false, error}` |
| `GET /drawings/{id}/versions` | Kept versions, newest first (one per ten minutes of editing, and before a restore) |
| `GET /drawings/{id}/versions/{vid}` · `/preview.svg` · `POST …/restore` | One version · its picture · put it back (the current state is kept as a version) |
| `POST /trash/empty` | `{deleted}` |

## Sharing and live sessions

A drawing can be shared with other people on this box, to `edit` or to `view`. Those it is shared with open it, its picture and its file; with `edit` they may change its `data` and `preview` (nothing else). It is listed for them under `view=shared`. In the trash it is the owner's alone.

| | |
|---|---|
| `GET /people` | Everyone else here `[{id, name, email}]`, to share with |
| `GET /drawings/{id}/shares` | Who it is shared with `[{user_id, name, email, role, here}]` (the owner asks) |
| `PUT /drawings/{id}/shares/{uid} {role: edit\|view}` | Share it, or change the role (the owner) |
| `DELETE /drawings/{id}/shares/{uid}` | Stop sharing; the person themselves may also leave it |
| `GET /live/{id}` (WebSocket, outside `/api/v1`) | A shared sketch, live. Send `pointer {pointer, button, selected}` and `scene {elements, files, full?, to?}` (editors); get `hello {you, peers, saver}`, `join`, `leave`, `saver`, `sync {to}`, `saved {rev}`, `reload`, `bye {reason}` and the others' messages with `from`. The server only relays; the `saver` (the owner when there, else the first editor) saves the merged drawing |

## Folders, library, overview

| | |
|---|---|
| `GET /folders` · `POST /folders {name}` · `PUT /folders/{id} {name?, sort?}` · `DELETE /folders/{id}` | Deleting a folder keeps its drawings |
| `GET /library` · `PUT /library {items}` | The person's Excalidraw library (shapes) |
| `GET /dashboard` | `{counts: {all, starred, recent, trash, no_folder, bytes, kinds, shared}, folders, recent}` — for Homepage |
| `GET /day/YYYY-MM-DD` | `{made, changed}` that day (what Pocket Day reads) |

## Import and export

| | |
|---|---|
| `POST /import?name=…&folder_id=…` | The file as the body (or multipart `file`): a Pocket Draw export, `.excalidraw`, `.excalidrawlib`, `.drawio`/`.xml`, `.mmd`, Markdown with ```` ```mermaid ```` blocks, or a Mermaid Live link → `{kind, counts, drawings}` |
| `GET /export.json` · `GET /export.csv` | Everything (re-importable, by `uid`) · a list |

## AI

`POST /ai/diagram {prompt, code?, type?}` → `{ok, title, code}` (drafts a text diagram, or changes `code` as asked) · `POST /ai/fix {code, error}` → `{ok, code, change}` · `GET /ai/status`.

## A drawing in a note or an issue

A drawing's page is `BASE_URL/d/{id}`; its picture answers under it at **`BASE_URL/d/{id}/picture.svg`** (outside `/api`, same auth: a signed-in browser, a token, or a `drawings:read` link). Markdown elsewhere embeds it as

```
![Floor plan](https://draw.example.com/d/12/picture.svg "Floor plan")
```

Pocket Notes and Pocket Projects show that picture through their own proxy (as the person, link `drawings:read`), and because the picture is under the drawing's page, the note or issue counts as linking to the drawing: `GET /drawings/{id}/used` finds it through their `/mentions`.

## Pocket Core app links

| Scope | Allows |
|---|---|
| `drawings:read` | `GET /dashboard`, `/drawings`, `/folders`, `/drawings/{id}` (+ `/preview.svg`, `/file`), `/day/{date}`, `/d/{id}/picture.svg` |
| `drawings:write` | `POST /drawings`; `PUT`/`DELETE /drawings/{id}` only for drawings that app added (`source` = its slug) |

Events on the bus: `draw.created`, `draw.edited` (at most once per ten minutes of editing, and on restore), `draw.deleted`; each carries `user_email`.
