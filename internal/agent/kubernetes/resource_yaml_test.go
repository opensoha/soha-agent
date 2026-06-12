package kubernetes

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestApplyResourceYAMLUpdatesDynamicResource(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "app-config",
			"namespace":       "platform",
			"resourceVersion": "1",
		},
		"data": map[string]any{"key": "old"},
	}}
	client := &Client{dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "ConfigMapList",
	}, existing)}

	view, err := client.ApplyResourceYAML(context.Background(), "platform", "ConfigMap", "app-config", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: platform
data:
  key: new
`)
	if err != nil {
		t.Fatalf("ApplyResourceYAML() error = %v", err)
	}
	if view.Kind != "ConfigMap" || view.Name != "app-config" || view.Namespace != "platform" {
		t.Fatalf("view = %#v, want configmap identity", view)
	}
	if !strings.Contains(view.Content, "key: new") {
		t.Fatalf("view content = %q, want updated data", view.Content)
	}

	updated, err := client.dynamic.Resource(gvr).Namespace("platform").Get(context.Background(), "app-config", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated configmap: %v", err)
	}
	value, _, _ := unstructured.NestedString(updated.Object, "data", "key")
	if value != "new" {
		t.Fatalf("data.key = %q, want new", value)
	}
}

func TestApplyResourceYAMLRejectsMismatchedNamespace(t *testing.T) {
	client := &Client{dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}

	_, err := client.ApplyResourceYAML(context.Background(), "platform", "ConfigMap", "app-config", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: other
`)
	if err == nil || !strings.Contains(err.Error(), "metadata.namespace") {
		t.Fatalf("ApplyResourceYAML() error = %v, want namespace mismatch", err)
	}
}

func TestDeleteResourceDeletesDynamicResource(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "app-config",
			"namespace": "platform",
		},
	}}
	client := &Client{dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "ConfigMapList",
	}, existing)}

	if err := client.DeleteResource(context.Background(), "platform", "ConfigMap", "app-config"); err != nil {
		t.Fatalf("DeleteResource() error = %v", err)
	}
	_, err := client.dynamic.Resource(gvr).Namespace("platform").Get(context.Background(), "app-config", metav1.GetOptions{})
	if err == nil {
		t.Fatal("deleted resource was still found")
	}
}
