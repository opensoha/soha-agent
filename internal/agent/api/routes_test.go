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
	if len(signatures) != 108 {
		t.Fatalf("route count = %d, want 108", len(signatures))
	}
	const expectedRouteDigest = "c3d63f61a0a236db69a43c055da341009ef4642bb99dddd8ef0aa9dbfbe1ebc8"
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
