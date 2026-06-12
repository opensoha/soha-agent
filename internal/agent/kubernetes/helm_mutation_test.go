package kubernetes

import (
	"strings"
	"testing"

	helmchart "helm.sh/helm/v3/pkg/chart"
	helmreleasepkg "helm.sh/helm/v3/pkg/release"

	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

func TestNormalizeAgentHelmChartInstallInputBoundsTimeout(t *testing.T) {
	input := normalizeAgentHelmChartInstallInput(domainresource.HelmChartInstallInput{
		RepositoryURL:  " https://charts.example ",
		ChartName:      " nginx ",
		Version:        " 1.2.3 ",
		ReleaseName:    " edge ",
		Namespace:      " platform ",
		TimeoutSeconds: maxAgentHelmTimeoutSeconds + 100,
	})
	if input.RepositoryURL != "https://charts.example" || input.ChartName != "nginx" || input.ReleaseName != "edge" || input.Namespace != "platform" {
		t.Fatalf("normalized input = %#v", input)
	}
	if input.TimeoutSeconds != maxAgentHelmTimeoutSeconds {
		t.Fatalf("timeout = %d, want max %d", input.TimeoutSeconds, maxAgentHelmTimeoutSeconds)
	}
}

func TestParseAgentHelmInstallValuesRejectsInvalidYAML(t *testing.T) {
	if _, err := parseAgentHelmInstallValues("replicaCount: ["); err == nil {
		t.Fatal("parseAgentHelmInstallValues() succeeded, want invalid yaml error")
	}
	values, err := parseAgentHelmInstallValues("replicaCount: 2\n")
	if err != nil {
		t.Fatalf("parseAgentHelmInstallValues() error = %v", err)
	}
	if values["replicaCount"] != float64(2) && values["replicaCount"] != 2 {
		t.Fatalf("replicaCount = %#v, want 2", values["replicaCount"])
	}
}

func TestMapAgentHelmChartInstallResultIncludesManifestResources(t *testing.T) {
	release := &helmreleasepkg.Release{
		Name:      "edge",
		Namespace: "platform",
		Version:   3,
		Info: &helmreleasepkg.Info{
			Status:      helmreleasepkg.StatusDeployed,
			Description: "install complete",
			Notes:       "ready",
		},
		Chart: &helmchart.Chart{Metadata: &helmchart.Metadata{
			Name:       "nginx",
			Version:    "1.2.3",
			AppVersion: "1.25.0",
		}},
		Manifest: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: edge
  namespace: platform
---
apiVersion: v1
kind: Service
metadata:
  name: edge
  namespace: platform
`,
	}

	result := mapAgentHelmChartInstallResult(release)
	if result.Name != "edge" || result.Revision != "3" || result.Chart != "nginx-1.2.3" || result.Status != "deployed" {
		t.Fatalf("result = %#v, want mapped release", result)
	}
	if len(result.Resources) != 2 || result.Resources[0].Kind != "Deployment" || result.Resources[1].Kind != "Service" {
		t.Fatalf("resources = %#v, want deployment and service", result.Resources)
	}
}

func TestAgentHelmSDKReleaseSatisfiesInstall(t *testing.T) {
	release := &helmreleasepkg.Release{
		Version: 1,
		Info:    &helmreleasepkg.Info{Status: helmreleasepkg.StatusDeployed},
		Chart: &helmchart.Chart{Metadata: &helmchart.Metadata{
			Name:    "nginx",
			Version: "1.2.3",
		}},
	}
	input := domainresource.HelmChartInstallInput{ChartName: "NGINX", Version: "1.2.3"}
	if !agentHelmSDKReleaseSatisfiesInstall(release, input) {
		t.Fatal("agentHelmSDKReleaseSatisfiesInstall() = false, want true")
	}
	release.Info.Status = helmreleasepkg.StatusFailed
	if agentHelmSDKReleaseSatisfiesInstall(release, input) {
		t.Fatal("failed release satisfied install")
	}
}

func TestAgentHelmReleaseNameUnavailableErrorIncludesStatusAndRevision(t *testing.T) {
	err := agentHelmReleaseNameUnavailableError("edge", "platform", &helmreleasepkg.Release{
		Version: 4,
		Info:    &helmreleasepkg.Info{Status: helmreleasepkg.StatusFailed},
	})
	if err == nil || !strings.Contains(err.Error(), "failed") || !strings.Contains(err.Error(), "revision 4") {
		t.Fatalf("error = %v, want status and revision", err)
	}
}
