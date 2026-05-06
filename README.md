# Logger Bot

Telegram bot that ingests voice messages, transcribes them via a companion
[Whisper service](https://github.com/santiago-jauregui/logger-bot-whisper), parses
workout data (exercise / reps / weight / category / unit), and appends each
set to a SQLite DB. Telegram commands `/last <exercise>` and `/sql <query>`
read back from that DB; `/health` is a liveness check.

## Architecture

```mermaid
flowchart LR
    user([User]) -- voice --> tg(Telegram)
    tg --> bot[logger-bot (Go)]
    bot -- gRPC --> wh[logger-bot-whisper (Python)]
    bot --> db[(SQLite)]
```

## Run with docker-compose

Pre-reqs: Docker, a Telegram bot token from [@BotFather](https://t.me/BotFather).

```bash
cp .env.example .env
# edit .env and set TELEGRAM_API_TOKEN
docker compose up -d
```

That pulls `ttl.sh/loggerbot:24h` and `ttl.sh/loggerbot-whisper:24h` from
the ephemeral registry. Compose waits for Whisper's health check to flip
to SERVING (model load takes ~10–30s on first cold start) before starting
the bot.

The SQLite database lives in the named volume `loggerbot-data`. To inspect
it, mount the volume into a sqlite3 container:

```bash
docker run --rm -it -v logger-bot_loggerbot-data:/data alpine \
  sh -c "apk add sqlite && sqlite3 /data/log.db"
```

## Bot commands

- `/last <exercise>` — most recent logged set for the exercise (substring match)
- `/sql <query>` — raw SELECT against the DB (read-only connection)
- `/health` — liveness ping

The bot understands voice messages with workout descriptions in Spanish
or English:

- *"Press de banca, 10 repeticiones, 80 kilos, categoría pecho"*
- *"Sentadilla 5 reps 100 kg"*
- *"Shoulder press 8 repetitions 60 lbs"*

## Develop

```bash
make gen        # regenerate proto stubs
go test ./...   # unit + store integration
make e2e        # docker-compose smoke test (requires grpcurl + jq)
```

## Configuration

| Env var | Purpose | Default |
| --- | --- | --- |
| `TELEGRAM_API_TOKEN` | required Telegram bot token | — |
| `WHISPER_ADDR` | gRPC target for the Whisper service | `whisper:50051` |
| `DB_PATH` | SQLite path | `/data/log.db` |
| `MODEL_SIZE` | Whisper model name (consumed by the Whisper service) | `base` |

## Companion repo

The Whisper service lives at [logger-bot-whisper](https://github.com/santiago-jauregui/logger-bot-whisper).
Its `proto/transcribe.proto` is a copy of this repo's; keep them in sync.
