package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageDigestFromWorkspaceArtifacts(t *testing.T) {
	workspace := t.TempDir()
	want := "sha256:" + strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(workspace, ".soha-image-digest"), []byte("registry.example/api@"+want+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := imageDigestFromWorkspaceArtifacts(workspace, []map[string]any{{
		"path": ".soha-image-digest", "status": "completed",
	}})
	if got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
}

func TestImageDigestFromWorkspaceArtifactsRejectsInvalidEvidence(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".soha-image-digest"), []byte("sha256:short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := imageDigestFromWorkspaceArtifacts(workspace, []map[string]any{{"path": ".soha-image-digest", "status": "completed"}}); got != "" {
		t.Fatalf("invalid digest accepted: %q", got)
	}
	if got := imageDigestFromWorkspaceArtifacts(workspace, []map[string]any{{"path": "../.soha-image-digest", "status": "completed"}}); got != "" {
		t.Fatalf("escaped artifact accepted: %q", got)
	}
}

func TestBuildFailureStages(t *testing.T) {
	if got := commandPipelineStage("docker push registry.example/api:v1"); got != "push" {
		t.Fatalf("push stage = %q", got)
	}
	if got := commandPipelineStage("docker image inspect app:v1 > .soha-image-digest"); got != "publish" {
		t.Fatalf("publish stage = %q", got)
	}
	if got := commandPipelineStage("docker build -t app:v1 ."); got != "build" {
		t.Fatalf("build stage = %q", got)
	}
	if got := workspaceFailureStage(os.ErrNotExist); got != "prepare" {
		t.Fatalf("prepare stage = %q", got)
	}
}
