# Tagbase

A storage system for collections of objects with sparse boolean tags evaluated on demand during queries. Tags are stored only when known; absence means unknown until evaluated by the tagging engine.

**Status:** implemented MVP.

---

## Architecture

```
┌─────────────┐      HTTP      ┌─────────────┐
│   Clients   │ ◄────────────► │   Storage   │
└─────────────┘                │   Service   │
                               └──────┬──────┘
                                      │
                         ┌────────────┼────────────┐
                         │            │            │
                    ┌────▼────┐ ┌─────▼─────┐ ┌───▼────┐
                    │ Postgres│ │  Tagging  │ │   S3   │
                    │ metadata│ │  Engine   │ │payloads│
                    └─────────┘ └───────────┘ └────────┘
```

**Services**

| Service | Module | Port | Role |
|---------|--------|------|------|
| [storage](storage/) | `mrsydar/tagbase/storage` | `:8080` | HTTP API for collections, objects, tag queries |
| [tagger](tagger/) | `mrsydar/tagbase/tagger` | `:8081` | Evaluates tags by fetching object data from storage |

**Infra**

| Service | Image | Port | Role |
|---------|-------|------|------|
| postgres | `postgres:15` | `:5432` | Collections, object metadata, tag values |
| minio | `minio/minio` | `:9000` / `:9001` | S3-compatible object storage |

---

## Quick Start

Requirements: Docker + Docker Compose.

```bash
docker compose up --build
```

Wait for services to become healthy (~10-15s).

**Test**

```bash
# 1. Create a collection
curl -s -X POST http://localhost:8080/v1/collections \
  -H "Content-Type: application/json" \
  -d '{"name":"jobs","data_type":"txt"}'

# 2. Upload an object
curl -s -X POST "http://localhost:8080/v1/collections/jobs/objects?data_type=txt" \
  -H "Content-Type: application/octet-stream" \
  -d 'hello golang qa'

# 3. Query objects by tags
curl -s -X POST http://localhost:8080/v1/collections/jobs/objects/query \
  -H "Content-Type: application/json" \
  -d '{"tags":{"golang":true},"limit":5}'

# 4. Inspect object tags directly
curl -s "http://localhost:8080/v1/collections/jobs/objects/{id}/tags?tags=golang,qa"
```

---

## End-to-End Tests

The `e2e/` directory contains end-to-end tests that exercise the storage public API against a live Docker Compose stack.

**Prerequisites:**
- `docker compose up --build` is running
- Storage readyz returns 200

**Run:**

```bash
cd e2e
GOWORK=off go test -v -count=1 .
```

The test suite covers: collections CRUD, object upload/retrieval/deletion, idempotent uploads, tag evaluation via the tagger, tag queries with AND semantics, and pagination.

---

## Project Structure

```
tagbase/
├── e2e/               # End-to-end tests (storage API only)
├── storage/           # Storage service
│   ├── cmd/storage/     # main entry point
│   ├── internal/        # private implementation
│   ├── pkg/client/      # public Go client for storage API
│   ├── migrations/
│   ├── Dockerfile
│   └── README.md
├── tagger/            # Tagging engine
│   ├── cmd/tagger/      # main entry point
│   ├── internal/        # private implementation
│   ├── pkg/client/      # public Go client (implements storage Tagger interface)
│   ├── Dockerfile
│   └── README.md
├── compose.yaml
├── go.work
└── README.md
```

---

## Configuration Overview

See each service's README for full env var documentation.

| Env Var | Default | Description |
|---------|---------|-------------|
| `TAGBASE_HTTP_ADDR` | `:8080` | Storage service listen address |
| `TAGBASE_PG_DSN` | — | Postgres connection string |
| `TAGBASE_S3_ENDPOINT` | — | S3-compatible endpoint |
| `TAGBASE_TAG_ENGINE_URL` | — | URL of the tagging engine |
| `TAGGER_HTTP_ADDR` | `:8081` | Tagger listen address |
| `TAGGER_STORAGE_BASE_URL` | `http://localhost:8080` | Storage service URL the tagger calls |
| `TAGGER_EVALUATOR_IMPL` | `false` | Evaluator to use: `grep` (substring match for `txt`) or `false` (all tags `false`) |

---

> **Security Note:** `compose.yaml` contains default development credentials (e.g. `minioadmin`, `tagbase`). These are intended for local development only. Do not use them in production.

## License

MIT
