package kubernetes

import (
	"strings"
	"testing"

	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
)

func TestBuildServiceDetailIncludesEndpointsAndPods(t *testing.T) {
	ready := true
	detail := buildServiceDetail(corev1.Service{}, []discoveryv1.EndpointSlice{{Endpoints: []discoveryv1.Endpoint{{
		Addresses: []string{"10.1.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready},
	}}}}, []domainresource.PodView{{Name: "api-1"}})
	if len(detail.Endpoints) != 1 || detail.Endpoints[0].Ready == nil || !*detail.Endpoints[0].Ready || len(detail.BackendPods) != 1 {
		t.Fatalf("buildServiceDetail() = %#v", detail)
	}
}

func TestMapPodIncludesRequestsAndLimits(t *testing.T) {
	t.Parallel()

	view := mapPod(corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    apiresource.MustParse("250m"),
							corev1.ResourceMemory: apiresource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    apiresource.MustParse("500m"),
							corev1.ResourceMemory: apiresource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	})

	if view.Requests.CPU != "250m" || view.Requests.Memory != "128Mi" {
		t.Fatalf("Requests = %+v, want cpu=250m memory=128Mi", view.Requests)
	}
	if view.Limits.CPU != "500m" || view.Limits.Memory != "256Mi" {
		t.Fatalf("Limits = %+v, want cpu=500m memory=256Mi", view.Limits)
	}
}

func TestBuildRESTConfigUsesInClusterWhenKubeconfigEmpty(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	_, err := buildRESTConfig(cfgpkg.KubernetesConfig{})
	if err == nil {
		t.Fatal("buildRESTConfig() succeeded, want in-cluster config error outside Kubernetes")
	}
	if !strings.Contains(err.Error(), "KUBERNETES_SERVICE_HOST") {
		t.Fatalf("buildRESTConfig() error = %v, want in-cluster config error", err)
	}
}
