package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
)

var (
	version = "dev"
)

func main() {
	var (
		kubeconfig      string
		metricsAddr     string
		metricsPath     string
		resourceCSV     string
		labelGroupsCSV  string
		logLevel        string
		logFormat       string
		resyncPeriod    string
		listPageSize    int
		disableNodeMetrics bool

		leaderElect              bool
		leaderElectLeaseName     string
		leaderElectNamespace     string
		leaderElectID            string
		leaderElectLeaseDuration string
		leaderElectRenewDeadline string
		leaderElectRetryPeriod   string
	)

	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (uses in-cluster config if empty)")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9101", "address to serve metrics on")
	flag.StringVar(&metricsPath, "metrics-path", "/metrics", "HTTP path for metrics endpoint")
	flag.StringVar(&resourceCSV, "resources", "cpu,memory", "comma-separated list of resources to track")
	flag.StringVar(&labelGroupsCSV, "label-groups", "", "comma-separated list of node label keys to group by (e.g., 'topology.kubernetes.io/zone,node.kubernetes.io/instance-type')")
	flag.BoolVar(&disableNodeMetrics, "disable-node-metrics", false, "disable per-node metrics to reduce cardinality (only emit cluster-wide and label-group metrics)")
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flag.StringVar(&logFormat, "log-format", "json", "log format: json, text")
	flag.StringVar(&resyncPeriod, "resync-period", "30m", "informer cache resync period (e.g., 1m, 30s, 1h30m)")
	flag.IntVar(&listPageSize, "list-page-size", 500, "number of resources to fetch per page during initial sync (0 = no pagination)")
	flag.BoolVar(&leaderElect, "leader-election", false, "enable leader election for HA (only the leader publishes binpacking metrics)")
	flag.StringVar(&leaderElectLeaseName, "leader-election-lease-name", "binpacking-exporter", "name of the Lease object used for leader election")
	flag.StringVar(&leaderElectNamespace, "leader-election-namespace", "", "namespace for the leader election Lease (auto-detected from service account if empty)")
	flag.StringVar(&leaderElectID, "leader-election-id", "", "unique identity for this participant in leader election (defaults to hostname)")
	flag.StringVar(&leaderElectLeaseDuration, "leader-election-lease-duration", "15s", "duration that non-leader candidates will wait before attempting to acquire leadership")
	flag.StringVar(&leaderElectRenewDeadline, "leader-election-renew-deadline", "10s", "duration that the leader will retry refreshing leadership before giving up")
	flag.StringVar(&leaderElectRetryPeriod, "leader-election-retry-period", "2s", "duration between leader election retries")
	flag.Parse()

	level := parseLogLevel(logLevel)
	handler := createLogHandler(logFormat, level)
	logger := slog.New(handler)
	logger.Info("starting kube-cluster-binpacking-exporter", "version", version, "log_level", logLevel, "log_format", logFormat)

	resources := parseResources(resourceCSV)
	logger.Info("tracking resources", "resources", resourceCSV)

	labelGroups := parseLabels(labelGroupsCSV)
	if len(labelGroups) > 0 {
		logger.Info("tracking label groups", "labels", labelGroupsCSV)
	}

	if disableNodeMetrics {
		logger.Info("per-node metrics disabled - only emitting cluster-wide and label-group metrics")
	}

	resync, err := time.ParseDuration(resyncPeriod)
	if err != nil {
		logger.Error("invalid resync period", "error", err, "value", resyncPeriod)
		os.Exit(1)
	}
	logger.Info("informer resync period", "duration", resync)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	nodeLister, podLister, readyChecker, syncInfo, clientset, err := setupKubernetes(ctx, logger, kubeconfig, resync, int64(listPageSize))
	if err != nil {
		logger.Error("failed to setup kubernetes client", "error", err)
		os.Exit(1)
	}

	// Leader election setup: when enabled, only the leader publishes binpacking metrics.
	var isLeader *atomic.Bool
	if leaderElect {
		isLeader = new(atomic.Bool) // starts as false (standby)

		ns, err := detectNamespace(leaderElectNamespace)
		if err != nil {
			logger.Error("leader election namespace detection failed", "error", err)
			os.Exit(1)
		}

		id, err := detectIdentity(leaderElectID)
		if err != nil {
			logger.Error("leader election identity detection failed", "error", err)
			os.Exit(1)
		}

		leaseDuration, err := time.ParseDuration(leaderElectLeaseDuration)
		if err != nil {
			logger.Error("invalid leader election lease duration", "error", err, "value", leaderElectLeaseDuration)
			os.Exit(1)
		}
		renewDeadline, err := time.ParseDuration(leaderElectRenewDeadline)
		if err != nil {
			logger.Error("invalid leader election renew deadline", "error", err, "value", leaderElectRenewDeadline)
			os.Exit(1)
		}
		retryPeriod, err := time.ParseDuration(leaderElectRetryPeriod)
		if err != nil {
			logger.Error("invalid leader election retry period", "error", err, "value", leaderElectRetryPeriod)
			os.Exit(1)
		}

		leConfig := LeaderElectionConfig{
			LeaseName:      leaderElectLeaseName,
			LeaseNamespace: ns,
			Identity:       id,
			LeaseDuration:  leaseDuration,
			RenewDeadline:  renewDeadline,
			RetryPeriod:    retryPeriod,
		}

		go runLeaderElection(ctx, clientset, leConfig, isLeader, logger)
	}

	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, labelGroups, !disableNodeMetrics, syncInfo, isLeader)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	mux := http.NewServeMux()

	// Homepage - links to all endpoints
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Kube Cluster Binpacking Exporter</title></head>
<body>
<h1>Kube Cluster Binpacking Exporter</h1>
<p>Version: %s</p>
<ul>
<li><a href="%s">%s</a> - Prometheus metrics</li>
<li><a href="/sync">/sync</a> - Cache sync status (JSON)</li>
<li><a href="/healthz">/healthz</a> - Liveness probe</li>
<li><a href="/readyz">/readyz</a> - Readiness probe</li>
</ul>
</body>
</html>`, version, metricsPath, metricsPath)
	})

	mux.Handle(metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	// Liveness probe - checks if process is alive
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	// Readiness probe - checks if informer cache is synced
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if readyChecker() {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintln(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintln(w, "not ready: informer cache not synced")
		}
	})

	// Sync status endpoint - shows cache sync information
	mux.HandleFunc("/sync", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{
  "last_sync": "%s",
  "sync_age_seconds": %.0f,
  "resync_period": "%s",
  "node_synced": %t,
  "pod_synced": %t
}`,
			syncInfo.LastSyncTime.Format(time.RFC3339),
			time.Since(syncInfo.LastSyncTime).Seconds(),
			syncInfo.ResyncPeriod,
			syncInfo.NodeSynced(),
			syncInfo.PodSynced())
	})

	srv := &http.Server{
		Addr:              metricsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("serving metrics", "addr", metricsAddr, "path", metricsPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "invalid log level %q, using info\n", level)
		return slog.LevelInfo
	}
}

func createLogHandler(format string, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(format) {
	case "text":
		return slog.NewTextHandler(os.Stdout, opts)
	case "json":
		return slog.NewJSONHandler(os.Stdout, opts)
	default:
		fmt.Fprintf(os.Stderr, "invalid log format %q, using json\n", format)
		return slog.NewJSONHandler(os.Stdout, opts)
	}
}

func parseResources(csv string) []corev1.ResourceName {
	parts := strings.Split(csv, ",")
	resources := make([]corev1.ResourceName, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			resources = append(resources, corev1.ResourceName(p))
		}
	}
	return resources
}

func parseLabels(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	labels := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			labels = append(labels, p)
		}
	}
	return labels
}
