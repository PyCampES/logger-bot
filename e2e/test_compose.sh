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
