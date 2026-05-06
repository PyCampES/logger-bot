# Split Logger Bot into Go bot + Python Whisper service — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the single-process Python logger bot into two services — a Go service owning Telegram + parsing + SQLite, and a Python service owning Whisper transcription — communicating over gRPC.

**Architecture:** Multi-repo. The current `logger-bot` repo is re-initialized as a Go project on a feature branch; a new sibling repo `logger-bot-whisper` is created from scratch for the Python service. They run together via docker-compose with a private network, a named volume for the SQLite DB, and `service_healthy` gating so the bot only dials Whisper after the model is loaded. Audio bytes never touch disk on either side (Go keeps them in memory; Python decodes via an in-memory ffmpeg pipe).

**Tech Stack:**
- Python 3.14, `uv`, `grpcio`, `grpcio-tools`, `openai-whisper`, `torch`, `numpy`, `ffmpeg`
- Go 1.23, `github.com/go-telegram/bot`, `google.golang.org/grpc`, `modernc.org/sqlite`
- Protocol Buffers, Docker + docker-compose, GitHub Actions, ttl.sh ephemeral registry

**Spec reference:** `docs/superpowers/specs/2026-05-06-split-whisper-go-bot-design.md`

---

## Working Directories

Both repos live as siblings under `~/Documents/git-repos/`:

```
~/Documents/git-repos/
├── logger-bot/             # current repo, re-initialized as Go (feature branch)
└── logger-bot-whisper/     # new repo, created during Task 1
```

All file paths in this plan are relative to **the repo root they belong to**. Each task's `Files:` block names the absolute repo (so it's unambiguous which repo to `cd` into).

---

## Conventions

- Every task starts with the failing test (where applicable) and ends with a commit.
- Commit messages use the existing repo's style: short imperative subject, no Conventional Commits prefix unless the task says otherwise. The current repo's history is "Adding torch as requirement", "Clarify logging method in README", "refactor: update WhisperTranscriber..." — sometimes prefixed, sometimes not. Match that mixed style; clarity over rigidity.
- Don't push to remote until the user asks. Local commits only during execution.
- The placeholder `<github-user>` (used in Go module paths and image tags) must be replaced with the actual GitHub username/org once known. Default for now: `santiago-jauregui`.

---

## Task 1: Bootstrap both workspaces

**Files:**
- Create: `~/Documents/git-repos/logger-bot-whisper/.gitignore`
- Create: `~/Documents/git-repos/logger-bot-whisper/.python-version`
- Create: `~/Documents/git-repos/logger-bot-whisper/pyproject.toml`
- Create: `~/Documents/git-repos/logger-bot-whisper/README.md`
- Modify (branch): current repo — create `refactor/go-whisper-split` branch

- [ ] **Step 1: Create the new repo directory and initialize git**

```bash
cd ~/Documents/git-repos
mkdir logger-bot-whisper
cd logger-bot-whisper
git init -b main
```

Expected: empty git repo on `main`.

- [ ] **Step 2: Pin Python version**

Create `~/Documents/git-repos/logger-bot-whisper/.python-version`:

```
3.14
```

- [ ] **Step 3: Add `.gitignore`**

Create `~/Documents/git-repos/logger-bot-whisper/.gitignore`:

```
.venv/
__pycache__/
*.pyc
*.pyo
.pytest_cache/
.idea/
.vscode/
.env
dist/
build/
src/whisper_service/pb/*.py
!src/whisper_service/pb/__init__.py
```

(Generated proto stubs are regenerated via `make gen`; do not commit them.)

- [ ] **Step 4: Initialize the Python project with uv**

Create `~/Documents/git-repos/logger-bot-whisper/pyproject.toml`:

```toml
[project]
name = "logger-bot-whisper"
version = "0.1.0"
description = "Whisper transcription microservice (gRPC) for logger-bot"
readme = "README.md"
requires-python = ">=3.12"
dependencies = [
    "grpcio>=1.73.0",
    "grpcio-health-checking>=1.73.0",
    "numpy>=2.0.0",
    "openai-whisper>=20250625",
    "torch>=2.10.0",
]

[dependency-groups]
dev = [
    "grpcio-tools>=1.73.0",
    "pytest>=8.0.0",
]

[build-system]
requires = ["uv_build>=0.11.2,<0.12.0"]
build-backend = "uv_build"

[tool.uv]
required-environments = [
  "sys_platform == 'darwin' and platform_machine == 'arm64'",
  "sys_platform == 'linux' and platform_machine == 'x86_64'",
  "sys_platform == 'linux' and platform_machine == 'aarch64'",
]
```

Then run:

```bash
cd ~/Documents/git-repos/logger-bot-whisper
uv sync
```

Expected: `uv.lock` created, `.venv/` populated.

- [ ] **Step 5: Add minimal README**

Create `~/Documents/git-repos/logger-bot-whisper/README.md`:

```markdown
# logger-bot-whisper

gRPC microservice that transcribes audio bytes to text using OpenAI Whisper.

Companion service to [logger-bot](https://github.com/<github-user>/logger-bot).

## Contract

- Service: `logger_bot.v1.Transcriber`
- RPC: `Transcribe(TranscribeRequest{audio: bytes, mime_type: string}) returns (TranscribeResponse{text: string})`
- Default port: `50051`
- Health: standard `grpc.health.v1.Health` service (used by `docker-compose` `service_healthy` gating).

The `.proto` contract is duplicated in both this repo and `logger-bot`. Keep them in sync manually.

## Run

```bash
uv sync
make gen
TELEGRAM_API_TOKEN=unused MODEL_SIZE=base uv run -m whisper_service.main
```

## Test

```bash
uv run pytest
```
```

(The Run section will be tightened up in later tasks once `make gen` exists.)

- [ ] **Step 6: Create initial source layout**

```bash
cd ~/Documents/git-repos/logger-bot-whisper
mkdir -p src/whisper_service/pb proto tests/fixtures
touch src/whisper_service/__init__.py src/whisper_service/pb/__init__.py
```

- [ ] **Step 7: Switch the existing repo to a feature branch**

```bash
cd ~/Documents/git-repos/logger-bot
git checkout -b refactor/go-whisper-split
git status
```

Expected: clean working tree on `refactor/go-whisper-split`.

- [ ] **Step 8: Commit**

In the new repo:

```bash
cd ~/Documents/git-repos/logger-bot-whisper
git add .
git commit -m "Initial repo skeleton (uv, python 3.14, gRPC + whisper deps)"
```

In the existing repo: nothing to commit yet (only the branch was created).

---

## Task 2: Define the proto contract and codegen target (Whisper repo)

**Files:**
- Create: `logger-bot-whisper/proto/transcribe.proto`
- Create: `logger-bot-whisper/Makefile`

- [ ] **Step 1: Add the proto file**

Create `logger-bot-whisper/proto/transcribe.proto`:

```proto
syntax = "proto3";

package logger_bot.v1;

option go_package = "github.com/<github-user>/logger-bot/internal/whisper/pb;pb";

service Transcriber {
  rpc Transcribe(TranscribeRequest) returns (TranscribeResponse);
}

message TranscribeRequest {
  bytes  audio     = 1;
  string mime_type = 2;
}

message TranscribeResponse {
  string text = 1;
}
```

- [ ] **Step 2: Add the Makefile**

Create `logger-bot-whisper/Makefile`:

```makefile
.PHONY: gen test run clean

PROTO_DIR := proto
OUT_DIR   := src/whisper_service/pb

gen:
	uv run python -m grpc_tools.protoc \
	    -I$(PROTO_DIR) \
	    --python_out=$(OUT_DIR) \
	    --grpc_python_out=$(OUT_DIR) \
	    --pyi_out=$(OUT_DIR) \
	    $(PROTO_DIR)/transcribe.proto
	# grpcio-tools writes "import transcribe_pb2" — patch to relative import.
	sed -i.bak 's/^import transcribe_pb2/from . import transcribe_pb2/' $(OUT_DIR)/transcribe_pb2_grpc.py
	rm -f $(OUT_DIR)/transcribe_pb2_grpc.py.bak

test:
	uv run pytest -v

run:
	uv run -m whisper_service.main

clean:
	rm -f $(OUT_DIR)/transcribe_pb2*.py $(OUT_DIR)/transcribe_pb2*.pyi
```

The `sed` step rewrites the relative import — `grpc_tools.protoc` defaults to a flat `import transcribe_pb2` which doesn't work inside a package.

- [ ] **Step 3: Run codegen and verify it works**

```bash
cd ~/Documents/git-repos/logger-bot-whisper
make gen
ls src/whisper_service/pb/
```

Expected output includes:
```
__init__.py
transcribe_pb2.py
transcribe_pb2.pyi
transcribe_pb2_grpc.py
```

Sanity check the import:

```bash
uv run python -c "from whisper_service.pb import transcribe_pb2, transcribe_pb2_grpc; print(transcribe_pb2.TranscribeRequest)"
```

Expected: `<class 'transcribe_pb2.TranscribeRequest'>` (the message class).

- [ ] **Step 4: Commit**

```bash
git add proto/transcribe.proto Makefile
git commit -m "Add proto contract and codegen Makefile target"
```

(Generated `pb/*.py` files are gitignored.)

---

## Task 3: Port `WhisperTranscriber` to in-memory bytes API (TDD)

**Files:**
- Create: `logger-bot-whisper/src/whisper_service/transcriber.py`
- Create: `logger-bot-whisper/tests/__init__.py`
- Create: `logger-bot-whisper/tests/test_transcriber.py`
- Create: `logger-bot-whisper/tests/fixtures/hello.wav` (generated locally, then committed)

- [ ] **Step 1: Generate the test fixture**

Use macOS `say` to produce a deterministic 16kHz mono WAV of the word "hello":

```bash
cd ~/Documents/git-repos/logger-bot-whisper
say "hello" -o tests/fixtures/hello.aiff --data-format=LEI16@16000
ffmpeg -y -i tests/fixtures/hello.aiff -acodec pcm_s16le -ar 16000 -ac 1 tests/fixtures/hello.wav
rm tests/fixtures/hello.aiff
```

Expected: `tests/fixtures/hello.wav` exists, ~30KB.

(If on Linux, substitute any short "hello" recording — the fixture lives in git and is committed once.)

- [ ] **Step 2: Write the failing test**

Create `logger-bot-whisper/tests/__init__.py` (empty file).

Create `logger-bot-whisper/tests/test_transcriber.py`:

```python
from pathlib import Path
import pytest

from whisper_service.transcriber import WhisperTranscriber

FIXTURE = Path(__file__).parent / "fixtures" / "hello.wav"


@pytest.fixture(scope="module")
def transcriber():
    return WhisperTranscriber(model_size="tiny")


def test_transcribe_hello_wav(transcriber):
    audio = FIXTURE.read_bytes()
    text = transcriber.transcribe(audio)
    assert isinstance(text, str)
    assert "hello" in text.lower()


def test_transcribe_corrupt_audio_raises(transcriber):
    with pytest.raises(RuntimeError):
        transcriber.transcribe(b"not actually audio bytes at all")
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd ~/Documents/git-repos/logger-bot-whisper
uv run pytest tests/test_transcriber.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'whisper_service.transcriber'`.

- [ ] **Step 4: Write the implementation**

Create `logger-bot-whisper/src/whisper_service/transcriber.py`:

```python
"""In-memory Whisper transcriber.

Audio bytes flow request → ffmpeg pipe → numpy float32 array → Whisper.
No file ever touches disk.
"""
import os
import subprocess

import numpy as np
import torch
import whisper

SAMPLE_RATE = 16_000


def _decode(audio_bytes: bytes) -> np.ndarray:
    """Decode arbitrary audio bytes to 16kHz mono float32 via ffmpeg.

    Mirrors what whisper.load_audio() does internally, but reads from
    stdin and writes to stdout instead of using a path argument.
    """
    cmd = [
        "ffmpeg", "-nostdin", "-threads", "0",
        "-i", "pipe:0",
        "-f", "s16le", "-ac", "1", "-acodec", "pcm_s16le",
        "-ar", str(SAMPLE_RATE),
        "pipe:1",
    ]
    try:
        proc = subprocess.run(cmd, input=audio_bytes, capture_output=True, check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"ffmpeg decode failed: {e.stderr.decode(errors='replace')}") from e
    return np.frombuffer(proc.stdout, np.int16).flatten().astype(np.float32) / 32768.0


def _select_device() -> str:
    if torch.cuda.is_available():
        return "cuda"
    if torch.backends.mps.is_available():
        return "mps"
    return "cpu"


class WhisperTranscriber:
    def __init__(self, model_size: str | None = None):
        model_size = model_size or os.environ.get("MODEL_SIZE", "base")
        device = _select_device()
        print(f"Loading Whisper model '{model_size}' on {device}...", flush=True)
        self.model = whisper.load_model(model_size, device=device)
        print("Whisper model loaded.", flush=True)

    def transcribe(self, audio_bytes: bytes) -> str:
        audio = _decode(audio_bytes)
        result = self.model.transcribe(audio)
        return result["text"].strip()
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
uv run pytest tests/test_transcriber.py -v
```

Expected: 2 passed (the first run downloads the `tiny` model — ~75MB — and may take 30-60s).

- [ ] **Step 6: Commit**

```bash
git add src/whisper_service/transcriber.py tests/__init__.py tests/test_transcriber.py tests/fixtures/hello.wav
git commit -m "Port WhisperTranscriber to in-memory bytes API"
```

---

## Task 4: Implement `TranscriberServicer` (TDD)

**Files:**
- Create: `logger-bot-whisper/src/whisper_service/server.py`
- Create: `logger-bot-whisper/tests/test_server.py`

- [ ] **Step 1: Write the failing test**

Create `logger-bot-whisper/tests/test_server.py`:

```python
from pathlib import Path

import grpc
import pytest

from whisper_service.pb import transcribe_pb2
from whisper_service.server import TranscriberServicer
from whisper_service.transcriber import WhisperTranscriber

FIXTURE = Path(__file__).parent / "fixtures" / "hello.wav"


class _FakeContext:
    """Stand-in for grpc.ServicerContext in unary tests."""
    def __init__(self):
        self.code = None
        self.details = None

    def set_code(self, code):
        self.code = code

    def set_details(self, details):
        self.details = details

    def abort(self, code, details):
        self.code = code
        self.details = details
        raise grpc.RpcError(details)


@pytest.fixture(scope="module")
def servicer():
    return TranscriberServicer(WhisperTranscriber(model_size="tiny"))


def test_transcribe_returns_text(servicer):
    audio = FIXTURE.read_bytes()
    req = transcribe_pb2.TranscribeRequest(audio=audio, mime_type="audio/wav")
    resp = servicer.Transcribe(req, _FakeContext())
    assert "hello" in resp.text.lower()


def test_transcribe_corrupt_audio_aborts_invalid_argument(servicer):
    req = transcribe_pb2.TranscribeRequest(audio=b"garbage bytes", mime_type="audio/x-bogus")
    ctx = _FakeContext()
    with pytest.raises(grpc.RpcError):
        servicer.Transcribe(req, ctx)
    assert ctx.code == grpc.StatusCode.INVALID_ARGUMENT


def test_transcribe_internal_failure_aborts_internal(servicer, monkeypatch):
    # Force the underlying transcriber to raise something other than RuntimeError
    monkeypatch.setattr(servicer.transcriber, "transcribe",
                        lambda _b: (_ for _ in ()).throw(ValueError("boom")))
    req = transcribe_pb2.TranscribeRequest(audio=b"x", mime_type="audio/wav")
    ctx = _FakeContext()
    with pytest.raises(grpc.RpcError):
        servicer.Transcribe(req, ctx)
    assert ctx.code == grpc.StatusCode.INTERNAL
```

- [ ] **Step 2: Run test to verify it fails**

```bash
uv run pytest tests/test_server.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'whisper_service.server'`.

- [ ] **Step 3: Write the implementation**

Create `logger-bot-whisper/src/whisper_service/server.py`:

```python
"""gRPC servicer for the Transcriber service.

Maps Python exceptions to gRPC status codes:
- ffmpeg decode failure (RuntimeError) -> INVALID_ARGUMENT
- everything else                      -> INTERNAL
"""
import logging

import grpc

from whisper_service.pb import transcribe_pb2, transcribe_pb2_grpc
from whisper_service.transcriber import WhisperTranscriber

log = logging.getLogger(__name__)


class TranscriberServicer(transcribe_pb2_grpc.TranscriberServicer):
    def __init__(self, transcriber: WhisperTranscriber):
        self.transcriber = transcriber

    def Transcribe(self, request, context):
        request_id = ""
        # Best-effort: extract x-request-id from metadata if the client sent one.
        try:
            md = dict(context.invocation_metadata() or [])
            request_id = md.get("x-request-id", "")
        except AttributeError:
            pass

        log.info("transcribe request_id=%s bytes=%d mime=%s",
                 request_id, len(request.audio), request.mime_type)

        try:
            text = self.transcriber.transcribe(request.audio)
        except RuntimeError as e:
            log.warning("transcribe decode error request_id=%s err=%s", request_id, e)
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(e))
        except Exception as e:
            log.exception("transcribe failed request_id=%s", request_id)
            context.abort(grpc.StatusCode.INTERNAL, f"transcription failed: {e}")

        log.info("transcribe ok request_id=%s text_len=%d", request_id, len(text))
        return transcribe_pb2.TranscribeResponse(text=text)
```

- [ ] **Step 4: Run tests**

```bash
uv run pytest tests/test_server.py -v
```

Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add src/whisper_service/server.py tests/test_server.py
git commit -m "Add TranscriberServicer with gRPC status mapping"
```

---

## Task 5: Wire the server entrypoint (`main.py`) with health service

**Files:**
- Create: `logger-bot-whisper/src/whisper_service/main.py`

- [ ] **Step 1: Write the entrypoint**

Create `logger-bot-whisper/src/whisper_service/main.py`:

```python
"""gRPC server entrypoint: load model, register Transcriber + health, serve."""
import logging
import os
import signal
from concurrent import futures

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

from whisper_service.pb import transcribe_pb2_grpc
from whisper_service.server import TranscriberServicer
from whisper_service.transcriber import WhisperTranscriber

MAX_MSG_BYTES = 32 * 1024 * 1024  # 32MB; covers Telegram's 20MB cap


def _setup_logging():
    logging.basicConfig(
        level=os.environ.get("LOG_LEVEL", "INFO"),
        format='{"ts":"%(asctime)s","lvl":"%(levelname)s","msg":"%(message)s","name":"%(name)s"}',
    )


def main():
    _setup_logging()
    port = int(os.environ.get("GRPC_PORT", "50051"))

    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4),
        options=[
            ("grpc.max_send_message_length", MAX_MSG_BYTES),
            ("grpc.max_receive_message_length", MAX_MSG_BYTES),
        ],
    )

    # Register Transcriber. Loading the model BLOCKS — do it before flipping
    # the health status to SERVING so compose's depends_on gating works.
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    health_servicer.set("", health_pb2.HealthCheckResponse.NOT_SERVING)

    transcriber = WhisperTranscriber()  # blocks until model is loaded
    transcribe_pb2_grpc.add_TranscriberServicer_to_server(
        TranscriberServicer(transcriber), server
    )

    server.add_insecure_port(f"[::]:{port}")
    server.start()
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    logging.info("gRPC server listening on :%d", port)

    # Graceful shutdown on SIGTERM / SIGINT (docker stop sends SIGTERM).
    import threading
    stop = threading.Event()
    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, lambda *_: stop.set())
    stop.wait()
    logging.info("Shutting down...")
    health_servicer.set("", health_pb2.HealthCheckResponse.NOT_SERVING)
    server.stop(grace=5).wait()


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Smoke-test the entrypoint locally**

In one terminal:

```bash
cd ~/Documents/git-repos/logger-bot-whisper
MODEL_SIZE=tiny GRPC_PORT=50051 uv run -m whisper_service.main
```

Expected: logs "Loading Whisper model 'tiny' on …", then "Whisper model loaded.", then "gRPC server listening on :50051".

In another terminal, verify the health service responds (requires `grpcurl` — install via `brew install grpcurl` if missing):

```bash
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
```

Expected: `{"status": "SERVING"}`.

Stop the server with Ctrl+C; expect a clean shutdown log.

- [ ] **Step 3: Commit**

```bash
git add src/whisper_service/main.py
git commit -m "Add server entrypoint with health service and graceful shutdown"
```

---

## Task 6: Add Dockerfile for the Whisper service

**Files:**
- Create: `logger-bot-whisper/Dockerfile`
- Create: `logger-bot-whisper/.dockerignore`

- [ ] **Step 1: Add `.dockerignore`**

Create `logger-bot-whisper/.dockerignore`:

```
.venv/
__pycache__/
*.pyc
.git/
.gitignore
.idea/
.vscode/
.python-version.local
tests/
docs/
.env
README.md
```

(Tests aren't shipped; they ran in CI before the build.)

- [ ] **Step 2: Add the Dockerfile**

Create `logger-bot-whisper/Dockerfile`:

```dockerfile
FROM python:3.14-slim

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg make \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .

RUN uv sync --locked --no-editable
RUN make gen

EXPOSE 50051

ENV GRPC_PORT=50051
ENV MODEL_SIZE=base

ENTRYPOINT ["uv", "run", "--no-sync", "python", "-u", "-m", "whisper_service.main"]
```

- [ ] **Step 3: Build and run the image locally to verify**

```bash
cd ~/Documents/git-repos/logger-bot-whisper
docker build -t loggerbot-whisper:dev .
docker run --rm -p 50051:50051 -e MODEL_SIZE=tiny loggerbot-whisper:dev &
DOCKER_PID=$!
sleep 30   # give the model time to load (tiny: ~10s, base: ~20s)
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
kill $DOCKER_PID
```

Expected: `{"status": "SERVING"}` from grpcurl.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "Add Dockerfile for Whisper service"
```

---

## Task 7: Add GitHub Actions workflow + finalize Whisper repo

**Files:**
- Create: `logger-bot-whisper/.github/workflows/docker-publish.yml`
- Modify: `logger-bot-whisper/README.md`

- [ ] **Step 1: Add the CI workflow**

Create `logger-bot-whisper/.github/workflows/docker-publish.yml`:

```yaml
name: docker-publish

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
        with:
          enable-cache: true
      - run: |
          sudo apt-get update
          sudo apt-get install -y ffmpeg make
      - run: uv sync --locked
      - run: make gen
      - run: make test

  publish:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ttl.sh/loggerbot-whisper:24h
```

(Mirrors the existing `logger-bot` workflow — public ttl.sh registry, no auth needed.)

- [ ] **Step 2: Update the README's Run / Test sections**

Modify `logger-bot-whisper/README.md` — replace the Run and Test sections with:

```markdown
## Run

```bash
uv sync
make gen
MODEL_SIZE=tiny make run    # local; first run downloads the model (~75MB for tiny)
```

## Test

```bash
make test
```

## Build the image

```bash
docker build -t loggerbot-whisper:local .
docker run --rm -p 50051:50051 -e MODEL_SIZE=tiny loggerbot-whisper:local
```
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/docker-publish.yml README.md
git commit -m "Add CI workflow and finalize README"
```

The Whisper repo is now end-to-end: tests pass, Docker image builds and runs, CI is wired. Push when ready (the user will do this manually since the GitHub repo doesn't exist yet).

---

## Task 8: Wipe Python code from `logger-bot` and initialize as Go

**Files:**
- Delete: `logger-bot/src/`, `logger-bot/pyproject.toml`, `logger-bot/uv.lock`, `logger-bot/.python-version`, `logger-bot/run_server.sh`, `logger-bot/launch_db_view.sh`, `logger-bot/Dockerfile`, `logger-bot/.dockerignore`
- Create: `logger-bot/go.mod`
- Create: `logger-bot/.gitignore` (rewrite)
- Create: `logger-bot/cmd/bot/main.go` (placeholder so `go build ./...` works)
- Create: directory tree under `logger-bot/internal/`

- [ ] **Step 1: Delete the Python code and old build artifacts**

```bash
cd ~/Documents/git-repos/logger-bot
git rm -r src/ pyproject.toml uv.lock .python-version run_server.sh launch_db_view.sh Dockerfile .dockerignore
```

(Confirm `git status` shows the deletions staged.)

- [ ] **Step 2: Rewrite `.gitignore` for a Go project**

Replace the contents of `logger-bot/.gitignore` with:

```
# Build artifacts
/bot
/dist

# IDE
.idea/
.vscode/

# Environment
.env
.env.local

# Test artifacts
*.test
*.out
coverage.txt

# Generated proto code
internal/whisper/pb/*.go
!internal/whisper/pb/.gitkeep

# OS
.DS_Store
```

- [ ] **Step 3: Initialize the Go module**

```bash
cd ~/Documents/git-repos/logger-bot
go mod init github.com/<github-user>/logger-bot
```

(Replace `<github-user>` with the actual user/org. If unknown, use a placeholder like `santiago-jauregui` and update later.)

Expected: `go.mod` created with `module github.com/<github-user>/logger-bot` and `go 1.23` (or higher — whatever's installed).

- [ ] **Step 4: Create the directory tree**

```bash
mkdir -p cmd/bot internal/telegram internal/extraction internal/store internal/whisper/pb proto e2e
touch internal/whisper/pb/.gitkeep
```

- [ ] **Step 5: Add a stub `main.go` so the module builds**

Create `logger-bot/cmd/bot/main.go`:

```go
package main

func main() {
	// Placeholder; the real entrypoint is wired in Task 16.
}
```

Verify the module builds:

```bash
go build ./...
```

Expected: no output, exit code 0.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "Wipe Python code; initialize as Go module"
```

---

## Task 9: Add proto contract + Go codegen to `logger-bot`

**Files:**
- Create: `logger-bot/proto/transcribe.proto`
- Create: `logger-bot/Makefile`

- [ ] **Step 1: Copy the proto file from the Whisper repo**

The contract is identical to the Whisper repo's. Create `logger-bot/proto/transcribe.proto`:

```proto
syntax = "proto3";

package logger_bot.v1;

option go_package = "github.com/<github-user>/logger-bot/internal/whisper/pb;pb";

service Transcriber {
  rpc Transcribe(TranscribeRequest) returns (TranscribeResponse);
}

message TranscribeRequest {
  bytes  audio     = 1;
  string mime_type = 2;
}

message TranscribeResponse {
  string text = 1;
}
```

(Replace `<github-user>` with the same value used in `go.mod`.)

- [ ] **Step 2: Install the Go protoc plugins**

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Verify:

```bash
which protoc-gen-go protoc-gen-go-grpc
```

Expected: paths under `$(go env GOPATH)/bin`.

(Also requires `protoc` itself: `brew install protobuf` on macOS; `apt-get install protobuf-compiler` on Debian/Ubuntu.)

- [ ] **Step 3: Add the Makefile**

Create `logger-bot/Makefile`:

```makefile
.PHONY: gen test build run e2e clean

PROTO_DIR := proto
OUT_DIR   := internal/whisper/pb

gen:
	@which protoc-gen-go        > /dev/null || (echo "Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; exit 1)
	@which protoc-gen-go-grpc   > /dev/null || (echo "Install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"; exit 1)
	protoc -I$(PROTO_DIR) \
	    --go_out=$(OUT_DIR)      --go_opt=paths=source_relative \
	    --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative \
	    $(PROTO_DIR)/transcribe.proto

test:
	go test ./... -count=1

build:
	go build -o bot ./cmd/bot

run:
	go run ./cmd/bot

e2e:
	./e2e/test_compose.sh

clean:
	rm -f bot $(OUT_DIR)/*.go
```

- [ ] **Step 4: Run codegen and verify**

```bash
make gen
ls internal/whisper/pb/
```

Expected: `transcribe.pb.go`, `transcribe_grpc.pb.go`, `.gitkeep`.

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Add gRPC + dependencies to `go.mod`**

```bash
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go mod tidy
```

- [ ] **Step 6: Commit**

```bash
git add proto/transcribe.proto Makefile go.mod go.sum internal/whisper/pb/.gitkeep
git commit -m "Add proto contract and Go codegen Makefile"
```

(Generated `pb/*.go` are gitignored.)

---

## Task 10: Implement the `extraction` package (TDD)

**Files:**
- Create: `logger-bot/internal/extraction/parse.go`
- Create: `logger-bot/internal/extraction/parse_test.go`

The extraction package is a port of `extraction.parse_text` from the original Python. It must produce identical output for the fixtures.

- [ ] **Step 1: Write the failing test**

Create `logger-bot/internal/extraction/parse_test.go`:

```go
package extraction

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Workout
	}{
		{
			name: "es full",
			in:   "Press de banca, 10 repeticiones, 80 kilos, categoría pecho",
			want: Workout{
				Category: "Pecho", Exercise: "Press De Banca",
				Reps: 10, Weight: 80.0, Unit: "kg",
				RawText: "Press de banca, 10 repeticiones, 80 kilos, categoría pecho",
			},
		},
		{
			name: "es minimal",
			in:   "Sentadilla 5 reps 100 kg",
			want: Workout{
				Exercise: "Sentadilla",
				Reps: 5, Weight: 100.0, Unit: "kg",
				RawText: "Sentadilla 5 reps 100 kg",
			},
		},
		{
			name: "en lbs",
			in:   "Shoulder press 8 repetitions 60 lbs",
			want: Workout{
				Exercise: "Shoulder Press",
				Reps: 8, Weight: 60.0, Unit: "lbs",
				RawText: "Shoulder press 8 repetitions 60 lbs",
			},
		},
		{
			name: "missing reps defaults to zero",
			in:   "sentadilla 100 kg",
			want: Workout{
				Exercise: "Sentadilla",
				Reps: 0, Weight: 100.0, Unit: "kg",
				RawText: "sentadilla 100 kg",
			},
		},
		{
			name: "empty input",
			in:   "",
			want: Workout{
				Exercise: "Unknown Exercise",
				Reps: 0, Weight: 0.0, Unit: "kg",
				RawText: "",
			},
		},
		{
			name: "spanish numbers filtered",
			in:   "remo cuatro repeticiones 60 kg",
			want: Workout{
				Exercise: "Remo",
				Reps: 0, Weight: 60.0, Unit: "kg",
				RawText: "remo cuatro repeticiones 60 kg",
			},
		},
		{
			name: "por lado phrase removed",
			in:   "lunge 8 reps por lado 20 kg",
			want: Workout{
				Exercise: "Lunge",
				Reps: 8, Weight: 20.0, Unit: "kg",
				RawText: "lunge 8 reps por lado 20 kg",
			},
		},
		{
			name: "weight before reps with kilos word",
			in:   "press 80 kilos 10 reps",
			want: Workout{
				Exercise: "Press",
				Reps: 10, Weight: 80.0, Unit: "kg",
				RawText: "press 80 kilos 10 reps",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q)\n  got:  %+v\n  want: %+v", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/extraction/ -v
```

Expected: FAIL with `undefined: Parse` and `undefined: Workout`.

- [ ] **Step 3: Write the implementation**

Create `logger-bot/internal/extraction/parse.go`:

```go
// Package extraction parses workout data (exercise / reps / weight / category / unit)
// from a free-text transcription. Bilingual (Spanish + English keywords).
//
// Always returns a fully-populated Workout — never errors. Falls back to
// "Unknown Exercise" and zeroed numeric fields rather than failing, because
// downstream handlers and the SQLite schema rely on every field being present.
package extraction

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Workout struct {
	Category string
	Exercise string
	Reps     int
	Weight   float64
	Unit     string
	RawText  string
}

var (
	weightRe   = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(kg|kilos?|lbs?|pounds?|libras?)`)
	repsRe     = regexp.MustCompile(`(\d+)\s*(repeticiones?|repetitions?|reps?)`)
	categoryRe = regexp.MustCompile(`categor[ií]a[\s,]+(?:de\s+)?(\w+)`)
	porLadoRe  = regexp.MustCompile(`por\s+lado`)
	spanishNumRe = regexp.MustCompile(`(cuatro|cinco|seis|siete|ocho|nueve|diez)\s+repeticiones?`)
	conRe      = regexp.MustCompile(`\bcon\b`)
	punctRe    = regexp.MustCompile(`[^\w\s]`)
	wsRe       = regexp.MustCompile(`\s+`)

	titleCaser = cases.Title(language.Und)
)

func normalizeUnit(raw string) string {
	switch raw {
	case "kilo", "kilos":
		return "kg"
	case "lb", "lbs", "pound", "pounds", "libra", "libras":
		return "lbs"
	default:
		return raw
	}
}

// Parse extracts workout data from a transcription. Mirrors the Python parser
// in src/logger_bot/extraction.py exactly.
func Parse(text string) Workout {
	w := Workout{Unit: "kg", RawText: text}
	low := strings.ToLower(text)

	// 1. Weight + unit
	if m := weightRe.FindStringSubmatchIndex(low); m != nil {
		full := low[m[0]:m[1]]
		num, _ := strconv.ParseFloat(low[m[2]:m[3]], 64)
		w.Weight = num
		w.Unit = normalizeUnit(low[m[4]:m[5]])
		low = strings.Replace(low, full, " ", 1)
	}

	// 2. Reps (numeric form only — Spanish word-numbers are stripped, not parsed)
	if m := repsRe.FindStringSubmatchIndex(low); m != nil {
		full := low[m[0]:m[1]]
		n, _ := strconv.Atoi(low[m[2]:m[3]])
		w.Reps = n
		low = strings.Replace(low, full, " ", 1)
	}

	// 3. Category
	if m := categoryRe.FindStringSubmatchIndex(low); m != nil {
		full := low[m[0]:m[1]]
		w.Category = titleCaser.String(low[m[2]:m[3]])
		low = strings.Replace(low, full, " ", 1)
	}

	// 4. Build exercise from what's left
	low = porLadoRe.ReplaceAllString(low, "")
	low = spanishNumRe.ReplaceAllString(low, "")
	low = conRe.ReplaceAllString(low, "")
	low = punctRe.ReplaceAllString(low, " ")
	low = wsRe.ReplaceAllString(strings.TrimSpace(low), " ")

	if low == "" {
		w.Exercise = "Unknown Exercise"
	} else {
		w.Exercise = titleCaser.String(low)
	}
	return w
}
```

- [ ] **Step 4: Add the casing dependency**

```bash
cd ~/Documents/git-repos/logger-bot
go get golang.org/x/text/cases
go get golang.org/x/text/language
go mod tidy
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/extraction/ -v
```

Expected: 8 passed. If any fail, compare the output against running the Python parser on the same input — the goal is byte-for-byte parity for these fixtures.

- [ ] **Step 6: Commit**

```bash
git add internal/extraction/ go.mod go.sum
git commit -m "Port regex parser to Go (internal/extraction)"
```

---

## Task 11: Implement the `store` package (TDD)

**Files:**
- Create: `logger-bot/internal/store/store.go`
- Create: `logger-bot/internal/store/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `logger-bot/internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"

	"github.com/<github-user>/logger-bot/internal/extraction"
)

func TestWriteThenReadLast(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")

	w, err := NewWriter(dbPath)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	for _, e := range []extraction.Workout{
		{Category: "Pecho", Exercise: "Press De Banca", Reps: 10, Weight: 80, Unit: "kg", RawText: "press 80kg 10 reps"},
		{Category: "Pecho", Exercise: "Press De Banca", Reps: 8,  Weight: 85, Unit: "kg", RawText: "press 85kg 8 reps"},
	} {
		if err := w.Write(e); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	r, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	got, err := r.Last("press de banca")
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if got == nil {
		t.Fatal("expected a row, got nil")
	}
	if got.Reps != "8" || got.Weight != "85" {
		t.Fatalf("expected most recent row (8 reps @ 85kg), got %+v", got)
	}
}

func TestLastReturnsNilWhenNoMatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")
	w, _ := NewWriter(dbPath)
	t.Cleanup(func() { _ = w.Close() })

	r, _ := NewReader(dbPath)
	t.Cleanup(func() { _ = r.Close() })

	got, err := r.Last("nonexistent")
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestReaderRejectsWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")
	w, _ := NewWriter(dbPath)
	t.Cleanup(func() { _ = w.Close() })

	r, _ := NewReader(dbPath)
	t.Cleanup(func() { _ = r.Close() })

	rows, err := r.Query("INSERT INTO workout (date) VALUES ('today')")
	if err == nil {
		t.Fatalf("expected error from write on read-only conn, got rows=%v", rows)
	}
	// modernc.org/sqlite returns "attempt to write a readonly database"
}

func TestQueryRoundTripsJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")
	w, _ := NewWriter(dbPath)
	t.Cleanup(func() { _ = w.Close() })
	_ = w.Write(extraction.Workout{Exercise: "Sentadilla", Reps: 5, Weight: 100, Unit: "kg"})

	r, _ := NewReader(dbPath)
	t.Cleanup(func() { _ = r.Close() })

	rows, err := r.Query("SELECT exercise, reps FROM workout")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["exercise"] != "Sentadilla" {
		t.Fatalf("got %+v", rows[0])
	}
}
```

(Replace `<github-user>` with the actual module path used in `go.mod`.)

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/store/ -v
```

Expected: FAIL with `undefined: NewWriter`, etc.

- [ ] **Step 3: Add the SQLite driver**

```bash
cd ~/Documents/git-repos/logger-bot
go get modernc.org/sqlite
go mod tidy
```

- [ ] **Step 4: Write the implementation**

Create `logger-bot/internal/store/store.go`:

```go
// Package store wraps a SQLite-backed workout log.
//
// Two roles, two connections (mirrors the original Python design):
//   - Writer:  read-write, owns the schema migration on first use.
//   - Reader:  ?mode=ro DSN, used by /last and /sql. Read-only is the
//              safety boundary that makes raw user-supplied SQL acceptable.
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/<github-user>/logger-bot/internal/extraction"
	_ "modernc.org/sqlite"
)

const tableName = "workout"

const createTableSQL = `
CREATE TABLE IF NOT EXISTS workout (
    date TEXT,
    time TEXT,
    category TEXT,
    exercise TEXT,
    reps TEXT,
    weight TEXT,
    unit TEXT,
    raw_text TEXT
)`

// Row mirrors the workout schema. Fields are strings because the original
// schema stores everything as TEXT.
type Row struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Category string `json:"category"`
	Exercise string `json:"exercise"`
	Reps     string `json:"reps"`
	Weight   string `json:"weight"`
	Unit     string `json:"unit"`
	RawText  string `json:"raw_text"`
}

// Writer handles INSERTs.
type Writer struct{ db *sql.DB }

func NewWriter(path string) (*Writer, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(createTableSQL); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Writer{db: db}, nil
}

func (w *Writer) Write(e extraction.Workout) error {
	now := time.Now()
	_, err := w.db.Exec(
		`INSERT INTO workout (date, time, category, exercise, reps, weight, unit, raw_text)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		now.Format("2006-01-02"),
		now.Format("15:04:05"),
		e.Category,
		e.Exercise,
		fmt.Sprintf("%d", e.Reps),
		fmt.Sprintf("%g", e.Weight),
		e.Unit,
		e.RawText,
	)
	return err
}

func (w *Writer) Close() error { return w.db.Close() }

// Reader executes SELECTs against a read-only DSN.
type Reader struct{ db *sql.DB }

func NewReader(path string) (*Reader, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	return &Reader{db: db}, nil
}

func (r *Reader) Close() error { return r.db.Close() }

// Last returns the most recent row for an exercise (case-insensitive substring),
// or nil if none. Mirrors the Python `/last` query.
func (r *Reader) Last(exercise string) (*Row, error) {
	const q = `SELECT date, time, category, exercise, reps, weight, unit, raw_text
	           FROM workout
	           WHERE exercise LIKE ? COLLATE NOCASE
	           ORDER BY date DESC, time DESC
	           LIMIT 1`
	row := r.db.QueryRow(q, "%"+exercise+"%")

	var x Row
	err := row.Scan(&x.Date, &x.Time, &x.Category, &x.Exercise, &x.Reps, &x.Weight, &x.Unit, &x.RawText)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &x, nil
}

// Query runs an arbitrary SQL string. Used by /sql. The read-only DSN is the
// safety boundary — DO NOT add a write connection here.
func (r *Reader) Query(query string) ([]map[string]any, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
```

(Replace `<github-user>` with the actual module path.)

- [ ] **Step 5: Run tests**

```bash
go test ./internal/store/ -v
```

Expected: 4 passed.

- [ ] **Step 6: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "Add store package (modernc.org/sqlite, writer + read-only reader)"
```

---

## Task 12: Implement the Whisper gRPC client

**Files:**
- Create: `logger-bot/internal/whisper/client.go`

The client is mostly a wrapper around the generated stub. No new test file — exercising it requires a running Whisper server, which the e2e smoke test handles.

- [ ] **Step 1: Write the implementation**

Create `logger-bot/internal/whisper/client.go`:

```go
// Package whisper is a thin client around the gRPC Transcriber service.
package whisper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/<github-user>/logger-bot/internal/whisper/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	maxMsgBytes    = 32 * 1024 * 1024 // 32MB; covers Telegram's 20MB cap
	defaultTimeout = 60 * time.Second
)

type Client struct {
	conn *grpc.ClientConn
	stub pb.TranscriberClient
}

func NewClient(addr string) (*Client, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgBytes),
			grpc.MaxCallSendMsgSize(maxMsgBytes),
		),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin",
			"healthCheckConfig": {"serviceName": ""}
		}`),
	)
	if err != nil {
		return nil, fmt.Errorf("dial whisper: %w", err)
	}
	return &Client{conn: conn, stub: pb.NewTranscriberClient(conn)}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Transcribe sends audio bytes to the Whisper service and returns the text.
// Generates an x-request-id and propagates it via metadata.
func (c *Client) Transcribe(ctx context.Context, audio []byte, mimeType string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", newRequestID())

	resp, err := c.stub.Transcribe(ctx, &pb.TranscribeRequest{
		Audio:    audio,
		MimeType: mimeType,
	})
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

func newRequestID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
```

(Replace `<github-user>` with the module path.)

- [ ] **Step 2: Verify it builds**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/whisper/client.go
git commit -m "Add gRPC client for Whisper service"
```

---

## Task 13: Implement Telegram middleware (panic recovery + error reply)

**Files:**
- Create: `logger-bot/internal/telegram/middleware.go`
- Create: `logger-bot/internal/telegram/middleware_test.go`

- [ ] **Step 1: Add the go-telegram/bot dependency**

```bash
cd ~/Documents/git-repos/logger-bot
go get github.com/go-telegram/bot@latest
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `logger-bot/internal/telegram/middleware_test.go`:

```go
package telegram

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// recordingHandler captures whether it was called and panics if asked to.
type recordingHandler struct {
	called bool
	panic  any
}

func (r *recordingHandler) handle(ctx context.Context, b *bot.Bot, u *models.Update) {
	r.called = true
	if r.panic != nil {
		panic(r.panic)
	}
}

func TestErrorReporter_PassesThroughOnSuccess(t *testing.T) {
	rec := &recordingHandler{}
	wrapped := ErrorReporter(rec.handle)
	wrapped(context.Background(), nil, &models.Update{
		Message: &models.Message{ID: 1, Chat: models.Chat{ID: 99}, Text: "hi"},
	})
	if !rec.called {
		t.Fatal("expected inner handler to be called")
	}
}

func TestErrorReporter_RecoversFromPanic(t *testing.T) {
	rec := &recordingHandler{panic: "boom"}
	wrapped := ErrorReporter(rec.handle)

	// Should not panic out of the middleware. (We pass nil bot; with no chat
	// available the middleware skips the SendMessage call but still recovers.)
	wrapped(context.Background(), nil, &models.Update{
		Message: nil,
	})
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/telegram/ -v
```

Expected: FAIL with `undefined: ErrorReporter`.

- [ ] **Step 4: Write the implementation**

Create `logger-bot/internal/telegram/middleware.go`:

```go
// Package telegram contains the bot's message handlers and middleware.
package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// ErrorReporter wraps a handler with a deferred recover() that turns panics
// into "Error: <msg>" replies. Mirrors the Python contract: the bot must
// not crash on a single bad message.
//
// Handlers should also return errors explicitly for known failure modes;
// this is the backstop for unexpected ones.
func ErrorReporter(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			slog.Error("handler panic", "panic", r)
			if update == nil || update.Message == nil || b == nil {
				return
			}
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("Error: %v", r),
			})
		}()
		next(ctx, b, update)
	}
}

// reply is a small convenience used by handlers to send a plain-text reply,
// swallowing the SendMessage error after logging it.
func reply(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		slog.Warn("send message failed", "err", err)
	}
}

// replyMarkdown sends a MarkdownV2 reply (used for JSON code blocks).
func replyMarkdown(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		slog.Warn("send markdown message failed", "err", err)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/telegram/ -v
```

Expected: 2 passed.

- [ ] **Step 6: Commit**

```bash
git add internal/telegram/middleware.go internal/telegram/middleware_test.go go.mod go.sum
git commit -m "Add panic-recovery middleware and reply helpers"
```

---

## Task 14: Implement the audio handler

**Files:**
- Create: `logger-bot/internal/telegram/audio.go`

The audio handler is the hot path. No unit test — it depends on `*bot.Bot`'s internals and the gRPC client. The e2e smoke test in Task 19 exercises the full path.

- [ ] **Step 1: Write the audio handler**

Dependencies:
- `*whisper.Client` — to transcribe
- `extraction.Parse` — pure function, called directly
- `*store.Writer` — to persist

Inject them via a constructor (matches the Python factory pattern).

Create `logger-bot/internal/telegram/audio.go`:

```go
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/<github-user>/logger-bot/internal/extraction"
	"github.com/<github-user>/logger-bot/internal/store"
	"github.com/<github-user>/logger-bot/internal/whisper"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AudioDeps wires the audio handler's two service dependencies.
type AudioDeps struct {
	Whisper *whisper.Client
	Writer  *store.Writer
}

// NewAudio returns the bot.HandlerFunc invoked for messages containing
// audio / voice / document. Mirrors the Python audio handler precedence:
// `audio or voice or document` (in that order).
func NewAudio(deps AudioDeps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil {
			return
		}
		fileID, mime, ok := pickAudio(msg)
		if !ok {
			reply(ctx, b, msg.Chat.ID, "No audio file found.")
			return
		}

		audio, err := downloadFile(ctx, b, fileID)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		if len(audio) > 20*1024*1024 {
			reply(ctx, b, msg.Chat.ID, "Error: file too large (>20MB)")
			return
		}

		text, err := deps.Whisper.Transcribe(ctx, audio, mime)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", friendlyTranscribeErr(err)))
			return
		}

		workout := extraction.Parse(text)
		if err := deps.Writer.Write(workout); err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}

		body, _ := json.Marshal(workout)
		slog.Info("transcribed and logged", "exercise", workout.Exercise, "reps", workout.Reps)
		reply(ctx, b, msg.Chat.ID, "I heard:\n"+string(body))
	}
}

// pickAudio mirrors `audio = msg.audio or msg.voice or msg.document` from the
// Python handler — checked in that order. Returns (fileID, mimeType, ok).
func pickAudio(msg *models.Message) (string, string, bool) {
	if msg.Audio != nil {
		return msg.Audio.FileID, msg.Audio.MimeType, true
	}
	if msg.Voice != nil {
		return msg.Voice.FileID, msg.Voice.MimeType, true
	}
	if msg.Document != nil {
		return msg.Document.FileID, msg.Document.MimeType, true
	}
	return "", "", false
}

// downloadFile resolves a Telegram file_id to a download URL and returns the bytes.
func downloadFile(ctx context.Context, b *bot.Bot, fileID string) ([]byte, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("getFile: %w", err)
	}
	url := b.FileDownloadLink(file)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	// 20MB cap as a hard backstop; Telegram's getFile already enforces this.
	return io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024+1))
}

// friendlyTranscribeErr converts gRPC status codes to user-facing messages.
func friendlyTranscribeErr(err error) string {
	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.InvalidArgument:
			return "could not decode audio"
		case codes.Internal:
			return "transcription failed"
		case codes.Unavailable:
			return "transcription service starting"
		case codes.DeadlineExceeded:
			return "transcription timed out"
		}
	}
	return err.Error()
}
```

(Replace `<github-user>` throughout with the actual module path used in `go.mod`.)

- [ ] **Step 2: Verify the build**

```bash
cd ~/Documents/git-repos/logger-bot
go mod tidy
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/telegram/audio.go go.mod go.sum
git commit -m "Add audio handler (download from Telegram, transcribe, parse, log)"
```

---

## Task 15: Implement `/last`, `/sql`, `/health` handlers

**Files:**
- Create: `logger-bot/internal/telegram/last.go`
- Create: `logger-bot/internal/telegram/sql.go`
- Create: `logger-bot/internal/telegram/health.go`

- [ ] **Step 1: Implement `/last`**

Create `logger-bot/internal/telegram/last.go`:

```go
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/<github-user>/logger-bot/internal/store"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewLast returns the handler for `/last <exercise>`.
//
// Empty result: replies "No previous workouts for <exercise>" — small UX
// improvement over the original which sent `null`.
func NewLast(reader *store.Reader) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil {
			return
		}
		args := commandArgs(msg.Text, "/last")
		switch len(args) {
		case 0:
			reply(ctx, b, msg.Chat.ID, "No exercise parsed")
			return
		case 1:
			// ok
		default:
			reply(ctx, b, msg.Chat.ID, "Only 1 arg is tolerated")
			return
		}
		exercise := args[0]

		row, err := reader.Last(exercise)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		if row == nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("No previous workouts for %s", exercise))
			return
		}
		body, _ := json.MarshalIndent(row, "", "  ")
		replyMarkdown(ctx, b, msg.Chat.ID, fmt.Sprintf("Last %s workout:\n```\n%s\n```", exercise, body))
	}
}

// commandArgs splits "/cmd a b c" -> ["a", "b", "c"].
func commandArgs(text, cmd string) []string {
	rest := strings.TrimSpace(strings.TrimPrefix(text, cmd))
	if rest == "" {
		return nil
	}
	return strings.Fields(rest)
}
```

- [ ] **Step 2: Implement `/sql`**

Create `logger-bot/internal/telegram/sql.go`:

```go
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/<github-user>/logger-bot/internal/store"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewSQL returns the handler for `/sql <query>`.
//
// Safety: the underlying connection is read-only (`?mode=ro`). SQLite itself
// rejects writes; no SQL parsing or allowlisting is needed here.
func NewSQL(reader *store.Reader) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil {
			return
		}
		query := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/sql"))
		if query == "" {
			reply(ctx, b, msg.Chat.ID, "No query detected")
			return
		}

		rows, err := reader.Query(query)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		body, _ := json.MarshalIndent(rows, "", "  ")
		replyMarkdown(ctx, b, msg.Chat.ID, fmt.Sprintf("```\n%s\n```", body))
	}
}
```

- [ ] **Step 3: Implement `/health`**

Create `logger-bot/internal/telegram/health.go`:

```go
package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func Health(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	reply(ctx, b, update.Message.Chat.ID, "Server is running")
}
```

- [ ] **Step 4: Verify the build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/telegram/last.go internal/telegram/sql.go internal/telegram/health.go
git commit -m "Add /last, /sql, /health handlers"
```

---

## Task 16: Wire `cmd/bot/main.go`

**Files:**
- Modify: `logger-bot/cmd/bot/main.go`

- [ ] **Step 1: Replace the placeholder `main.go`**

Replace `logger-bot/cmd/bot/main.go` with:

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/<github-user>/logger-bot/internal/store"
	"github.com/<github-user>/logger-bot/internal/telegram"
	"github.com/<github-user>/logger-bot/internal/whisper"
	"github.com/go-telegram/bot"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required env var missing", "key", key)
		os.Exit(2)
	}
	return v
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	token := mustEnv("TELEGRAM_API_TOKEN")
	whisperAddr := envOr("WHISPER_ADDR", "whisper:50051")
	dbPath := envOr("DB_PATH", "/data/log.db")

	writer, err := store.NewWriter(dbPath)
	if err != nil {
		slog.Error("open writer", "err", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader, err := store.NewReader(dbPath)
	if err != nil {
		slog.Error("open reader", "err", err)
		os.Exit(1)
	}
	defer reader.Close()

	wclient, err := whisper.NewClient(whisperAddr)
	if err != nil {
		slog.Error("dial whisper", "err", err)
		os.Exit(1)
	}
	defer wclient.Close()

	audioHandler := telegram.ErrorReporter(telegram.NewAudio(telegram.AudioDeps{
		Whisper: wclient,
		Writer:  writer,
	}))

	b, err := bot.New(token, bot.WithDefaultHandler(audioHandler))
	if err != nil {
		slog.Error("bot new", "err", err)
		os.Exit(1)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, "/last", bot.MatchTypePrefix,
		telegram.ErrorReporter(telegram.NewLast(reader)))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sql", bot.MatchTypePrefix,
		telegram.ErrorReporter(telegram.NewSQL(reader)))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/health", bot.MatchTypeExact,
		telegram.ErrorReporter(telegram.Health))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("Server up, you're ready to go", "whisper_addr", whisperAddr, "db_path", dbPath)
	b.Start(ctx)
	slog.Info("shutdown complete")
}
```

- [ ] **Step 2: Verify the build**

```bash
cd ~/Documents/git-repos/logger-bot
go build -o /tmp/bot ./cmd/bot && rm /tmp/bot
go vet ./...
```

Expected: no errors.

- [ ] **Step 3: Verify all tests still pass**

```bash
go test ./...
```

Expected: all green (parser tests + store tests + middleware tests).

- [ ] **Step 4: Commit**

```bash
git add cmd/bot/main.go
git commit -m "Wire entrypoint: env loading, deps, bot startup"
```

---

## Task 17: Add Dockerfile for the Go bot

**Files:**
- Create: `logger-bot/Dockerfile`
- Create: `logger-bot/.dockerignore`

- [ ] **Step 1: Add `.dockerignore`**

Create `logger-bot/.dockerignore`:

```
.git/
.gitignore
.idea/
.vscode/
.env
.env.local
docs/
e2e/
README.md
*.test
*.out
coverage.txt
internal/whisper/pb/
proto/
Makefile
.github/
```

- [ ] **Step 2: Add the multi-stage Dockerfile**

Create `logger-bot/Dockerfile`:

```dockerfile
# --- build stage ---
FROM golang:1.23-bookworm AS build

# Install protoc + plugins so codegen happens in the build
RUN apt-get update \
 && apt-get install -y --no-install-recommends protobuf-compiler make \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Need proto/ + Makefile + source
COPY . .
RUN make gen
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot

ENV WHISPER_ADDR=whisper:50051
ENV DB_PATH=/data/log.db

COPY --from=build /out/bot /usr/local/bin/bot
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
```

(Pure-Go SQLite means `CGO_ENABLED=0` works; the binary is fully static and runs on `distroless/static`.)

- [ ] **Step 3: Build the image locally**

```bash
cd ~/Documents/git-repos/logger-bot
docker build -t loggerbot:dev .
```

Expected: image builds successfully. Don't `docker run` it yet — it needs the Whisper service and a real Telegram token. We'll smoke-test via compose in Task 19.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "Add Dockerfile (multi-stage, distroless static)"
```

---

## Task 18: docker-compose

**Files:**
- Create: `logger-bot/docker-compose.yml`
- Create: `logger-bot/docker-compose.test.yml`
- Create: `logger-bot/.env.example`

- [ ] **Step 1: Add `.env.example`**

Create `logger-bot/.env.example`:

```
TELEGRAM_API_TOKEN=replace-me-with-your-bot-token
MODEL_SIZE=base
```

(`.env` itself is gitignored. Users `cp .env.example .env` and edit.)

- [ ] **Step 2: Add the main compose file**

Create `logger-bot/docker-compose.yml`:

```yaml
services:
  whisper:
    image: ttl.sh/loggerbot-whisper:24h
    environment:
      MODEL_SIZE: ${MODEL_SIZE:-base}
      GRPC_PORT: "50051"
    healthcheck:
      # Use grpc.health.v1.Health via grpc_health_probe (statically baked into the
      # whisper image at build time would be cleaner; for now we shell out to a
      # python one-liner that's already available).
      test: ["CMD", "python", "-c", "import grpc; from grpc_health.v1 import health_pb2, health_pb2_grpc; ch = grpc.insecure_channel('localhost:50051'); s = health_pb2_grpc.HealthStub(ch); s.Check(health_pb2.HealthCheckRequest()); print('ok')"]
      interval: 5s
      timeout: 3s
      retries: 30
      start_period: 60s
    restart: unless-stopped

  bot:
    image: ttl.sh/loggerbot:24h
    depends_on:
      whisper:
        condition: service_healthy
    environment:
      TELEGRAM_API_TOKEN: ${TELEGRAM_API_TOKEN}
      WHISPER_ADDR: whisper:50051
      DB_PATH: /data/log.db
    volumes:
      - loggerbot-data:/data
    restart: unless-stopped

volumes:
  loggerbot-data:
```

- [ ] **Step 3: Add the test override compose file**

Create `logger-bot/docker-compose.test.yml`:

```yaml
# Override that exposes Whisper's gRPC port to localhost so the e2e
# smoke test can hit it directly. Production runs do NOT use this file.
services:
  whisper:
    ports:
      - "50051:50051"
```

- [ ] **Step 4: Verify compose parses correctly**

```bash
cd ~/Documents/git-repos/logger-bot
TELEGRAM_API_TOKEN=dummy docker compose config > /dev/null
TELEGRAM_API_TOKEN=dummy docker compose -f docker-compose.yml -f docker-compose.test.yml config > /dev/null
```

Expected: no output, exit code 0.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml docker-compose.test.yml .env.example
git commit -m "Add docker-compose orchestration with healthcheck gating"
```

---

## Task 19: End-to-end smoke test

**Files:**
- Create: `logger-bot/e2e/test_compose.sh`
- Create: `logger-bot/e2e/hello.wav` (copied from the Whisper repo's fixture)

- [ ] **Step 1: Copy the WAV fixture from the Whisper repo**

```bash
cp ~/Documents/git-repos/logger-bot-whisper/tests/fixtures/hello.wav \
   ~/Documents/git-repos/logger-bot/e2e/hello.wav
```

- [ ] **Step 2: Write the smoke-test script**

Create `logger-bot/e2e/test_compose.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

REQUIRED=(docker grpcurl jq)
for cmd in "${REQUIRED[@]}"; do
  if ! command -v "$cmd" >/dev/null; then
    echo "missing required command: $cmd" >&2
    exit 1
  fi
done

# Use a dummy token; the bot service will fail to long-poll, but the Whisper
# service is what we're smoke-testing here. The bot's startup races the test.
export TELEGRAM_API_TOKEN="dummy-token-for-smoke-test"
export MODEL_SIZE="${MODEL_SIZE:-tiny}"

cleanup() {
  docker compose -f docker-compose.yml -f docker-compose.test.yml down -v
}
trap cleanup EXIT

echo "==> Starting compose..."
docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --wait whisper

echo "==> Sending hello.wav to Whisper..."
# Portable base64 (macOS BSD and Linux GNU): collapse newlines from `base64`
# output before interpolating into JSON.
AUDIO_B64=$(base64 < e2e/hello.wav | tr -d '\n')
RESPONSE=$(grpcurl -plaintext \
  -d "{\"audio\":\"${AUDIO_B64}\",\"mime_type\":\"audio/wav\"}" \
  -import-path proto -proto transcribe.proto \
  localhost:50051 \
  logger_bot.v1.Transcriber/Transcribe)

echo "Response: $RESPONSE"
echo "$RESPONSE" | jq -e '.text | test("hello"; "i")' > /dev/null

echo "==> Smoke test passed."
```

Make it executable:

```bash
chmod +x e2e/test_compose.sh
```

- [ ] **Step 3: Run the smoke test locally**

Prereqs (install if missing):

```bash
brew install grpcurl jq
# docker is already required for the rest of the project
```

Then:

```bash
cd ~/Documents/git-repos/logger-bot
./e2e/test_compose.sh
```

Expected output: `==> Smoke test passed.`

(The script `--wait`s for the `whisper` service only — the `bot` service will be in a CrashLoopBackOff because the Telegram token is fake. That's fine for this smoke test, which only validates the gRPC contract end-to-end.)

- [ ] **Step 4: Commit**

```bash
git add e2e/test_compose.sh e2e/hello.wav
git commit -m "Add docker-compose e2e smoke test"
```

---

## Task 20: GitHub Actions for the Go bot

**Files:**
- Modify: `logger-bot/.github/workflows/docker-publish.yml` (replace the existing Python-flavored one)

- [ ] **Step 1: Replace the workflow**

Replace the contents of `logger-bot/.github/workflows/docker-publish.yml` with:

```yaml
name: docker-publish

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  workflow_dispatch:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache: true

      - name: Install protoc + plugins
        run: |
          sudo apt-get update
          sudo apt-get install -y protobuf-compiler make
          go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
          echo "$(go env GOPATH)/bin" >> $GITHUB_PATH

      - run: make gen
      - run: go vet ./...
      - run: go test ./... -count=1

  publish:
    needs: test
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ttl.sh/loggerbot:24h

  smoke:
    needs: publish
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install grpcurl + jq
        run: |
          sudo apt-get update
          sudo apt-get install -y jq
          GRPCURL_VERSION=1.9.1
          curl -L "https://github.com/fullstorydev/grpcurl/releases/download/v${GRPCURL_VERSION}/grpcurl_${GRPCURL_VERSION}_linux_x86_64.tar.gz" \
            | sudo tar -xz -C /usr/local/bin grpcurl
      - run: ./e2e/test_compose.sh
        env:
          MODEL_SIZE: tiny
```

- [ ] **Step 2: Commit**

```bash
cd ~/Documents/git-repos/logger-bot
git add .github/workflows/docker-publish.yml
git commit -m "Replace CI with Go test + build + e2e smoke"
```

---

## Task 21: Update the bot repo's README

**Files:**
- Modify: `logger-bot/README.md`

- [ ] **Step 1: Rewrite the README to match the new architecture**

Replace `logger-bot/README.md` with:

```markdown
# Logger Bot

Telegram bot that ingests voice messages, transcribes them via a companion
[Whisper service](https://github.com/<github-user>/logger-bot-whisper), parses
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

The Whisper service lives at [logger-bot-whisper](https://github.com/<github-user>/logger-bot-whisper).
Its `proto/transcribe.proto` is a copy of this repo's; keep them in sync.
```

- [ ] **Step 2: Remove the now-obsolete imgs reference (if needed)**

The `imgs/` directory contains screenshots from the old Python flow. Keep
them — the bot's user-facing behavior is identical. No change needed.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Update README for Go + Whisper service architecture"
```

---

## Final verification

- [ ] **Step 1: Confirm both repos build and test cleanly**

```bash
cd ~/Documents/git-repos/logger-bot-whisper && uv run pytest -v
cd ~/Documents/git-repos/logger-bot         && go test ./... -count=1
```

Expected: all green in both.

- [ ] **Step 2: Confirm the e2e smoke test passes**

```bash
cd ~/Documents/git-repos/logger-bot
./e2e/test_compose.sh
```

Expected: `==> Smoke test passed.`

- [ ] **Step 3: Confirm with a real Telegram token**

This is the only test that requires user intervention — it can't be
automated without a real bot.

```bash
cd ~/Documents/git-repos/logger-bot
cp .env.example .env
$EDITOR .env       # set TELEGRAM_API_TOKEN
MODEL_SIZE=tiny docker compose up
```

Send a voice message in Telegram saying e.g. *"Sentadilla 5 reps 100 kg"*.
Expected reply within ~5–15s:

```
I heard:
{"Category":"","Exercise":"Sentadilla","Reps":5,"Weight":100,"Unit":"kg","RawText":"sentadilla 5 reps 100 kg"}
```

Run `/last sentadilla` and `/health` to confirm the other handlers work.

- [ ] **Step 4: Push and merge**

When the user confirms behavior matches expectations:

```bash
# logger-bot-whisper repo
cd ~/Documents/git-repos/logger-bot-whisper
git push -u origin main

# logger-bot repo (still on the feature branch)
cd ~/Documents/git-repos/logger-bot
git push -u origin refactor/go-whisper-split
gh pr create --title "Split into Go bot + Python Whisper service" --body "Implements docs/superpowers/specs/2026-05-06-split-whisper-go-bot-design.md"
```

Do not merge unless the user explicitly approves.

---

## Spec coverage cross-check

| Spec section / requirement | Task |
| --- | --- |
| Two repos, multi-repo layout | Task 1, 8 |
| `transcribe.proto` (copy-paste in both) | Tasks 2, 9 |
| `WhisperTranscriber` in-memory ffmpeg pipe | Task 3 |
| `TranscriberServicer` + gRPC status mapping | Task 4 |
| Whisper main entrypoint + grpc/health | Task 5 |
| Whisper Dockerfile | Task 6 |
| Whisper CI | Task 7 |
| Wipe Python, Go module init | Task 8 |
| Go proto codegen | Task 9 |
| `internal/extraction` + table-driven tests | Task 10 |
| `internal/store` Writer + read-only Reader | Task 11 |
| `internal/whisper` gRPC client + max msg size | Task 12 |
| `internal/telegram/middleware` panic recovery | Task 13 |
| Audio handler (file_id → bytes → gRPC → parse → DB) | Task 14 |
| /last (no-results UX), /sql (read-only safety), /health | Task 15 |
| Wiring in `cmd/bot/main.go` | Task 16 |
| Bot Dockerfile (distroless static) | Task 17 |
| docker-compose with healthcheck gate + named volume | Task 18 |
| e2e smoke test + override exposing 50051 | Task 19 |
| Bot CI (test + build + smoke) | Task 20 |
| Updated README | Task 21 |
| Request-ID propagation via gRPC metadata | Tasks 4, 12 |
| Schema unchanged (TEXT columns, `workout` table) | Task 11 |
| Audio bytes never on disk on either side | Tasks 3 (whisper), 14 (bot) |
