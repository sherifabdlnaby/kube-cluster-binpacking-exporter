package main

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
)

var (
	nodeAllocated = prometheus.NewDesc(
		"kube_binpacking_node_allocated",
		"Total resource requested by pods on this node",
		[]string{"node", "resource"}, nil,
	)
	nodeAllocatable = prometheus.NewDesc(
		"kube_binpacking_node_allocatable",
		"Total allocatable resource on this node",
		[]string{"node", "resource"}, nil,
	)
	nodeUtilization = prometheus.NewDesc(
		"kube_binpacking_node_utilization_ratio",
		"Ratio of allocated to allocatable (0.0-1.0+)",
		[]string{"node", "resource"}, nil,
	)
	clusterAllocated = prometheus.NewDesc(
		"kube_binpacking_cluster_allocated",
		"Cluster-wide total resource requested",
		[]string{"resource"}, nil,
	)
	clusterAllocatable = prometheus.NewDesc(
		"kube_binpacking_cluster_allocatable",
		"Cluster-wide total allocatable resource",
		[]string{"resource"}, nil,
	)
	clusterUtilization = prometheus.NewDesc(
		"kube_binpacking_cluster_utilization_ratio",
		"Cluster-wide allocation ratio",
		[]string{"resource"}, nil,
	)
	groupAllocated = prometheus.NewDesc(
		"kube_binpacking_group_allocated",
		"Total resource requested by pods on nodes in this label group",
		[]string{"label_group", "label_group_value", "resource"}, nil,
	)
	groupAllocatable = prometheus.NewDesc(
		"kube_binpacking_group_allocatable",
		"Total allocatable resource on nodes in this label group",
		[]string{"label_group", "label_group_value", "resource"}, nil,
	)
	groupUtilization = prometheus.NewDesc(
		"kube_binpacking_group_utilization_ratio",
		"Ratio of allocated to allocatable for nodes in this label group (0.0-1.0+)",
		[]string{"label_group", "label_group_value", "resource"}, nil,
	)
	groupNodeCount = prometheus.NewDesc(
		"kube_binpacking_group_node_count",
		"Number of nodes in this label group",
		[]string{"label_group", "label_group_value"}, nil,
	)
	clusterNodeCount = prometheus.NewDesc(
		"kube_binpacking_cluster_node_count",
		"Total number of nodes in the cluster",
		nil, nil,
	)
	cacheAge = prometheus.NewDesc(
		"kube_binpacking_cache_age_seconds",
		"Time since last informer cache sync",
		nil, nil,
	)
	leaderStatus = prometheus.NewDesc(
		"kube_binpacking_leader_status",
		"Whether this instance is the leader (1) or standby (0). Only present when leader election is enabled",
		nil, nil,
	)
)

// BinpackingCollector implements prometheus.Collector using informer caches.
type BinpackingCollector struct {
	nodeLister        listerscorev1.NodeLister
	podLister         listerscorev1.PodLister
	logger            *slog.Logger
	resources         []corev1.ResourceName
	labelGroups       [][]string
	enableNodeMetrics bool
	syncInfo          *SyncInfo
	isLeader          *atomic.Bool // nil = leader election disabled (always emit); non-nil = check value
}

// calculatePodRequest computes the effective resource request for a pod.
// Kubernetes reserves the max of:
// 1. Sum of all regular container requests
// 2. Highest init container request (they run sequentially)
func calculatePodRequest(pod *corev1.Pod, resource corev1.ResourceName) (float64, podRequestDetails) {
	details := podRequestDetails{}

	// Sum regular container requests
	var regularSum float64
	for _, container := range pod.Spec.Containers {
		if req, ok := container.Resources.Requests[resource]; ok {
			val := req.AsApproximateFloat64()
			regularSum += val
			details.containerCount++
		}
	}
	details.regularSum = regularSum

	// Find max init container request
	var initMax float64
	var initMaxContainer string
	for _, container := range pod.Spec.InitContainers {
		if req, ok := container.Resources.Requests[resource]; ok {
			val := req.AsApproximateFloat64()
			if val > initMax {
				initMax = val
				initMaxContainer = container.Name
			}
			details.initContainerCount++
		}
	}
	details.initMax = initMax
	details.initMaxContainer = initMaxContainer

	// Return the maximum
	if initMax > regularSum {
		details.effective = initMax
		details.usedInit = true
		return initMax, details
	}
	details.effective = regularSum
	return regularSum, details
}

type podRequestDetails struct {
	regularSum         float64
	initMax            float64
	effective          float64
	containerCount     int
	initContainerCount int
	initMaxContainer   string
	usedInit           bool
}

func NewBinpackingCollector(
	nodeLister listerscorev1.NodeLister,
	podLister listerscorev1.PodLister,
	logger *slog.Logger,
	resources []corev1.ResourceName,
	labelGroups [][]string,
	enableNodeMetrics bool,
	syncInfo *SyncInfo,
	isLeader *atomic.Bool,
) *BinpackingCollector {
	return &BinpackingCollector{
		nodeLister:        nodeLister,
		podLister:         podLister,
		logger:            logger,
		resources:         resources,
		labelGroups:       labelGroups,
		enableNodeMetrics: enableNodeMetrics,
		syncInfo:          syncInfo,
		isLeader:          isLeader,
	}
}

func (c *BinpackingCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.enableNodeMetrics {
		ch <- nodeAllocated
		ch <- nodeAllocatable
		ch <- nodeUtilization
	}
	ch <- clusterAllocated
	ch <- clusterAllocatable
	ch <- clusterUtilization
	ch <- clusterNodeCount
	if len(c.labelGroups) > 0 {
		ch <- groupAllocated
		ch <- groupAllocatable
		ch <- groupUtilization
		ch <- groupNodeCount
	}
	ch <- cacheAge
	if c.isLeader != nil {
		ch <- leaderStatus
	}
}

func (c *BinpackingCollector) Collect(ch chan<- prometheus.Metric) {
	// Emit cache age metric
	if c.syncInfo != nil {
		ageSeconds := time.Since(c.syncInfo.LastSyncTime).Seconds()
		ch <- prometheus.MustNewConstMetric(cacheAge, prometheus.GaugeValue, ageSeconds)
	}

	// Leader election gate: when enabled, emit leader_status and return early if standby.
	if c.isLeader != nil {
		leader := c.isLeader.Load()
		ch <- prometheus.MustNewConstMetric(leaderStatus, prometheus.GaugeValue, boolToFloat64(leader))
		if !leader {
			return // standby: only cache_age + leader_status
		}
	}

	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		c.logger.Error("failed to list nodes", "error", err)
		return
	}

	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		c.logger.Error("failed to list pods", "error", err)
		return
	}

	c.logger.Debug("scraping metrics", "node_count", len(nodes), "pod_count", len(pods))

	// Build podsByNode map, filtering out unscheduled and terminated pods.
	podsByNode := make(map[string][]*corev1.Pod)
	var unscheduledCount, terminatedCount int
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			unscheduledCount++
			c.logger.Debug("skipping unscheduled pod", "pod", pod.Namespace+"/"+pod.Name)
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			terminatedCount++
			c.logger.Debug("skipping terminated pod", "pod", pod.Namespace+"/"+pod.Name, "phase", pod.Status.Phase)
			continue
		}
		podsByNode[pod.Spec.NodeName] = append(podsByNode[pod.Spec.NodeName], pod)
	}

	if unscheduledCount > 0 || terminatedCount > 0 {
		c.logger.Debug("filtered pods", "unscheduled", unscheduledCount, "terminated", terminatedCount)
	}

	// Track cluster-wide totals per resource.
	clusterAllocatedTotals := make(map[corev1.ResourceName]float64)
	clusterAllocatableTotals := make(map[corev1.ResourceName]float64)

	for _, node := range nodes {
		nodePods := podsByNode[node.Name]

		c.logger.Debug("processing node", "node", node.Name, "pod_count", len(nodePods))

		for _, res := range c.resources {
			resStr := string(res)

			// Sum pod requests for this resource on this node.
			// For each pod, take the max of:
			// 1. Sum of all regular container requests
			// 2. Max init container request (they run sequentially)
			var allocated float64
			for _, pod := range nodePods {
				podRequest, details := calculatePodRequest(pod, res)
				allocated += podRequest

				if c.logger.Enabled(context.TODO(), slog.LevelDebug) && podRequest > 0 {
					if details.usedInit {
						c.logger.Debug("pod resource request (init container dominates)",
							"pod", pod.Namespace+"/"+pod.Name,
							"resource", resStr,
							"effective", details.effective,
							"init_max", details.initMax,
							"init_container", details.initMaxContainer,
							"regular_sum", details.regularSum)
					} else {
						c.logger.Debug("pod resource request",
							"pod", pod.Namespace+"/"+pod.Name,
							"resource", resStr,
							"effective", details.effective,
							"containers", details.containerCount,
							"init_containers", details.initContainerCount)
					}
				}
			}

			// Get node allocatable for this resource.
			var allocatable float64
			if qty, ok := node.Status.Allocatable[res]; ok {
				allocatable = qty.AsApproximateFloat64()
			}

			// Compute ratio.
			var ratio float64
			if allocatable > 0 {
				ratio = allocated / allocatable
			}

			// Emit per-node metrics if enabled
			if c.enableNodeMetrics {
				c.logger.Debug("node metrics",
					"node", node.Name,
					"resource", resStr,
					"allocated", allocated,
					"allocatable", allocatable,
					"utilization", ratio)

				ch <- prometheus.MustNewConstMetric(nodeAllocated, prometheus.GaugeValue, allocated, node.Name, resStr)
				ch <- prometheus.MustNewConstMetric(nodeAllocatable, prometheus.GaugeValue, allocatable, node.Name, resStr)
				ch <- prometheus.MustNewConstMetric(nodeUtilization, prometheus.GaugeValue, ratio, node.Name, resStr)
			}

			clusterAllocatedTotals[res] += allocated
			clusterAllocatableTotals[res] += allocatable
		}
	}

	// Emit cluster-aggregate metrics.
	for _, res := range c.resources {
		resStr := string(res)
		allocated := clusterAllocatedTotals[res]
		allocatable := clusterAllocatableTotals[res]

		var ratio float64
		if allocatable > 0 {
			ratio = allocated / allocatable
		}

		c.logger.Debug("cluster metrics",
			"resource", resStr,
			"allocated", allocated,
			"allocatable", allocatable,
			"utilization", ratio)

		ch <- prometheus.MustNewConstMetric(clusterAllocated, prometheus.GaugeValue, allocated, resStr)
		ch <- prometheus.MustNewConstMetric(clusterAllocatable, prometheus.GaugeValue, allocatable, resStr)
		ch <- prometheus.MustNewConstMetric(clusterUtilization, prometheus.GaugeValue, ratio, resStr)
	}

	// Emit cluster node count
	ch <- prometheus.MustNewConstMetric(clusterNodeCount, prometheus.GaugeValue, float64(len(nodes)))

	// Emit label-group metrics if configured.
	if len(c.labelGroups) > 0 {
		c.collectLabelGroupMetrics(ch, nodes, podsByNode)
	}
}

// collectLabelGroupMetrics calculates and emits binpacking metrics grouped by node label combinations.
// Each group is a slice of label keys. Nodes are grouped by the composite value of all keys in the group.
func (c *BinpackingCollector) collectLabelGroupMetrics(ch chan<- prometheus.Metric, nodes []*corev1.Node, podsByNode map[string][]*corev1.Pod) {
	for _, group := range c.labelGroups {
		labelGroupKey := strings.Join(group, ",")

		// Group nodes by composite label value.
		nodesByCompositeValue := make(map[string][]*corev1.Node)
		for _, node := range nodes {
			values := make([]string, len(group))
			for i, key := range group {
				if v, ok := node.Labels[key]; ok {
					values[i] = v
				} else {
					values[i] = "<none>"
				}
			}
			compositeValue := strings.Join(values, ",")
			nodesByCompositeValue[compositeValue] = append(nodesByCompositeValue[compositeValue], node)
		}

		c.logger.Debug("grouping nodes by label combination",
			"label_group", labelGroupKey,
			"group_count", len(nodesByCompositeValue))

		// For each composite value, calculate aggregate binpacking metrics.
		for compositeValue, groupNodes := range nodesByCompositeValue {
			allocatedTotals := make(map[corev1.ResourceName]float64)
			allocatableTotals := make(map[corev1.ResourceName]float64)

			for _, node := range groupNodes {
				nodePods := podsByNode[node.Name]

				for _, res := range c.resources {
					var allocated float64
					for _, pod := range nodePods {
						podRequest, _ := calculatePodRequest(pod, res)
						allocated += podRequest
					}

					var allocatable float64
					if qty, ok := node.Status.Allocatable[res]; ok {
						allocatable = qty.AsApproximateFloat64()
					}

					allocatedTotals[res] += allocated
					allocatableTotals[res] += allocatable
				}
			}

			// Emit metrics for this combination group.
			for _, res := range c.resources {
				resStr := string(res)
				allocated := allocatedTotals[res]
				allocatable := allocatableTotals[res]

				var ratio float64
				if allocatable > 0 {
					ratio = allocated / allocatable
				}

				c.logger.Debug("group metrics",
					"label_group", labelGroupKey,
					"label_group_value", compositeValue,
					"resource", resStr,
					"allocated", allocated,
					"allocatable", allocatable,
					"utilization", ratio,
					"node_count", len(groupNodes))

				ch <- prometheus.MustNewConstMetric(groupAllocated, prometheus.GaugeValue, allocated, labelGroupKey, compositeValue, resStr)
				ch <- prometheus.MustNewConstMetric(groupAllocatable, prometheus.GaugeValue, allocatable, labelGroupKey, compositeValue, resStr)
				ch <- prometheus.MustNewConstMetric(groupUtilization, prometheus.GaugeValue, ratio, labelGroupKey, compositeValue, resStr)
			}

			ch <- prometheus.MustNewConstMetric(groupNodeCount, prometheus.GaugeValue, float64(len(groupNodes)), labelGroupKey, compositeValue)
		}
	}
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
