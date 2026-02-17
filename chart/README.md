# Kube Cluster Binpacking Exporter Helm Chart

Prometheus exporter for Kubernetes cluster binpacking efficiency metrics. Tracks resource allocation (CPU, memory, or any custom resource) by comparing pod requests against node allocatable capacity.

## Features

- 🎯 **Resource-agnostic**: Track CPU, memory, GPU, or any custom resource
- 📊 **Multi-dimensional metrics**: Per-node, cluster-wide, and label-based grouping
- ⚡ **Zero API overhead**: Informer-based caching with zero API calls per scrape
- 📉 **Cardinality control**: Optional per-node metrics disable for large clusters
- 🏷️ **Label grouping**: Per-zone, per-instance-type, or custom label groupings
- 🔧 **Init container aware**: Correctly accounts for init container resource requests

## TL;DR

```bash
# Install from OCI registry
helm install binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter

# Install from local chart
helm install binpacking-exporter ./chart
```

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installing the Chart

### From OCI Registry (Recommended)

```bash
helm install binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter \
  --version 0.2.0
```

### From Source

```bash
# Clone the repository
git clone https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter.git
cd kube-cluster-binpacking-exporter

# Install the chart
helm install binpacking-exporter ./chart
```

### With Custom Values

```bash
helm install binpacking-exporter ./chart \
  --set labelGroups[0]="topology.kubernetes.io/zone" \
  --set labelGroups[1]="node.kubernetes.io/instance-type" \
  --set logLevel=debug
```

Or create a `values.yaml` file:

```yaml
# custom-values.yaml
labelGroups:
  - topology.kubernetes.io/zone
  - node.kubernetes.io/instance-type

disableNodeMetrics: false
logLevel: info
```

Then install:

```bash
helm install binpacking-exporter ./chart -f custom-values.yaml
```

## Uninstalling the Chart

```bash
helm uninstall binpacking-exporter
```

## Configuration

### Core Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `ghcr.io/sherifabdlnaby/kube-cluster-binpacking-exporter` |
| `image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Exporter Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources` | List of Kubernetes resources to track | `["cpu", "memory"]` |
| `metricsPort` | Port to serve metrics on | `9101` |
| `metricsPath` | HTTP path for metrics endpoint | `/metrics` |
| `resyncPeriod` | Informer cache resync period | `5m` |
| `listPageSize` | Resources per page during initial sync (0 = no pagination) | `500` |

### Logging Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `logLevel` | Log level (debug, info, warn, error) | `info` |
| `logFormat` | Log format (json, text) | `json` |

### Label-Based Grouping

| Parameter | Description | Default |
|-----------|-------------|---------|
| `labelGroups` | List of node label keys to group by | `[]` |
| `disableNodeMetrics` | Disable per-node metrics to reduce cardinality | `false` |

### ServiceMonitor (Prometheus Operator)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceMonitor.enabled` | Create ServiceMonitor resource | `false` |
| `serviceMonitor.interval` | Scrape interval | `30s` |
| `serviceMonitor.scrapeTimeout` | Scrape timeout | `10s` |
| `serviceMonitor.additionalLabels` | Additional labels for ServiceMonitor | `{}` |

### Resource Limits

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podResources.requests.cpu` | CPU request | `50m` |
| `podResources.requests.memory` | Memory request | `64Mi` |
| `podResources.limits.memory` | Memory limit | `128Mi` |

### Kubernetes Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity rules | `{}` |

## Examples

### Basic Installation

Default installation with CPU and memory tracking:

```bash
helm install binpacking-exporter ./chart
```

### Track Additional Resources (GPU)

```yaml
# values.yaml
resources:
  - cpu
  - memory
  - nvidia.com/gpu
```

```bash
helm install binpacking-exporter ./chart -f values.yaml
```

### Label-Based Grouping (Per-Zone Metrics)

Track binpacking efficiency per availability zone:

```yaml
# values.yaml
labelGroups:
  - topology.kubernetes.io/zone
```

```bash
helm install binpacking-exporter ./chart -f values.yaml
```

This generates metrics like:
```
binpacking_label_group_utilization_ratio{label_key="topology.kubernetes.io/zone",label_value="us-east-1a",resource="cpu"} 0.75
binpacking_label_group_node_count{label_key="topology.kubernetes.io/zone",label_value="us-east-1a"} 3
```

### Multi-Dimensional Grouping

Track by both zone and instance type:

```yaml
# values.yaml
labelGroups:
  - topology.kubernetes.io/zone
  - node.kubernetes.io/instance-type
```

### Large Cluster (Reduced Cardinality)

For clusters with 100+ nodes, disable per-node metrics:

```yaml
# values.yaml
disableNodeMetrics: true
labelGroups:
  - topology.kubernetes.io/zone
  - node.kubernetes.io/instance-type
```

**Impact**: Reduces metric cardinality by 90%+ while preserving cluster-wide and zone/instance-type insights.

### Enable Prometheus Operator Integration

```yaml
# values.yaml
serviceMonitor:
  enabled: true
  interval: 30s
  additionalLabels:
    prometheus: kube-prometheus
```

### Debug Mode

Enable verbose logging for troubleshooting:

```yaml
# values.yaml
logLevel: debug
logFormat: text  # Human-readable colored output
```

## Metrics Exported

All metrics are computed at scrape time from the informer cache (zero API calls per scrape).

### Per-Node Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `binpacking_node_allocated` | Gauge | `node`, `resource` | Total resource requests on node |
| `binpacking_node_allocatable` | Gauge | `node`, `resource` | Total allocatable resource on node |
| `binpacking_node_utilization_ratio` | Gauge | `node`, `resource` | Ratio of allocated to allocatable |

**Note**: Disabled when `disableNodeMetrics: true`

### Cluster-Wide Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `binpacking_cluster_allocated` | Gauge | `resource` | Cluster-wide total requests |
| `binpacking_cluster_allocatable` | Gauge | `resource` | Cluster-wide total capacity |
| `binpacking_cluster_utilization_ratio` | Gauge | `resource` | Cluster-wide ratio |
| `binpacking_cluster_node_count` | Gauge | - | Total number of nodes |

### Label Group Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `binpacking_label_group_allocated` | Gauge | `label_key`, `label_value`, `resource` | Total requests for label group |
| `binpacking_label_group_allocatable` | Gauge | `label_key`, `label_value`, `resource` | Total capacity for label group |
| `binpacking_label_group_utilization_ratio` | Gauge | `label_key`, `label_value`, `resource` | Ratio for label group |
| `binpacking_label_group_node_count` | Gauge | `label_key`, `label_value` | Node count for label group |

**Note**: Only emitted when `labelGroups` is configured

### Cache Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `binpacking_cache_age_seconds` | Gauge | Time since last informer sync |

## PromQL Examples

### High Utilization Nodes

```promql
binpacking_node_utilization_ratio{resource="cpu"} > 0.8
```

### Per-Zone Utilization

```promql
binpacking_label_group_utilization_ratio{
  label_key="topology.kubernetes.io/zone",
  resource="cpu"
}
```

### Wasted Capacity (Low Utilization)

```promql
binpacking_cluster_allocatable{resource="cpu"}
- binpacking_cluster_allocated{resource="cpu"}
```

### Nodes Per Zone

```promql
binpacking_label_group_node_count{label_key="topology.kubernetes.io/zone"}
```

## Troubleshooting

### Exporter Won't Start

1. Check RBAC permissions:
```bash
kubectl logs -l app.kubernetes.io/name=kube-cluster-binpacking-exporter
```

2. Verify service account has cluster-reader permissions:
```bash
kubectl describe clusterrole binpacking-exporter
```

### No Metrics Showing

1. Check readiness:
```bash
kubectl get pods -l app.kubernetes.io/name=kube-cluster-binpacking-exporter
```

2. Check `/readyz` endpoint:
```bash
kubectl port-forward svc/binpacking-exporter 9101:9101
curl http://localhost:9101/readyz
```

3. Verify pods have resource requests:
```bash
kubectl get pods --all-namespaces -o json | \
  jq '.items[] | select(.spec.containers[].resources.requests == null)'
```

### Cache Sync Issues

Check sync status:
```bash
curl http://localhost:9101/sync
```

Enable debug logging:
```yaml
logLevel: debug
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](../CONTRIBUTING.md) for details.

## License

Apache 2.0 - See [LICENSE](../LICENSE) for details.

## Links

- **GitHub**: https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter
- **Issues**: https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter/issues
- **Docker Images**: https://ghcr.io/sherifabdlnaby/kube-cluster-binpacking-exporter
- **Helm Charts**: https://ghcr.io/sherifabdlnaby/charts/kube-cluster-binpacking-exporter
