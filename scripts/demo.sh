#!/usr/bin/env bash
# End-to-end walkthrough of the Player Profile API against a running server.
# Usage: ./scripts/demo.sh            (server at localhost:8080)
#        PLAYER_ADDR=host:port ./scripts/demo.sh
set -euo pipefail
cd "$(dirname "$0")/.."

PLAYER="demo-$(date +%s)"
CLI=bin/playerctl

step() { printf '\n\033[1;34m== %s\033[0m\n' "$*"; }
run()  { printf '\033[2m$ playerctl %s\033[0m\n' "$*"; "$CLI" "$@"; }

go build -o "$CLI" ./cmd/playerctl

step "Create profile on first play"
run create -player "$PLAYER" -name Nathan -country FR

step "Create again: idempotent, returns the same profile"
run create -player "$PLAYER" -name Impostor -country US

step "Update only the country"
run update -player "$PLAYER" -country DE

step "Record three gameplay events"
run record -player "$PLAYER" -type level_complete -payload '{"level":1,"stars":3}'
run record -player "$PLAYER" -type level_complete -payload '{"level":2,"stars":2}'
run record -player "$PLAYER" -type purchase -payload '{"sku":"gems_100","price":{"amount":1.99,"currency":"EUR"}}'

step "Read the 2 most recent events"
run events -player "$PLAYER" -limit 2

step "Session summary: profile + recent events + counts by type (7 days)"
run summary -player "$PLAYER"

step "Errors are typed gRPC statuses"
run get -player does-not-exist || true
run create -player "$PLAYER" -name x -country France || true

printf '\n\033[1;32mDone. Player id: %s\033[0m\n' "$PLAYER"
