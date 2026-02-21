# Kube Binpacking Exporter (KBE)

[![CI](https://github.com/sherifabdlnaby/kube-binpacking-exporter/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/sherifabdlnaby/kube-binpacking-exporter/actions/workflows/ci.yaml)
[![Release](https://github.com/sherifabdlnaby/kube-binpacking-exporter/actions/workflows/release.yaml/badge.svg)](https://github.com/sherifabdlnaby/kube-binpacking-exporter/actions/workflows/release.yaml)
[![CodeQL](https://img.shields.io/badge/codeql-analyzed-blue?logo=github&logoColor=white)](https://github.com/sherifabdlnaby/kube-binpacking-exporter/security/code-scanning?query=tool%3ACodeQL)
[![GitHub Release](https://img.shields.io/github/v/release/sherifabdlnaby/kube-binpacking-exporter)](https://github.com/sherifabdlnaby/kube-binpacking-exporter/releases/latest)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kube-binpacking-exporter-chart)](https://artifacthub.io/packages/search?repo=kube-binpacking-exporter-chart)
[![Kubernetes](https://img.shields.io/badge/kubernetes-%3E%3D%201.29-blue?logo=kubernetes&logoColor=white)](https://github.com/sherifabdlnaby/kube-binpacking-exporter)
[![License](https://img.shields.io/github/license/sherifabdlnaby/kube-binpacking-exporter)](LICENSE)

A simple exporter of the ratio of **allocated** (Requests) to **allocatable** resources across your Kubernetes cluster, per node group (via label combinations), and cluster-wide.

You can use KBE to monitor how well your cluster bin-packs overtime, and how your bin-packing optimization reflects over extended period of time.  It is Like running [eks-node-viewer](https://github.com/awslabs/eks-node-viewer) in a loop to export historical metrics to your O11Y stack. 


## Why not use Kube O11Y tools?

While the combination of `kube-state-metrics`, `kubelet` and `cAdvisor` metrics can be used, they fall short because:

    1. These metrics are pulled from different sources at different intervals. This causes aggregation to not give an accurate *snapshot* of the cluster binpacking over time.
    2. Queries get extremely complex, and you have to handle edge cases ( e.g exclude failed & completed pods, handle init containers, not count pending pods, complex `joins` to group by node labels )
    3. Some O11Y tools  query language ( looking at you Datadog ) lacks the flexibility to join & combine metrics from different data sources.

**How is KBE better?**: KBE Mirrors the cluster state and returns an atomic snapshot of the cluster binpacking state on each scrape.


# Installation

## Helm (Recommended):
```bash
helm install kube-binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-binpacking-exporter \
  --version <check-releases>
```

Check Helm [values.yaml](./chart/values.yaml) for options, most importantly how your O11Y stack pulls the metrics from `/metrics` at `:9101`.

<details>
<summary><strong>To Run Locally</strong></summary>

Note: you must have `get|list|watch` permissions on `pods` and `nodes` to run KBE locally.

#### Docker

```bash
# Build
docker build -t kube-binpacking-exporter:dev .

# Run (mount your kubeconfig)
docker run --rm -p 9101:9101 \
  -v ~/.kube/config:/home/nonroot/.kube/config:ro \
  kube-binpacking-exporter:dev \
  --kubeconfig /home/nonroot/.kube/config
```

Once running, open `http://localhost:9101` for the homepage with links to all endpoints.

#### Go

```bash
# Basic (uses your current kubeconfig context)
go run . --kubeconfig ~/.kube/config

# Debug logging
go run . --kubeconfig ~/.kube/config --log-level=debug

# With label grouping 
go run . --kubeconfig ~/.kube/config \
  --label-group=topology.kubernetes.io/zone,node.kubernetes.io/instance-type \
  --label-group=topology.kubernetes.io/zone
```


</details>

## Features Highlights

- **Informer-based**: Zero API calls per metric scrape, so it's very light on the Kube API server. 
- **Per-node and cluster-wide metrics**: Individual node utilization plus cluster aggregates.
- **Combination label grouping**: Calculate binpacking metrics grouped by node label combinations (e.g., per-zone, per-zone+instance-type).
- **Cardinality control**: Disable per-node metrics via `--disable-node-metrics`.
- Track Daemonset Overhead.


### Planned

- Support more Node resources. (e.g `storage` and `gpu`)
- Prometheus & Datadog Dashboard.

### Out of scope

KBE's only concern is **Are Pods' _requests_ being satisfied in the most efficient way possible**. Tracking if pods are setting the correct requests, and if they are under-utilizing requests is out of the scope of this tool.

--- 

# Metrics

## Binpacking Metrics

Resource allocation metrics use a `resource` label to identify the resource type (cpu, memory, etc.):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kube_binpacking_node_allocated` | Gauge | `node`, `resource` | Total resource requested by pods on this node |
| `kube_binpacking_node_allocatable` | Gauge | `node`, `resource` | Total allocatable resource on this node |
| `kube_binpacking_node_utilization_ratio` | Gauge | `node`, `resource` | Ratio of allocated to allocatable (0.0–1.0+) |
| `kube_binpacking_cluster_allocated` | Gauge | `resource` | Cluster-wide total resource requested |
| `kube_binpacking_cluster_allocatable` | Gauge | `resource` | Cluster-wide total allocatable resource |
| `kube_binpacking_cluster_utilization_ratio` | Gauge | `resource` | Cluster-wide allocation ratio |
| `kube_binpacking_cluster_node_count` | Gauge | - | Total number of nodes in the cluster |
| `kube_binpacking_group_allocated` | Gauge | `label_group`, `label_group_value`, `resource` | Total resource requested on nodes in this label group |
| `kube_binpacking_group_allocatable` | Gauge | `label_group`, `label_group_value`, `resource` | Total allocatable resource on nodes in this label group |
| `kube_binpacking_group_utilization_ratio` | Gauge | `label_group`, `label_group_value`, `resource` | Ratio for nodes in this label group (0.0–1.0+) |
| `kube_binpacking_group_node_count` | Gauge | `label_group`, `label_group_value` | Number of nodes in this label group |

**Notes**:
- Per-node metrics can be disabled via `--disable-node-metrics` to reduce cardinality in large clusters
- Group metrics are only emitted when `--label-group` is configured

<details>
<summary><strong>Example Output</strong></summary>

```
kube_binpacking_node_allocated{node="worker-1",resource="cpu"} 3.5
kube_binpacking_node_allocatable{node="worker-1",resource="cpu"} 4
kube_binpacking_node_utilization_ratio{node="worker-1",resource="cpu"} 0.875
kube_binpacking_node_allocated{node="worker-1",resource="memory"} 4294967296
kube_binpacking_node_allocatable{node="worker-1",resource="memory"} 8589934592
kube_binpacking_node_utilization_ratio{node="worker-1",resource="memory"} 0.5

kube_binpacking_cluster_allocated{resource="cpu"} 12.5
kube_binpacking_cluster_allocatable{resource="cpu"} 16
kube_binpacking_cluster_utilization_ratio{resource="cpu"} 0.78125
kube_binpacking_cluster_node_count 4

kube_binpacking_group_allocated{label_group="topology.kubernetes.io/zone",label_group_value="us-east-1a",resource="cpu"} 6.5
kube_binpacking_group_allocatable{label_group="topology.kubernetes.io/zone",label_group_value="us-east-1a",resource="cpu"} 8
kube_binpacking_group_utilization_ratio{label_group="topology.kubernetes.io/zone",label_group_value="us-east-1a",resource="cpu"} 0.8125
kube_binpacking_group_node_count{label_group="topology.kubernetes.io/zone",label_group_value="us-east-1a"} 2
```

</details>

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | (auto) | Path to kubeconfig (uses in-cluster config if empty) |
| `--metrics-addr` | `:9101` | Address to serve metrics on |
| `--metrics-path` | `/metrics` | HTTP path for metrics endpoint |
| `--resources` | `cpu,memory` | Comma-separated list of resources to track |
| `--label-group` | (none) | Repeatable. Comma-separated label keys defining one combination group (e.g., `--label-group=zone,instance-type --label-group=zone`) |
| `--disable-node-metrics` | `false` | Disable per-node metrics to reduce cardinality (only emit cluster-wide and label-group metrics) |
| `--log-level` | `info` | Log level: debug, info, warn, error |
| `--log-format` | `json` | Log format: json, text |
| `--resync-period` | `30m` | Informer cache resync period (e.g., 1m, 30s, 1h30m) |
| `--list-page-size` | `500` | Number of resources to fetch per page during initial sync (0 = no pagination) |

### HTTP Endpoints

Defaults to port `:9101`

| Endpoint | Purpose |
|----------|---------|
| `/metrics` | Prometheus metrics (configured via `--metrics-path`) |
| `/sync` | Cache sync status - returns JSON with last sync time, age, and sync state |
| `/healthz` | Liveness probe - returns 200 if process is alive |
| `/readyz` | Readiness probe - returns 200 if informer cache is synced, 503 otherwise |

# Development

## Build

```bash
# Build binary
go build -o kube-binpacking-exporter .

# Build Docker image
docker build -t kube-binpacking-exporter:dev .
```

## Test

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

## Lint & Verify

```bash
go vet ./...
golangci-lint run
helm lint chart
```

# Contributing

See [TODO.md](TODO.md) for planned features and improvements.

# Disclaimer

This project was developed with the assistance of AI agents, specifically [Claude Code](https://docs.anthropic.com/en/docs/claude-code). All code has been reviewed and approved by the maintainer.

# License

MIT
