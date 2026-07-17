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
	if len(signatures) != 110 {
		t.Fatalf("route count = %d, want 110", len(signatures))
	}
	const expectedRouteDigest = "521d2aadded0d29c6590372080a3bcc7886f6e837c4d88e359d11d92e70a25c8"
	if routeDigest != expectedRouteDigest {
		t.Fatalf("route digest = %s, want %s", routeDigest, expectedRouteDigest)
	}

	expected := []string{
		http.MethodGet + " /healthz",
		http.MethodGet + " /api/v1/diagnostics",
		http.MethodGet + " /api/v1/platform/workloads/pods",
		http.MethodGet + " /api/v1/platform/configuration/configmaps",
		http.MethodGet + " /api/v1/platform/access-control/roles",
		http.MethodGet + " /api/v1/platform/network/services",
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
