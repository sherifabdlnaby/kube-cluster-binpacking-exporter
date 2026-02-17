# CLAUDE.md

## Project Overview

Prometheus exporter that monitors Kubernetes cluster binpacking efficiency. Compares pod resource requests against node allocatable capacity using informer-based caching (zero API calls per scrape).

**Purpose**: Helps identify scheduling inefficiency by showing how well pods are packed onto nodes. High allocatable with low allocated means wasted capacity. High allocated means good utilization.

## Architecture

- **Flat layout**: 3 Go files in `package main` at root — no `pkg/`, `internal/`, `cmd/`
- **Plain client-go + informers**: No controller-runtime. This is an exporter, not a controller
- **Resource-agnostic**: Metrics use a `resource` label. Adding GPU/ephemeral-storage is config-only (`--resources`)
- **Scrape-time computation**: `MustNewConstMetric` in `Collect()` — avoids stale metrics for removed nodes
- **Init container aware**: Correctly accounts for init containers using Kubernetes scheduler semantics

## File Map

| File | Role | Key Functions |
|------|------|---------------|
| `main.go` | Entry point, HTTP server | Flag parsing, signal handling, `/metrics`, `/healthz`, `/readyz`, `/sync` endpoints |
| `kubernetes.go` | Kube client setup | `setupKubernetes()` - config resolution, informer factory, cache sync with progress logging |
| `collector.go` | Prometheus collector | `Collect()` - computes metrics, `calculatePodRequest()` - init container logic |
| `Dockerfile` | Container image | Multi-stage: `golang:1.25-alpine` → `distroless/static-debian12:nonroot` |
| `chart/` | Helm chart | RBAC, ServiceMonitor, published to OCI registry at ghcr.io |
| `.github/workflows/` | CI/CD | `ci.yaml` - build/vet/lint/test, `release.yaml` - multi-arch Docker + GoReleaser + Helm OCI push, `auto-release.yaml` - semantic versioning from PR labels |

## Metrics Exported

All metrics computed at scrape time from informer cache:

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `binpacking_node_allocated` | Gauge | `node`, `resource` | Total resource requests on node |
| `binpacking_node_allocatable` | Gauge | `node`, `resource` | Node capacity |
| `binpacking_node_utilization_ratio` | Gauge | `node`, `resource` | allocated / allocatable (0.0-1.0+) |
| `binpacking_cluster_allocated` | Gauge | `resource` | Cluster-wide total requests |
| `binpacking_cluster_allocatable` | Gauge | `resource` | Cluster-wide capacity |
| `binpacking_cluster_utilization_ratio` | Gauge | `resource` | Cluster-wide ratio |
| `binpacking_cache_age_seconds` | Gauge | - | Time since last informer sync |

## HTTP Endpoints

| Endpoint | Purpose | Returns |
|----------|---------|---------|
| `/` | Homepage | HTML page with links to all endpoints and configuration |
| `/metrics` | Prometheus scrape target | Text exposition format |
| `/healthz` | Liveness probe | 200 if process alive |
| `/readyz` | Readiness probe | 200 if cache synced, 503 otherwise |
| `/sync` | Cache status | JSON with last sync time, age, resync period, sync state |

## Build & Verify

```bash
# Build
go build -o kube-cluster-binpacking-exporter .

# Verify
go vet ./...
helm lint chart

# Run locally
go run . --kubeconfig ~/.kube/config
go run . --kubeconfig ~/.kube/config --debug           # verbose
go run . --resync-period=1m --debug                     # fast resync
go run . --list-page-size=500 --debug                   # with pagination (recommended for >1000 pods)
go run . --list-page-size=0                             # disable pagination (small clusters)
```

## Testing

**Quick start**: `go test -v ./...` | **Coverage**: `go test -coverprofile=coverage.out ./...`

Tests use mock listers (no cluster required) and cover:
- **Init container logic** - `calculatePodRequest()` follows K8s scheduler semantics (`max(sum_regular, max_init)`)
- **Pod filtering** - Running/pending included, unscheduled/terminated excluded
- **Metrics collection** - Node + cluster aggregates, cache age
- **HTTP endpoints** - `/healthz`, `/readyz`, `/sync`
- **Error handling** - Lister failures, missing sync info, edge cases

**CI**: Tests run on every push (`go test`, `go vet`, `golangci-lint`)

See [TESTING.md](TESTING.md) for detailed test infrastructure, conventions, helpers, and TDD workflow.

## Key Design Decisions

**Logging**: `slog` stdlib (JSON, no deps) | `--debug` enables verbose mode with conditional event handlers (zero production overhead)

**Kubernetes**: Config resolution: flag → kubeconfig → in-cluster | Fail-fast `ServerVersion()` check | 2-min sync timeout with progress logging every 5s

**Pod Accounting**: `max(sum_regular, max_init)` matches K8s scheduler | Filters unscheduled (`NodeName=""`) and terminated (`Succeeded|Failed`) pods

**Metrics**: Scrape-time `MustNewConstMetric` (auto-handles node churn) | Custom registry (no Go runtime metrics) | `resource` label for extensibility | Cache age metric for stale alerts

**Health Checks**: `/healthz` = process alive, `/readyz` = cache synced | Readiness: 5s delay/10s period, Liveness: 10s delay/30s period

**Informers**: `--resync-period` (default 5m) | SharedInformerFactory (single watch) | `--list-page-size` pagination (default 500, ~40% memory reduction, client-go handles Continue tokens)

## Conventions

### Code Style
- **Flat structure**: No `pkg/` or `internal/` until genuinely needed
- **Package main**: All code in main package — this is a simple binary, not a library
- **Helper functions at top**: `calculatePodRequest()` defined before `Collect()` that uses it

### Naming
- **Flags**: Use `--kebab-case` (Go `flag` package standard)
- **Helm templates**: Use `binpacking-exporter.*` helper prefix for consistency
- **Metrics**: All prefixed with `binpacking_` for namespacing
- **Port**: 9101 (avoids collision with node-exporter on 9100, Prometheus on 9090)

### Configuration
- **Defaults optimized for production**: 5m resync, info logging, port 9101
- **Debug mode changes behavior**: Adds event handlers, increases log verbosity
- **Helm values mirror flags**: `debug`, `resyncPeriod`, `resources` directly map to CLI flags

## Dependencies

Minimal footprint — only official Kubernetes and Prometheus libraries:

| Package | Purpose |
|---------|---------|
| `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` | Kubernetes client, informers, listers, resource types |
| `github.com/prometheus/client_golang` | Prometheus collector interface, HTTP handler |

No controller-runtime, no operator SDK — just the essentials.

## Release Process

**Fully automated** via PR labels on merge to main:

| Label | Bump | Example | Use When |
|-------|------|---------|----------|
| `major` | Breaking | 1.2.3 → 2.0.0 | API changes, removed features |
| `minor` | Feature | 1.2.3 → 1.3.0 | New functionality (backward-compatible) |
| `patch` | Fix | 1.2.3 → 1.2.4 | Bug fixes, docs (default if no label) |
| `skip-release` | None | - | CI/test changes only |

**Flow**: PR + label → merge → auto-tag → release (Docker + Helm + binaries + changelog)

**Artifacts**:
- **Docker**: `ghcr.io/sherifabdlnaby/kube-cluster-binpacking-exporter:v1.2.3` (linux/amd64, linux/arm64)
- **Helm**: `oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter:1.2.3`
- **Binaries**: Linux/macOS/Windows (multiple architectures)

**Install**: `helm install binpacking-exporter oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter`

**Manual** (if needed): `git tag v1.0.0 && git push origin v1.0.0`

## Troubleshooting

### Exporter won't start
1. Check logs with `--debug` for detailed error messages
2. Verify kubeconfig is valid: `kubectl cluster-info`
3. Verify RBAC permissions (needs get/list/watch on nodes and pods)

### Cache sync hangs
- Check API server connectivity: `kubectl cluster-info`
- Look for "still waiting for cache sync" debug logs showing which informer is stuck
- Informer sync timeout is 2 minutes — check if API server is responding slowly

### Metrics show zero
- Check `/readyz` — if not ready, cache hasn't synced yet
- Verify pods have resource requests defined (we measure requests, not limits)
- Enable `--debug` to see pod filtering decisions

### Cache age keeps growing
- Check `/sync` endpoint for sync state
- Verify API server watch connections aren't dropping
- Consider shorter `--resync-period` if needed

## Future Enhancements

See `TODO.md` for planned features:
- Per-node-label binpacking calculations
- Human-readable log output option with colors
- Unit tests with coverage reporting
- Event-handler based pre-computation for O(nodes) scrapes
- Paginated initial list for large clusters
