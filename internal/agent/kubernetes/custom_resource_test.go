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

	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

func TestCustomResourceCRUDUsesDynamicClient(t *testing.T) {
	definition := domainresource.CRDResourceDefinition{
		CRDName:    "widgets.example.com",
		Group:      "example.com",
		Version:    "v1",
		Resource:   "widgets",
		Kind:       "Widget",
		Namespaced: true,
	}
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name":            "sample",
			"namespace":       "platform",
			"resourceVersion": "1",
		},
		"spec": map[string]any{"size": "small"},
	}}
	client := &Client{dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "WidgetList",
	}, existing)}

	items, err := client.ListCustomResources(context.Background(), definition, "platform")
	if err != nil {
		t.Fatalf("ListCustomResources() error = %v", err)
	}
	if len(items) != 1 || items[0].Name != "sample" || items[0].Kind != "Widget" {
		t.Fatalf("items = %#v, want sample widget", items)
	}

	view, err := client.GetCustomResourceYAML(context.Background(), definition, "platform", "sample")
	if err != nil {
		t.Fatalf("GetCustomResourceYAML() error = %v", err)
	}
	if !strings.Contains(view.Content, "size: small") {
		t.Fatalf("yaml = %q, want existing spec", view.Content)
	}

	created, err := client.CreateCustomResourceYAML(context.Background(), definition, "platform", `
apiVersion: example.com/v1
kind: Widget
metadata:
  name: created
  namespace: platform
spec:
  size: medium
`)
	if err != nil {
		t.Fatalf("CreateCustomResourceYAML() error = %v", err)
	}
	if created.Name != "created" || created.Namespace != "platform" {
		t.Fatalf("created = %#v, want created/platform", created)
	}

	updated, err := client.ApplyCustomResourceYAML(context.Background(), definition, "platform", "sample", `
apiVersion: example.com/v1
kind: Widget
metadata:
  name: sample
  namespace: platform
spec:
  size: large
`)
	if err != nil {
		t.Fatalf("ApplyCustomResourceYAML() error = %v", err)
	}
	if !strings.Contains(updated.Content, "size: large") {
		t.Fatalf("updated yaml = %q, want large", updated.Content)
	}

	if err := client.DeleteCustomResource(context.Background(), definition, "platform", "sample"); err != nil {
		t.Fatalf("DeleteCustomResource() error = %v", err)
	}
	_, err = client.dynamic.Resource(gvr).Namespace("platform").Get(context.Background(), "sample", metav1.GetOptions{})
	if err == nil {
		t.Fatal("deleted custom resource was still found")
	}
}

func TestCustomResourceRejectsMismatchedKind(t *testing.T) {
	definition := domainresource.CRDResourceDefinition{
		Group:      "example.com",
		Version:    "v1",
		Resource:   "widgets",
		Kind:       "Widget",
		Namespaced: true,
	}
	client := &Client{dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}

	_, err := client.CreateCustomResourceYAML(context.Background(), definition, "platform", `
apiVersion: example.com/v1
kind: Gadget
metadata:
  name: sample
  namespace: platform
`)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("CreateCustomResourceYAML() error = %v, want kind mismatch", err)
	}
}
