# Testing Guide

Comprehensive unit tests cover core logic, HTTP endpoints, and edge cases. Tests use mock listers to avoid requiring a real Kubernetes cluster.

## Running Tests

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

## Test Files

| File | Coverage | Key Tests |
|------|----------|-----------|
| `collector_test.go` | Collector logic | Init container accounting, pod filtering, metric collection, error handling |
| `kubernetes_test.go` | Kubernetes setup | SyncInfo struct, readiness checker function |
| `main_test.go` | HTTP handlers | `/healthz`, `/readyz`, `/sync` endpoints, resource parsing |

## Test Infrastructure

### Mock Listers

Fake implementations avoid real Kubernetes dependencies:

```go
fakeNodeLister   // Returns test nodes, no API calls
fakePodLister    // Returns test pods, no API calls
```

### Helper Functions

Create test objects consistently:

```go
makeContainer(name, cpu, memory)            // Create container with resource requests
makeNode(name, cpu, memory)                 // Create node with allocatable resources
makePodWithResources(...)                   // Create pod with containers and init containers
```

### Float Comparison

`floatEquals()` handles floating-point precision issues:

```go
floatEquals(a, b float64) bool  // Uses epsilon for approximate equality
```

## What's Tested

### Init Container Logic (`TestCalculatePodRequest`)
- Regular containers only (sum of requests)
- Init container dominates (max init > sum regular)
- Regular containers dominate (sum regular > max init)
- Multiple init containers (takes max)
- Missing resource requests
- Memory vs CPU resource calculations

### Pod Filtering (`TestBinpackingCollector_PodFiltering`)
- Running pods (included)
- Pending pods with NodeName (included)
- Unscheduled pods - no NodeName (excluded)
- Succeeded/Failed pods (excluded)

### Metrics Collection (`TestBinpackingCollector_Collect`)
- Per-node metrics (allocated, allocatable, utilization)
- Cluster-wide aggregates
- Cache age metric
- Correct metric counts (nodes × resources)

### HTTP Endpoints
- `/healthz`: Always returns 200 OK
- `/readyz`: Returns 200 if cache synced, 503 otherwise
- `/sync`: Returns JSON with cache state

### Error Handling (`TestBinpackingCollector_ErrorHandling`)
- Node lister failures
- Pod lister failures
- Missing SyncInfo (no cache age metric)

### Edge Cases
- Zero allocatable resources
- Debug logging enabled (no crashes)
- Empty pods/nodes

## Testing Conventions

### Table-Driven Tests

Each test uses subtests with descriptive names:

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

### Test Names

Use descriptive names that explain the scenario:

```
TestCalculatePodRequest/init_container_dominates
TestBinpackingCollector_PodFiltering/unscheduled_pod
```

### Coverage Target

Aim for >80% coverage on core logic (collector, kubernetes setup):
- Use `go test -coverprofile=coverage.out` to measure
- Exclude error paths that can't realistically happen
- Focus on business logic over boilerplate

### Test Isolation

Each test is independent:
- Creates its own fake listers
- No shared state between tests
- Tests can run in parallel (future enhancement)

## Adding New Tests

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

4. **Update this guide** if adding new test patterns or infrastructure

## CI Integration

Tests run automatically on every push via `.github/workflows/ci.yaml`:
- `go test ./...` - Run all tests
- `go vet ./...` - Static analysis
- `golangci-lint run` - Linter checks

See `TODO.md` for planned testing enhancements:
- Makefile for standardized test commands
- Coverage reporting in CI
- Benchmark tests for performance optimization paths
