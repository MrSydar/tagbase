# AGENTS.md

Tagbase is a Go monorepo with two workspace modules. It is simpler than it looks — the key is remembering that both modules exist and one imports the other.

## Monorepo layout

- Root uses `go.work` with Go 1.26.4.
- Modules: `storage/` (`mrsydar/tagbase/storage`) and `tagger/` (`mrsydar/tagbase/tagger`).
- Cross-module dependency: `tagger` imports `storage/pkg/client`.
  - Docker builds always copy both modules.
  - Building locally from a module directory works because Go workspace resolves the sibling module.

## Running the full stack

```bash
docker compose up --build
```

- Postgres (`:5432`), MinIO (`:9000`, `:9001`), tagger (`:8081`), and storage (`:8080`) all come up.
- Storage waits for tagger to be healthy before starting (`depends_on` with `condition: service_healthy`).
- Storage fails fast on startup if it cannot fetch supported types from tagger.
- Wait for healthy; then test via `README.md` Quick Start curl commands.

## Building / running a single module locally

Requires Postgres + S3-compatible storage running (e.g., the Docker Compose infra).

**Storage service:**

```bash
cd storage
go mod download
go run ./cmd/storage
```

**Tagger service:**

```bash
cd tagger
go mod download
go run ./cmd/tagger
```

**Build binaries:**

```bash
cd storage && go build -o /tmp/storage ./cmd/storage
cd tagger && go build -o /tmp/tagger ./cmd/tagger
```

## CLI client

A reference CLI client exists in the storage module:

```bash
cd storage
go run ./cmd/client --url http://localhost:8080 <command>
```

See `storage/cmd/client/main.go` for available commands.

## Testing

**Unit tests:** `storage/` and `tagger/` have none currently. Add `*_test.go` files as usual — standard Go only.

**End-to-end tests:** In the `e2e/` directory (its own module, outside the workspace):

```bash
cd e2e
GOWORK=off go test -v -count=1 .
```

Requires the Docker Compose stack running and `storage` readyz returning `200`.

## Important quirks

- **Custom migrations runner:** Storage applies migrations on startup by executing all `*.up.sql` files in `storage/migrations/` in lexicographic order. It is not using `golang-migrate`.
- **Tagger dogfoods storage public API:** The tagger only talks to storage via the public `pkg/client` HTTP client (no internal APIs, no DB access). Storage's `readyz` checks DB + S3; tagger's `readyz` always returns 200.
- **Startup ordering matters:** Storage must reach tagger on startup to fetch supported types. In Docker Compose this is enforced by `depends_on` + healthchecks. Running storage standalone without tagger causes a fatal error.
- **Hash-based idempotency:** Object upload computes SHA-256 over raw bytes. Duplicate uploads in the same collection return the existing object; the newly uploaded S3 object is not retained.
- **No CI, Makefile, or task runner:** Everything is command-line or Docker Compose. Do not assume a build script exists.

## References

- `README.md` — architecture, API shapes, quick start curls

- `storage/README.md` — env vars, CLI client usage, internal structure
- `tagger/README.md` — tag evaluation logic, env vars
