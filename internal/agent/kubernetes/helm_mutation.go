package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	helmreleasepkg "helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"

	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

const (
	defaultAgentHelmTimeoutSeconds     = 300
	maxAgentHelmTimeoutSeconds         = 3600
	maxAgentHelmInstallResourceSummary = 300
)

func (c *Client) InstallHelmChart(ctx context.Context, input domainresource.HelmChartInstallInput) (domainresource.HelmChartInstallResult, error) {
	input = normalizeAgentHelmChartInstallInput(input)
	values, err := parseAgentHelmInstallValues(input.ValuesYAML)
	if err != nil {
		return domainresource.HelmChartInstallResult{}, err
	}
	actionConfig, err := c.helmActionConfig(input.Namespace)
	if err != nil {
		return domainresource.HelmChartInstallResult{}, err
	}
	if existing, ok, err := existingAgentHelmInstallResultFromSDK(actionConfig, input); err != nil {
		return domainresource.HelmChartInstallResult{}, err
	} else if ok {
		return existing, nil
	}

	settings, err := newAgentHelmEnvSettings(input.Namespace)
	if err != nil {
		return domainresource.HelmChartInstallResult{}, err
	}
	installer := action.NewInstall(actionConfig)
	installer.ReleaseName = input.ReleaseName
	installer.Namespace = input.Namespace
	installer.CreateNamespace = input.CreateNamespace
	installer.Wait = input.Wait
	installer.WaitForJobs = input.Wait
	installer.Timeout = time.Duration(input.TimeoutSeconds) * time.Second
	installer.DependencyUpdate = true
	installer.ChartPathOptions.RepoURL = input.RepositoryURL
	installer.ChartPathOptions.Version = input.Version

	chartPath, err := installer.ChartPathOptions.LocateChart(input.ChartName, settings)
	if err != nil {
		return domainresource.HelmChartInstallResult{}, fmt.Errorf("locate helm chart: %w", err)
	}
	chart, err := loader.Load(chartPath)
	if err != nil {
		return domainresource.HelmChartInstallResult{}, fmt.Errorf("load helm chart: %w", err)
	}
	release, err := installer.RunWithContext(ctx, chart, values)
	if err != nil {
		if isAgentHelmReleaseNameInUseError(err) {
			if existing, ok, lookupErr := existingAgentHelmInstallResultFromSDK(actionConfig, input); lookupErr != nil {
				return domainresource.HelmChartInstallResult{}, lookupErr
			} else if ok {
				return existing, nil
			}
			return domainresource.HelmChartInstallResult{}, agentHelmReleaseNameUnavailableError(input.ReleaseName, input.Namespace, nil)
		}
		return domainresource.HelmChartInstallResult{}, fmt.Errorf("install helm chart: %w", err)
	}
	return mapAgentHelmChartInstallResult(release), nil
}

func (c *Client) UpdateHelmReleaseValues(ctx context.Context, namespace, name, content string) (domainresource.HelmValuesView, error) {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	content = normalizeAgentHelmValuesContent(content)
	values, err := parseAgentHelmInstallValues(content)
	if err != nil {
		return domainresource.HelmValuesView{}, err
	}
	actionConfig, err := c.helmActionConfig(namespace)
	if err != nil {
		return domainresource.HelmValuesView{}, err
	}
	current, err := action.NewGet(actionConfig).Run(name)
	if err != nil {
		return domainresource.HelmValuesView{}, fmt.Errorf("get helm release %s: %w", name, err)
	}
	if current == nil || current.Chart == nil {
		return domainresource.HelmValuesView{}, fmt.Errorf("helm release %s has no chart payload", name)
	}
	upgrader := action.NewUpgrade(actionConfig)
	upgrader.Namespace = namespace
	upgrader.ResetValues = true
	upgrader.Wait = true
	upgrader.WaitForJobs = true
	upgrader.Timeout = time.Duration(defaultAgentHelmTimeoutSeconds) * time.Second
	release, err := upgrader.RunWithContext(ctx, name, current.Chart, values)
	if err != nil {
		return domainresource.HelmValuesView{}, fmt.Errorf("update helm release values %s: %w", name, err)
	}
	if release == nil {
		return domainresource.HelmValuesView{}, fmt.Errorf("update helm release values %s returned no release", name)
	}
	return domainresource.HelmValuesView{
		Name:        strings.TrimSpace(release.Name),
		Namespace:   strings.TrimSpace(release.Namespace),
		Revision:    strconv.Itoa(release.Version),
		Content:     content,
		Original:    content,
		Editable:    true,
		DiffEnabled: true,
	}, nil
}

func (c *Client) DeleteHelmRelease(ctx context.Context, namespace, name string) error {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	actionConfig, err := c.helmActionConfig(namespace)
	if err != nil {
		return err
	}
	uninstaller := action.NewUninstall(actionConfig)
	uninstaller.Wait = true
	uninstaller.Timeout = time.Duration(defaultAgentHelmTimeoutSeconds) * time.Second
	if _, err := uninstaller.Run(name); err != nil {
		return fmt.Errorf("delete helm release %s: %w", name, err)
	}
	return nil
}

func (c *Client) helmActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)
	getter := agentHelmRESTClientGetter{restConfig: c.restConfig, namespace: namespace}
	if err := actionConfig.Init(getter, namespace, "secrets", func(string, ...interface{}) {}); err != nil {
		return nil, fmt.Errorf("initialize helm action: %w", err)
	}
	return actionConfig, nil
}

func normalizeAgentHelmChartInstallInput(input domainresource.HelmChartInstallInput) domainresource.HelmChartInstallInput {
	input.RepositoryName = strings.TrimSpace(input.RepositoryName)
	input.RepositoryURL = strings.TrimSpace(input.RepositoryURL)
	input.ChartName = strings.TrimSpace(input.ChartName)
	input.Version = strings.TrimSpace(input.Version)
	input.ReleaseName = strings.TrimSpace(input.ReleaseName)
	input.Namespace = strings.TrimSpace(input.Namespace)
	if input.TimeoutSeconds <= 0 {
		input.TimeoutSeconds = defaultAgentHelmTimeoutSeconds
	}
	if input.TimeoutSeconds > maxAgentHelmTimeoutSeconds {
		input.TimeoutSeconds = maxAgentHelmTimeoutSeconds
	}
	return input
}

func parseAgentHelmInstallValues(valuesYAML string) (map[string]interface{}, error) {
	values := map[string]interface{}{}
	if strings.TrimSpace(valuesYAML) == "" {
		return values, nil
	}
	if err := yaml.Unmarshal([]byte(valuesYAML), &values); err != nil {
		return nil, fmt.Errorf("invalid values yaml: %w", err)
	}
	if values == nil {
		return map[string]interface{}{}, nil
	}
	return values, nil
}

func normalizeAgentHelmValuesContent(content string) string {
	if strings.TrimSpace(content) == "" {
		return "{}\n"
	}
	return content
}

func newAgentHelmEnvSettings(namespace string) (*cli.EnvSettings, error) {
	root := filepath.Join(os.TempDir(), "soha-agent-helm")
	cacheDir := filepath.Join(root, "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("prepare helm cache: %w", err)
	}
	settings := cli.New()
	settings.SetNamespace(namespace)
	settings.RepositoryCache = cacheDir
	settings.RepositoryConfig = filepath.Join(root, "repositories.yaml")
	settings.RegistryConfig = filepath.Join(root, "registry.json")
	settings.PluginsDirectory = filepath.Join(root, "plugins")
	return settings, nil
}

func existingAgentHelmInstallResultFromSDK(actionConfig *action.Configuration, input domainresource.HelmChartInstallInput) (domainresource.HelmChartInstallResult, bool, error) {
	release, err := action.NewGet(actionConfig).Run(input.ReleaseName)
	if err != nil {
		if isAgentHelmReleaseNotFoundError(err) {
			return domainresource.HelmChartInstallResult{}, false, nil
		}
		return domainresource.HelmChartInstallResult{}, false, fmt.Errorf("inspect existing helm release: %w", err)
	}
	if agentHelmSDKReleaseSatisfiesInstall(release, input) {
		result := mapAgentHelmChartInstallResult(release)
		if strings.TrimSpace(result.Description) == "" {
			result.Description = "Release already deployed; install request already satisfied"
		}
		return result, true, nil
	}
	return domainresource.HelmChartInstallResult{}, false, agentHelmReleaseNameUnavailableError(input.ReleaseName, input.Namespace, release)
}

func agentHelmSDKReleaseSatisfiesInstall(release *helmreleasepkg.Release, input domainresource.HelmChartInstallInput) bool {
	if release == nil || release.Chart == nil || release.Chart.Metadata == nil {
		return false
	}
	status := ""
	if release.Info != nil {
		status = strings.TrimSpace(string(release.Info.Status))
	}
	if !strings.EqualFold(status, "deployed") {
		return false
	}
	metadata := release.Chart.Metadata
	chartName := strings.TrimSpace(metadata.Name)
	chartVersion := strings.TrimSpace(metadata.Version)
	return strings.EqualFold(chartName, strings.TrimSpace(input.ChartName)) && chartVersion == strings.TrimSpace(input.Version)
}

func isAgentHelmReleaseNameInUseError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "cannot re-use a name that is still in use")
}

func isAgentHelmReleaseNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "release: not found") || strings.Contains(lowered, "not found")
}

func agentHelmReleaseNameUnavailableError(releaseName, namespace string, release *helmreleasepkg.Release) error {
	status := ""
	revision := ""
	if release != nil {
		if release.Info != nil {
			status = strings.TrimSpace(string(release.Info.Status))
		}
		if release.Version > 0 {
			revision = strconv.Itoa(release.Version)
		}
	}
	parts := []string{
		fmt.Sprintf("releaseName %q in namespace %q is already used by Helm release history", strings.TrimSpace(releaseName), strings.TrimSpace(namespace)),
	}
	if status != "" {
		parts = append(parts, fmt.Sprintf("status %q", status))
	}
	if revision != "" {
		parts = append(parts, fmt.Sprintf("revision %s", revision))
	}
	return fmt.Errorf("%s; choose another release name or uninstall the existing release before installing again", strings.Join(parts, ", "))
}

func mapAgentHelmChartInstallResult(release *helmreleasepkg.Release) domainresource.HelmChartInstallResult {
	if release == nil {
		return domainresource.HelmChartInstallResult{}
	}
	result := domainresource.HelmChartInstallResult{
		Name:      strings.TrimSpace(release.Name),
		Namespace: strings.TrimSpace(release.Namespace),
		Revision:  strconv.Itoa(release.Version),
	}
	if release.Info != nil {
		result.Status = strings.TrimSpace(string(release.Info.Status))
		result.Description = strings.TrimSpace(release.Info.Description)
		result.Notes = strings.TrimSpace(release.Info.Notes)
	}
	if release.Chart != nil && release.Chart.Metadata != nil {
		result.ChartName = strings.TrimSpace(release.Chart.Metadata.Name)
		result.ChartVersion = strings.TrimSpace(release.Chart.Metadata.Version)
		result.AppVersion = strings.TrimSpace(release.Chart.Metadata.AppVersion)
		if result.ChartName != "" && result.ChartVersion != "" {
			result.Chart = result.ChartName + "-" + result.ChartVersion
		} else {
			result.Chart = result.ChartName
		}
	}
	result.Resources = mapAgentHelmInstallManifestResources(release.Manifest)
	return result
}

func mapAgentHelmInstallManifestResources(manifest string) []domainresource.HelmChartInstallResourceView {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return nil
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(manifest), 4096)
	resources := make([]domainresource.HelmChartInstallResourceView, 0)
	for len(resources) < maxAgentHelmInstallResourceSummary {
		var object unstructured.Unstructured
		if err := decoder.Decode(&object); err != nil {
			if err == io.EOF {
				break
			}
			break
		}
		if object.Object == nil {
			continue
		}
		name := strings.TrimSpace(object.GetName())
		kind := strings.TrimSpace(object.GetKind())
		if name == "" || kind == "" {
			continue
		}
		resources = append(resources, domainresource.HelmChartInstallResourceView{
			APIVersion: strings.TrimSpace(object.GetAPIVersion()),
			Kind:       kind,
			Namespace:  strings.TrimSpace(object.GetNamespace()),
			Name:       name,
		})
	}
	return resources
}

type agentHelmRESTClientGetter struct {
	restConfig *rest.Config
	namespace  string
}

func (g agentHelmRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	if g.restConfig == nil {
		return nil, fmt.Errorf("missing kubernetes rest config")
	}
	return rest.CopyConfig(g.restConfig), nil
}

func (g agentHelmRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := g.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(client), nil
}

func (g agentHelmRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}
	return restmapper.NewDiscoveryRESTMapper(groupResources), nil
}

func (g agentHelmRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	namespace := strings.TrimSpace(g.namespace)
	if namespace == "" {
		namespace = "default"
	}
	rawConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {Server: ""},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"context": {Cluster: "cluster", AuthInfo: "user", Namespace: namespace},
		},
		CurrentContext: "context",
	}
	return clientcmd.NewDefaultClientConfig(rawConfig, &clientcmd.ConfigOverrides{
		Context: clientcmdapi.Context{Namespace: namespace},
	})
}
