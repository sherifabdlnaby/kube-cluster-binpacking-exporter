# Kube Cluster Binpacking Exporter

[![CI](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/ci.yaml)
[![Release](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/release.yaml/badge.svg)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/actions/workflows/release.yaml)
[![Trivy](https://img.shields.io/badge/trivy-scanned-blue?logo=aquasecurity&logoColor=white)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/security/code-scanning?query=tool%3ATrivy)
[![CodeQL](https://img.shields.io/badge/codeql-analyzed-blue?logo=github&logoColor=white)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/security/code-scanning?query=tool%3ACodeQL)
[![GitHub Release](https://img.shields.io/github/v/release/sherifabdlnaby/kube-cluster-binpacking-exporter)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sherifabdlnaby/kube-cluster-binpacking-exporter)](go.mod)
[![Kubernetes](https://img.shields.io/badge/kubernetes-%3E%3D%201.29-blue?logo=kubernetes&logoColor=white)](https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter)
[![Platforms](https://img.shields.io/badge/platforms-linux%2Famd64%20%7C%20linux%2Farm64-blue)](#building)
[![License](https://img.shields.io/github/license/sherifabdlnaby/kube-cluster-binpacking-exporter)](LICENSE)

Prometheus exporter for Kubernetes cluster binpacking efficiency metrics. Tracks resource allocation (CPU, memory, or any custom resource) by comparing pod requests against node allocatable capacity.

## Features

- **Resource-agnostic design**: Track any Kubernetes resource (cpu, memory, nvidia.com/gpu, etc.) via `--resources` flag
- **Informer-based caching**: Zero API calls per Prometheus scrape — all data served from in-memory cache
- **Per-node and cluster-wide metrics**: Individual node utilization plus cluster aggregates
- **Label-based grouping**: Calculate binpacking metrics grouped by node labels (e.g., per-zone, per-instance-type) via `--label-groups` flag
- **Cardinality control**: Disable per-node metrics via `--disable-node-metrics` to reduce metric cardinality by 90%+ in large clusters
- **Init container support**: Correctly accounts for init container resource requests (uses max of init vs sum of regular containers)
- **Configurable resync period**: Control informer cache refresh frequency
- **Pure client-go**: No controller-runtime dependency, minimal footprint
- **Debug logging**: Detailed pod filtering, resource calculations, init container handling, and informer events

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
| `binpacking_label_group_utilization_ratio` | Gauge | `label_key`, `label_value`, `resource` | Ratio for nodes with this label value (0.0–1.0+) |
| `binpacking_label_group_node_count` | Gauge | `label_key`, `label_value` | Number of nodes with this label value |

**Notes**:
- Per-node metrics can be disabled via `--disable-node-metrics` to reduce cardinality in large clusters
- Label group metrics are only emitted when `--label-groups` is configured

### Cache Metrics

Informer cache synchronization metrics:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `binpacking_cache_age_seconds` | Gauge | - | Time since last informer cache sync (in seconds) |

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

binpacking_cache_age_seconds 42
```

## Usage

### Local Development

```bash
# Run against local cluster
go run . --kubeconfig ~/.kube/config

# Enable debug logging
go run . --kubeconfig ~/.kube/config --debug

# Track additional resources (e.g., GPU)
go run . --kubeconfig ~/.kube/config --resources=cpu,memory,nvidia.com/gpu

# Group by node labels (e.g., zone and instance type)
go run . --kubeconfig ~/.kube/config --label-groups=topology.kubernetes.io/zone,node.kubernetes.io/instance-type

# Disable per-node metrics to reduce cardinality (only cluster and label-group metrics)
go run . --kubeconfig ~/.kube/config --disable-node-metrics --label-groups=topology.kubernetes.io/zone

# Test metrics endpoint
curl localhost:9101/metrics
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

| Endpoint | Purpose |
|----------|---------|
| `/` | Homepage with links to all endpoints and configuration details |
| `/metrics` | Prometheus metrics (configured via `--metrics-path`) |
| `/healthz` | Liveness probe - returns 200 if process is alive |
| `/readyz` | Readiness probe - returns 200 if informer cache is synced, 503 otherwise |
| `/sync` | Cache sync status - returns JSON with last sync time, age, and sync state |

**Example `/sync` response:**
```json
{
  "last_sync": "2026-02-16T18:45:23Z",
  "sync_age_seconds": 127,
  "resync_period": "5m0s",
  "node_synced": true,
  "pod_synced": true
}
```

### Helm Installation

```bash
helm install binpacking-exporter charts/kube-cluster-binpacking-exporter

# With custom values
helm install binpacking-exporter charts/kube-cluster-binpacking-exporter \
  --set debug=true \
  --set resources='{cpu,memory,nvidia.com/gpu}' \
  --set serviceMonitor.enabled=true
```

#### Helm Values

| Value | Default | Description |
|-------|---------|-------------|
| `resources` | `[cpu, memory]` | List of resources to track |
| `debug` | `false` | Enable debug logging |
| `metricsPort` | `9101` | Metrics server port |
| `metricsPath` | `/metrics` | Metrics HTTP path |
| `serviceMonitor.enabled` | `false` | Create Prometheus ServiceMonitor |
| `serviceMonitor.interval` | `30s` | Scrape interval |

See [values.yaml](charts/kube-cluster-binpacking-exporter/values.yaml) for all options.

## Debug Logging

Enable debug logging with `--debug` to troubleshoot:

**Pod filtering:**
```json
{"level":"DEBUG","msg":"skipping unscheduled pod","pod":"default/pending-pod"}
{"level":"DEBUG","msg":"skipping terminated pod","pod":"default/completed-job","phase":"Succeeded"}
{"level":"DEBUG","msg":"filtered pods","unscheduled":3,"terminated":5}
```

**Resource calculations:**
```json
{"level":"DEBUG","msg":"pod container request","pod":"default/nginx","container":"nginx","resource":"cpu","request":0.5}
{"level":"DEBUG","msg":"node metrics","node":"worker-1","resource":"cpu","allocated":3.5,"allocatable":4,"utilization":0.875}
```

**Informer events:**
```json
{"level":"DEBUG","msg":"pod added","pod":"default/new-pod","node":"worker-1"}
{"level":"DEBUG","msg":"pod updated","pod":"default/nginx","node":"worker-1","phase":"Running"}
{"level":"DEBUG","msg":"node deleted","node":"worker-3"}
```

## Example PromQL Queries

```promql
# Nodes over 80% CPU utilization
binpacking_node_utilization_ratio{resource="cpu"} > 0.8

# Total cluster memory allocated
binpacking_cluster_allocated{resource="memory"}

# Most utilized node by CPU
topk(5, binpacking_node_utilization_ratio{resource="cpu"})

# Average cluster utilization
avg(binpacking_node_utilization_ratio{resource="cpu"})

# Alert if cache is stale (older than 10 minutes)
binpacking_cache_age_seconds > 600
```

## Init Container Handling

The exporter correctly accounts for init container resource requests following Kubernetes scheduler semantics:

**Rule**: For each resource, a pod reserves the **maximum** of:
1. Sum of all regular container requests
2. Highest single init container request (they run sequentially)

**Examples**:
- Pod with 500m CPU regular container + 1000m CPU init container → reserves **1000m** (init wins)
- Pod with two 500m CPU regular containers (sum=1000m) + 800m CPU init container → reserves **1000m** (sum wins)
- Pod with 300m CPU regular + 300m CPU init → reserves **300m** (equal)

With `--debug`, the exporter logs when init containers dominate:
```json
{"level":"DEBUG","msg":"pod resource request (init container dominates)",
 "pod":"default/myapp","resource":"cpu","effective":1000,"init_max":1000,
 "init_container":"setup","regular_sum":500}
```

## Architecture

- **Informer-based**: `SharedInformerFactory` watches Nodes and Pods via long-lived watch connections
- **In-memory cache**: Each Prometheus scrape reads from local cache (zero API calls per scrape)
- **`MustNewConstMetric`**: Metrics computed at scrape time, automatically handles node topology changes
- **Init container aware**: Uses same reservation logic as Kubernetes scheduler (max of init vs sum of regular)
- **Flat structure**: 3 Go files (`main.go`, `kubernetes.go`, `collector.go`) in `package main`

## Building

```bash
# Build binary
go build -o kube-cluster-binpacking-exporter .

# Build Docker image
docker build -t binpacking-exporter:dev .

# Run tests (TODO)
go test ./...
```

## Contributing

See [TODO.md](TODO.md) for planned features and improvements.

## License

MIT
