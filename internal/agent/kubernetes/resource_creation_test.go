package kubernetes

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

func TestPreflightResourceCreateUsesDiscoveryDryRunWithoutPersisting(t *testing.T) {
	client, actions := newResourceCreationTestClient(t)
	result := client.PreflightResourceCreate(context.Background(), resourceCreateRequest(configMapCreateDocument(0, "platform", "app")))

	if !result.Ready || len(result.Items) != 1 || result.Items[0].DryRun.Status != domainresource.KubernetesResourceDryRunStatusPassed {
		t.Fatalf("PreflightResourceCreate() = %#v, want ready item", result)
	}
	if len(*actions) != 1 {
		t.Fatalf("create actions = %#v, want one dry-run", *actions)
	}
	if options := resourceCreateOptions(true); len(options.DryRun) != 1 || options.DryRun[0] != metav1.DryRunAll {
		t.Fatalf("resourceCreateOptions(true) = %#v, want DryRunAll", options)
	}
}

func TestPreflightResourceCreateRejectsResolvedTargetExpansion(t *testing.T) {
	cases := []struct {
		name     string
		document domainresource.KubernetesResourceAgentCreateDocument
		wantCode string
	}{
		{name: "namespace", document: configMapCreateDocument(0, "other", "app"), wantCode: "namespace_mismatch"},
		{name: "kind", document: configMapCreateDocument(0, "platform", "app"), wantCode: "resource_kind_mismatch"},
		{name: "multiple documents", document: configMapCreateDocument(0, "platform", "app"), wantCode: "multi_document_not_allowed"},
		{name: "cluster", document: configMapCreateDocument(0, "platform", "app"), wantCode: "resource_capability_unsupported"},
		{name: "hash", document: configMapCreateDocument(0, "platform", "app"), wantCode: "resource_kind_mismatch"},
		{name: "scope", document: configMapCreateDocument(0, "platform", "app"), wantCode: "namespace_mismatch"},
	}
	cases[0].document.ResourceRef.Namespace = "platform"
	cases[1].document.Document.Kind = "Secret"
	cases[1].document.ResourceRef.Kind = "Secret"
	cases[2].document.Content += "\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: second\n  namespace: platform\n"
	cases[2].document.Document.ContentHash = contentHash(cases[2].document.Content)
	cases[3].document.ResourceRef.ClusterID = "other-cluster"
	cases[4].document.Document.ContentHash = strings.Repeat("0", 64)
	cases[5].document.Document.ScopeMode = domainresource.KubernetesResourceScopeModeCluster

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, actions := newResourceCreationTestClient(t)
			result := client.PreflightResourceCreate(context.Background(), resourceCreateRequest(tc.document))
			if result.Ready || len(result.Items) != 1 || len(result.Items[0].Errors) != 1 || string(result.Items[0].Errors[0].Code) != tc.wantCode {
				t.Fatalf("PreflightResourceCreate() = %#v, want code %q", result, tc.wantCode)
			}
			if len(*actions) != 0 {
				t.Fatalf("create actions = %#v, want zero", *actions)
			}
		})
	}
}

func TestPreflightResourceCreateStripsClusterScopedNamespace(t *testing.T) {
	client, actions := newResourceCreationTestClient(t)
	content := "apiVersion: v1\nkind: Node\nmetadata:\n  name: worker-1\n  namespace: ignored\n"
	document := domainresource.KubernetesResourceAgentCreateDocument{
		Document:    domainresource.KubernetesResourceDocument{Index: 0, APIVersion: "v1", Kind: "Node", Name: "worker-1", Namespace: "ignored", ScopeMode: domainresource.KubernetesResourceScopeModeCluster, ContentHash: contentHash(content)},
		ResourceRef: domainresource.KubernetesResourceRef{ClusterID: "agent-cluster", APIVersion: "v1", Kind: "Node", Name: "worker-1", ScopeMode: domainresource.KubernetesResourceScopeModeCluster},
		Content:     content,
	}
	result := client.PreflightResourceCreate(context.Background(), resourceCreateRequest(document))

	if !result.Ready || len(result.Items[0].Warnings) != 1 || result.Items[0].Warnings[0].Code != "cluster_scoped_namespace_ignored" {
		t.Fatalf("PreflightResourceCreate() = %#v, want cluster scope warning", result)
	}
	if len(*actions) != 1 || (*actions)[0].namespace != "" {
		t.Fatalf("create actions = %#v, want cluster-scoped request", *actions)
	}
}

func TestCreateResourcesPreflightsAllDocumentsBeforeWriting(t *testing.T) {
	client, actions := newResourceCreationTestClient(t)
	calls := 0
	client.dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "configmaps", func(action ktesting.Action) (bool, runtime.Object, error) {
		calls++
		create := action.(ktesting.CreateAction)
		if create.GetObject().(*unstructured.Unstructured).GetName() == "denied" {
			return true, nil, errors.New("dry-run rejected")
		}
		return true, create.GetObject(), nil
	})
	request := resourceCreateRequest(
		configMapCreateDocument(0, "platform", "allowed"),
		configMapCreateDocument(1, "platform", "denied"),
	)
	result := client.CreateResources(context.Background(), request)

	if result.Status != domainresource.KubernetesResourceCreateBatchStatusFailed || result.Items[0].Status != domainresource.KubernetesResourceCreateResultStatusNotStarted || result.Items[1].Status != domainresource.KubernetesResourceCreateResultStatusFailed {
		t.Fatalf("CreateResources() = %#v, want zero writes after failed preflight", result)
	}
	if calls != 2 || len(*actions) != 0 {
		t.Fatalf("create calls = %d recorded=%#v, want only two preflight calls", calls, *actions)
	}
}

func TestCreateResourcesCreatesOnlyAfterSuccessfulPreflight(t *testing.T) {
	client, actions := newResourceCreationTestClient(t)
	result := client.CreateResources(context.Background(), resourceCreateRequest(configMapCreateDocument(0, "platform", "app")))

	if result.Status != domainresource.KubernetesResourceCreateBatchStatusSucceeded || result.Items[0].Status != domainresource.KubernetesResourceCreateResultStatusSucceeded {
		t.Fatalf("CreateResources() = %#v, want succeeded", result)
	}
	if len(*actions) != 2 {
		t.Fatalf("create actions = %#v, want dry-run then persistent create", *actions)
	}
}

type recordedResourceCreate struct {
	namespace string
}

func newResourceCreationTestClient(t *testing.T) (*Client, *[]recordedResourceCreate) {
	t.Helper()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	actions := make([]recordedResourceCreate, 0)
	dynamicClient.PrependReactor("create", "*", func(action ktesting.Action) (bool, runtime.Object, error) {
		create := action.(ktesting.CreateAction)
		object := create.GetObject().(*unstructured.Unstructured).DeepCopy()
		actions = append(actions, recordedResourceCreate{namespace: action.GetNamespace()})
		return true, object, nil
	})
	discoveryClient := &fakediscovery.FakeDiscovery{Fake: &ktesting.Fake{}}
	discoveryClient.Resources = []*metav1.APIResourceList{{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap"},
			{Name: "nodes", SingularName: "node", Namespaced: false, Kind: "Node"},
		},
	}}
	return &Client{cfg: cfgpkg.KubernetesConfig{ID: "agent-cluster"}, dynamic: dynamicClient, discovery: discoveryClient}, &actions
}

func resourceCreateRequest(documents ...domainresource.KubernetesResourceAgentCreateDocument) domainresource.KubernetesResourceAgentCreateRequest {
	return domainresource.KubernetesResourceAgentCreateRequest{OperationID: "operation-1", Documents: documents}
}

func configMapCreateDocument(index int, namespace, name string) domainresource.KubernetesResourceAgentCreateDocument {
	content := strings.Join([]string{"apiVersion: v1", "kind: ConfigMap", "metadata:", "  name: " + name, "  namespace: " + namespace, ""}, "\n")
	return domainresource.KubernetesResourceAgentCreateDocument{
		Document: domainresource.KubernetesResourceDocument{
			Index: index, APIVersion: "v1", Kind: "ConfigMap", Namespace: namespace, Name: name,
			ScopeMode: domainresource.KubernetesResourceScopeModeNamespace, ContentHash: contentHash(content),
		},
		ResourceRef: domainresource.KubernetesResourceRef{
			ClusterID: "agent-cluster", APIVersion: "v1", Kind: "ConfigMap", Namespace: namespace, Name: name,
			ScopeMode: domainresource.KubernetesResourceScopeModeNamespace,
		},
		Content: content,
	}
}
