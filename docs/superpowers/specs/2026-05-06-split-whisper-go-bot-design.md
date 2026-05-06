# Split Logger Bot into a Go bot and a Python Whisper service

**Status:** Approved (design phase)
**Date:** 2026-05-06
**Owner:** Santiago Jauregui Oderda

## Context

The current bot is a single Python process that handles Telegram updates,
transcribes audio with OpenAI Whisper, parses workout data with regex, and
writes to SQLite. Source lives in `src/logger_bot/` (three modules:
`main.py`, `extraction.py`, `loggers.py`). It runs as one container.

We are splitting it into two services so the heavy ML dependency tree
(torch, whisper, ffmpeg) is isolated from the rest of the bot, and so the
non-ML logic can move to Go.

## Decisions

| Decision | Choice |
| --- | --- |
| Inter-service protocol | gRPC + protobuf |
| Audio transport | Unary RPC, bytes inline (`bytes audio = 1`) |
| Deployment | docker-compose, single host |
| Existing data | Fresh `log.db` in a named volume, schema unchanged |
| Test scope | Minimal but real (parser, store, gRPC handler, compose smoke) |
| Repo layout | Multi-repo, two repos |
| Proto sharing | Copy-paste, kept manually in sync |
| Repo identity | `logger-bot` becomes the Go bot; new repo `logger-bot-whisper` for the Python service |

## Architecture

```
┌────────────┐     long-poll       ┌────────────────────────┐
│  Telegram  │  ◄────────────────► │  logger-bot (Go)       │
│  servers   │      getUpdates     │  ────────────────       │
└────────────┘      getFile        │  • Telegram handlers   │
                                   │  • Regex extraction    │
                                   │  • SQLite read+write   │     gRPC unary
                                   │  • Whisper gRPC client │ ──────────────► ┌──────────────────────┐
                                   └────────────────────────┘  Transcribe()   │ logger-bot-whisper   │
                                              │                               │ (Python)             │
                                              │ writes/reads                  │ ───────────────       │
                                              ▼                               │ • gRPC server        │
                                       ┌─────────────┐                        │ • OpenAI Whisper     │
                                       │  log.db     │                        │ • Health service     │
                                       │  (volume)   │                        └──────────────────────┘
                                       └─────────────┘
```

- Two repos, two containers, one compose file (lives in `logger-bot`).
- Private compose network. Go reaches Whisper at `whisper:50051` over insecure
  gRPC; the services are never publicly addressable, so no auth between them.
- Named volume `loggerbot-data` mounted at `/data` in the Go container.
  `log.db` lives at `/data/log.db`. Whisper container has no volumes.
- Whisper exposes the standard gRPC health service. Compose uses
  `depends_on.whisper.condition: service_healthy` so the bot only dials once
  the model is loaded — avoids a flurry of UNAVAILABLE on cold start.
- CI: each repo has its own GitHub Actions workflow (analogous to the
  current `docker-publish.yml`) that pushes `ttl.sh/loggerbot:24h` and
  `ttl.sh/loggerbot-whisper:24h` respectively. The compose file references
  both tags.

## Components

### Proto contract

Lives at `proto/transcribe.proto` in **both** repos, copied verbatim.
Drift is mitigated by the contract being tiny and rarely changing; each
README links to the other repo as a reminder.

```proto
syntax = "proto3";
package logger_bot.v1;

option go_package = "github.com/<github-user>/logger-bot/internal/whisper/pb;pb";  // <github-user> filled in when the Go repo is initialized

service Transcriber {
  rpc Transcribe(TranscribeRequest) returns (TranscribeResponse);
}

message TranscribeRequest {
  bytes audio = 1;          // raw bytes; Telegram caps at 20MB
  string mime_type = 2;     // for logging only; ffmpeg auto-detects
}

message TranscribeResponse {
  string text = 1;
}
```

Both gRPC ends configure `MaxCallRecvMsgSize` / `MaxCallSendMsgSize` at
32MB to comfortably cover the 20MB Telegram getFile cap.

### `logger-bot-whisper` (Python, new repo)

```
logger-bot-whisper/
├── pyproject.toml              # uv-managed; deps: grpcio, grpcio-tools, openai-whisper, torch, numpy
├── .python-version             # 3.14
├── Dockerfile                  # python:3.14-slim + ffmpeg + uv sync
├── docker-entrypoint.sh
├── proto/transcribe.proto      # copy of contract
├── Makefile                    # `make gen` → grpcio-tools generates _pb2.py / _pb2_grpc.py
├── src/whisper_service/
│   ├── __init__.py
│   ├── main.py                 # entrypoint: load model, start server, register grpc/health
│   ├── server.py               # TranscriberServicer.Transcribe(request, context)
│   ├── transcriber.py          # WhisperTranscriber (in-memory ffmpeg pipe, see Data Flow)
│   └── pb/                     # generated stubs
└── tests/
    ├── test_server.py
    └── fixtures/hello.wav      # ~1s WAV used by the gRPC handler test
```

### `logger-bot` (this repo, re-initialized as Go)

```
logger-bot/
├── go.mod / go.sum
├── Dockerfile                  # multi-stage: golang:1.23 → distroless static
├── docker-compose.yml          # both services, named volume, healthcheck gate
├── .env                        # TELEGRAM_API_TOKEN (loaded by compose)
├── proto/transcribe.proto      # copy of contract
├── Makefile                    # `make gen` → protoc-gen-go / -go-grpc
├── cmd/bot/main.go             # entrypoint: load env, wire deps, register handlers
├── internal/
│   ├── telegram/
│   │   ├── audio.go            # voice/audio/document handler
│   │   ├── last.go             # /last <exercise>
│   │   ├── sql.go              # /sql <query>  (read-only conn)
│   │   ├── health.go           # /health
│   │   └── middleware.go       # error-reporting / panic-recovery middleware
│   ├── extraction/
│   │   ├── parse.go            # ported regex parser, bilingual
│   │   └── parse_test.go       # table-driven tests with es/en fixtures
│   ├── store/
│   │   ├── store.go            # NewWriter(path), NewReader(path) — modernc.org/sqlite
│   │   └── store_test.go       # write+read roundtrip on tmp DB
│   └── whisper/
│       ├── client.go           # gRPC client wrapper exposing Transcribe(ctx, []byte) (string, error)
│       └── pb/                 # generated stubs
├── e2e/
│   ├── test_compose.sh         # docker-compose smoke test
│   └── hello.wav               # fixture used by smoke test
└── .github/workflows/docker-publish.yml
```

Library choices, grounded in current upstream docs:

- **Telegram bot:** `github.com/go-telegram/bot` (zero-deps, idiomatic
  context-based API, full Bot API coverage including `GetFile` and
  `FileDownloadLink`).
- **gRPC:** `google.golang.org/grpc` v1.73+ (`grpc.NewServer` /
  `grpc.NewClient`, standard `grpc/health` package).
- **SQLite:** `modernc.org/sqlite` (pure-Go, no CGO → static binary,
  distroless image possible).

### What gets deleted from the current repo

`src/logger_bot/`, `pyproject.toml`, `uv.lock`, `.python-version`,
`run_server.sh`, `launch_db_view.sh`, the existing `Dockerfile`. The
`datasette` view flow is dropped; if needed later, it can be re-added as
a third compose service that mounts the same `loggerbot-data` volume.

### Wiring (Go-side `main.go` shape)

Mirrors the current Python DI pattern (constructor functions that close
over deps):

```go
func main() {
    token       := mustEnv("TELEGRAM_API_TOKEN")
    whisperAddr := envOr("WHISPER_ADDR", "whisper:50051")
    dbPath      := envOr("DB_PATH", "/data/log.db")

    writer  := store.NewWriter(dbPath)         // owns CREATE TABLE IF NOT EXISTS
    reader  := store.NewReader(dbPath)         // ?mode=ro DSN
    wclient := whisper.NewClient(whisperAddr)  // dial, max msg size, health-check service config
    parser  := extraction.New()

    b, _ := bot.New(token,
        bot.WithMiddlewares(telegram.ErrorReporter),
        bot.WithDefaultHandler(telegram.NewAudio(wclient, parser, writer)),
    )
    b.RegisterHandler(bot.HandlerTypeMessageText, "/last",   bot.MatchTypePrefix, telegram.NewLast(reader))
    b.RegisterHandler(bot.HandlerTypeMessageText, "/sql",    bot.MatchTypePrefix, telegram.NewSQL(reader))
    b.RegisterHandler(bot.HandlerTypeMessageText, "/health", bot.MatchTypeExact,  telegram.Health)

    b.Start(context.Background())
}
```

## Data flow

### Audio message (the hot path)

```
1. User sends voice message to Telegram bot
2. Go: bot.go-telegram poller receives Update via getUpdates;
   default handler dispatches if Message has Voice/Audio/Document
3. Go: telegram/audio.go
   ├─► picks msg.Voice ?? msg.Audio ?? msg.Document  (mirrors current Python order)
   ├─► b.GetFile(ctx, &GetFileParams{FileID: id})    → File{FilePath}
   ├─► b.FileDownloadLink(file)                      → https URL
   ├─► http.Get(url)                                 → audio bytes in []byte (capped at 20MB)
   └─► whisper.Client.Transcribe(ctx, bytes, mime)   (gRPC unary, 60s deadline)
4. Python: server.py TranscriberServicer.Transcribe
   └─► transcriber.transcribe(request.audio)
       ├─► pipe bytes through ffmpeg via stdin/stdout into a numpy float32 array
       ├─► WhisperTranscriber.model.transcribe(audio_np) → text
       └─► return text
5. Python: return TranscribeResponse(text=text)
6. Go: back in audio.go
   ├─► parser.Parse(text)                           → extraction.Workout struct
   ├─► writer.Write(workout)                        → INSERT into log.db
   └─► b.SendMessage("I heard:\n" + json(workout))
```

**Audio bytes never touch disk.** The Go side keeps them in memory
(`http.Get` → `[]byte` → gRPC). The Whisper side decodes via an
in-memory ffmpeg pipe (subprocess reads from stdin, writes 16kHz mono
float32 to stdout — exactly what `whisper.load_audio` does internally,
just rewritten to skip the path argument):

```python
def _decode(audio_bytes: bytes) -> np.ndarray:
    cmd = ["ffmpeg", "-nostdin", "-threads", "0",
           "-i", "pipe:0",
           "-f", "s16le", "-ac", "1", "-acodec", "pcm_s16le",
           "-ar", "16000", "pipe:1"]
    out = subprocess.run(cmd, input=audio_bytes, capture_output=True, check=True).stdout
    return np.frombuffer(out, np.int16).flatten().astype(np.float32) / 32768.0
```

`model.transcribe()` accepts a numpy array directly (string-path is just
one of three accepted input types).

### `/last <exercise>` and `/sql <query>`

```
Telegram update → telegram/last.go (or sql.go)
   ├─► reader.QueryLast(exercise)  /  reader.QueryRaw(query)
   ├─► JSON-encode result rows
   └─► reply with MarkdownV2 code block, same shape as today
```

`reader` opens the DSN `file:/data/log.db?mode=ro` via
`modernc.org/sqlite`. The read-only flag is the safety boundary that
makes raw `/sql` acceptable, identical to the current Python guarantee.
The `/sql` handler does **no** SQL parsing or allowlisting; SQLite
itself rejects writes.

`/last` returns `No previous workouts for <exercise>` on an empty
result set — small UX win over today's `null`.

### `/health`

```
Telegram update → telegram/health.go → reply "Server is running"
```

Bot-side liveness ping for the human user. Distinct from the gRPC
health service Whisper exposes to compose.

### Schema migration on startup

`store.NewWriter(path)` runs:

```sql
CREATE TABLE IF NOT EXISTS workout (
    date TEXT, time TEXT,
    category TEXT, exercise TEXT,
    reps TEXT, weight TEXT, unit TEXT,
    raw_text TEXT
)
```

Same schema as today, so the named volume's `log.db` is interchangeable
with one written by the current Python bot if the user copies their
existing file in.

### Configuration surface

| Env var | Service | Purpose | Default |
| --- | --- | --- | --- |
| `TELEGRAM_API_TOKEN` | bot | required | — |
| `WHISPER_ADDR` | bot | gRPC target | `whisper:50051` |
| `DB_PATH` | bot | sqlite path | `/data/log.db` |
| `MODEL_SIZE` | whisper | Whisper model name | `base` |
| `GRPC_PORT` | whisper | listen port | `50051` |

`.env` lives at the root of the `logger-bot` repo and is loaded by
docker-compose automatically.

### Request-ID propagation

The bot generates a short request ID (e.g. UUIDv7 or 8-char base32) per
audio message and attaches it to outbound gRPC calls via metadata
(`x-request-id`). Whisper logs it alongside its own log lines. Both
services log structured JSON to stdout (`slog` for Go, `logging` with
JSON formatter for Python) — a slow transcription is traceable across
the two containers.

## Error handling

The current Python contract is preserved: the bot must not crash on a
single bad message; every failure becomes an `Error: <msg>` reply.

### Per-handler envelope (Go)

A middleware wraps every handler with a deferred `recover()`:

```go
func ErrorReporter(next bot.HandlerFunc) bot.HandlerFunc {
    return func(ctx context.Context, b *bot.Bot, update *models.Update) {
        defer func() {
            if r := recover(); r != nil && update.Message != nil {
                b.SendMessage(ctx, &bot.SendMessageParams{
                    ChatID: update.Message.Chat.ID,
                    Text:   fmt.Sprintf("Error: %v", r),
                })
            }
        }()
        next(ctx, b, update)
    }
}
```

Handlers also return errors explicitly for expected failures and
convert them to the same `Error: ...` format. Two layers: explicit for
the known cases, panic-recovery as a backstop.

### gRPC status mapping (Whisper → bot)

| Failure | Status code | Bot-side reply |
| --- | --- | --- |
| `ffmpeg` non-zero exit (corrupt audio) | `INVALID_ARGUMENT` | `Error: could not decode audio` |
| Whisper inference exception | `INTERNAL` | `Error: transcription failed` |
| Model not yet loaded | `UNAVAILABLE` | `Error: transcription service starting` |
| Caller deadline exceeded (60s) | `DEADLINE_EXCEEDED` | `Error: transcription timed out` |

The Go client checks `status.FromError(err)` and renders a friendly
message per code; everything else falls through to `Error: <raw>`.

### Service-level resilience

- **Whisper:** `try / except` around the `transcribe()` body so a
  single bad request can't kill the gRPC server. Model load failure on
  startup *does* exit; compose `restart: unless-stopped` brings it
  back. The health check only flips to `SERVING` after `load_model()`
  returns.
- **Bot:** `restart: unless-stopped` too. Telegram long-polling resumes
  from the last `update_id` after a restart, so messages aren't lost.
- **Compose ordering:** `depends_on.whisper.condition: service_healthy`
  prevents the bot from racing the model load.

### Specific cases

- **Audio > 20MB** (Telegram's getFile cap): caught client-side before
  the gRPC call. `Error: file too large`. Defensive 32MB max-message
  size on both gRPC ends as a backstop.
- **`/sql` write attempt:** SQLite returns
  `attempt to write a readonly database`; bubbled up as `Error: <sqlite
  message>`. Same safety boundary as today.
- **Empty Telegram update (no voice/audio/document):** matches current
  behavior — reply `No audio file found.`

## Testing

### Go — `internal/extraction/parse_test.go` (unit, table-driven)

Most valuable surface: pure function, deterministic, ports the current
Python parser. Fixtures lifted from the README and current behavior:

```go
var cases = []struct {
    name string
    in   string
    want Workout
}{
    {"es full",     "Press de banca, 10 repeticiones, 80 kilos, categoría pecho",
                    Workout{Category: "Pecho", Exercise: "Press De Banca", Reps: 10, Weight: 80.0, Unit: "kg"}},
    {"es minimal",  "Sentadilla 5 reps 100 kg",
                    Workout{Exercise: "Sentadilla", Reps: 5, Weight: 100.0, Unit: "kg"}},
    {"en lbs",      "Shoulder press 8 repetitions 60 lbs",
                    Workout{Exercise: "Shoulder Press", Reps: 8, Weight: 60.0, Unit: "lbs"}},
    {"missing reps","sentadilla 100 kg",
                    Workout{Exercise: "Sentadilla", Weight: 100.0, Unit: "kg"}},
    {"empty",       "",
                    Workout{Exercise: "Unknown Exercise", Unit: "kg"}},
    {"spanish numbers filtered", "remo cuatro repeticiones 60 kg",
                    Workout{Exercise: "Remo", Weight: 60.0, Unit: "kg"}},
    {"por lado",    "lunge 8 reps por lado 20 kg",
                    Workout{Exercise: "Lunge", Reps: 8, Weight: 20.0, Unit: "kg"}},
}
```

The Python `parse_text` is the oracle: every fixture is verified
against the current Python output once during the port, so behavior is
preserved bit-for-bit (modulo deliberately fixed bugs).

### Go — `internal/store/store_test.go` (integration, real SQLite)

```go
func TestWriteThenReadLast(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "log.db")
    w := store.NewWriter(dbPath)
    w.Write(Workout{Exercise: "Sentadilla", Reps: 5, Weight: 100, Unit: "kg", ...})

    r := store.NewReader(dbPath)
    got, _ := r.Last("sentadilla")
    if got.Reps != 5 { t.Fatal(...) }
}
```

Uses `modernc.org/sqlite` end-to-end (no mocks). `t.TempDir()` gives an
isolated DB per test. Catches schema drift and `?mode=ro` semantics.

### Python — `tests/test_server.py` (gRPC handler, in-process)

Tests `TranscriberServicer.Transcribe` directly without the gRPC server,
passing a fake `context` and a real `TranscribeRequest`:

```python
def test_transcribe_known_audio():
    transcriber = WhisperTranscriber(model_size="tiny")
    servicer    = TranscriberServicer(transcriber)
    with open("tests/fixtures/hello.wav", "rb") as f:
        audio = f.read()
    resp = servicer.Transcribe(TranscribeRequest(audio=audio, mime_type="audio/wav"), None)
    assert "hello" in resp.text.lower()
```

Uses `tiny` model so CI doesn't need a large checkpoint. Exercises the
ffmpeg pipe + Whisper integration in one shot.

### End-to-end — `e2e/test_compose.sh`

```bash
docker compose up -d --wait
# Whisper's 50051 is published only via a docker-compose.test.yml override
# so production runs keep the port internal. The override adds:
#   services.whisper.ports: ["50051:50051"]
grpcurl -plaintext -d "$(jq -Rs '{audio: .|@base64, mime_type: "audio/wav"}' < e2e/hello.wav)" \
        localhost:50051 logger_bot.v1.Transcriber/Transcribe \
  | jq -e '.text | test("hello"; "i")'
docker compose down -v
```

The `e2e/hello.wav` fixture is the same audio file as
`logger-bot-whisper/tests/fixtures/hello.wav` — copy it across when the
repos are initialized.

Single shell script, runs locally and in CI. Verifies: compose comes
up, healthcheck gating works, the gRPC contract matches across both
languages, audio survives the wire.

### Not tested (deliberately)

- Telegram handlers themselves — `go-telegram/bot` has no clean
  in-process test mode. Handlers are thin glue; the parts that can
  break are tested above.
- gRPC client retry / timeout semantics — covered by libraries.
- Whisper model accuracy — personal-use bot.

### CI

Each repo's GitHub Actions workflow runs its own tests before the
docker build:

- `logger-bot`: `go test ./...` (unit + store integration)
- `logger-bot-whisper`: `uv run pytest` (downloads `tiny` model on
  first run, cached after)
- The compose smoke test runs in `logger-bot`'s workflow only — pulls
  the latest `loggerbot-whisper:24h` from ttl.sh.

## Open items (to resolve at implementation time)

- **GitHub username/org** for the proto `go_package`. The placeholder
  `<github-user>` in the proto is filled in once the Go repo's import
  path is known (e.g. `github.com/santiagoderda/logger-bot`).
- **`logger-bot-whisper` repo creation.** Has to exist before the
  compose file references its image; the first push to ttl.sh seeds
  the tag.

## Out of scope

- Whisper engine swap (e.g. `faster-whisper`, `whisper.cpp`). Trivial
  to do later given the boundary; not bundled in.
- Production deployment beyond a single host (TLS, mTLS, separate-host
  topology). Easy to add — drop `insecure.NewCredentials()` for TLS,
  add a bearer-token interceptor — but unnecessary today.
- `datasette` UI for browsing the DB. Can be re-added as a third
  compose service if needed.
- Schema redesign (proper INTEGER/REAL types, primary key, sets vs.
  sessions split). Punt to a follow-up; not required by this refactor.
- Migrating the user's existing `log.db` automatically. The named
  volume starts empty by user choice; manual copy is possible.
