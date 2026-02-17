# Kube Cluster Binpacking Exporter (KCP)

[![CI](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/ci.yaml)
[![Release](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/release.yaml/badge.svg)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/release.yaml)
[![Trivy](https://img.shields.io/badge/trivy-scanned-blue?logo=aquasecurity&logoColor=white)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/security/code-scanning?query=tool%3ATrivy)
[![CodeQL](https://img.shields.io/badge/codeql-analyzed-blue?logo=github&logoColor=white)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/security/code-scanning?query=tool%3ACodeQL)
[![GitHub Release](https://img.shields.io/github/v/release/sherifabdlnaby/kube-cluster-binpacking-exporter)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sherifabdlnaby/kube-cluster-binpacking-exporter)](go.mod)
[![Kubernetes](https://img.shields.io/badge/kubernetes-%3E%3D%201.29-blue?logo=kubernetes&logoColor=white)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter)
[![Platforms](https://img.shields.io/badge/platforms-linux%2Famd64%20%7C%20linux%2Farm64-blue)](#development)
[![License](https://img.shields.io/github/license/sherifabdlnaby/kube-cluster-binpacking-exporter)](LICENSE)

Export straight-forward metrics to track Kubernetes cluster nodes binpacking efficiency, across individual nodes, by node groups (via Labels), or across the entire cluster, that are easier to aggregate over longer periods of time.

- **What kind of metrics?**: Calculate the % of **allocated** resources (via Requests) to **allocatable** resources. Used to track binpacking efficiency and scheduling fragmentation waste.

- **Why not use Kube O11Y tools?** While the combination of `kube-state-metrics`, `kubelet` and `cAdvisor` metrics can be used, they fall short because:

    1. These metrics are pulled from different sources at different intervals. This causes aggregation to not give an accurate *snapshot* of the cluster and not reflect accurate numbers, especially when tracking improvement over time.
        1. Inaccuracy is very high in highly-dynamic clusters with a lot of pod movement.
    2. Queries get extremely complex ( e.g exclude failed & completed pods, handle init containers, complex `joins` to group by node labels )
    3. Some O11Y tools ( looking at you DD ) query language lacks the flexibility to accurately combine and aggregate these metrics.

- **How is KCP better?**: Mirrors the cluster state and returns an atomic snapshot of the cluster binpacking state on each scrape. It's like running [eks-node-viewer](https://github.com/awslabs/eks-node-viewer) in a loop.

### Who typically uses KCP?

Anyone 🤷🏻‍♂️ But specifically Platform Engineers, and Cluster Administrators trying to optimize their Cluster Binpacking efficiency (e.g tinkering with [karpenter](https://karpenter.sh/docs/concepts/scheduling/) configurations) and want to track progress over time.

# Installation

**Helm (Recommended):**
```bash
helm install binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter \
  --version <check-releases>
```

Check Helm [values.yaml](./chart/values.yaml) for options, most importantly how your O11Y stack pulls the metrics from `/metrics` at `:9101`.

## Features Highlights

- **Informer-based**: Zero API calls per metric scrape — all data served from in-memory cache.
- **Per-node and cluster-wide metrics**: Individual node utilization plus cluster aggregates.
- **Label-based grouping**: Calculate binpacking metrics grouped by node labels (e.g., per-zone, per-instance-type) via `--label-groups` flag
- **Cardinality control**: Disable per-node metrics via `--disable-node-metrics` to reduce metric cardinality.
- **Configurable resync period**: Control informer cache refresh frequency.
- Multi-arch build.

### Planned

- Support more Node resources. (e.g `storage` and `gpu`)
- Calculate Daemonset Overhead.
- Advanced Label Grouping (Group by two labels values).

### Out of scope

KCP's only concern is **Are Pods' _requests_ being satisfied in the most efficient way possible**. Tracking if pods are setting the correct requests, and if they are under-utilizing requests is out of the scope of this tool.

## Metrics

### Binpacking Metrics

Resource allocation metrics use a `resource` label to identify the resource type (cpu, memory, etc.):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `binpacking_node_allocated` | Gauge | `node`, `resource` | Total resource requested by pods on this node |
| `binpacking_node_allocatable` | Gauge | `node`, `resource` | Total allocatable resource on this node |
| `binpacking_node_utilization_ratio` | Gauge | `node`, `resource` | Ratio of allocated to allocatable (0.0–1.0+) |
| `binpacking_cluster_allocated` | Gauge | `resource` | Cluster-wide total resource requested |
| `binpacking_cluster_allocatable` | Gauge | `resource` | Cluster-wide total allocatable resource |
| `binpacking_cluster_utilization_ratio` | Gauge | `resource` | Cluster-wide allocation ratio |
| `binpacking_cluster_node_count` | Gauge | - | Total number of nodes in the cluster |
| `binpacking_label_group_allocated` | Gauge | `label_key`, `label_value`, `resource` | Total resource requested on nodes with this label value |
| `binpacking_label_group_allocatable` | Gauge | `label_key`, `label_value`, `resource` | Total allocatable resource on nodes with this label value |
| `binpacking_label_group_utilization_ratio` | Gauge | `label_key`, `label_value`, `resource` | Ratio for nodes with this label value (0.0–1.0) |
| `binpacking_label_group_node_count` | Gauge | `label_key`, `label_value` | Number of nodes with this label value |

**Notes**:
- Per-node metrics can be disabled via `--disable-node-metrics` to reduce cardinality in large clusters
- Label group metrics are only emitted when `--label-groups` is configured

### Example Output

```
binpacking_node_allocated{node="worker-1",resource="cpu"} 3.5
binpacking_node_allocatable{node="worker-1",resource="cpu"} 4
binpacking_node_utilization_ratio{node="worker-1",resource="cpu"} 0.875
binpacking_node_allocated{node="worker-1",resource="memory"} 4294967296
binpacking_node_allocatable{node="worker-1",resource="memory"} 8589934592
binpacking_node_utilization_ratio{node="worker-1",resource="memory"} 0.5

binpacking_cluster_allocated{resource="cpu"} 12.5
binpacking_cluster_allocatable{resource="cpu"} 16
binpacking_cluster_utilization_ratio{resource="cpu"} 0.78125
binpacking_cluster_node_count 4

binpacking_label_group_allocated{label_key="topology.kubernetes.io/zone",label_value="us-east-1a",resource="cpu"} 6.5
binpacking_label_group_allocatable{label_key="topology.kubernetes.io/zone",label_value="us-east-1a",resource="cpu"} 8
binpacking_label_group_utilization_ratio{label_key="topology.kubernetes.io/zone",label_value="us-east-1a",resource="cpu"} 0.8125
binpacking_label_group_node_count{label_key="topology.kubernetes.io/zone",label_value="us-east-1a"} 2

```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | (auto) | Path to kubeconfig (uses in-cluster config if empty) |
| `--metrics-addr` | `:9101` | Address to serve metrics on |
| `--metrics-path` | `/metrics` | HTTP path for metrics endpoint |
| `--resources` | `cpu,memory` | Comma-separated list of resources to track |
| `--label-groups` | (none) | Comma-separated list of node label keys to group by (e.g., `topology.kubernetes.io/zone,node.kubernetes.io/instance-type`) |
| `--disable-node-metrics` | `false` | Disable per-node metrics to reduce cardinality (only emit cluster-wide and label-group metrics) |
| `--log-level` | `info` | Log level: debug, info, warn, error |
| `--log-format` | `json` | Log format: json, text |
| `--resync-period` | `5m` | Informer cache resync period (e.g., 1m, 30s, 1h30m) |
| `--list-page-size` | `500` | Number of resources to fetch per page during initial sync (0 = no pagination) |

### HTTP Endpoints

Defaults to port `:9101`

| Endpoint | Purpose |
|----------|---------|
| `/` | Homepage with links to all endpoints and configuration details |
| `/metrics` | Prometheus metrics (configured via `--metrics-path`) |
| `/healthz` | Liveness probe - returns 200 if process is alive |
| `/readyz` | Readiness probe - returns 200 if informer cache is synced, 503 otherwise |
| `/sync` | Cache sync status - returns JSON with last sync time, age, and sync state |

## Development

### Build

```bash
# Build binary
go build -o kube-cluster-binpacking-exporter .

# Build Docker image
docker build -t binpacking-exporter:dev .
```

### Test

```bash
# Run all tests
go test -v ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -func=coverage.out          # summary
go tool cover -html=coverage.out          # detailed HTML report

# Run a specific test
go test -v -run TestCalculatePodRequest

# Race detector
go test -race ./...
```

Tests use mock listers — no cluster required. See [TESTING.md](TESTING.md) for full details.

### Lint & Verify

```bash
go vet ./...
golangci-lint run
helm lint chart
```

### Run Locally

```bash
# Basic (uses your current kubeconfig context)
go run . --kubeconfig ~/.kube/config

# Debug logging
go run . --kubeconfig ~/.kube/config --log-level=debug

# With label grouping
go run . --kubeconfig ~/.kube/config \
  --label-groups=topology.kubernetes.io/zone,node.kubernetes.io/instance-type
```

Once running, open `http://localhost:9101` for the homepage with links to all endpoints.

## Contributing

See [TODO.md](TODO.md) for planned features and improvements.

## Disclaimer

This project was developed with the assistance of AI agents, specifically [Claude Code](https://docs.anthropic.com/en/docs/claude-code). All code has been reviewed and approved by the maintainer.

## License

MIT
