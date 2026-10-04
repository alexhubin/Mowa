# Mowa

Mowa is a minimalist web application for one-to-one and group voice calls with screen sharing. Accounts, friendships, calls, and messages are stored in PostgreSQL, while browsers send media traffic directly through a dedicated self-hosted LiveKit SFU.

## MVP features

- Persistent accounts with unique usernames and profiles
- Email OTP and Google sign-in with secure 30-day sessions and a shared two-account registration limit
- User search, friend requests, and a friends list
- Persistent direct conversations with offline delivery; the same conversation is available during a one-to-one call
- Direct calls to friends and incoming call notifications
- Persistent rooms with shareable invitation links
- Group voice calls
- A separate persistent chat for each group room, updated in real time and deleted when the empty room is removed
- Screen sharing from desktop or mobile devices when supported by the browser
- Participant list and active speaker indicators
- Microphone mute and unmute controls
- Microphone and audio output selection in supported browsers
- Screen share quality presets: 720p/30 at 2 Mbps or 1080p/30 at 5 Mbps
- VP9/SVC for screen sharing with automatic VP8 fallback
- Full-screen viewing of the active screen share
- Room join and leave flows

There is no camera UI. LiveKit tokens do not prohibit video publishing at the protocol level, so camera support can be added later without replacing the media server.

## Architecture

```text
Browser ── HTTPS ──> Caddy ──> Svelte / Go API ──> PostgreSQL 18.6
   │                                 │
   │          short-lived JWT <──────┘
   │
   └──── WebRTC / WSS ───────> LiveKit SFU
```

The API never proxies audio or screen-sharing traffic. It validates cookie sessions, manages rooms, stores messages in PostgreSQL, and delivers new-message notifications over SSE. For each media session, the API issues a LiveKit JWT that expires after 10 minutes. A signed LiveKit webhook deletes a room 30 seconds after the last participant leaves; group messages are removed through cascading deletion, while direct conversations are stored separately and remain available.

## Tech stack

- Go 1.27.1, `chi`, `database/sql`, the pure-Go `pgx` driver, and `go-webauthn`
- PostgreSQL 18.6; the backend is built with `CGO_ENABLED=0`
- `sqlc` for type-safe queries and `goose` for embedded migrations
- Node.js 24, Svelte 5, TypeScript, Vite, TanStack Svelte Query, and Tailwind CSS
- LiveKit Server 1.13.7, Docker Compose, and Caddy

## Local setup

Docker Engine and Docker Compose v2 are required.

```bash
cp .env.example .env
docker compose up --build
```

Once the stack is running, the application is available at [http://localhost](http://localhost) and LiveKit at `ws://localhost:7880`. PostgreSQL data is stored in the `postgres_data` Docker volume, and the API applies goose migrations on startup. With PostgreSQL 18, the volume is intentionally mounted at `/var/lib/postgresql`, as required by the current official image.

Verify the deployment:

```bash
curl http://localhost/api/health
docker compose ps
```

### Docker Desktop (macOS / Windows)

Use the Desktop override for testing in browsers on the same computer. It puts
LiveKit on the Compose bridge network, publishes its signaling and media ports on
loopback, and delivers webhooks directly to the API container. Docker Desktop's
optional host networking setting is not required. Compose 2.24.4+ is required.

```bash
docker compose -f compose.yaml -f compose.desktop.yaml up --build -d
docker compose -f compose.yaml -f compose.desktop.yaml ps
curl http://localhost/api/health
```

Open [http://localhost](http://localhost). The override fixes the local origins
and LiveKit URL even if `.env` contains production addresses. Create a local user
with the command below, adding `-f compose.yaml -f compose.desktop.yaml` after
`docker compose`. Password changes, profiles, messages, and rooms persist in the
local PostgreSQL volume.

Stop this stack without removing its data:

```bash
docker compose -f compose.yaml -f compose.desktop.yaml down
```

The Desktop override is for this computer only; use the production configuration
for access from other devices.

### Create a user

Sign in using Google or a six-digit email code. New users choose a username after verifying their identity. Both methods share a hardcoded two-account limit; existing users can still sign in when registration is closed. A Google identity with the same verified email links to the existing account.

Set server-only `RESEND_KEY`, `RESEND_FROM`, `GOOGLE_CLIENT_ID`, and `GOOGLE_CLIENT_SECRET` in `.env`. The sender must use a verified Resend domain (for example `Mowa <noreply@hubindev.cc>`). No secrets are bundled into the web or desktop client.

Create a Google OAuth **Web application** client with exact redirect URIs:
- `http://localhost/api/auth/google/callback`
- `https://mowa.hubindev.cc/api/auth/google/callback`

The server derives the callback from `APP_ORIGIN`. Google accounts using a non-Gmail address outside a verified Workspace domain should use email OTP to prove current ownership. Codes expire after ten minutes, permit five attempts, and can be requested once per minute and six times per hour per email, with a global hourly cap. Authentication proofs are single-use and browser-bound. Google uses state, nonce, PKCE and verified OIDC signatures, issuer and audience. Desktop continues to request its own independent session through the browser approval page.

Legacy password endpoints remain for administrator-provisioned accounts, but public password registration is disabled. New accounts have no usable password hash. Resetting development users is an explicit maintenance action, never an automatic migration.

### Invite a guest

An account holder creates a group room and shares its `/r/<invite-code>` link.
Visitors enter a display name and join without an account or password. Guests
can use the microphone, screen sharing, room chat, and local device settings.
They cannot create rooms, access account settings or contacts, or join private
one-to-one calls as guests.

Guest sessions use a separate HttpOnly cookie scoped to the room API path and
are checked against that room on every API request. They expire after 24 hours
or on explicit exit. The existing signed LiveKit `room_finished` webhook deletes
the room, guest sessions, and room chat when the call has ended; its invitation
then stops accepting new guests. LiveKit access tokens already issued retain
their configured expiry. A room admits up to 100 unexpired guest sessions.
Anyone with a group invitation can join it, so share the link only with invitees.

To preview locally, open `http://localhost` as the owner and open the room link
in a private browser window as the guest. The Docker Desktop configuration is
limited to this Mac; the localhost invitation will not work on a friend's device.

Stop the stack without deleting its data:

```bash
docker compose down
```

Do not add `-v` if you want to preserve accounts, conversations, and rooms.

## Development

Backend:

```bash
make test
```

Frontend:

```bash
cd frontend
npm ci
npm run dev
npm test
npm run build
```

`make test` starts an isolated PostgreSQL 18.6 instance through the Compose test profile and runs the API integration tests. Vite proxies `/api` to `localhost:8080`. To run the API outside Compose, provide an accessible PostgreSQL DSN:

```bash
DATABASE_URL='postgres://mova:password@localhost:5432/mova?sslmode=disable' go run ./cmd/api
```

Regenerate the database code after changing the SQL schema or queries:

```bash
make generate
```

This command uses the pinned `sqlc/sqlc:1.29.0` Docker image. Generated files are committed to the repository.

## Configuration

| Variable | Purpose | Local value |
|---|---|---|
| `APP_ADDRESS` | Site address used by Caddy | `http://localhost` |
| `LIVEKIT_ADDRESS` | LiveKit endpoint address used by Caddy | `http://livekit.localhost` |
| `APP_ORIGIN` | Allowed browser origin | `http://localhost` |
| `COOKIE_SECURE` | Restrict cookies to HTTPS | `false` |
| `POSTGRES_PASSWORD` | Password for the internal PostgreSQL user | Development password |
| `DATABASE_URL` | PostgreSQL DSN when running the API outside Compose | Localhost DSN |
| `LIVEKIT_URL` | Public SFU URL returned by the API | `ws://localhost:7880` |
| `LIVEKIT_API_KEY` | Shared API and SFU key | `devkey` |
| `LIVEKIT_API_SECRET` | Shared secret with at least 32 characters | Local development secret |
| `WEBAUTHN_RP_ID` | Passkey domain without scheme or port; derived from `APP_ORIGIN` by default | `localhost` |
| `WEBAUTHN_RP_NAME` | Service name shown in the system passkey dialog | `Mowa` |

Always use a unique key and a randomly generated secret in production. `.env` is ignored by Git.

## Production deployment

Microphone access and screen sharing require HTTPS and domains that point to the server:

- `mova.example.com` → server IP address
- `livekit.example.com` → server IP address

Example production `.env`:

```dotenv
APP_ADDRESS=mova.example.com
LIVEKIT_ADDRESS=livekit.example.com
APP_ORIGIN=https://mova.example.com
COOKIE_SECURE=true
POSTGRES_PASSWORD=replace-with-a-long-random-password
LIVEKIT_URL=wss://livekit.example.com
LIVEKIT_API_KEY=replace-with-random-key
LIVEKIT_API_SECRET=replace-with-at-least-32-random-characters
WEBAUTHN_RP_NAME=Mowa
```

`WEBAUTHN_RP_ID` may be omitted: the API safely derives `mova.example.com` from `APP_ORIGIN`. Do not change the RP ID after creating passkeys, because existing credentials are bound to the domain.

Open these ports in the external firewall:

- `80/tcp` and `443/tcp` — certificate validation, website, and WSS
- `443/udp` — optional HTTP/3
- `7881/tcp` — WebRTC over TCP
- `7882/udp` — WebRTC UDP mux

TURN is disabled. Clients use UDP 7882 with TCP 7881 as a fallback. Networks that block both transports require a future TURN/TLS deployment.

LiveKit uses `network_mode: host` so it can advertise correct WebRTC candidates without routing media through Docker NAT. Before starting the stack, make sure these ports and ports `80`/`443` are not already occupied by another project. If the server already has a shared reverse proxy, do not start the `caddy` service from this Compose configuration without an override. Connect `api:8080`, `web:8080`, and LiveKit at `127.0.0.1:7880` to the existing proxy instead.

For a standalone deployment on a server that has nothing else on it:

```bash
docker compose pull
docker compose up -d --build
docker compose ps
curl -fsS https://mova.example.com/api/health
```

### Continuous deployment to the shared VPS

`mowa.hubindev.cc` and `livekit.hubindev.cc` run on a VPS shared with other projects, and every push to `main` deploys there through [`.github/workflows/ci.yml`](.github/workflows/ci.yml). The checks — Go tests against a real PostgreSQL, frontend lint/test/build, `sqlc diff`, and Caddy validation — run in parallel with the image builds; only the deploy waits for all of them.

The pieces:

| Where | What |
|---|---|
| `compose.vps.yaml` | The whole production stack. Overwritten by every deploy. Pulls `api` and `web` from ghcr and starts its own PostgreSQL and LiveKit; it is not an overlay on `compose.yaml`. |
| `/opt/mova/.env` | Secrets, mode `600`. Never in Git and never in CI; the deploy only rewrites the `API_IMAGE` and `WEB_IMAGE` lines. |
| `deploy/caddy.caddy` | Both Caddy server blocks, deployed transactionally to `/opt/gateway/sites/mova.caddy` and imported by the shared `/opt/gateway/Caddyfile`. |

Repository secrets: `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`, and `VPS_HOST_KEY` — the pinned SSH host key, so the deploy never trusts whatever answers on the address.

The deploy pushes `compose.vps.yaml`, pins both image tags to the commit in `.env`, pulls and restarts, syncs the Caddy fragment — validating the whole assembled config and reloading only if the fragment changed — then verifies the containers from inside the Docker network and every site on the box from the outside.

`api` publishes `127.0.0.1:18080`. That port is required, not a leftover: LiveKit runs in the host network namespace, so its webhooks reach the API only through a published port. Nothing else is published — the shared Caddy reaches `api` and `web` by their `mova-api` and `mova-web` aliases on the external `proxy` network, and proxies LiveKit through `host.docker.internal:7880`. Keep the LiveKit DNS record in DNS-only mode so WebRTC traffic reaches the server directly.

For direct DNS records, allow incoming TCP 80/443 from the internet in the cloud
firewall as well as UFW, so browsers and the certificate authority can reach
Caddy. Keep `livekit.hubindev.cc` in DNS-only mode. On this VPS, configure UFW:

```bash
sudo ufw allow 7881/tcp comment 'Mowa WebRTC TCP'
sudo ufw allow 7882/udp comment 'Mowa WebRTC UDP'
sudo ufw allow from 172.19.0.0/16 to any port 7880 proto tcp comment 'Mowa LiveKit from Docker proxy'
```

The shared `proxy` network uses the fixed `172.19.0.0/16` subnet. UFW permits
TCP 7880 only from that subnet, so Caddy can reach LiveKit without exposing its
signaling port to arbitrary containers.

To roll back, point both image tags at an earlier commit and restart:

```bash
ssh admin@<vps> 'cd /opt/mova && sed -i "s|^API_IMAGE=.*|API_IMAGE=ghcr.io/alexhubin/mowa-api:<sha>|;s|^WEB_IMAGE=.*|WEB_IMAGE=ghcr.io/alexhubin/mowa-web:<sha>|" .env && docker compose -f compose.vps.yaml up -d'
```

## Security and MVP limitations

- Passwords are hashed with Argon2id and a unique salt.
- Passkeys use discoverable WebAuthn credentials with mandatory user verification. The private key remains on the device; the API stores the public credential record and its updatable signature counter.
- WebAuthn challenges expire after 5 minutes, can be used only once, and are bound to a random `HttpOnly`, `SameSite=Strict`, `Secure` cookie in production.
- Public registration creates full accounts and starts a session. The backend-only constant `registrationAccountLimit` in `internal/api/registration.go` is 2, counting all existing users. Registration serializes the count and insert in PostgreSQL; lowering the constant blocks new registrations without removing accounts. No quota is exposed in the interface.
- Rooms require full accounts. Legacy guest cookies no longer grant access.
- Mowa Desktop opens `/desktop-login` in the system browser. The signed-in user explicitly approves; a five-minute, single-use request bound to the app's random verifier creates a separate desktop session. Browser passwords and session tokens are never placed in URLs.
- Sessions use random opaque tokens; only their SHA-256 hashes are stored in PostgreSQL.
- Session cookies are `HttpOnly` and `SameSite=Lax`; `Secure` is enabled in production.
- State-changing requests validate the `Origin` header.
- LiveKit JWTs are restricted to one room and expire after 10 minutes. The data channel is disabled because persistent chat uses the Go API and PostgreSQL.
- Messages are limited to 2,000 characters and rendered by the frontend as plain text without HTML.
- PostgreSQL does not expose port `5432` on the host and is available only to the API on the internal Docker network.
- Direct-call rooms enforce membership; knowing an invitation code is not enough to obtain a LiveKit JWT.
- A single LiveKit instance is sufficient for the MVP but does not provide high availability.
- For the best connectivity from restricted corporate networks, a future release should add TURN/TLS on a dedicated domain and Redis for LiveKit scaling.

## Project structure

```text
cmd/api/                         Go API entry point
cmd/create-user/                 Administrative temporary account creation
internal/api/                    HTTP routes and tests
internal/auth/                   Argon2id and sessions
internal/database/migrations/    PostgreSQL goose migrations
internal/database/queries/       sqlc SQL queries
internal/database/dbgen/         Generated Go code
internal/media/                  LiveKit JWT issuance
frontend/                        Svelte application
deploy/Caddyfile                 Edge routing and TLS
compose.yaml                     Full local/production stack
```

## Desktop call diagnostics

The desktop app sends authenticated, event-only reports to `POST /api/desktop/diagnostics`.
It sends call start/connection, screen start/stop, codec changes, GPU-to-CPU fallback,
deduplicated error categories, reconnect events, and one final summary on normal call exit.
There are no periodic uploads. RTP statistics are sampled locally in memory every 15 seconds
and aggregated for the final summary (sampled positive video FPS, cumulative traffic,
packet loss, video freezes/dropped frames, audio concealment, maximum jitter).
`nvenc` / `amf` identify the selected external Windows encoder; `software` is CPU encoding.
`sdk_auto` on macOS means the SDK selected the implementation, not proof of hardware acceleration.

The report never includes raw SDK logs, error text, tokens, device/window names, messages,
IP addresses, audio or video. Reports include account identity (from the authenticated
session), per-call random ID, platform, architecture and app build revision. A visible
**Diagnostics** switch disables collection/upload; only that boolean setting is saved to disk.

Each JSON report is at most 4 KiB. Desktop allows at most 30 intermediate events plus a
final summary per call, with a 16-entry memory queue and a 3-second upload timeout.
Duplicate errors are counted in the summary. Uploads make one attempt, with no disk queue
or retries. Network failure, forced termination or crashes can lose reports, including the
final summary. Absence of a final report does not by itself prove a crash. The server accepts
at most 120 events per account per hour, deduplicates event IDs, and purges reports older
than 14 days on startup and hourly. There is no public report-reading endpoint.

Read the latest reports over the existing administrative SSH connection:

```sh
ssh northstar 'cd /opt/mova && docker compose -f compose.vps.yaml exec -T postgres psql -U mova -d mova' < scripts/desktop-diagnostics.sql
```

To investigate one user, add `AND u.username = 'friend'` to the query's WHERE clause.
Never publish database dumps or raw report exports in public issue trackers.
