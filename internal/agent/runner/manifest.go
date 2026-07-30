package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sohaapi "github.com/opensoha/soha-contracts/gen/go/sohaapi"
)

func (r *Runner) executeManifestTask(ctx context.Context, task ExecutionTask) {
	if r.manifestExecutor == nil {
		r.finalCallback(ctx, task, "failed", map[string]any{"error": "manifest execution is unavailable"})
		r.metrics.markOutcome(metricScopeExecution, "failed")
		return
	}
	payload, err := decodeManifestPayload(task.Payload)
	if err != nil {
		r.finalCallback(ctx, task, "failed", map[string]any{"error": "invalid manifest task payload"})
		r.metrics.markOutcome(metricScopeExecution, "failed")
		return
	}
	if payload.ClusterID != "" && payload.ClusterID != r.manifestClusterID {
		r.finalCallback(ctx, task, "failed", map[string]any{"error": "manifest task targets a different cluster"})
		r.metrics.markOutcome(metricScopeExecution, "failed")
		return
	}

	remoteTask, ok := r.callback(ctx, task, "running", map[string]any{
		"action":      payload.Action,
		"generation":  payload.Generation,
		"heartbeatAt": time.Now().UTC().Format(time.RFC3339),
	})
	if ok && shouldStopLocalExecution(remoteTask.Status) {
		return
	}
	result, executeErr := r.manifestExecutor.ExecuteManifestTask(ctx, payload)
	resultPayload := manifestResultMap(result)
	status := "completed"
	if executeErr != nil {
		status = "failed"
		resultPayload["error"] = publicManifestError(executeErr)
	}
	r.finalCallback(context.WithoutCancel(ctx), task, status, resultPayload)
	r.metrics.markOutcome(metricScopeExecution, status)
}

func decodeManifestPayload(value map[string]any) (sohaapi.ManifestExecutionTaskPayload, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return sohaapi.ManifestExecutionTaskPayload{}, err
	}
	var payload sohaapi.ManifestExecutionTaskPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return sohaapi.ManifestExecutionTaskPayload{}, err
	}
	if strings.TrimSpace(payload.PackageID) == "" || payload.Generation < 1 || strings.TrimSpace(payload.IdempotencyKey) == "" {
		return sohaapi.ManifestExecutionTaskPayload{}, fmt.Errorf("required manifest task identity is missing")
	}
	return payload, nil
}

func manifestResultMap(value sohaapi.ManifestExecutionTaskResult) map[string]any {
	encoded, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func publicManifestError(err error) string {
	return "manifest execution failed"
}
