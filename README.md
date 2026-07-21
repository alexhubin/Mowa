# Mova

Mova — минималистичное веб-приложение для групповых голосовых звонков и демонстрации экрана. Go API хранит пользователей и комнаты, а браузеры отправляют медиатрафик напрямую в отдельный self-hosted LiveKit SFU.

## Что входит в MVP

- регистрация, вход и выход;
- создание постоянной комнаты со ссылкой-приглашением;
- групповой голосовой звонок;
- демонстрация экрана компьютера или телефона, если её поддерживает браузер;
- список участников и индикация говорящего;
- включение и отключение микрофона;
- подключение к комнате и выход из неё.

Текстового чата и интерфейса камеры нет. LiveKit-токен не запрещает видеопубликацию архитектурно, поэтому камеру можно добавить позже без смены медиасервера.

## Архитектура

```text
Браузер ── HTTPS ──> Caddy ──> React / Go API ──> SQLite (WAL)
   │                                 │
   │       короткоживущий JWT <──────┘
   │
   └──── WebRTC / WSS ───────> LiveKit SFU
```

API никогда не проксирует аудио или демонстрацию экрана. Он только проверяет cookie-сессию, находит комнату и выпускает LiveKit JWT на 10 минут.

## Стек

- Go 1.26, `chi`, `database/sql`;
- только `modernc.org/sqlite` — CGO не используется;
- `sqlc` для типизированных запросов, `goose` для встроенных миграций;
- React, TypeScript, Vite, TanStack Router, TanStack Query, Tailwind CSS;
- LiveKit Server, Docker Compose, Caddy.

## Локальный запуск

Нужны Docker Engine и Docker Compose v2.

```bash
cp .env.example .env
docker compose up --build
```

После запуска приложение доступно на [http://localhost](http://localhost), LiveKit — на `ws://localhost:7880`. База хранится в Docker volume `mova_data`; миграции применяются API при старте.

Для проверки:

```bash
curl http://localhost/api/health
docker compose ps
```

Остановка без удаления данных:

```bash
docker compose down
```

Не добавляйте `-v`, если хотите сохранить аккаунты и комнаты.

## Разработка

Backend:

```bash
go test ./cmd/... ./internal/...
```

Frontend:

```bash
cd frontend
npm ci
npm run dev
npm test
npm run build
```

Vite проксирует `/api` на `localhost:8080`. Для запуска API вне Compose:

```bash
mkdir -p data
go run ./cmd/api
```

После изменения SQL-схемы или запросов пересоздайте код:

```bash
make generate
```

Команда использует зафиксированный Docker-образ `sqlc/sqlc:1.29.0`. Generated-файлы коммитятся в репозиторий.

## Конфигурация

| Переменная | Назначение | Локальное значение |
|---|---|---|
| `APP_ADDRESS` | адрес сайта для Caddy | `http://localhost` |
| `LIVEKIT_ADDRESS` | адрес WSS endpoint для Caddy | `http://livekit.localhost` |
| `APP_ORIGIN` | допустимый браузерный Origin | `http://localhost` |
| `COOKIE_SECURE` | cookie только через HTTPS | `false` |
| `LIVEKIT_URL` | публичный URL SFU, возвращаемый API | `ws://localhost:7880` |
| `LIVEKIT_API_KEY` | общий ключ API и SFU | `devkey` |
| `LIVEKIT_API_SECRET` | общий секрет, минимум 32 символа | локальный dev-секрет |

В production обязательно задайте собственные ключ и случайный секрет. `.env` игнорируется Git.

## Production-развёртывание

Для полноценной работы микрофона и демонстрации экрана нужны HTTPS и домены, указывающие на сервер:

- `mova.example.com` → IP сервера;
- `livekit.example.com` → IP сервера.

Пример production `.env`:

```dotenv
APP_ADDRESS=mova.example.com
LIVEKIT_ADDRESS=livekit.example.com
APP_ORIGIN=https://mova.example.com
COOKIE_SECURE=true
LIVEKIT_URL=wss://livekit.example.com
LIVEKIT_API_KEY=replace-with-random-key
LIVEKIT_API_SECRET=replace-with-at-least-32-random-characters
```

Откройте во внешнем firewall следующие порты:

- `80/tcp`, `443/tcp`, `443/udp` — сайт, WSS и HTTP/3;
- `7881/tcp` — WebRTC через TCP;
- `7882/udp` — WebRTC UDP mux;
- `3478/udp` — встроенный TURN/UDP.
- `40000:40100/udp` — ограниченный диапазон relay-портов TURN.

LiveKit работает с `network_mode: host`, чтобы корректно публиковать WebRTC-кандидаты и не прогонять медиа через Docker NAT. Перед запуском на сервере убедитесь, что эти порты и `80/443` не заняты другими проектами. Если на сервере уже есть общий reverse proxy, не запускайте сервис `caddy` из этого Compose без override: подключите `api:8080`, `web:8080` и LiveKit `127.0.0.1:7880` к существующему proxy.

Для текущего VPS добавлен `compose.vps.yaml`: он не запускает второй Caddy и подключает `api`/`web` к внешней сети `northstar_default`. Файл `deploy/Caddyfile.vps-snippet` содержит два изолированных server block для общего proxy. Это окружение использует временные адреса `sslip.io`; после добавления собственных DNS-записей замените их в snippet и `.env`.

```bash
docker compose -f compose.yaml -f compose.vps.yaml up -d --build
```

Запуск:

```bash
docker compose pull
docker compose up -d --build
docker compose ps
curl -fsS https://mova.example.com/api/health
```

## Безопасность и ограничения MVP

- Пароли хешируются Argon2id с уникальной солью.
- Сессии — случайные opaque-токены; в SQLite хранится только SHA-256 хеш.
- Cookie имеет `HttpOnly` и `SameSite=Lax`; в production включается `Secure`.
- Изменяющие запросы проверяют `Origin`.
- LiveKit JWT ограничен одной комнатой и сроком 10 минут; data channel отключён.
- SQLite использует WAL, `busy_timeout` и foreign keys.
- Один LiveKit инстанс подходит для MVP, но не обеспечивает high availability.
- Для максимальной доступности в корпоративных сетях позже стоит добавить TURN/TLS на отдельном домене и Redis для масштабирования LiveKit.

## Структура

```text
cmd/api/                         точка входа Go API
internal/api/                    HTTP-маршруты и тесты
internal/auth/                   Argon2id и сессии
internal/database/migrations/    goose-миграции
internal/database/queries/       SQL-запросы sqlc
internal/database/dbgen/         сгенерированный Go-код
internal/media/                  выпуск LiveKit JWT
frontend/                        React-приложение
deploy/Caddyfile                 edge routing и TLS
compose.yaml                     полный локальный/production стек
```
