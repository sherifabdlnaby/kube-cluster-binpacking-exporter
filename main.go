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
		kubeconfig   string
		metricsAddr  string
		metricsPath  string
		resourceCSV  string
		logLevel     string
		logFormat    string
		resyncPeriod string
	)

	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (uses in-cluster config if empty)")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9101", "address to serve metrics on")
	flag.StringVar(&metricsPath, "metrics-path", "/metrics", "HTTP path for metrics endpoint")
	flag.StringVar(&resourceCSV, "resources", "cpu,memory", "comma-separated list of resources to track")
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flag.StringVar(&logFormat, "log-format", "json", "log format: json, text")
	flag.StringVar(&resyncPeriod, "resync-period", "5m", "informer cache resync period (e.g., 1m, 30s, 1h30m)")
	flag.Parse()

	level := parseLogLevel(logLevel)
	handler := createLogHandler(logFormat, level)
	logger := slog.New(handler)
	logger.Info("starting kube-cluster-binpacking-exporter", "version", version, "log_level", logLevel, "log_format", logFormat)

	resources := parseResources(resourceCSV)
	logger.Info("tracking resources", "resources", resourceCSV)

	resync, err := time.ParseDuration(resyncPeriod)
	if err != nil {
		logger.Error("invalid resync period", "error", err, "value", resyncPeriod)
		os.Exit(1)
	}
	logger.Info("informer resync period", "duration", resync)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	nodeLister, podLister, readyChecker, syncInfo, err := setupKubernetes(ctx, logger, kubeconfig, resync)
	if err != nil {
		logger.Error("failed to setup kubernetes client", "error", err)
		os.Exit(1)
	}

	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, syncInfo)
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
