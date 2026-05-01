#!/usr/bin/env bash
set -euo pipefail

uv run sqlite-utils insert log.db logs log.csv --csv
# Launch Datasette locally
lsof -ti :8001 

PORT=8001
PID=$(lsof -ti :$PORT)
if [ -n "$PID" ]; then
  kill "$PID"
  echo "Killed process $PID running on port $PORT."
fi

while ss -ltn | grep -q ":$PORT "; do
  echo "Port $PORT is still in use. Waiting..."
  sleep 0.1
done

uv run datasette log.db
