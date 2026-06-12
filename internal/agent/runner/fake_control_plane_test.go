package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	"go.uber.org/zap"
)

func TestExecutionRunnerFakeControlPlaneCompletesWithCallbackRetryAndMetrics(t *testing.T) {
	task := ExecutionTask{
		ID:                       "task-1",
		ApplicationID:            "app-1",
		ApplicationEnvironmentID: "env-1",
		TaskKind:                 "build",
		ProviderKind:             "ci_agent_runner",
		Status:                   "queued",
		CallbackToken:            "callback-token",
		Payload: map[string]any{
			"commands": []any{"printf done"},
		},
	}

	var mu sync.Mutex
	claimCount := 0
	completedAttempts := 0
	statuses := make([]string, 0, 4)
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/delivery/execution-tasks/claim":
			if got := r.Header.Get("Authorization"); got != "Bearer runner-token" {
				t.Fatalf("unexpected claim authorization header %q", got)
			}
			mu.Lock()
			defer mu.Unlock()
			claimCount++
			if claimCount == 1 {
				return jsonResponse(t, http.StatusAccepted, map[string]any{"data": task}), nil
			}
			return jsonResponse(t, http.StatusAccepted, map[string]any{"data": ExecutionTask{}}), nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/delivery/execution-tasks/task-1/runner-status":
			next := task
			next.Status = "running"
			return jsonResponse(t, http.StatusOK, map[string]any{"data": next}), nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/delivery/execution-callbacks":
			var req callbackRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode callback request: %v", err)
			}
			mu.Lock()
			statuses = append(statuses, req.Status)
			if req.Status == "completed" {
				completedAttempts++
				if completedAttempts == 1 {
					mu.Unlock()
					return jsonResponse(t, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "transient callback failure"}}), nil
				}
			}
			mu.Unlock()
			next := task
			next.Status = req.Status
			return jsonResponse(t, http.StatusAccepted, map[string]any{"data": next}), nil
		default:
			t.Fatalf("unexpected fake control-plane request %s %s", r.Method, r.URL.Path)
			return jsonResponse(t, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "not found"}}), nil
		}
	})

	runner := New(cfgpkg.ControlPlaneConfig{
		BaseURL:        "http://control-plane",
		BearerToken:    "runner-token",
		ProviderKinds:  []string{"ci_agent_runner"},
		WorkspaceRoot:  filepath.Join(t.TempDir(), "workspace"),
		DefaultTimeout: 5 * time.Second,
		CallbackRetry: cfgpkg.CallbackRetryConfig{
			MaxAttempts: 2,
			Backoff:     time.Millisecond,
		},
	}, zap.NewNop())
	runner.httpClient = &http.Client{Transport: transport}

	claimed, ok := runner.claim(context.Background())
	if !ok {
		t.Fatal("expected fake control plane to return a task")
	}
	runner.execute(context.Background(), claimed)

	mu.Lock()
	defer mu.Unlock()
	if completedAttempts != 2 {
		t.Fatalf("completed callback attempts = %d, want 2", completedAttempts)
	}
	if !containsStatus(statuses, "running") || !containsStatus(statuses, "completed") {
		t.Fatalf("expected running and completed callbacks, got %v", statuses)
	}
	metrics := runner.MetricsSnapshot()
	if metrics.Execution.Claims != 1 || metrics.Execution.Completed != 1 || metrics.Execution.CallbackFailures != 1 || metrics.Execution.FinalCallbacks != 1 {
		t.Fatalf("unexpected execution metrics: %#v", metrics.Execution)
	}
}

func TestExecutionRunnerTimeoutSendsFinalStateWithDetachedContext(t *testing.T) {
	task := ExecutionTask{
		ID:            "task-timeout",
		ApplicationID: "app-timeout",
		TaskKind:      "build",
		ProviderKind:  "ci_agent_runner",
		Status:        "queued",
		CallbackToken: "callback-token",
		Payload: map[string]any{
			"commands": []any{"sleep 1"},
			"timeout":  "50ms",
		},
	}

	var mu sync.Mutex
	statuses := make([]string, 0, 2)
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/delivery/execution-callbacks":
			var req callbackRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode callback request: %v", err)
			}
			mu.Lock()
			statuses = append(statuses, req.Status)
			mu.Unlock()
			next := task
			next.Status = req.Status
			return jsonResponse(t, http.StatusAccepted, map[string]any{"data": next}), nil
		default:
			t.Fatalf("unexpected fake control-plane request %s %s", r.Method, r.URL.Path)
			return jsonResponse(t, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "not found"}}), nil
		}
	})

	runner := New(cfgpkg.ControlPlaneConfig{
		BaseURL:       "http://control-plane",
		BearerToken:   "runner-token",
		WorkspaceRoot: filepath.Join(t.TempDir(), "workspace"),
		CallbackRetry: cfgpkg.CallbackRetryConfig{
			MaxAttempts: 2,
			Backoff:     time.Millisecond,
		},
	}, zap.NewNop())
	runner.httpClient = &http.Client{Transport: transport}

	runner.execute(context.Background(), task)

	mu.Lock()
	defer mu.Unlock()
	if !containsStatus(statuses, "callback_timeout") {
		t.Fatalf("expected callback_timeout final callback, got %v", statuses)
	}
	metrics := runner.MetricsSnapshot()
	if metrics.Execution.TimedOut != 1 || metrics.Execution.FinalCallbacks != 1 {
		t.Fatalf("unexpected timeout metrics: %#v", metrics.Execution)
	}
}

func TestRunnerExecutionSlotsRespectMaxConcurrency(t *testing.T) {
	runner := New(cfgpkg.ControlPlaneConfig{MaxConcurrency: 1}, zap.NewNop())
	if !runner.tryAcquireExecutionSlot() {
		t.Fatal("first execution slot acquire failed")
	}
	if runner.tryAcquireExecutionSlot() {
		t.Fatal("second execution slot acquire succeeded, want max concurrency guard")
	}
	runner.releaseExecutionSlot()
	if !runner.tryAcquireExecutionSlot() {
		t.Fatal("execution slot acquire after release failed")
	}
	runner.releaseExecutionSlot()
}

func containsStatus(statuses []string, status string) bool {
	for _, item := range statuses {
		if strings.TrimSpace(item) == status {
			return true
		}
	}
	return false
}

func jsonResponse(t *testing.T, status int, payload any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal fake control-plane response: %v", err)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
