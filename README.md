# Mowa

Mowa is a minimalist web application for one-to-one and group voice calls with screen sharing. Accounts, friendships, calls, and messages are stored in PostgreSQL, while browsers send media traffic directly through a dedicated self-hosted LiveKit SFU.

## MVP features

- Persistent accounts with unique usernames, profiles, and password changes
- Sign-in and sign-out with secure 30-day sessions; public registration is disabled
- Passwordless sign-in with passkeys (Touch ID, Face ID, Windows Hello, or a hardware security key)
- Mandatory temporary password change on first sign-in
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
Browser ── HTTPS ──> Caddy ──> React / Go API ──> PostgreSQL 18
   │                                 │
   │          short-lived JWT <──────┘
   │
   └──── WebRTC / WSS ───────> LiveKit SFU
```

The API never proxies audio or screen-sharing traffic. It validates cookie sessions, manages rooms, stores messages in PostgreSQL, and delivers new-message notifications over SSE. For each media session, the API issues a LiveKit JWT that expires after 10 minutes. A signed LiveKit webhook deletes a room 30 seconds after the last participant leaves; group messages are removed through cascading deletion, while direct conversations are stored separately and remain available.

## Tech stack

- Go 1.26, `chi`, `database/sql`, the pure-Go `pgx` driver, and `go-webauthn`
- PostgreSQL 18; the backend is built with `CGO_ENABLED=0`
- `sqlc` for type-safe queries and `goose` for embedded migrations
- Node.js 24, React, TypeScript, Vite, TanStack Router, TanStack Query, and Tailwind CSS
- LiveKit Server, Docker Compose, and Caddy

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

### Create a user

Accounts are created by an administrator. Pass the temporary password through an environment variable so it does not appear in process arguments:

```bash
docker compose run --rm \
  -e MOVA_TEMP_PASSWORD='replace-with-temporary-password' \
  --entrypoint mova-create-user api \
  -email user@example.com \
  -username user_name \
  -name 'User Name'
```

Until the user replaces the temporary password, the only available actions are signing out and setting a new password. Friends, settings, rooms, and LiveKit tokens remain blocked by the API.

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

`make test` starts an isolated PostgreSQL 18 instance through the Compose test profile and runs the API integration tests. Vite proxies `/api` to `localhost:8080`. To run the API outside Compose, provide an accessible PostgreSQL DSN:

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

- `80/tcp`, `443/tcp`, and `443/udp` — website, WSS, and HTTP/3
- `7881/tcp` — WebRTC over TCP
- `7882/udp` — WebRTC UDP mux
- `3478/udp` — built-in TURN over UDP
- `40000:40100/udp` — restricted TURN relay port range

LiveKit uses `network_mode: host` so it can advertise correct WebRTC candidates without routing media through Docker NAT. Before starting the stack, make sure these ports and ports `80`/`443` are not already occupied by another project. If the server already has a shared reverse proxy, do not start the `caddy` service from this Compose configuration without an override. Connect `api:8080`, `web:8080`, and LiveKit at `127.0.0.1:7880` to the existing proxy instead.

The repository includes `compose.vps.yaml` for deployments that use an existing shared reverse proxy. It disables the second Caddy instance and connects `api` and `web` to the external `northstar_default` network. `deploy/Caddyfile.vps-snippet` contains isolated server blocks for the shared proxy. Keep the LiveKit DNS record in DNS-only mode so WebRTC traffic reaches the server directly.

Deploy with the VPS override:

```bash
docker compose -f compose.yaml -f compose.vps.yaml up -d --build
```

For a standalone deployment:

```bash
docker compose pull
docker compose up -d --build
docker compose ps
curl -fsS https://mova.example.com/api/health
```

## Security and MVP limitations

- Passwords are hashed with Argon2id and a unique salt.
- Passkeys use discoverable WebAuthn credentials with mandatory user verification. The private key remains on the device; the API stores the public credential record and its updatable signature counter.
- WebAuthn challenges expire after 5 minutes, can be used only once, and are bound to a random `HttpOnly`, `SameSite=Strict`, `Secure` cookie in production.
- There is no public registration endpoint; temporary accounts can only be created through the administrative CLI.
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
frontend/                        React application
deploy/Caddyfile                 Edge routing and TLS
compose.yaml                     Full local/production stack
```
