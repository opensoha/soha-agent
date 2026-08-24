package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func (c *Client) GetResourceGraph(ctx context.Context, namespace, kind, name string) (domainresource.ResourceGraph, error) {
	builder := newAgentResourceGraphBuilder(c, c.cfg.ID, namespace)
	rootID := builder.addNode(resourceAPIVersion(kind), kind, namespace, name, "")
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "deployment":
		detail, err := c.GetDeploymentDetail(ctx, namespace, name)
		if err != nil {
			return domainresource.ResourceGraph{}, err
		}
		builder.nodes[rootID] = graphNode(c.cfg.ID, "apps/v1", "Deployment", namespace, name, deploymentHealth(detail.ReadyReplicas, detail.DesiredReplicas))
		builder.addRelations(ctx, rootID, detail.Pods, detail.RelatedResources)
		builder.addHPAAndPDB(ctx, rootID, namespace, "Deployment", name, detail.Selector)
	case "pod":
		detail, err := c.GetPodDetail(ctx, namespace, name)
		if err != nil {
			return domainresource.ResourceGraph{}, err
		}
		builder.nodes[rootID] = graphNode(c.cfg.ID, "v1", "Pod", namespace, name, detail.Phase)
		builder.addPodRelated(rootID, detail.RelatedResources)
	case "service":
		detail, err := c.GetServiceDetail(ctx, namespace, name)
		if err != nil {
			return domainresource.ResourceGraph{}, err
		}
		for _, pod := range detail.BackendPods {
			podID := builder.addNode("v1", "Pod", pod.Namespace, pod.Name, pod.Phase)
			builder.addEdge(rootID, podID, "selects")
		}
		if ingresses, err := c.ListIngresses(ctx, namespace); err == nil {
			for _, ingress := range ingresses {
				for _, backend := range ingress.BackendServices {
					if backend == name {
						ingressID := builder.addNode("networking.k8s.io/v1", "Ingress", namespace, ingress.Name, "")
						builder.addEdge(ingressID, rootID, "routes-to")
						break
					}
				}
			}
		}
	case "ingress":
		detail, err := c.GetIngressDetail(ctx, namespace, name)
		if err != nil {
			return domainresource.ResourceGraph{}, err
		}
		for _, backend := range detail.Backends {
			serviceID := builder.addNode("v1", "Service", namespace, backend.ServiceName, "")
			builder.addEdge(rootID, serviceID, "routes-to")
			for _, pod := range backend.Pods {
				podID := builder.addNode("v1", "Pod", pod.Namespace, pod.Name, pod.Phase)
				builder.addEdge(serviceID, podID, "selects")
			}
		}
	default:
		if _, err := c.GetResourceYAML(ctx, namespace, kind, name); err != nil {
			return domainresource.ResourceGraph{}, err
		}
	}
	if events, err := c.ListClusterEvents(ctx, namespace, 100); err == nil {
		builder.addEvents(events)
	}
	return builder.graph(rootID), nil
}

type agentResourceGraphBuilder struct {
	client               *Client
	clusterID, namespace string
	nodes                map[string]domainresource.ResourceGraphNode
	edges                map[string]domainresource.ResourceGraphEdge
	evidence             []domainresource.ResourceEvidence
}

func newAgentResourceGraphBuilder(client *Client, clusterID, namespace string) *agentResourceGraphBuilder {
	return &agentResourceGraphBuilder{client: client, clusterID: clusterID, namespace: namespace, nodes: map[string]domainresource.ResourceGraphNode{}, edges: map[string]domainresource.ResourceGraphEdge{}, evidence: []domainresource.ResourceEvidence{}}
}

func (b *agentResourceGraphBuilder) addRelations(ctx context.Context, rootID string, pods []domainresource.PodView, related []domainresource.WorkloadRelationView) {
	for _, pod := range pods {
		podID := b.addNode("v1", "Pod", pod.Namespace, pod.Name, pod.Phase)
		ownerID := rootID
		if detail, err := b.client.GetPodDetail(ctx, pod.Namespace, pod.Name); err == nil {
			for _, relation := range detail.RelatedResources {
				if graphContainsString(relation.Relations, "owner") {
					candidate := b.addNode(resourceAPIVersion(relation.Kind), relation.Kind, relation.Namespace, relation.Name, "")
					b.addEdge(rootID, candidate, "owns")
					ownerID = candidate
				}
			}
		}
		b.addEdge(ownerID, podID, "owns")
	}
	b.addRelated(rootID, related)
}

func (b *agentResourceGraphBuilder) addPodRelated(rootID string, related []domainresource.PodRelatedResourceView) {
	for _, relation := range related {
		id := b.addNode(resourceAPIVersion(relation.Kind), relation.Kind, relation.Namespace, relation.Name, "")
		if graphContainsString(relation.Relations, "owner") {
			b.addEdge(id, rootID, "owns")
			continue
		}
		relationName := "related"
		if len(relation.Relations) > 0 {
			relationName = relation.Relations[0]
		}
		b.addEdge(rootID, id, relationName)
	}
}

func graphContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (b *agentResourceGraphBuilder) addRelated(rootID string, related []domainresource.WorkloadRelationView) {
	for _, relation := range related {
		id := b.addNode(resourceAPIVersion(relation.Kind), relation.Kind, relation.Namespace, relation.Name, "")
		if relation.Relation == "owner" {
			b.addEdge(id, rootID, "owns")
		} else {
			b.addEdge(rootID, id, relation.Relation)
		}
	}
}

func (b *agentResourceGraphBuilder) addHPAAndPDB(ctx context.Context, rootID, namespace, kind, name string, workloadLabels map[string]string) {
	if hpas, err := b.client.typed.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, hpa := range hpas.Items {
			if strings.EqualFold(hpa.Spec.ScaleTargetRef.Kind, kind) && hpa.Spec.ScaleTargetRef.Name == name {
				hpaID := b.addNode("autoscaling/v2", "HorizontalPodAutoscaler", namespace, hpa.Name, "")
				b.addEdge(hpaID, rootID, "scales")
			}
		}
	}
	if pdbs, err := b.client.typed.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, pdb := range pdbs.Items {
			selector, selectorErr := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
			if selectorErr == nil && selector.Matches(labels.Set(workloadLabels)) {
				pdbID := b.addNode("policy/v1", "PodDisruptionBudget", namespace, pdb.Name, "")
				b.addEdge(pdbID, rootID, "protects")
			}
		}
	}
}

func (b *agentResourceGraphBuilder) addNode(apiVersion, kind, namespace, name, status string) string {
	id := apiVersion + ":" + kind + ":" + namespace + "/" + name
	b.nodes[id] = graphNode(b.clusterID, apiVersion, kind, namespace, name, status)
	return id
}

func graphNode(clusterID, apiVersion, kind, namespace, name, status string) domainresource.ResourceGraphNode {
	scope := domainresource.ResourceScopeModeNamespace
	if namespace == "" {
		scope = domainresource.ResourceScopeModeCluster
	}
	return domainresource.ResourceGraphNode{ID: apiVersion + ":" + kind + ":" + namespace + "/" + name, Resource: domainresource.ResourceRef{ClusterID: clusterID, APIVersion: apiVersion, Kind: kind, Name: name, Namespace: namespace, ScopeMode: scope}, Status: status}
}

func (b *agentResourceGraphBuilder) addEdge(sourceID, targetID, relation string) {
	if sourceID == "" || targetID == "" || sourceID == targetID {
		return
	}
	id := relation + ":" + sourceID + ":" + targetID
	b.edges[id] = domainresource.ResourceGraphEdge{ID: id, SourceID: sourceID, TargetID: targetID, Relation: relation}
}

func (b *agentResourceGraphBuilder) addEvents(events []domainresource.ClusterEventView) {
	for _, event := range events {
		resourceID := ""
		for id, node := range b.nodes {
			if strings.EqualFold(node.Resource.Kind, event.InvolvedKind) && node.Resource.Name == event.InvolvedName && node.Resource.Namespace == event.Namespace {
				resourceID = id
				break
			}
		}
		if resourceID == "" {
			continue
		}
		severity := "info"
		if strings.EqualFold(event.Type, "Warning") {
			severity = "warning"
		}
		observedAt := event.LastTimestamp
		if observedAt == "" {
			observedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		b.evidence = append(b.evidence, domainresource.ResourceEvidence{ID: "event/" + event.Namespace + "/" + event.Name, Type: "kubernetes-event", Severity: severity, Summary: strings.TrimSpace(event.Reason + " " + event.Message), ObservedAt: observedAt, ResourceID: resourceID, SourceRef: "Event/" + event.Namespace + "/" + event.Name})
	}
}

func (b *agentResourceGraphBuilder) graph(rootID string) domainresource.ResourceGraph {
	graph := domainresource.ResourceGraph{ClusterID: b.clusterID, Namespace: b.namespace, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), RootID: rootID, Nodes: []domainresource.ResourceGraphNode{}, Edges: []domainresource.ResourceGraphEdge{}, Evidence: b.evidence, Warnings: []string{}}
	for _, node := range b.nodes {
		graph.Nodes = append(graph.Nodes, node)
	}
	for _, edge := range b.edges {
		graph.Edges = append(graph.Edges, edge)
	}
	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].ID < graph.Nodes[j].ID })
	sort.Slice(graph.Edges, func(i, j int) bool { return graph.Edges[i].ID < graph.Edges[j].ID })
	return graph
}

func resourceAPIVersion(kind string) string {
	switch strings.ToLower(kind) {
	case "deployment", "replicaset", "statefulset", "daemonset":
		return "apps/v1"
	case "job", "cronjob":
		return "batch/v1"
	case "ingress", "networkpolicy", "ingressclass":
		return "networking.k8s.io/v1"
	case "horizontalpodautoscaler":
		return "autoscaling/v2"
	case "poddisruptionbudget":
		return "policy/v1"
	default:
		return "v1"
	}
}

func deploymentHealth(ready, desired int32) string {
	if desired > 0 && ready >= desired {
		return "healthy"
	}
	return "progressing"
}

var (
	agentKubescapeConfigurationGVR = schema.GroupVersionResource{Group: "spdx.softwarecomposition.kubescape.io", Version: "v1beta1", Resource: "workloadconfigurationscansummaries"}
	agentKubescapeVulnerabilityGVR = schema.GroupVersionResource{Group: "spdx.softwarecomposition.kubescape.io", Version: "v1beta1", Resource: "vulnerabilitymanifestsummaries"}
)

func (c *Client) GetSecurityPosture(ctx context.Context, namespace string, limit int) (domainresource.SecurityPosture, error) {
	configurations, configErr := c.listKubescapeSummaries(ctx, agentKubescapeConfigurationGVR, namespace)
	vulnerabilities, vulnerabilityErr := c.listKubescapeSummaries(ctx, agentKubescapeVulnerabilityGVR, namespace)
	if apierrors.IsNotFound(configErr) && apierrors.IsNotFound(vulnerabilityErr) {
		return domainresource.SecurityPosture{ClusterID: c.cfg.ID, Provider: "kubescape", Status: "unsupported", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Message: "Kubescape summary resources are not installed or have not produced results.", Findings: []domainresource.SecurityFinding{}, Warnings: []string{}}, nil
	}
	posture := buildAgentKubescapePosture(c.cfg.ID, configurations, vulnerabilities, limit)
	if configErr != nil || vulnerabilityErr != nil {
		posture.Status = "partial"
		posture.Warnings = append(posture.Warnings, "Some Kubescape summary resources were unavailable.")
	}
	return posture, nil
}

func (c *Client) listKubescapeSummaries(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error) {
	resource := c.dynamic.Resource(gvr)
	var reader dynamic.ResourceInterface = resource
	if namespace != "" {
		reader = resource.Namespace(namespace)
	}
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	page, err := reader.List(queryCtx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func buildAgentKubescapePosture(clusterID string, configurations, vulnerabilities []unstructured.Unstructured, limit int) domainresource.SecurityPosture {
	posture := domainresource.SecurityPosture{ClusterID: clusterID, Provider: "kubescape", Status: "available", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Findings: []domainresource.SecurityFinding{}, Warnings: []string{}}
	for _, item := range configurations {
		severities, _, _ := unstructured.NestedMap(item.Object, "spec", "severities")
		addAgentFlatCounts(&posture.Counts, severities)
		controls, _, _ := unstructured.NestedMap(item.Object, "spec", "controls")
		for key, raw := range controls {
			control, _ := raw.(map[string]any)
			status, _, _ := unstructured.NestedString(control, "status", "status")
			if !strings.EqualFold(status, "failed") && !strings.EqualFold(status, "warning") {
				continue
			}
			severity, _, _ := unstructured.NestedString(control, "severity", "severity")
			info, _, _ := unstructured.NestedString(control, "status", "info")
			posture.Findings = append(posture.Findings, domainresource.SecurityFinding{ID: "configuration:" + key + ":" + item.GetNamespace() + ":" + item.GetName(), Category: "configuration", Severity: normalizeAgentSeverity(severity), Title: nonEmpty(info, "Kubescape configuration control failed"), Status: status, ControlID: key, Resource: agentKubescapeResource(clusterID, item)})
		}
	}
	for _, item := range vulnerabilities {
		severities, _, _ := unstructured.NestedMap(item.Object, "spec", "severities")
		highest, total := addAgentNestedCounts(&posture.Counts, severities)
		if total > 0 {
			posture.Findings = append(posture.Findings, domainresource.SecurityFinding{ID: "vulnerability:" + item.GetNamespace() + ":" + item.GetName(), Category: "vulnerability", Severity: highest, Title: fmt.Sprintf("%d container image vulnerabilities", total), Status: "detected", Resource: agentKubescapeResource(clusterID, item)})
		}
	}
	sort.SliceStable(posture.Findings, func(i, j int) bool { return posture.Findings[i].ID < posture.Findings[j].ID })
	if limit > 0 && len(posture.Findings) > limit {
		posture.Findings = posture.Findings[:limit]
		posture.Warnings = append(posture.Warnings, "Security findings were truncated to the requested limit.")
	}
	return posture
}

func agentKubescapeResource(clusterID string, item unstructured.Unstructured) *domainresource.ResourceRef {
	labels := item.GetLabels()
	kind, name := labels["kubescape.io/workload-kind"], labels["kubescape.io/workload-name"]
	if kind == "" || name == "" {
		kind, name = item.GetKind(), item.GetName()
	}
	namespace := labels["kubescape.io/workload-namespace"]
	if namespace == "" {
		namespace = item.GetNamespace()
	}
	apiVersion := labels["kubescape.io/workload-api-version"]
	if group := labels["kubescape.io/workload-api-group"]; group != "" {
		apiVersion = group + "/" + apiVersion
	}
	return &domainresource.ResourceRef{ClusterID: clusterID, APIVersion: nonEmpty(apiVersion, item.GetAPIVersion()), Kind: kind, Name: name, Namespace: namespace, ScopeMode: domainresource.ResourceScopeModeNamespace}
}

func addAgentFlatCounts(counts *domainresource.SecuritySeverityCounts, values map[string]any) {
	counts.Critical += agentInt(values["critical"])
	counts.High += agentInt(values["high"])
	counts.Medium += agentInt(values["medium"])
	counts.Low += agentInt(values["low"])
	counts.Unknown += agentInt(values["unknown"])
}

func addAgentNestedCounts(counts *domainresource.SecuritySeverityCounts, values map[string]any) (string, int64) {
	totals := map[string]int64{}
	for _, severity := range []string{"critical", "high", "medium", "low", "unknown"} {
		entry, _ := values[severity].(map[string]any)
		totals[severity] = agentInt(entry["all"])
	}
	counts.Critical += totals["critical"]
	counts.High += totals["high"]
	counts.Medium += totals["medium"]
	counts.Low += totals["low"]
	counts.Unknown += totals["unknown"]
	total := totals["critical"] + totals["high"] + totals["medium"] + totals["low"] + totals["unknown"]
	for _, severity := range []string{"critical", "high", "medium", "low", "unknown"} {
		if totals[severity] > 0 {
			return severity, total
		}
	}
	return "unknown", total
}

func agentInt(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}

func normalizeAgentSeverity(value string) string {
	switch strings.ToLower(value) {
	case "critical", "high", "medium", "low":
		return strings.ToLower(value)
	default:
		return "unknown"
	}
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
