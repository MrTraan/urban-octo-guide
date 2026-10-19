#!/usr/bin/env bash
# Generates steady traffic so the Grafana dashboard has something to show.
# Usage: ./scripts/load.sh [seconds]   (default 60; Ctrl-C to stop early)
set -euo pipefail
cd "$(dirname "$0")/.."

DURATION="${1:-60}"
CLI=bin/playerctl
go build -o "$CLI" ./cmd/playerctl

TYPES=(level_complete level_failed purchase login)
COUNTRIES=(FR US DE JP BR)
END=$((SECONDS + DURATION))
n=0

echo "Sending traffic for ${DURATION}s... (dashboard: http://localhost:3000)"
while ((SECONDS < END)); do
  player="load-$((RANDOM % 50))"
  "$CLI" create  -player "$player" -name "Player $player" -country "${COUNTRIES[RANDOM % 5]}" >/dev/null
  "$CLI" record  -player "$player" -type "${TYPES[RANDOM % 4]}" -payload "{\"n\":$n}" >/dev/null
  "$CLI" summary -player "$player" >/dev/null
  if ((n % 10 == 0)); then   # an occasional NotFound so the error panels are not flat
    "$CLI" get -player "missing-$RANDOM" >/dev/null 2>&1 || true
  fi
  n=$((n + 1))
  sleep 0.2
done
echo "Done: $n iterations."
