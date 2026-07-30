package runner

import (
	"errors"
	"testing"
)

func TestDecodeManifestPayloadRequiresStableIdentity(t *testing.T) {
	payload, err := decodeManifestPayload(map[string]any{
		"action": "apply", "packageId": "package-1", "generation": float64(4),
		"idempotencyKey": "manifest:deployment-1:4:apply",
	})
	if err != nil {
		t.Fatalf("decodeManifestPayload() error = %v", err)
	}
	if payload.PackageID != "package-1" || payload.Generation != 4 {
		t.Fatalf("payload = %#v", payload)
	}
	if _, err := decodeManifestPayload(map[string]any{"action": "apply", "generation": float64(1)}); err == nil {
		t.Fatal("decodeManifestPayload() error = nil, want missing identity rejection")
	}
}

func TestPublicManifestErrorDoesNotExposeProviderDetails(t *testing.T) {
	secret := "token=super-secret kubeconfig=/private/config"
	if got := publicManifestError(errors.New(secret)); got != "manifest execution failed" {
		t.Fatalf("publicManifestError() = %q, want generic public error", got)
	}
}
