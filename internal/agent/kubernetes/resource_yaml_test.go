package kubernetes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
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
	client := &Client{dynamic: newSSAFakeDynamicClient(t, gvr, existing)}

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

func TestDryRunResourceYAMLUsesServerSideDryRun(t *testing.T) {
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
	client := &Client{dynamic: newSSAFakeDynamicClient(t, gvr, existing)}

	analysis, err := client.DryRunResourceYAML(context.Background(), "platform", "ConfigMap", "app-config", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: platform
data:
  key: new
`)
	if err != nil {
		t.Fatalf("DryRunResourceYAML() error = %v", err)
	}
	if analysis.FieldManager != "opensoha-resource-edit/v1" || len(analysis.ChangedFields) != 1 || analysis.ChangedFields[0] != "/data/key" {
		t.Fatalf("analysis = %#v", analysis)
	}

	actions := client.dynamic.(*dynamicfake.FakeDynamicClient).Actions()
	patch, ok := actions[len(actions)-1].(clienttesting.PatchActionImpl)
	if !ok || patch.GetPatchType() != types.ApplyPatchType || len(patch.PatchOptions.DryRun) != 1 || patch.PatchOptions.DryRun[0] != metav1.DryRunAll || patch.PatchOptions.FieldManager != "opensoha-resource-edit/v1" || patch.PatchOptions.Force == nil || *patch.PatchOptions.Force {
		t.Fatalf("last action = %#v, want server-side apply with DryRunAll and force=false", actions[len(actions)-1])
	}
}

func newSSAFakeDynamicClient(t *testing.T, gvr schema.GroupVersionResource, existing *unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	t.Helper()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "ConfigMapList"}, existing)
	client.PrependReactor("patch", gvr.Resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
		patch := action.(clienttesting.PatchActionImpl)
		var object map[string]any
		if err := json.Unmarshal(patch.GetPatch(), &object); err != nil {
			return true, nil, err
		}
		item := &unstructured.Unstructured{Object: object}
		item.SetResourceVersion("2")
		if len(patch.PatchOptions.DryRun) == 0 {
			if err := client.Tracker().Update(gvr, item, patch.GetNamespace()); err != nil {
				return true, nil, err
			}
		}
		return true, item, nil
	})
	return client
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

func TestResourceGVRForKindSupportsWorkloadControllers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind          string
		resource      string
		canonicalKind string
	}{
		{kind: "ReplicaSet", resource: "replicasets", canonicalKind: "ReplicaSet"},
		{kind: "StatefulSet", resource: "statefulsets", canonicalKind: "StatefulSet"},
		{kind: "DaemonSet", resource: "daemonsets", canonicalKind: "DaemonSet"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()

			gvr, namespaceScoped, canonicalKind, err := resourceGVRForKind(tc.kind)
			if err != nil {
				t.Fatalf("resourceGVRForKind(%q) error = %v", tc.kind, err)
			}
			if gvr.Resource != tc.resource {
				t.Fatalf("resourceGVRForKind(%q) resource = %q, want %q", tc.kind, gvr.Resource, tc.resource)
			}
			if !namespaceScoped {
				t.Fatalf("resourceGVRForKind(%q) namespaceScoped = false, want true", tc.kind)
			}
			if canonicalKind != tc.canonicalKind {
				t.Fatalf("resourceGVRForKind(%q) canonicalKind = %q, want %q", tc.kind, canonicalKind, tc.canonicalKind)
			}
		})
	}
}
