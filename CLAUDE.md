# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Telegram bot that ingests voice messages, transcribes them with OpenAI Whisper, parses workout data (exercise / reps / weight / category / unit) with regex, and appends each set to a SQLite DB. Telegram commands `/last <exercise>` and `/sql <query>` read back from that DB; `/health` is a liveness check.

## Tooling & runtime

- Package manager: `uv` (lockfile is committed). Python is pinned to **3.14** via `.python-version` and the Dockerfile, even though `pyproject.toml` only requires `>=3.12` — prefer 3.14 when reproducing locally.
- Build backend is `uv_build`; the project is installed as the `logger_bot` package from `src/logger_bot/`.
- `ffmpeg` is a runtime dep of Whisper (installed in the Dockerfile, must be on PATH locally).

## Commands

```bash
./run_server.sh                # uv run src/logger_bot/main.py — long-polling Telegram bot
./launch_db_view.sh [db_path]  # kills anything on :8001, then `uv run datasette` against ./log.db (default)
uv sync                        # install deps from uv.lock
uv run python src/logger_bot/main.py   # equivalent to run_server.sh
docker build -f Dockerfile . -t bot && docker run -it --env-file .env bot
```

There is **no test suite, no linter config, and no CI lint/test step** — only `.github/workflows/docker-publish.yml`, which on push to `main` builds and pushes to the ephemeral registry `ttl.sh/loggerbot:24h` (images expire after 24h).

## Environment

- `TELEGRAM_API_TOKEN` — **required**, read via `python-dotenv` from `.env` at project root.
- `MODEL_SIZE` — optional Whisper model name (`tiny`/`base`/`small`/`medium`/`large`/etc.); defaults to `base`. Set this when running on a GPU box to upgrade quality.

`WhisperTranscriber.__init__` auto-selects the device in this order: CUDA → Apple MPS → CPU. No flag overrides this.

## Architecture (the parts that span files)

`main.py` wires three independent handlers onto a single `python-telegram-bot` `Application`. Each handler is built by a **factory function** (`create_audio_handler`, `create_last_handler`, `create_sql_handler`) that closes over its dependencies (transcriber, logger, db connection). This is the project's DI pattern — when adding a handler, follow the same shape rather than reaching for module-level singletons.

The DB is touched through **two separate connections** that must stay consistent:

1. `SqliteLogger` (in `loggers.py`) opens its own write connection per `write_record` call. It owns schema creation (`CREATE TABLE IF NOT EXISTS workout(...)`).
2. `main.py` opens a **read-only** connection (`sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)`) and shares it between `/last` and `/sql`. The read-only mode is the safety boundary that makes `/sql` (which executes raw user-supplied SQL) acceptable — do not "upgrade" this to a writable connection without replacing `/sql` with a parser/allowlist.

Hardcoded in `main.py`: `db_path="./log.db"`, `table_name="workout"`. The `Logger` ABC in `loggers.py` has two implementations (`CSVLogger`, `SqliteLogger`); only `SqliteLogger` is wired up in `main`. `CSVLogger` is kept around because the README's `launch_db_view.sh` flow originally imported `log.csv` into `log.db` via `sqlite-utils insert` (now commented out).

`extraction.parse_text` is regex-only and bilingual (Spanish + English keywords for kg/lbs, reps, "categoría"). It always returns the same dict shape, falling back to `"Unknown Exercise"` and zeroed numeric fields rather than raising — downstream handlers and the DB schema rely on every key being present.

## Conventions worth preserving

- Handler bodies catch `Exception` and report `Error: {e}` back to the chat. Keep that pattern when adding handlers — the bot must not crash on a single bad message.
- Telegram replies that contain JSON use `parse_mode="MarkdownV2"` wrapped in triple backticks; raw error/status replies use plain text.
- The audio handler accepts `audio | voice | document` (in that order) and downloads to `received_{file_id}` in the working directory — it does not clean up downloaded files.