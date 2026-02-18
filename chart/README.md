# Kube Binpacking Exporter Helm Chart

Prometheus exporter for Kubernetes binpacking efficiency metrics. Tracks resource allocation (CPU, memory, or any custom resource) by comparing pod requests against node allocatable capacity.

## Features

- **Resource-agnostic**: Track CPU, memory, GPU, or any custom resource
- **Multi-dimensional metrics**: Per-node, cluster-wide, and label-based grouping
- **Zero API overhead**: Informer-based caching with zero API calls per scrape
- **Cardinality control**: Optional per-node metrics disable for large clusters
- **Label grouping**: Per-zone, per-instance-type, or custom label groupings
- **Init container aware**: Correctly accounts for init container resource requests

## TL;DR

```bash
# Install from OCI registry
helm install kube-binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-binpacking-exporter

# Install from local chart
helm install kube-binpacking-exporter ./chart
```

## Prerequisites

- Kubernetes 1.29+
- Helm 3.0+

## Installing the Chart

### From OCI Registry (Recommended)

```bash
helm install kube-binpacking-exporter \
  oci://ghcr.io/sherifabdlnaby/charts/kube-binpacking-exporter \
  --version 0.0.0
```

### From Source

```bash
git clone https://github.com/sherifabdlnaby/kube-binpacking-exporter.git
cd kube-binpacking-exporter
helm install kube-binpacking-exporter ./chart
```

### With Custom Values

```bash
helm install kube-binpacking-exporter ./chart \
  --set labelGroups[0]="topology.kubernetes.io/zone" \
  --set labelGroups[1]="node.kubernetes.io/instance-type" \
  --set logLevel=debug
```

## Uninstalling the Chart

```bash
helm uninstall kube-binpacking-exporter
```

## Parameters

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for pod scheduling |
| disableNodeMetrics | bool | `false` | Disable per-node metrics to reduce cardinality. Recommended for clusters with >100 nodes |
| fullnameOverride | string | `""` | Override the full release name |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. Valid values: `Always`, `IfNotPresent`, `Never` |
| image.repository | string | `"ghcr.io/sherifabdlnaby/kube-binpacking-exporter"` | Container image repository |
| image.tag | string | `""` | Image tag. Defaults to the chart's `appVersion` when empty |
| imagePullSecrets | list | `[]` | Image pull secrets for private registries |
| labelGroups | list | `[]` | Node label keys to group metrics by. Enables per-zone, per-instance-type metrics. Example: `["topology.kubernetes.io/zone"]` |
| leaderElection.enabled | bool | `false` | Enable leader election for HA active-passive mode. Only the leader publishes binpacking metrics. Auto-enabled when `replicaCount > 1` |
| leaderElection.leaseDuration | string | `"15s"` | Duration that non-leader candidates will wait before attempting to acquire leadership |
| leaderElection.leaseName | string | `"kube-binpacking-exporter"` | Name of the Lease object used for leader election |
| leaderElection.renewDeadline | string | `"10s"` | Duration that the leader will retry refreshing leadership before giving up |
| leaderElection.retryPeriod | string | `"2s"` | Duration between leader election retries |
| listPageSize | int | `500` | Page size for initial list calls. Use `0` to disable pagination. Recommended `500` for clusters with >1000 pods |
| logFormat | string | `"json"` | Log format. Valid values: `json`, `text` |
| logLevel | string | `"info"` | Log level. Valid values: `debug`, `info`, `warn`, `error` |
| metricsPath | string | `"/metrics"` | HTTP path for the metrics endpoint |
| metricsPort | int | `9101` | Port on which the exporter serves metrics |
| nameOverride | string | `""` | Override the chart name |
| nodeSelector | object | `{}` | Node selector for pod scheduling |
| podAnnotations | object | `{}` | Additional pod annotations. See chart README for Datadog auto-discovery example |
| podDisruptionBudget.enabled | bool | `false` | Create a PodDisruptionBudget resource |
| podDisruptionBudget.maxUnavailable | string | `""` | Maximum number of pods that can be unavailable. Cannot be set together with `minAvailable` |
| podDisruptionBudget.minAvailable | string | `""` | Minimum number of pods that must remain available. Cannot be set together with `maxUnavailable` |
| podLabels | object | `{}` | Additional pod labels |
| podResources.limits.memory | string | `"150Mi"` | Memory limit for the exporter pod |
| podResources.requests.cpu | string | `"50m"` | CPU request for the exporter pod |
| podResources.requests.memory | string | `"100Mi"` | Memory request for the exporter pod |
| priorityClassName | string | `""` | Priority class name for pod scheduling. Use an existing PriorityClass name |
| replicaCount | int | `1` | Number of replicas for the exporter deployment |
| resources | list | `["cpu","memory"]` | Kubernetes resource types to track. Common values: `cpu`, `memory`, `nvidia.com/gpu` |
| resyncPeriod | string | `"30m"` | Informer cache resync period. Uses Go duration format (e.g. `1m`, `5m`, `1h30m`) |
| service.port | int | `9101` | Service port |
| service.type | string | `"ClusterIP"` | Kubernetes service type |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account (e.g. for IAM role bindings) |
| serviceAccount.create | bool | `true` | Create a service account for the exporter |
| serviceAccount.name | string | `""` | Override the service account name. Defaults to the release name when empty |
| serviceMonitor.additionalLabels | object | `{}` | Additional labels for the ServiceMonitor (use to match your Prometheus selector labels) |
| serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor resource |
| serviceMonitor.interval | string | `"30s"` | Scrape interval |
| serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout |
| tolerations | list | `[]` | Tolerations for pod scheduling |
| topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |

## Examples

### Basic Installation

Default installation with CPU and memory tracking:

```bash
helm install kube-binpacking-exporter ./chart
```

### Track Additional Resources (GPU)

```yaml
resources:
  - cpu
  - memory
  - nvidia.com/gpu
```

### Label-Based Grouping (Per-Zone Metrics)

Track binpacking efficiency per availability zone:

```yaml
labelGroups:
  - topology.kubernetes.io/zone
```

This generates metrics like:
```
kube_binpacking_label_group_utilization_ratio{label_key="topology.kubernetes.io/zone",label_value="us-east-1a",resource="cpu"} 0.75
kube_binpacking_label_group_node_count{label_key="topology.kubernetes.io/zone",label_value="us-east-1a"} 3
```

### Multi-Dimensional Grouping

Track by both zone and instance type:

```yaml
labelGroups:
  - topology.kubernetes.io/zone
  - node.kubernetes.io/instance-type
```

### Large Cluster (Reduced Cardinality)

For clusters with 100+ nodes, disable per-node metrics:

```yaml
disableNodeMetrics: true
labelGroups:
  - topology.kubernetes.io/zone
  - node.kubernetes.io/instance-type
```

**Impact**: Reduces metric cardinality by 90%+ while preserving cluster-wide and zone/instance-type insights.

### Enable Prometheus Operator Integration

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  additionalLabels:
    prometheus: kube-prometheus
```

### Datadog Integration (OpenMetrics Auto-Discovery)

If you use the Datadog Agent instead of Prometheus Operator, configure scraping via pod annotations.
The Datadog Agent auto-discovers the endpoint at runtime using the `%%host%%` template variable.

```yaml
podAnnotations:
  ad.datadoghq.com/kube-binpacking-exporter.checks: |
    {
      "openmetrics": {
        "instances": [
          {
            "openmetrics_endpoint": "http://%%host%%:9101/metrics",
            "namespace": "kube_binpacking",
            "metrics": ["kube_binpacking_.*"]
          }
        ]
      }
    }
```

**What this does:**
- Instructs the Datadog Agent to scrape `/metrics` on port `9101`
- Collects all metrics matching `kube_binpacking_.*`
- Prefixes metrics in Datadog with `kube_binpacking.` (from `namespace`)
- `%%host%%` resolves to the pod IP automatically — no service DNS needed

**Datadog metric names after collection:**

| Prometheus Metric | Datadog Metric |
|-------------------|----------------|
| `kube_binpacking_node_utilization_ratio` | `kube_binpacking.node.utilization_ratio` |
| `kube_binpacking_cluster_utilization_ratio` | `kube_binpacking.cluster.utilization_ratio` |
| `kube_binpacking_cluster_allocated` | `kube_binpacking.cluster.allocated` |
| `kube_binpacking_cache_age_seconds` | `kube_binpacking.cache.age_seconds` |

**Requirements:**
- Datadog Agent 7.27+ (OpenMetrics v2)
- Agent must be running as a DaemonSet with pod annotation auto-discovery enabled (default)

### Debug Mode

Enable verbose logging for troubleshooting:

```yaml
logLevel: debug
logFormat: text  # Human-readable output
```

## Metrics Exported

All metrics are computed at scrape time from the informer cache (zero API calls per scrape).

### Per-Node Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kube_binpacking_node_allocated` | Gauge | `node`, `resource` | Total resource requests on node |
| `kube_binpacking_node_allocatable` | Gauge | `node`, `resource` | Total allocatable resource on node |
| `kube_binpacking_node_utilization_ratio` | Gauge | `node`, `resource` | Ratio of allocated to allocatable |

**Note**: Disabled when `disableNodeMetrics: true`

### Cluster-Wide Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kube_binpacking_cluster_allocated` | Gauge | `resource` | Cluster-wide total requests |
| `kube_binpacking_cluster_allocatable` | Gauge | `resource` | Cluster-wide total capacity |
| `kube_binpacking_cluster_utilization_ratio` | Gauge | `resource` | Cluster-wide ratio |
| `kube_binpacking_cluster_node_count` | Gauge | - | Total number of nodes |

### Label Group Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kube_binpacking_label_group_allocated` | Gauge | `label_key`, `label_value`, `resource` | Total requests for label group |
| `kube_binpacking_label_group_allocatable` | Gauge | `label_key`, `label_value`, `resource` | Total capacity for label group |
| `kube_binpacking_label_group_utilization_ratio` | Gauge | `label_key`, `label_value`, `resource` | Ratio for label group |
| `kube_binpacking_label_group_node_count` | Gauge | `label_key`, `label_value` | Node count for label group |

**Note**: Only emitted when `labelGroups` is configured

### Cache Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `kube_binpacking_cache_age_seconds` | Gauge | Time since last informer sync |

## PromQL Examples

### High Utilization Nodes

```promql
kube_binpacking_node_utilization_ratio{resource="cpu"} > 0.8
```

### Per-Zone Utilization

```promql
kube_binpacking_label_group_utilization_ratio{
  label_key="topology.kubernetes.io/zone",
  resource="cpu"
}
```

### Wasted Capacity (Low Utilization)

```promql
kube_binpacking_cluster_allocatable{resource="cpu"}
- kube_binpacking_cluster_allocated{resource="cpu"}
```

### Nodes Per Zone

```promql
kube_binpacking_label_group_node_count{label_key="topology.kubernetes.io/zone"}
```

## Troubleshooting

### Exporter Won't Start

1. Check RBAC permissions:
```bash
kubectl logs -l app.kubernetes.io/name=kube-binpacking-exporter
```

2. Verify service account has cluster-reader permissions:
```bash
kubectl describe clusterrole kube-binpacking-exporter
```

### No Metrics Showing

1. Check readiness:
```bash
kubectl get pods -l app.kubernetes.io/name=kube-binpacking-exporter
```

2. Check `/readyz` endpoint:
```bash
kubectl port-forward svc/kube-binpacking-exporter 9101:9101
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

## License

MIT - See [LICENSE](../LICENSE) for details.

## Links

- **GitHub**: https://github.com/sherifabdlnaby/kube-binpacking-exporter
- **Issues**: https://github.com/sherifabdlnaby/kube-binpacking-exporter/issues
- **Docker Images**: https://ghcr.io/sherifabdlnaby/kube-binpacking-exporter
- **Helm Charts**: https://ghcr.io/sherifabdlnaby/charts/kube-binpacking-exporter
