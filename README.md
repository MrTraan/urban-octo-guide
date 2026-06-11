# Player Profile API — Home Assignment Scaffold

Minimal starter for the Madbox Senior Backend home assignment. Only the
truly boilerplate wiring is provided; everything else is part of what we
evaluate.

## What's wired for you

- `main.go` — dials Mongo via `internal/mongo.Connect` on startup and
  starts an HTTP server on `HTTP_ADDR`; registers a single example
  `GET /healthz` handler that pings Mongo
- `internal/mongo/client.go` — a small helper that dials Mongo and pings
- `main_test.go` — a `TestMain` that boots a real Mongo via
  `testcontainers-go`, connects, wires `newMux` into an `httptest` server,
  and exposes `testMongo` and `testServer` as package globals for your
  tests. Includes a minimal `TestHealthz` to show the pattern; add your
  own `Test…` functions alongside it.

## What's up to you

Everything else — business handlers, schema, indexes, graceful shutdown,
middleware, structured logging, tracing, concurrency, Dockerfile/compose,
Makefile, lint config, and your broader test strategy — is part of the
assignment. There is no "right" layout; we evaluate the choices you make.

The `/healthz` endpoint and the `internal/mongo` package are reference
examples, not requirements — delete or rework either if your design wants
something else.

## Run

```bash
go run .
# or with a custom port:
HTTP_ADDR=:9000 go run .
```

The server dials Mongo on startup at `MONGO_URI` (default
`mongodb://localhost:27017`). Bring up Mongo however you like — a local
install, `docker run mongo:7`, a compose file you write, etc. A quick
option:

```bash
docker run -d --rm -p 27017:27017 --name scaffold-mongo mongo:7
```

## Test

```bash
go test ./...
```

Integration tests require Docker (testcontainers will pull `mongo:7` on
first run).

## Configuration

| Var | Default |
|---|---|
| `HTTP_ADDR` | `:8080` |
| `MONGO_URI` | `mongodb://localhost:27017` |

## Notes

- AI usage is encouraged. Keep brief notes on what you used it for and
  include them at the end of this README before submission.
