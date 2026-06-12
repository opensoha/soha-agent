package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

func (c *Client) ListCustomResources(ctx context.Context, definition domainresource.CRDResourceDefinition, namespace string) ([]domainresource.CustomResourceView, error) {
	resource, _, err := c.customResource(definition, namespace, nil)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := resource.List(queryCtx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	views := make([]domainresource.CustomResourceView, 0, len(items.Items))
	for _, item := range items.Items {
		views = append(views, mapCustomResource(item, definition))
	}
	return views, nil
}

func (c *Client) GetCustomResourceYAML(ctx context.Context, definition domainresource.CRDResourceDefinition, namespace, name string) (domainresource.ResourceYAMLView, error) {
	resource, _, err := c.customResource(definition, namespace, nil)
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := resource.Get(queryCtx, name, metav1.GetOptions{})
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	unstructured.RemoveNestedField(item.Object, "metadata", "managedFields")
	content, err := yaml.Marshal(item.Object)
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	return domainresource.ResourceYAMLView{
		Kind:      definition.Kind,
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		Content:   string(content),
	}, nil
}

func (c *Client) CreateCustomResourceYAML(ctx context.Context, definition domainresource.CRDResourceDefinition, namespace, content string) (domainresource.ResourceYAMLView, error) {
	item, effectiveNamespace, err := buildCustomResourceFromYAML(definition, content, namespace, "")
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	resource, _, err := c.customResource(definition, effectiveNamespace, item)
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item.SetResourceVersion("")
	created, err := resource.Create(queryCtx, item, metav1.CreateOptions{})
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	return renderCustomResourceYAML(definition.Kind, created)
}

func (c *Client) ApplyCustomResourceYAML(ctx context.Context, definition domainresource.CRDResourceDefinition, namespace, name, content string) (domainresource.ResourceYAMLView, error) {
	item, effectiveNamespace, err := buildCustomResourceFromYAML(definition, content, namespace, name)
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	resource, _, err := c.customResource(definition, effectiveNamespace, item)
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if item.GetResourceVersion() == "" {
		current, err := resource.Get(queryCtx, name, metav1.GetOptions{})
		if err != nil {
			return domainresource.ResourceYAMLView{}, err
		}
		item.SetResourceVersion(current.GetResourceVersion())
	}
	updated, err := resource.Update(queryCtx, item, metav1.UpdateOptions{})
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	return renderCustomResourceYAML(definition.Kind, updated)
}

func (c *Client) DeleteCustomResource(ctx context.Context, definition domainresource.CRDResourceDefinition, namespace, name string) error {
	resource, _, err := c.customResource(definition, namespace, nil)
	if err != nil {
		return err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return resource.Delete(queryCtx, name, metav1.DeleteOptions{})
}

func (c *Client) customResource(definition domainresource.CRDResourceDefinition, namespace string, item *unstructured.Unstructured) (dynamic.ResourceInterface, string, error) {
	gvr, err := customResourceGVR(definition)
	if err != nil {
		return nil, "", err
	}
	if !definition.Namespaced {
		if item != nil && strings.TrimSpace(item.GetNamespace()) != "" {
			return nil, "", fmt.Errorf("yaml metadata.namespace must be empty for cluster-scoped custom resource")
		}
		if item != nil {
			item.SetNamespace("")
		}
		return c.dynamic.Resource(gvr), "", nil
	}
	effectiveNamespace := strings.TrimSpace(namespace)
	if item != nil {
		if strings.TrimSpace(item.GetNamespace()) == "" {
			item.SetNamespace(effectiveNamespace)
		}
		if effectiveNamespace == "" {
			effectiveNamespace = item.GetNamespace()
		}
		if item.GetNamespace() != effectiveNamespace {
			return nil, "", fmt.Errorf("yaml metadata.namespace does not match target custom resource")
		}
	}
	if effectiveNamespace == "" {
		return nil, "", fmt.Errorf("namespace is required for namespaced custom resource kind %s", definition.Kind)
	}
	return c.dynamic.Resource(gvr).Namespace(effectiveNamespace), effectiveNamespace, nil
}

func customResourceGVR(definition domainresource.CRDResourceDefinition) (schema.GroupVersionResource, error) {
	group := strings.TrimSpace(definition.Group)
	version := strings.TrimSpace(definition.Version)
	resource := strings.TrimSpace(definition.Resource)
	if group == "" || version == "" || resource == "" || strings.TrimSpace(definition.Kind) == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("custom resource definition group, version, resource, and kind are required")
	}
	return schema.GroupVersionResource{Group: group, Version: version, Resource: resource}, nil
}

func buildCustomResourceFromYAML(definition domainresource.CRDResourceDefinition, content, namespace, expectedName string) (*unstructured.Unstructured, string, error) {
	if strings.TrimSpace(content) == "" {
		return nil, "", fmt.Errorf("yaml content is required")
	}
	var object map[string]any
	if err := yaml.Unmarshal([]byte(content), &object); err != nil {
		return nil, "", fmt.Errorf("invalid yaml: %w", err)
	}
	item := &unstructured.Unstructured{Object: object}
	if item.GetKind() == "" {
		item.SetKind(definition.Kind)
	}
	if !strings.EqualFold(item.GetKind(), definition.Kind) {
		return nil, "", fmt.Errorf("yaml kind %s does not match target %s", item.GetKind(), definition.Kind)
	}
	if item.GetAPIVersion() == "" {
		item.SetAPIVersion(definition.Group + "/" + definition.Version)
	}
	if strings.TrimSpace(item.GetName()) == "" {
		if strings.TrimSpace(expectedName) == "" {
			return nil, "", fmt.Errorf("yaml metadata.name is required")
		}
		item.SetName(expectedName)
	}
	if expectedName = strings.TrimSpace(expectedName); expectedName != "" && item.GetName() != expectedName {
		return nil, "", fmt.Errorf("yaml metadata.name does not match target custom resource")
	}
	effectiveNamespace, err := requiredCustomResourceNamespace(definition, firstNonEmpty(item.GetNamespace(), namespace))
	if err != nil {
		return nil, "", err
	}
	if definition.Namespaced {
		item.SetNamespace(effectiveNamespace)
	} else {
		item.SetNamespace("")
	}
	return item, effectiveNamespace, nil
}

func requiredCustomResourceNamespace(definition domainresource.CRDResourceDefinition, namespace string) (string, error) {
	namespace = strings.TrimSpace(namespace)
	if definition.Namespaced {
		if namespace == "" {
			return "", fmt.Errorf("namespace is required for namespaced custom resource kind %s", definition.Kind)
		}
		return namespace, nil
	}
	if namespace != "" {
		return "", fmt.Errorf("namespace must be empty for cluster-scoped custom resource kind %s", definition.Kind)
	}
	return "", nil
}

func renderCustomResourceYAML(kind string, item *unstructured.Unstructured) (domainresource.ResourceYAMLView, error) {
	content, err := yaml.Marshal(item.Object)
	if err != nil {
		return domainresource.ResourceYAMLView{}, err
	}
	return domainresource.ResourceYAMLView{
		Kind:      kind,
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		Content:   string(content),
	}, nil
}

func mapCustomResource(item unstructured.Unstructured, definition domainresource.CRDResourceDefinition) domainresource.CustomResourceView {
	apiVersion := strings.TrimSpace(item.GetAPIVersion())
	if apiVersion == "" && definition.Group != "" && definition.Version != "" {
		apiVersion = definition.Group + "/" + definition.Version
	}
	return domainresource.CustomResourceView{
		APIVersion: apiVersion,
		Kind:       definition.Kind,
		Name:       item.GetName(),
		Namespace:  item.GetNamespace(),
		Labels:     item.GetLabels(),
		CreatedAt:  item.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
		AgeSeconds: secondsSince(item.GetCreationTimestamp().Time),
	}
}
