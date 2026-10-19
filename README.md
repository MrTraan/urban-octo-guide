# Player Profile Service

A small gRPC + MongoDB service in Go that powers the **Player Profile** feature of a
game client: create/fetch/update a profile, record gameplay events, read recent
events, and get a one-call session summary.

## Quick start

Requires Docker. Two commands bring everything up and walk through the whole API:

```bash
make up     # mongo:7 + the service on localhost:8080
make demo   # builds the CLI and runs an end-to-end scenario
```

Then poke at it yourself:

```bash
bin/playerctl create  -player p1 -name Nathan -country FR
bin/playerctl record  -player p1 -type level_complete -payload '{"level":3,"stars":2}'
bin/playerctl events  -player p1 -limit 5
bin/playerctl summary -player p1
```

Server reflection is on, so `grpcurl` works too:

```bash
grpcurl -plaintext localhost:8080 list player.v1.PlayerService
grpcurl -plaintext -d '{"player_id":"p1"}' localhost:8080 player.v1.PlayerService/GetSessionSummary
grpcurl -plaintext localhost:8080 grpc.health.v1.Health/Check
```

Other targets: `make test` (integration + unit, needs Docker), `make test-unit`,
`make lint`, `make run` (local server against `MONGO_URI`), `make proto`
(regenerate stubs), `make down`. Local builds need Go 1.26+.

| Env var      | Default                     |
|--------------|-----------------------------|
| `GRPC_ADDR`  | `:8080`                     |
| `MONGO_URI`  | `mongodb://localhost:27017` |
| `MONGO_DB`   | `player`                    |
| `PLAYER_ADDR` (CLI only) | `localhost:8080` |

## API

Defined once in [`proto/player/v1/player.proto`](proto/player/v1/player.proto);
generated Go code is committed under `gen/` so reviewers don't need `protoc`.

| RPC                 | Purpose                                                        |
|---------------------|----------------------------------------------------------------|
| `CreateProfile`     | Create on first play. **Idempotent**: a second call returns the existing profile unchanged. |
| `GetProfile`        | Fetch; `NOT_FOUND` if missing.                                 |
| `UpdateProfile`     | Partial update of `display_name` / `country` (proto3 `optional`). |
| `RecordEvent`       | Store an event: type, timestamp (defaults to now), free-form JSON payload. |
| `ListRecentEvents`  | Newest first, `limit` defaults to 20, capped at 100.           |
| `GetSessionSummary` | Profile + recent events + event counts by type over `window_days` (default 7). |

Errors are plain gRPC statuses: `INVALID_ARGUMENT`, `NOT_FOUND`, `INTERNAL`.

## Approach and design choices

**Scope first.** The brief asks for a small surface built well, in ~4h. So: one proto
file, one domain package, no framework, no interfaces that have a single
implementation. Everything a reviewer needs to read is in four files
(`store.go`, `service.go`, `validate.go`, `cmd/server/main.go`).

**gRPC + protobuf.** The consumer is a game client, not a browser: a typed contract,
small binary payloads and generated clients fit better than REST. Reflection is on
so the API stays explorable with `grpcurl`. The standard `grpc.health.v1` service
replaces the scaffold's `/healthz`, and tracks Mongo reachability.

**Layout.**

```
proto/player/v1/     the contract
gen/player/v1/       generated code (committed)
cmd/server/          wiring: config, logging, health, reflection, graceful shutdown
cmd/playerctl/       CLI client, one subcommand per RPC
internal/player/     store.go (Mongo) -> service.go (handlers) -> validate.go
internal/mongo/      scaffold's connect helper, unchanged except one BSON option
scripts/demo.sh      end-to-end scenario
```

Every handler has the same three-line shape: validate, call the store, convert to
proto. The store returns small private structs that mirror the Mongo documents;
proto messages are the only public types.

**Data model.**

- `profiles`: `_id` is the player id (client-supplied device/account id), so lookups
  need no extra index. `created_at` / `updated_at` for auditability.
- `events`: `{player_id, type, timestamp, payload}` with one compound index
  `{player_id: 1, timestamp: -1}`. Every event query is "this player, newest first",
  including the summary's `$match` + `$group` aggregation, so one index serves all.
- Payload is `google.protobuf.Struct` on the wire and a BSON sub-document in Mongo:
  free-form, yet still queryable server-side later.

**Idempotent create.** "Create on first play" means the client often doesn't know if
it is the first play. `CreateProfile` is a single `FindOneAndUpdate` with
`$setOnInsert` + upsert: safe to retry, no read-then-write race, one round trip.

**Session summary** fans out its three independent reads with `errgroup`. It's the
one place concurrency buys latency for the client, and it is three lines.

**Validation** is deliberately tiny and explicit (`validate.go`): bounded string
lengths, ISO alpha-2 country, clamped limits. Bad input gets `INVALID_ARGUMENT`
with a readable message.

**Operations.** Structured JSON logs via `log/slog` with one line per RPC (method,
status code, duration) from a single interceptor. Graceful shutdown on
SIGINT/SIGTERM lets in-flight RPCs finish before Mongo is disconnected. Distroless,
non-root container image.

**Tests.** Integration tests run the real `Service` against a real Mongo
(testcontainers) over an in-memory gRPC connection (`bufconn`), so they exercise the
exact code path a client hits, including proto round-trips and status codes. Each
test uses its own player id for isolation. Validation has plain table-driven unit
tests; `go test -short` runs only those.

## What I'd do with another day

- **Identity.** Take the player id from an authenticated token (interceptor), not
  the request body, so players can only touch their own data.
- **Pagination** for events with an opaque cursor (`timestamp` + `_id`) instead of
  a bare limit.
- **Observability.** OpenTelemetry tracing + RED metrics per RPC; a Mongo
  slow-query log.
- **Data lifecycle.** A TTL index or archival job on `events`; the summary's
  per-type counts could become a pre-aggregated daily document if the 7-day
  aggregation ever shows up in p99.
- **Payload hygiene.** A size cap and per-type schema versioning for payloads.
- **Hardening.** Request rate limiting, server-side timeouts via interceptor,
  `golangci-lint` + CI (lint, unit, integration), image scanning.

## AI usage notes

Claude (Claude Code) was used throughout as a pair programmer:

- Drafting the design plan from the brief and iterating on the choices above
  (transport, create semantics, tooling) before writing code.
- Generating first drafts of the store/service/tests/CLI, which I reviewed and
  reshaped for readability.
- Debugging two test failures (BSON `bson.M` vs `map[string]any` when converting
  payloads to `structpb`, and millisecond timestamp precision in Mongo).
- Writing this README from the design notes.
