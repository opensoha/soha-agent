package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
)

func TestNewRegistersRouteFamilies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New(
		cfgpkg.Config{HTTP: cfgpkg.HTTPConfig{BasePath: "/api/v1"}},
		nil,
		&k8sagent.Client{},
		&fakeRuntimeController{},
	)
	router, ok := server.httpServer.Handler.(*gin.Engine)
	if !ok {
		t.Fatalf("handler type = %T, want *gin.Engine", server.httpServer.Handler)
	}
	registered := make(map[string]struct{}, len(router.Routes()))
	signatures := make([]string, 0, len(router.Routes()))
	for _, route := range router.Routes() {
		signature := route.Method + " " + route.Path
		registered[signature] = struct{}{}
		signatures = append(signatures, signature)
	}
	sort.Strings(signatures)
	routeDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(signatures, "\n"))))
	if len(signatures) != 248 {
		t.Fatalf("route count = %d, want 248", len(signatures))
	}
	const expectedRouteDigest = "abf68b1e67db932a984a6a1f044c24dd36b30ee3652e684fa1ad44dd44615481"
	if routeDigest != expectedRouteDigest {
		t.Fatalf("route digest = %s, want %s", routeDigest, expectedRouteDigest)
	}

	expected := []string{
		http.MethodPost + " /api/v1/platform/ownership-v1/resources/yaml/preflight",
		http.MethodPost + " /api/v1/platform/ownership-v2/resources/yaml/preflight",
		http.MethodPost + " /api/v1/platform/ownership-v2/manifests/rollout/observe",
		http.MethodPost + " /api/v1/platform/ownership-v2/manifests/rollout/control",
		http.MethodPost + " /api/v1/platform/extensions/custom-resources/delete-observed",
		http.MethodPost + " /api/v1/platform/helm/delivery/prepare",
		http.MethodPost + " /api/v1/platform/helm/delivery/preflight",
		http.MethodPost + " /api/v1/platform/helm/delivery/observe",
		http.MethodGet + " /healthz",
		http.MethodGet + " /api/v1/diagnostics",
		http.MethodGet + " /api/v1/platform/workloads/pods",
		http.MethodPost + " /api/v1/platform/logs/query",
		http.MethodPost + " /api/v1/platform/logs/stream",
		http.MethodGet + " /api/v1/platform/configuration/configmaps",
		http.MethodGet + " /api/v1/platform/access-control/roles",
		http.MethodPost + " /api/v1/platform/access-control/access-reviews",
		http.MethodGet + " /api/v1/platform/network/services",
		http.MethodGet + " /api/v1/platform/network/services/:name/detail",
		http.MethodPost + " /api/v1/platform/resources/yaml/preflight",
		http.MethodGet + " /api/v1/platform/resources/graph",
		http.MethodGet + " /api/v1/platform/resources/stream",
		http.MethodGet + " /api/v1/platform/security/posture",
		http.MethodGet + " /api/v1/platform/helm/releases/:name/manifest",
		http.MethodPost + " /api/v1/platform/helm/releases/:name/rollback/preflight",
		http.MethodPost + " /api/v1/platform/helm/releases/:name/rollback",
		http.MethodGet + " /api/v1/platform/storage/persistentvolumes",
		http.MethodGet + " /api/v1/platform/helm/releases",
		http.MethodGet + " /api/v1/runtime/execution-tasks",
		http.MethodPost + " /api/v1/docker/runtime/logs",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("route %q is not registered", route)
		}
	}
}
