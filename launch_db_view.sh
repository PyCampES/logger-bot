#!/usr/bin/env bash
set -euo pipefail

# Only needed if the initial export was on csv
# uv run sqlite-utils insert log.db workout log.csv --csv
DB_FILE="${1:-./log.db}"
   
PORT=8001
PID=$(lsof -ti :$PORT || true)
if [ -n "$PID" ]; then
  echo "in loop"
  kill "$PID"
  echo "Killed process $PID running on port $PORT."
else
  echo "No process running on port $PORT, continue."
fi

while ss -ltn | grep -q ":$PORT "; do
  echo "Port $PORT is still in use. Waiting..."
  sleep 0.1
done
uv run datasette "${DB_FILE}"
