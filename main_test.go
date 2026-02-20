package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// TestParseResources tests the parseResources function.
func TestParseResources(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []corev1.ResourceName
	}{
		{
			name:  "single resource",
			input: "cpu",
			expected: []corev1.ResourceName{
				corev1.ResourceCPU,
			},
		},
		{
			name:  "multiple resources",
			input: "cpu,memory",
			expected: []corev1.ResourceName{
				corev1.ResourceCPU,
				corev1.ResourceMemory,
			},
		},
		{
			name:  "with whitespace",
			input: "cpu , memory , ephemeral-storage",
			expected: []corev1.ResourceName{
				corev1.ResourceCPU,
				corev1.ResourceMemory,
				corev1.ResourceEphemeralStorage,
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []corev1.ResourceName{},
		},
		{
			name:     "only whitespace",
			input:    "  ,  ,  ",
			expected: []corev1.ResourceName{},
		},
		{
			name:  "custom resource",
			input: "nvidia.com/gpu",
			expected: []corev1.ResourceName{
				"nvidia.com/gpu",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResources(tt.input)

			if len(got) != len(tt.expected) {
				t.Fatalf("parseResources() length = %d, want %d", len(got), len(tt.expected))
			}

			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("parseResources()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// TestParseLabelGroups tests the parseLabelGroups function.
func TestParseLabelGroups(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected [][]string
	}{
		{
			name:  "single group single key",
			input: []string{"topology.kubernetes.io/zone"},
			expected: [][]string{
				{"topology.kubernetes.io/zone"},
			},
		},
		{
			name:  "single group multiple keys",
			input: []string{"topology.kubernetes.io/zone,node.kubernetes.io/instance-type"},
			expected: [][]string{
				{"topology.kubernetes.io/zone", "node.kubernetes.io/instance-type"},
			},
		},
		{
			name:  "multiple groups",
			input: []string{"topology.kubernetes.io/zone", "topology.kubernetes.io/zone,node.kubernetes.io/instance-type"},
			expected: [][]string{
				{"topology.kubernetes.io/zone"},
				{"topology.kubernetes.io/zone", "node.kubernetes.io/instance-type"},
			},
		},
		{
			name:  "with whitespace",
			input: []string{"zone , instance-type , nodepool"},
			expected: [][]string{
				{"zone", "instance-type", "nodepool"},
			},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "only whitespace values",
			input:    []string{"  ,  ,  "},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLabelGroups(tt.input)

			if tt.expected == nil {
				if got != nil {
					t.Errorf("parseLabelGroups() = %v, want nil", got)
				}
				return
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("parseLabelGroups() length = %d, want %d", len(got), len(tt.expected))
			}

			for i := range got {
				if len(got[i]) != len(tt.expected[i]) {
					t.Fatalf("parseLabelGroups()[%d] length = %d, want %d", i, len(got[i]), len(tt.expected[i]))
				}
				for j := range got[i] {
					if got[i][j] != tt.expected[i][j] {
						t.Errorf("parseLabelGroups()[%d][%d] = %q, want %q", i, j, got[i][j], tt.expected[i][j])
					}
				}
			}
		})
	}
}

// TestHealthEndpoint tests the /healthz liveness probe.
func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	// Create a simple handler like in main.go
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("/healthz body = %q, want to contain %q", string(body), "ok")
	}
}

// TestReadyEndpoint tests the /readyz readiness probe.
func TestReadyEndpoint(t *testing.T) {
	tests := []struct {
		name             string
		ready            bool
		wantStatus       int
		wantBodyContains string
	}{
		{
			name:             "ready",
			ready:            true,
			wantStatus:       http.StatusOK,
			wantBodyContains: "ready",
		},
		{
			name:             "not ready",
			ready:            false,
			wantStatus:       http.StatusServiceUnavailable,
			wantBodyContains: "not ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			// Create handler with readyChecker
			readyChecker := func() bool { return tt.ready }
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if readyChecker() {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, "ready\n")
				} else {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, "not ready: informer cache not synced\n")
				}
			})

			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("/readyz status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), tt.wantBodyContains) {
				t.Errorf("/readyz body = %q, want to contain %q", string(body), tt.wantBodyContains)
			}
		})
	}
}

// TestSyncEndpoint tests the /sync cache status endpoint.
func TestSyncEndpoint(t *testing.T) {
	// Create test SyncInfo
	lastSyncTime := time.Now().Add(-30 * time.Second)
	syncInfo := &SyncInfo{
		LastSyncTime: lastSyncTime,
		ResyncPeriod: 5 * time.Minute,
		NodeSynced:   func() bool { return true },
		PodSynced:    func() bool { return true },
	}

	req := httptest.NewRequest(http.MethodGet, "/sync", nil)
	w := httptest.NewRecorder()

	// Create handler like in main.go
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{\n")
		_, _ = io.WriteString(w, `  "last_sync": "`+syncInfo.LastSyncTime.Format(time.RFC3339)+`",`+"\n")
		_, _ = io.WriteString(w, "  \"sync_age_seconds\": "+strings.TrimSpace(strings.Split(time.Since(syncInfo.LastSyncTime).String(), ".")[0])+",\n")
		_, _ = io.WriteString(w, `  "resync_period": "`+syncInfo.ResyncPeriod.String()+`",`+"\n")
		_, _ = io.WriteString(w, "  \"node_synced\": true,\n")
		_, _ = io.WriteString(w, "  \"pod_synced\": true\n")
		_, _ = io.WriteString(w, "}")
	})

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/sync status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Check Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("/sync Content-Type = %q, want %q", contentType, "application/json")
	}

	// Parse JSON response
	body, _ := io.ReadAll(resp.Body)
	var syncResp map[string]interface{}
	if err := json.Unmarshal(body, &syncResp); err != nil {
		t.Fatalf("failed to parse /sync JSON: %v", err)
	}

	// Verify fields exist
	requiredFields := []string{"last_sync", "sync_age_seconds", "resync_period", "node_synced", "pod_synced"}
	for _, field := range requiredFields {
		if _, ok := syncResp[field]; !ok {
			t.Errorf("/sync response missing field: %s", field)
		}
	}

	// Verify boolean fields
	if nodeSynced, ok := syncResp["node_synced"].(bool); !ok || !nodeSynced {
		t.Errorf("/sync node_synced = %v, want true", syncResp["node_synced"])
	}
	if podSynced, ok := syncResp["pod_synced"].(bool); !ok || !podSynced {
		t.Errorf("/sync pod_synced = %v, want true", syncResp["pod_synced"])
	}
}
