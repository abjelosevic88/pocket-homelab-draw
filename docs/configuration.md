# Configuration

Environment variables; every one has a default. Personal settings (new drawings, sketch and diagram looks, notifications, connections) live in the UI per person — the `NOTIFY_*`/`OPENAI_*` values below are copied once into the first admin's settings, `DRAWIO_URL` once into the box settings.

| Variable | Default | What |
|---|---|---|
| `PORT`, `HOST` | `8080`, `0.0.0.0` | Listen address |
| `BASE_URL` | – | Public URL (links in pushes and events, the OIDC callback) |
| `DATA_DIR` | `./data` | Database, Core token, backups |
| `DB_PATH` | `$DATA_DIR/pocket-draw.db` | SQLite file |
| `MAX_BODY_BYTES` | `67108864` | Largest request: a drawing being saved (pictures pasted into a sketch travel inside it) or an export being imported (64 MB) |
| `TZ` | – | Time zone for days (Pocket Day) and the night clean-up |
| `DRAWIO_INTERNAL_URL` | – (`http://drawio:8080` in the sample compose file) | A `jgraph/drawio` container that Pocket Draw serves at `/drawio/` on its own address (the browser never needs to reach the container). Makes `/drawio` the default `DRAWIO_URL` |
| `DRAWIO_URL` | – (`/drawio` with `DRAWIO_INTERNAL_URL`) | The draw.io server diagrams open in: `/drawio` (the one above), your own `jgraph/drawio` at an address the browser reaches, or `https://embed.diagrams.net`. Empty: the Diagram kind is greyed out; sketches and text diagrams need nothing |
| `AUTH_MODE` | `local` | `local` (accounts), `proxy` (trust `AUTH_PROXY_HEADER`/`AUTH_PROXY_EMAIL_HEADER`), `none` |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | Seed the first admin while the database is empty |
| `SESSION_TTL`, `SECURE_COOKIES` | `720h`, `false` | Set `SECURE_COOKIES=true` behind HTTPS |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_NAME`, `OIDC_AUTO_CREATE` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL`, `NOTIFY_FORMAT` | – | Apprise (`…/notify/key`), ntfy topic URL or JSON webhook; format `apprise`/`ntfy`/`json` |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` | – | Drafting and fixing text diagrams without Pocket AI |
| `USER_AGENT` | `PocketDraw/1.0 (…)` | How Draw identifies itself to other services |
| `CORE_URL`, `CORE_REGISTRATION_KEY`, `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY`, `METRICS_ENABLED`, `LOG_LEVEL`, `LOG_FORMAT` | `true`, `true`, `info`, `text` | |
| `DEMO_DATA` | `false` | Seed a few months of made-up sketches, text diagrams and draw.io diagrams into the owner account (screenshots, trying it out) |

Commands: `pocket-draw backup [-o FILE]`, `pocket-draw reset-password [-email] [-password]`, `pocket-draw version`.
