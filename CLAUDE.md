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

Comprehensive unit tests cover core logic, HTTP endpoints, and edge cases. Tests use mock listers to avoid requiring a real Kubernetes cluster.

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...

# View coverage report in browser
go tool cover -html=coverage.out

# View coverage summary
go tool cover -func=coverage.out

# Run specific test
go test -v -run TestCalculatePodRequest

# Run tests with race detector
go test -race ./...
```

### Test Files

| File | Coverage | Key Tests |
|------|----------|-----------|
| `collector_test.go` | Collector logic | Init container accounting, pod filtering, metric collection, error handling |
| `kubernetes_test.go` | Kubernetes setup | SyncInfo struct, readiness checker function |
| `main_test.go` | HTTP handlers | `/healthz`, `/readyz`, `/sync` endpoints, resource parsing |

### Test Infrastructure

**Mock Listers**: Fake implementations avoid real Kubernetes dependencies
```go
fakeNodeLister   // Returns test nodes, no API calls
fakePodLister    // Returns test pods, no API calls
```

**Helper Functions**: Create test objects consistently
```go
makeContainer(name, cpu, memory)            // Create container with resource requests
makeNode(name, cpu, memory)                 // Create node with allocatable resources
makePodWithResources(...)                   // Create pod with containers and init containers
```

**Float Comparison**: `floatEquals()` handles floating-point precision issues
```go
floatEquals(a, b float64) bool  // Uses epsilon for approximate equality
```

### What's Tested

**Init Container Logic** (`TestCalculatePodRequest`)
- Regular containers only (sum of requests)
- Init container dominates (max init > sum regular)
- Regular containers dominate (sum regular > max init)
- Multiple init containers (takes max)
- Missing resource requests
- Memory vs CPU resource calculations

**Pod Filtering** (`TestBinpackingCollector_PodFiltering`)
- Running pods (included)
- Pending pods with NodeName (included)
- Unscheduled pods - no NodeName (excluded)
- Succeeded/Failed pods (excluded)

**Metrics Collection** (`TestBinpackingCollector_Collect`)
- Per-node metrics (allocated, allocatable, utilization)
- Cluster-wide aggregates
- Cache age metric
- Correct metric counts (nodes × resources)

**HTTP Endpoints**
- `/healthz`: Always returns 200 OK
- `/readyz`: Returns 200 if cache synced, 503 otherwise
- `/sync`: Returns JSON with cache state

**Error Handling** (`TestBinpackingCollector_ErrorHandling`)
- Node lister failures
- Pod lister failures
- Missing SyncInfo (no cache age metric)

**Edge Cases**
- Zero allocatable resources
- Debug logging enabled (no crashes)
- Empty pods/nodes

### Testing Conventions

**Table-Driven Tests**: Each test uses subtests with descriptive names
```go
tests := []struct {
    name     string
    input    X
    expected Y
}{...}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { ... })
}
```

**Test Names**: Use descriptive names that explain the scenario
```
TestCalculatePodRequest/init_container_dominates
TestBinpackingCollector_PodFiltering/unscheduled_pod
```

**Coverage Target**: Aim for >80% coverage on core logic (collector, kubernetes setup)
- Use `go test -coverprofile=coverage.out` to measure
- Exclude error paths that can't realistically happen
- Focus on business logic over boilerplate

**Test Isolation**: Each test is independent
- Creates its own fake listers
- No shared state between tests
- Tests can run in parallel (future enhancement)

### Adding New Tests

When adding new functionality:

1. **Write test first** (TDD approach):
   ```bash
   # Create test case
   go test -v -run TestNewFunction
   # Implement function
   # Verify test passes
   ```

2. **Use existing helpers** for test data:
   ```go
   pod := makePodWithResources("default", "test", "node-1", corev1.PodRunning,
       []corev1.Container{makeContainer("app", "100m", "128Mi")}, nil)
   ```

3. **Check coverage impact**:
   ```bash
   go test -coverprofile=coverage.out ./...
   go tool cover -func=coverage.out | grep "total:"
   ```

4. **Update this section** if adding new test patterns or infrastructure

### CI Integration

Tests run automatically on every push via `.github/workflows/ci.yaml`:
- `go test ./...` - Run all tests
- `go vet ./...` - Static analysis
- `golangci-lint run` - Linter checks

See `TODO.md` for planned testing enhancements:
- Makefile for standardized test commands
- Coverage reporting in CI
- Benchmark tests for performance optimization paths

## Key Design Decisions

### Logging
- **`slog` stdlib**: Structured JSON logging, no external deps
- **Debug flag**: `--debug` enables verbose logging (pod filtering, resource calculations, informer events)
- **Conditional event handlers**: Only registered when debug enabled via `logger.Enabled(ctx, slog.LevelDebug)` — zero overhead in production

### Kubernetes Client
- **Config resolution order**: explicit flag → `~/.kube/config` → in-cluster
- **API connectivity test**: Calls `ServerVersion()` before setting up informers to fail fast
- **Progress logging during sync**: Updates every 5 seconds showing which informers have synced
- **Sync timeout**: 2-minute timeout prevents hanging forever on connection issues

### Pod Accounting
- **Init container logic**: `calculatePodRequest()` uses `max(sum_of_regular_containers, max_init_container)` — matches Kubernetes scheduler
- **Pod filtering**: Excludes `NodeName == ""` (unscheduled) and `Phase == Succeeded|Failed` (terminated) from binpacking calculations
- **Debug visibility**: Logs when init containers dominate resource reservation

### Metrics Design
- **Scrape-time computation**: Metrics created fresh on each scrape using `MustNewConstMetric` — automatically handles node add/remove
- **Custom registry**: Uses `prometheus.NewRegistry()` instead of `prometheus.DefaultRegistry` to avoid Go runtime metrics
- **Resource-agnostic labels**: `resource` label instead of separate metric per resource type (cpu, memory, etc.)
- **Cache age metric**: `binpacking_cache_age_seconds` enables alerting on stale cache

### Health Checks
- **Liveness vs Readiness**: `/healthz` checks process health, `/readyz` checks cache sync state
- **Readiness function**: `setupKubernetes()` returns closure that checks `HasSynced()` on both informers
- **Probe timing**: Readiness uses shorter delay/period (5s/10s), liveness uses longer (10s/30s)

### Informer Configuration
- **Configurable resync**: `--resync-period` flag (default 5m) controls how often informers refresh from API server
- **SharedInformerFactory**: Single watch connection shared between node and pod informers
- **SyncInfo tracking**: Records last sync time, exposes via `/sync` endpoint and `cache_age_seconds` metric
- **Pagination support**: `--list-page-size` flag (default 500) enables paginated initial LIST requests
  - Reduces memory spikes during initial sync by ~40% in large clusters
  - Uses client-go's `WithTweakListOptions` to set `Limit` on LIST requests
  - Transparent Continue token handling - no manual pagination logic needed
  - Set to 0 to disable pagination for small clusters
  - Progress logging shows elapsed time during paginated sync

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

Minimal dependency footprint — only official Kubernetes and Prometheus libs:

| Package | Purpose | Version Constraint |
|---------|---------|-------------------|
| `k8s.io/client-go` | Kubernetes API client, informers, listers | Latest stable |
| `k8s.io/api` | Kubernetes resource types (Pod, Node, etc.) | Match client-go |
| `k8s.io/apimachinery` | Kubernetes primitives (Quantity, etc.) | Match client-go |
| `github.com/prometheus/client_golang` | Prometheus collector interface, HTTP handler | Latest stable |

No controller-runtime, no operator SDK — just the essentials.

## Release Process

### Automated Releases via PR Labels

Releases are **fully automated** when merging PRs to main. The version is determined by PR labels:

| Label | Version Bump | Example | Use When |
|-------|-------------|---------|----------|
| `major` | Breaking change | 1.2.3 → 2.0.0 | API changes, removed features, major refactors |
| `minor` | New feature | 1.2.3 → 1.3.0 | New functionality, backward-compatible |
| `patch` | Bug fix | 1.2.3 → 1.2.4 | Bug fixes, documentation, default if no label |
| `skip-release` | No release | - | CI changes, tests, docs-only updates |

**Workflow:**
1. Create PR with changes
2. Add appropriate version label (`major`, `minor`, `patch`, or `skip-release`)
3. Merge PR to `main`
4. Auto-release workflow:
   - Detects PR merge and reads labels
   - Calculates new semantic version
   - Creates and pushes git tag (e.g., `v1.2.3`)
   - Comments on PR with release details
5. Release workflow (triggered by tag):
   - Builds multi-arch Docker images (linux/amd64, linux/arm64)
   - Builds cross-platform binaries (Linux/macOS/Windows for amd64/arm64/arm)
   - Generates changelog from commits
   - Creates GitHub Release with binaries and checksums
   - Pushes Docker images to ghcr.io

**Example PR Labels:**
- Adding new metric → `minor`
- Fixing cache sync bug → `patch`
- Changing metric names → `major`
- Updating CI workflow → `skip-release`

### Manual Release (if needed)

```bash
# Create tag manually
git tag v1.0.0
git push origin v1.0.0

# Release workflow automatically triggers
```

### Release Artifacts

Each release includes:
- **Docker images**: `ghcr.io/sherifabdlnaby/kube-cluster-binpacking-exporter:v1.2.3`
  - Multi-arch manifest: linux/amd64, linux/arm64
  - Also tagged as: `1.2.3`, `1.2`, `sha-<commit>`
- **Helm chart**: `oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter:1.2.3`
  - Published to OCI registry (versioned, matches release)
  - Includes Kubernetes icon, keywords, maintainer info
- **Binaries**: Cross-platform archives
  - Linux: amd64, arm64, arm/v7 (tar.gz)
  - macOS: amd64 (Intel), arm64 (Apple Silicon) (tar.gz)
  - Windows: amd64, arm64 (zip)
- **Checksums**: SHA256 checksums.txt for verification
- **Changelog**: Auto-generated, grouped by type (Features, Bug Fixes, etc.)

### Installing from OCI Registry

```bash
# Install latest version
helm install binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter

# Install specific version
helm install binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter \
  --version 1.2.3

# With custom values
helm install binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter \
  --set debug=true \
  --set resyncPeriod=1m \
  --set resources.requests.cpu=50m
```

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
