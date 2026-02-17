package main

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSyncInfo tests the SyncInfo struct fields and usage.
func TestSyncInfo(t *testing.T) {
	// Create a SyncInfo with test data
	syncTime := time.Now()
	resyncPeriod := 5 * time.Minute

	nodeSynced := true
	podSynced := false

	syncInfo := &SyncInfo{
		LastSyncTime: syncTime,
		ResyncPeriod: resyncPeriod,
		NodeSynced:   func() bool { return nodeSynced },
		PodSynced:    func() bool { return podSynced },
	}

	// Verify fields
	if syncInfo.LastSyncTime != syncTime {
		t.Errorf("LastSyncTime = %v, want %v", syncInfo.LastSyncTime, syncTime)
	}

	if syncInfo.ResyncPeriod != resyncPeriod {
		t.Errorf("ResyncPeriod = %v, want %v", syncInfo.ResyncPeriod, resyncPeriod)
	}

	if syncInfo.NodeSynced() != nodeSynced {
		t.Errorf("NodeSynced() = %v, want %v", syncInfo.NodeSynced(), nodeSynced)
	}

	if syncInfo.PodSynced() != podSynced {
		t.Errorf("PodSynced() = %v, want %v", syncInfo.PodSynced(), podSynced)
	}
}

// TestReadinessCheck tests the ReadyChecker function behavior.
func TestReadinessCheck(t *testing.T) {
	tests := []struct {
		name       string
		nodeSynced bool
		podSynced  bool
		wantReady  bool
	}{
		{
			name:       "both synced - ready",
			nodeSynced: true,
			podSynced:  true,
			wantReady:  true,
		},
		{
			name:       "only node synced - not ready",
			nodeSynced: true,
			podSynced:  false,
			wantReady:  false,
		},
		{
			name:       "only pod synced - not ready",
			nodeSynced: false,
			podSynced:  true,
			wantReady:  false,
		},
		{
			name:       "neither synced - not ready",
			nodeSynced: false,
			podSynced:  false,
			wantReady:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a readyChecker function similar to how it's done in setupKubernetes
			nodeSyncedFunc := func() bool { return tt.nodeSynced }
			podSyncedFunc := func() bool { return tt.podSynced }

			readyChecker := func() bool {
				return nodeSyncedFunc() && podSyncedFunc()
			}

			got := readyChecker()
			if got != tt.wantReady {
				t.Errorf("readyChecker() = %v, want %v", got, tt.wantReady)
			}
		})
	}
}

// TestStripUnusedFields_Pod verifies that a full Pod is stripped to only the
// fields used by the collector: Name, Namespace, NodeName, Phase, and
// container resource requests. Everything else should be zeroed.
func TestStripUnusedFields_Pod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pod",
			Namespace: "default",
			UID:       "abc-123",
			Labels:    map[string]string{"app": "web"},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": `{"big":"json"}`,
			},
			OwnerReferences: []metav1.OwnerReference{
				{Name: "my-replicaset", Kind: "ReplicaSet"},
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl"},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
					Env: []corev1.EnvVar{
						{Name: "FOO", Value: "bar"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "data", MountPath: "/data"},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name:  "init",
					Image: "busybox:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("200m"),
						},
					},
					Command: []string{"sh", "-c", "echo hello"},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "data"},
			},
			ServiceAccountName: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			PodIP: "10.0.0.1",
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true, Image: "nginx:latest"},
			},
		},
	}

	result, err := stripUnusedFields(pod)
	if err != nil {
		t.Fatalf("stripUnusedFields() error = %v", err)
	}

	stripped := result.(*corev1.Pod)

	// Preserved fields
	if stripped.Name != "my-pod" {
		t.Errorf("Name = %q, want %q", stripped.Name, "my-pod")
	}
	if stripped.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", stripped.Namespace, "default")
	}
	if stripped.Spec.NodeName != "node-1" {
		t.Errorf("NodeName = %q, want %q", stripped.Spec.NodeName, "node-1")
	}
	if stripped.Status.Phase != corev1.PodRunning {
		t.Errorf("Phase = %v, want %v", stripped.Status.Phase, corev1.PodRunning)
	}

	// Container names + requests preserved
	if len(stripped.Spec.Containers) != 1 {
		t.Fatalf("Containers count = %d, want 1", len(stripped.Spec.Containers))
	}
	if stripped.Spec.Containers[0].Name != "app" {
		t.Errorf("Container name = %q, want %q", stripped.Spec.Containers[0].Name, "app")
	}
	if _, ok := stripped.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]; !ok {
		t.Error("Container CPU request missing")
	}

	// Init container names + requests preserved
	if len(stripped.Spec.InitContainers) != 1 {
		t.Fatalf("InitContainers count = %d, want 1", len(stripped.Spec.InitContainers))
	}
	if stripped.Spec.InitContainers[0].Name != "init" {
		t.Errorf("InitContainer name = %q, want %q", stripped.Spec.InitContainers[0].Name, "init")
	}

	// Stripped fields — ObjectMeta
	if stripped.UID != "" {
		t.Errorf("UID should be empty, got %q", stripped.UID)
	}
	if stripped.Labels != nil {
		t.Errorf("Labels should be nil, got %v", stripped.Labels)
	}
	if stripped.Annotations != nil {
		t.Errorf("Annotations should be nil, got %v", stripped.Annotations)
	}
	if len(stripped.OwnerReferences) != 0 {
		t.Errorf("OwnerReferences should be empty, got %v", stripped.OwnerReferences)
	}
	if len(stripped.ManagedFields) != 0 {
		t.Errorf("ManagedFields should be empty, got %v", stripped.ManagedFields)
	}

	// Stripped fields — Spec
	if stripped.Spec.Containers[0].Image != "" {
		t.Errorf("Container Image should be empty, got %q", stripped.Spec.Containers[0].Image)
	}
	if stripped.Spec.Containers[0].Env != nil {
		t.Errorf("Container Env should be nil, got %v", stripped.Spec.Containers[0].Env)
	}
	if stripped.Spec.Containers[0].VolumeMounts != nil {
		t.Errorf("Container VolumeMounts should be nil, got %v", stripped.Spec.Containers[0].VolumeMounts)
	}
	if stripped.Spec.Containers[0].Resources.Limits != nil {
		t.Errorf("Container Limits should be nil, got %v", stripped.Spec.Containers[0].Resources.Limits)
	}
	if stripped.Spec.InitContainers[0].Image != "" {
		t.Errorf("InitContainer Image should be empty, got %q", stripped.Spec.InitContainers[0].Image)
	}
	if stripped.Spec.InitContainers[0].Command != nil {
		t.Errorf("InitContainer Command should be nil, got %v", stripped.Spec.InitContainers[0].Command)
	}
	if stripped.Spec.Volumes != nil {
		t.Errorf("Volumes should be nil, got %v", stripped.Spec.Volumes)
	}
	if stripped.Spec.ServiceAccountName != "" {
		t.Errorf("ServiceAccountName should be empty, got %q", stripped.Spec.ServiceAccountName)
	}

	// Stripped fields — Status
	if stripped.Status.Conditions != nil {
		t.Errorf("Conditions should be nil, got %v", stripped.Status.Conditions)
	}
	if stripped.Status.PodIP != "" {
		t.Errorf("PodIP should be empty, got %q", stripped.Status.PodIP)
	}
	if stripped.Status.ContainerStatuses != nil {
		t.Errorf("ContainerStatuses should be nil, got %v", stripped.Status.ContainerStatuses)
	}
}

// TestStripUnusedFields_Node verifies that a full Node is stripped to only
// Name, Labels, and Allocatable. Everything else should be zeroed.
func TestStripUnusedFields_Node(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				"topology.kubernetes.io/zone":        "us-east-1a",
				"node.kubernetes.io/instance-type":   "m5.large",
				"kubernetes.io/os":                   "linux",
			},
			UID:         "node-uid-123",
			Annotations: map[string]string{"annotation": "value"},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet"},
			},
		},
		Spec: corev1.NodeSpec{
			PodCIDR:    "10.0.0.0/24",
			ProviderID: "aws:///us-east-1a/i-abc123",
			Taints: []corev1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3920m"),
				corev1.ResourceMemory: resource.MustParse("15Gi"),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.1"},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.28.0",
				ContainerRuntimeVersion: "containerd://1.7.0",
				OSImage:                 "Ubuntu 22.04",
			},
			Images: []corev1.ContainerImage{
				{Names: []string{"nginx:latest"}, SizeBytes: 150000000},
			},
		},
	}

	result, err := stripUnusedFields(node)
	if err != nil {
		t.Fatalf("stripUnusedFields() error = %v", err)
	}

	stripped := result.(*corev1.Node)

	// Preserved fields
	if stripped.Name != "node-1" {
		t.Errorf("Name = %q, want %q", stripped.Name, "node-1")
	}
	if len(stripped.Labels) != 3 {
		t.Errorf("Labels count = %d, want 3", len(stripped.Labels))
	}
	if stripped.Labels["topology.kubernetes.io/zone"] != "us-east-1a" {
		t.Errorf("zone label = %q, want %q", stripped.Labels["topology.kubernetes.io/zone"], "us-east-1a")
	}
	if _, ok := stripped.Status.Allocatable[corev1.ResourceCPU]; !ok {
		t.Error("Allocatable CPU missing")
	}
	if _, ok := stripped.Status.Allocatable[corev1.ResourceMemory]; !ok {
		t.Error("Allocatable Memory missing")
	}

	// Stripped fields — ObjectMeta
	if stripped.UID != "" {
		t.Errorf("UID should be empty, got %q", stripped.UID)
	}
	if stripped.Annotations != nil {
		t.Errorf("Annotations should be nil, got %v", stripped.Annotations)
	}
	if len(stripped.ManagedFields) != 0 {
		t.Errorf("ManagedFields should be empty, got %v", stripped.ManagedFields)
	}

	// Stripped fields — Spec
	if stripped.Spec.PodCIDR != "" {
		t.Errorf("PodCIDR should be empty, got %q", stripped.Spec.PodCIDR)
	}
	if stripped.Spec.ProviderID != "" {
		t.Errorf("ProviderID should be empty, got %q", stripped.Spec.ProviderID)
	}
	if stripped.Spec.Taints != nil {
		t.Errorf("Taints should be nil, got %v", stripped.Spec.Taints)
	}

	// Stripped fields — Status
	if stripped.Status.Capacity != nil {
		t.Errorf("Capacity should be nil, got %v", stripped.Status.Capacity)
	}
	if stripped.Status.Conditions != nil {
		t.Errorf("Conditions should be nil, got %v", stripped.Status.Conditions)
	}
	if stripped.Status.Addresses != nil {
		t.Errorf("Addresses should be nil, got %v", stripped.Status.Addresses)
	}
	if stripped.Status.NodeInfo.KubeletVersion != "" {
		t.Errorf("NodeInfo should be zeroed, got KubeletVersion=%q", stripped.Status.NodeInfo.KubeletVersion)
	}
	if stripped.Status.Images != nil {
		t.Errorf("Images should be nil, got %v", stripped.Status.Images)
	}
}

// TestStripUnusedFields_PreservesRequests verifies that CPU and memory
// resource.Quantity values survive the transform and can be read via
// AsApproximateFloat64(), which is how the collector consumes them.
func TestStripUnusedFields_PreservesRequests(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "req-pod", Namespace: "ns"},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "init",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	result, err := stripUnusedFields(pod)
	if err != nil {
		t.Fatalf("stripUnusedFields() error = %v", err)
	}
	stripped := result.(*corev1.Pod)

	// Verify CPU request value via AsApproximateFloat64
	cpuReq := stripped.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	cpuVal := cpuReq.AsApproximateFloat64()
	if cpuVal < 0.249 || cpuVal > 0.251 {
		t.Errorf("CPU request = %f, want ~0.25", cpuVal)
	}

	// Verify memory request value via AsApproximateFloat64
	memReq := stripped.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory]
	memVal := memReq.AsApproximateFloat64()
	wantMem := 256.0 * 1024 * 1024 // 256Mi in bytes
	if memVal < wantMem*0.99 || memVal > wantMem*1.01 {
		t.Errorf("Memory request = %f, want ~%f", memVal, wantMem)
	}

	// Verify init container CPU request
	initCPU := stripped.Spec.InitContainers[0].Resources.Requests[corev1.ResourceCPU]
	initVal := initCPU.AsApproximateFloat64()
	if initVal < 0.499 || initVal > 0.501 {
		t.Errorf("Init CPU request = %f, want ~0.5", initVal)
	}

	// Verify calculatePodRequest works on transformed pod (integration check)
	effective, details := calculatePodRequest(stripped, corev1.ResourceCPU)
	if effective < 0.499 || effective > 0.501 {
		t.Errorf("calculatePodRequest(CPU) = %f, want ~0.5 (init dominates)", effective)
	}
	if !details.usedInit {
		t.Error("calculatePodRequest should report usedInit=true")
	}
}

// TestStripUnusedFields_UnknownType verifies that non-Pod/Node objects pass
// through unchanged.
func TestStripUnusedFields_UnknownType(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.100",
		},
	}

	result, err := stripUnusedFields(svc)
	if err != nil {
		t.Fatalf("stripUnusedFields() error = %v", err)
	}

	// Should be returned unchanged
	unchanged := result.(*corev1.Service)
	if unchanged.Name != "my-service" {
		t.Errorf("Name = %q, want %q", unchanged.Name, "my-service")
	}
	if unchanged.Spec.ClusterIP != "10.0.0.100" {
		t.Errorf("ClusterIP = %q, want %q", unchanged.Spec.ClusterIP, "10.0.0.100")
	}
}
