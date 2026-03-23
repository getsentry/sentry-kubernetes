# Agent Instructions

## Package Manager

Use **Go modules**: `go mod tidy`, `go mod download`

## Build & Dev Commands

| Command              | Description                                  |
| -------------------- | -------------------------------------------- |
| `make build`         | Build the Go module                          |
| `make build-dist`    | Build optimized production binary to `dist/` |
| `make build-docker`  | Build Docker image                           |
| `make test`          | Run all tests                                |
| `make test-coverage` | Run tests with coverage report               |
| `make vet`           | Run `go vet` static analysis                 |
| `make lint`          | Run `golangci-lint`                          |
| `make lint-fix`      | Lint with auto-fix                           |
| `make format`        | Format code and tidy modules                 |
| `make mod-tidy`      | Check go.mod tidiness (CI check)             |
| `make fmt-check`     | Check formatting (CI check)                  |
| `make upgrade-deps`  | Update all dependencies                      |
| `make help`          | Show all available commands                  |

## Commit Attribution

AI commits MUST include:

```
Co-Authored-By: (the agent model's name and attribution byline)
```

## Commit Messages

- Follow [Sentry commit conventions](https://develop.sentry.dev/engineering-practices/commit-messages/)
- Format: `<type>(<scope>): <subject>`
- Types: `feat`, `fix`, `ref`, `perf`, `docs`, `test`, `build`, `ci`, `chore`
- Imperative mood, no period, max 70 chars

## Project Structure

- `cmd/agent/main.go` - Application entry point
- `internal/agent/` - All business logic and handlers
- `k8s/manifests/` - Kubernetes deployment manifests
- `k8s/errors/` - Test error scenario manifests
- `.github/workflows/` - CI/CD pipelines

## Architecture

- **Read-only Kubernetes agent** - watches cluster events, sends to Sentry
- Never writes to the Kubernetes API
- Uses informers for Jobs, CronJobs, Deployments, ReplicaSets
- Uses watchers for Events and Pods
- Enhancers enrich Sentry events with K8s metadata
- Supports custom DSNs via `k8s.sentry.io/dsn` annotation

## Setup

- First-time setup: `make init` (installs Brew deps, Go modules, pre-commit hooks)
- Pre-commit hooks run: build, test, go fix, vet, lint, and dprint formatting

## Key Conventions

- Logging: `github.com/rs/zerolog`
- K8s client: `k8s.io/client-go` v0.35
- Error reporting: `github.com/getsentry/sentry-go`
- Docker: multi-stage build, `gcr.io/distroless/static:nonroot` runtime
- RBAC: read-only (`watch`, `list`, `get` only)
- Run `go fix ./...` to apply Go modernizations (also in pre-commit)

## Environment Variables

- `SENTRY_DSN` - Required. Sentry project DSN
- `SENTRY_ENVIRONMENT` - Sentry environment name
- `SENTRY_K8S_WATCH_NAMESPACES` - Comma-separated namespaces, `__all__` for all
- `SENTRY_K8S_MONITOR_CRONJOBS` - `1` to enable Crons integration
- `SENTRY_K8S_CUSTOM_DSNS` - `1` to enable per-resource DSN annotations
- `SENTRY_K8S_LOG_LEVEL` - `trace`/`debug`/`info`/`warn`/`error`/`fatal`/`panic`/`disabled`
- `SENTRY_K8S_GLOBAL_TAG_*` - Custom tags (prefix stripped)
- `SENTRY_K8S_CLUSTER_CONFIG_TYPE` - `auto`/`in-cluster`/`out-cluster`
