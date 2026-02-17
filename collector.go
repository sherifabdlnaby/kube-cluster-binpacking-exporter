package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
)

var (
	nodeAllocated = prometheus.NewDesc(
		"binpacking_node_allocated",
		"Total resource requested by pods on this node",
		[]string{"node", "resource"}, nil,
	)
	nodeAllocatable = prometheus.NewDesc(
		"binpacking_node_allocatable",
		"Total allocatable resource on this node",
		[]string{"node", "resource"}, nil,
	)
	nodeUtilization = prometheus.NewDesc(
		"binpacking_node_utilization_ratio",
		"Ratio of allocated to allocatable (0.0-1.0+)",
		[]string{"node", "resource"}, nil,
	)
	clusterAllocated = prometheus.NewDesc(
		"binpacking_cluster_allocated",
		"Cluster-wide total resource requested",
		[]string{"resource"}, nil,
	)
	clusterAllocatable = prometheus.NewDesc(
		"binpacking_cluster_allocatable",
		"Cluster-wide total allocatable resource",
		[]string{"resource"}, nil,
	)
	clusterUtilization = prometheus.NewDesc(
		"binpacking_cluster_utilization_ratio",
		"Cluster-wide allocation ratio",
		[]string{"resource"}, nil,
	)
	cacheAge = prometheus.NewDesc(
		"binpacking_cache_age_seconds",
		"Time since last informer cache sync",
		nil, nil,
	)
)

// BinpackingCollector implements prometheus.Collector using informer caches.
type BinpackingCollector struct {
	nodeLister listerscorev1.NodeLister
	podLister  listerscorev1.PodLister
	logger     *slog.Logger
	resources  []corev1.ResourceName
	syncInfo   *SyncInfo
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
	syncInfo *SyncInfo,
) *BinpackingCollector {
	return &BinpackingCollector{
		nodeLister: nodeLister,
		podLister:  podLister,
		logger:     logger,
		resources:  resources,
		syncInfo:   syncInfo,
	}
}

func (c *BinpackingCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nodeAllocated
	ch <- nodeAllocatable
	ch <- nodeUtilization
	ch <- clusterAllocated
	ch <- clusterAllocatable
	ch <- clusterUtilization
	ch <- cacheAge
}

func (c *BinpackingCollector) Collect(ch chan<- prometheus.Metric) {
	// Emit cache age metric
	if c.syncInfo != nil {
		ageSeconds := time.Since(c.syncInfo.LastSyncTime).Seconds()
		ch <- prometheus.MustNewConstMetric(cacheAge, prometheus.GaugeValue, ageSeconds)
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

			c.logger.Debug("node metrics",
				"node", node.Name,
				"resource", resStr,
				"allocated", allocated,
				"allocatable", allocatable,
				"utilization", ratio)

			ch <- prometheus.MustNewConstMetric(nodeAllocated, prometheus.GaugeValue, allocated, node.Name, resStr)
			ch <- prometheus.MustNewConstMetric(nodeAllocatable, prometheus.GaugeValue, allocatable, node.Name, resStr)
			ch <- prometheus.MustNewConstMetric(nodeUtilization, prometheus.GaugeValue, ratio, node.Name, resStr)

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
}
