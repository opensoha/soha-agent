package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestBasicResourcesThroughAuthenticatedHTTPAndKubernetesREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var mu sync.Mutex
	secret := corev1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "team", UID: "fixture-uid", ResourceVersion: "1"}, Data: map[string][]byte{"token": []byte("old")}}
	kube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/team/secrets/auth":
			if r.Method == http.MethodPut {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if _, _, err := scheme.Codecs.UniversalDeserializer().Decode(raw, nil, &secret); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
			}
			_ = json.NewEncoder(w).Encode(secret)
		case "/api/v1/namespaces/team/secrets/denied":
			w.WriteHeader(403)
			_ = json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: 403, Message: "private-provider-and-secret-value"})
		case "/api/v1/nodes/node":
			_ = json.NewEncoder(w).Encode(corev1.Node{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Node"}, ObjectMeta: metav1.ObjectMeta{Name: "node", UID: "node-uid"}, Spec: corev1.NodeSpec{Unschedulable: r.Method == http.MethodPatch}})
		case "/api/v1/pods":
			_ = json.NewEncoder(w).Encode(corev1.PodList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}, Items: []corev1.Pod{}})
		default:
			t.Errorf("unexpected Kubernetes request %s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer kube.Close()
	client, err := k8sagent.New(cfgpkg.KubernetesConfig{KubeconfigData: fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: fixture\ncontexts:\n- name: fixture\n  context: {cluster: fixture, user: fixture}\nclusters:\n- name: fixture\n  cluster: {server: %s}\nusers:\n- name: fixture\n  user: {}\n", kube.URL)})
	if err != nil {
		t.Fatal(err)
	}
	server := New(cfgpkg.Config{HTTP: cfgpkg.HTTPConfig{BasePath: "/api/v1"}, Auth: cfgpkg.AuthConfig{BearerToken: "fixture-token"}, Security: cfgpkg.SecurityConfig{AllowedActions: []string{actionPlatformResourcesApply, actionPlatformNodesDrain}}}, zap.NewNop(), client, nil)
	agent := httptest.NewServer(server.httpServer.Handler)
	defer agent.Close()
	call := func(method, path, body string, authenticated bool) (int, string) {
		t.Helper()
		req, err := http.NewRequest(method, agent.URL+"/api/v1/platform/ownership-v2"+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.Header.Set("Authorization", "Bearer fixture-token")
		}
		resp, err := agent.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(raw)
	}
	if status, _ := call("GET", "/configuration/secrets/auth/detail?namespace=team", "", false); status != 401 {
		t.Fatalf("unauthenticated request: %d", status)
	}
	oversized := `{"padding":"` + strings.Repeat("x", 2<<20) + `"}`
	for _, request := range []struct{ method, path string }{
		{"PUT", "/configuration/secrets/auth/data?namespace=team"},
		{"PUT", "/configuration/configmaps/config/data?namespace=team"},
		{"PUT", "/namespaces/team"},
		{"PUT", "/infrastructure/nodes/node"},
		{"PUT", "/infrastructure/nodes/node/schedulability"},
		{"POST", "/infrastructure/nodes/node/drain"},
		{"POST", "/workloads/cronjobs/job/suspend?namespace=team"},
	} {
		if status, _ := call(request.method, request.path, oversized, true); status != 400 {
			t.Fatalf("oversized %s %s: %d", request.method, request.path, status)
		}
	}
	status, body := call("PUT", "/configuration/secrets/auth/data?namespace=team", `{"data":{"token":"new-value"}}`, true)
	if status != 200 || !strings.Contains(body, "bmV3LXZhbHVl") {
		t.Fatalf("Secret update: %d %s", status, body)
	}
	mu.Lock()
	value := string(secret.Data["token"])
	mu.Unlock()
	if value != "new-value" {
		t.Fatal("Kubernetes received incorrect Secret bytes")
	}
	status, body = call("GET", "/configuration/secrets/denied/detail?namespace=team", "", true)
	if status != 403 || strings.Contains(body, "private-provider") {
		t.Fatalf("RBAC failure: %d %s", status, body)
	}
	if status, body = call("POST", "/infrastructure/nodes/node/drain", `{"timeoutSeconds":30}`, true); status != 204 {
		t.Fatalf("drain over real HTTP: %d %s", status, body)
	}
}
