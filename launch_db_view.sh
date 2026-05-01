#!/usr/bin/env bash
set -euo pipefail

sqlite-utils insert log.db logs log.csv --csv
# Launch Datasette locally
datasette log.db
