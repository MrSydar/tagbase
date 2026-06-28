# Contributing to tagbase

Thank you for your interest in contributing!

## Development Workflow

### Prerequisites

- Go 1.21+
- Docker and Docker Compose (for running the full stack)

### Building

```bash
make all        # builds storage, tagger, and CLI client binaries into bin/
make build-storage
make build-tagger
make build-client
```

### Formatting

Before opening a pull request, run:

```bash
make fmt
```

This runs `gofmt -w` on all `.go` files in `storage/` and `tagger/`. CI will reject unformatted code.

### Running Tests

End-to-end tests require the full Docker stack running:

```bash
make docker-up
make e2e
make docker-down
```

### Pull Request Checklist

- [ ] `make fmt` has been run and there are no formatting changes left
- [ ] New code includes tests where applicable
- [ ] `make all` builds successfully

### Getting Help

Open an issue if you have questions before starting on a large change.
