package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// ReadyChecker returns true if the system is ready to serve traffic.
type ReadyChecker func() bool

// SyncInfo tracks informer synchronization state.
type SyncInfo struct {
	LastSyncTime time.Time
	ResyncPeriod time.Duration
	NodeSynced   func() bool
	PodSynced    func() bool
}

func setupKubernetes(ctx context.Context, logger *slog.Logger, kubeconfigPath string, resyncPeriod time.Duration, listPageSize int64) (listerscorev1.NodeLister, listerscorev1.PodLister, ReadyChecker, *SyncInfo, error) {
	config, configSource, err := buildConfig(kubeconfigPath)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("building kubeconfig: %w", err)
	}

	logger.Info("kubernetes client config",
		"source", configSource,
		"host", config.Host,
		"qps", config.QPS,
		"burst", config.Burst)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	// Test connectivity before setting up informers
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to connect to kubernetes API: %w", err)
	}
	logger.Info("connected to kubernetes API",
		"version", serverVersion.String(),
		"platform", serverVersion.Platform)

	// Create factory with or without pagination based on listPageSize
	var factory informers.SharedInformerFactory
	if listPageSize > 0 {
		logger.Info("configuring informers with pagination",
			"page_size", listPageSize)

		factory = informers.NewSharedInformerFactoryWithOptions(
			clientset,
			resyncPeriod,
			informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
				opts.Limit = listPageSize
			}),
		)
	} else {
		logger.Info("configuring informers without pagination")
		factory = informers.NewSharedInformerFactory(clientset, resyncPeriod)
	}

	nodeInformer := factory.Core().V1().Nodes()
	podInformer := factory.Core().V1().Pods()

	// Add event handlers for debug logging.
	if logger.Enabled(ctx, slog.LevelDebug) {
		_, err = nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				node := obj.(*corev1.Node)
				logger.Debug("node added", "node", node.Name)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				node := newObj.(*corev1.Node)
				logger.Debug("node updated", "node", node.Name)
			},
			DeleteFunc: func(obj interface{}) {
				node := obj.(*corev1.Node)
				logger.Debug("node deleted", "node", node.Name)
			},
		})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("adding node event handler: %w", err)
		}

		_, err = podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*corev1.Pod)
				logger.Debug("pod added", "pod", pod.Namespace+"/"+pod.Name, "node", pod.Spec.NodeName)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				pod := newObj.(*corev1.Pod)
				logger.Debug("pod updated", "pod", pod.Namespace+"/"+pod.Name, "node", pod.Spec.NodeName, "phase", pod.Status.Phase)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*corev1.Pod)
				logger.Debug("pod deleted", "pod", pod.Namespace+"/"+pod.Name)
			},
		})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("adding pod event handler: %w", err)
		}
	}

	nodeLister := nodeInformer.Lister()
	podLister := podInformer.Lister()

	factory.Start(ctx.Done())
	logger.Info("starting informers and waiting for cache sync (this may take 10-30 seconds)")

	// Wait with timeout and periodic progress updates
	syncCtx, syncCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer syncCancel()

	startTime := time.Now()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime)
				logger.Info("still waiting for cache sync...",
					"node_synced", nodeInformer.Informer().HasSynced(),
					"pod_synced", podInformer.Informer().HasSynced(),
					"elapsed_seconds", int(elapsed.Seconds()))
			case <-syncCtx.Done():
				return
			}
		}
	}()

	if !cache.WaitForCacheSync(syncCtx.Done(), nodeInformer.Informer().HasSynced, podInformer.Informer().HasSynced) {
		return nil, nil, nil, nil, fmt.Errorf("failed to sync informer caches within timeout")
	}

	logger.Info("informer cache synced successfully")

	// ReadyChecker returns true if both informers have synced.
	readyChecker := func() bool {
		return nodeInformer.Informer().HasSynced() && podInformer.Informer().HasSynced()
	}

	// Track sync information
	syncInfo := &SyncInfo{
		LastSyncTime: time.Now(),
		ResyncPeriod: resyncPeriod,
		NodeSynced:   nodeInformer.Informer().HasSynced,
		PodSynced:    podInformer.Informer().HasSynced,
	}

	return nodeLister, podLister, readyChecker, syncInfo, nil
}

func buildConfig(kubeconfigPath string) (*rest.Config, string, error) {
	if kubeconfigPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		return cfg, fmt.Sprintf("explicit flag: %s", kubeconfigPath), err
	}

	// Try default kubeconfig location.
	if home := homedir.HomeDir(); home != "" {
		defaultPath := filepath.Join(home, ".kube", "config")
		if cfg, err := clientcmd.BuildConfigFromFlags("", defaultPath); err == nil {
			return cfg, fmt.Sprintf("default location: %s", defaultPath), nil
		}
	}

	// Fall back to in-cluster config.
	cfg, err := rest.InClusterConfig()
	return cfg, "in-cluster", err
}
