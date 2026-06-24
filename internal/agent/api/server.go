package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/opensoha/soha-agent/internal/agent/buildinfo"
	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	runnerpkg "github.com/opensoha/soha-agent/internal/agent/runner"
	apiMiddleware "github.com/opensoha/soha-agent/internal/api/middleware"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
	"go.uber.org/zap"
)

type Server struct {
	httpServer *http.Server
}

type RuntimeTaskController interface {
	ListActiveTasks() []runnerpkg.ActiveTask
	GetActiveTask(string) (runnerpkg.ActiveTask, bool)
	CancelActiveTask(string, string) bool
}

type RuntimeMetricsController interface {
	MetricsSnapshot() runnerpkg.MetricsSnapshot
}

type restartDeploymentRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type scaleDeploymentRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int32  `json:"replicas"`
}

type restartStatefulSetRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type scaleStatefulSetRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int32  `json:"replicas"`
}

type restartDaemonSetRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type updateDeploymentImageRequest struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	ContainerName string `json:"containerName,omitempty"`
	Image         string `json:"image"`
}

type execPodRequest struct {
	Command        string `json:"command"`
	Container      string `json:"container,omitempty"`
	TimeoutSeconds int64  `json:"timeoutSeconds,omitempty"`
}

type rollbackDeploymentRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Revision  string `json:"revision"`
}

type cancelRuntimeTaskRequest struct {
	Reason string `json:"reason"`
}

type diagnosticsView struct {
	Build        buildinfo.Info              `json:"build"`
	App          diagnosticsAppView          `json:"app"`
	HTTP         diagnosticsHTTPView         `json:"http"`
	Security     diagnosticsSecurityView     `json:"security"`
	Kubernetes   diagnosticsKubernetesView   `json:"kubernetes"`
	Capabilities diagnosticsCapabilitiesView `json:"capabilities"`
	ControlPlane diagnosticsControlPlaneView `json:"controlPlane"`
	Runtime      diagnosticsRuntimeView      `json:"runtime"`
	Metrics      *runnerpkg.MetricsSnapshot  `json:"metrics,omitempty"`
}

type diagnosticsAppView struct {
	Name string `json:"name"`
	Env  string `json:"env"`
}

type diagnosticsHTTPView struct {
	AddrConfigured      bool   `json:"addrConfigured"`
	BasePath            string `json:"basePath"`
	AllowedOriginsCount int    `json:"allowedOriginsCount"`
}

type diagnosticsSecurityView struct {
	AllowedActionsCount int  `json:"allowedActionsCount"`
	AuditFileConfigured bool `json:"auditFileConfigured"`
}

type diagnosticsKubernetesView struct {
	Enabled              bool   `json:"enabled"`
	ClientAvailable      bool   `json:"clientAvailable"`
	ID                   string `json:"id,omitempty"`
	Name                 string `json:"name,omitempty"`
	Region               string `json:"region,omitempty"`
	Environment          string `json:"environment,omitempty"`
	ContextConfigured    bool   `json:"contextConfigured"`
	KubeconfigConfigured bool   `json:"kubeconfigConfigured"`
	KubeconfigDataLoaded bool   `json:"kubeconfigDataLoaded"`
	LabelsCount          int    `json:"labelsCount"`
}

type diagnosticsCapabilitiesView struct {
	Mode          string                      `json:"mode"`
	Status        string                      `json:"status"`
	RequiredKeys  []string                    `json:"requiredKeys"`
	AvailableKeys []string                    `json:"availableKeys,omitempty"`
	DegradedKeys  []string                    `json:"degradedKeys,omitempty"`
	Items         []diagnosticsCapabilityItem `json:"items"`
	Message       string                      `json:"message,omitempty"`
}

type diagnosticsCapabilityItem struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type diagnosticsControlPlaneView struct {
	Enabled                   bool     `json:"enabled"`
	BaseURLConfigured         bool     `json:"baseUrlConfigured"`
	AgentID                   string   `json:"agentId,omitempty"`
	RuntimeEndpointConfigured bool     `json:"runtimeEndpointConfigured"`
	MaxConcurrency            int      `json:"maxConcurrency"`
	PollInterval              string   `json:"pollInterval,omitempty"`
	DefaultTimeout            string   `json:"defaultTimeout,omitempty"`
	ProviderKinds             []string `json:"providerKinds,omitempty"`
	WorkspaceRootConfigured   bool     `json:"workspaceRootConfigured"`
	CallbackRetry             struct {
		MaxAttempts int    `json:"maxAttempts"`
		Backoff     string `json:"backoff,omitempty"`
	} `json:"callbackRetry"`
	Docker       diagnosticsDockerRunnerView `json:"docker"`
	AgentRuntime diagnosticsAgentRuntimeView `json:"agentRuntime"`
}

type diagnosticsDockerRunnerView struct {
	Enabled               bool     `json:"enabled"`
	WorkerIDConfigured    bool     `json:"workerIdConfigured"`
	HostCount             int      `json:"hostCount"`
	OperationKinds        []string `json:"operationKinds,omitempty"`
	ComposeRootConfigured bool     `json:"composeRootConfigured"`
	PollInterval          string   `json:"pollInterval,omitempty"`
}

type diagnosticsAgentRuntimeView struct {
	Enabled                 bool     `json:"enabled"`
	WorkerIDConfigured      bool     `json:"workerIdConfigured"`
	ProviderIDs             []string `json:"providerIds,omitempty"`
	ProviderKinds           []string `json:"providerKinds,omitempty"`
	ProviderCount           int      `json:"providerCount"`
	HermesCommandConfigured bool     `json:"hermesCommandConfigured"`
	WorkspaceRootConfigured bool     `json:"workspaceRootConfigured"`
	PollInterval            string   `json:"pollInterval,omitempty"`
}

type diagnosticsRuntimeView struct {
	ControllerAvailable bool `json:"controllerAvailable"`
	ActiveTasks         int  `json:"activeTasks"`
	MetricsAvailable    bool `json:"metricsAvailable"`
}

func New(cfg cfgpkg.Config, logger *zap.Logger, client *k8sagent.Client, runtime RuntimeTaskController) *Server {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(apiMiddleware.RequestID())
	auditSink := newActionAuditSink(cfg.Audit, logger)
	actions := newActionPolicy(cfg.Security, logger, auditSink)

	router.GET("/healthz", func(c *gin.Context) {
		apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET(fmt.Sprintf("%s/healthz", cfg.HTTP.BasePath), func(c *gin.Context) {
		apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	buildInfoHandler := func(c *gin.Context) {
		apiresponse.Item(c, http.StatusOK, buildinfo.Current())
	}
	router.GET("/version", buildInfoHandler)
	router.GET(fmt.Sprintf("%s/version", cfg.HTTP.BasePath), buildInfoHandler)
	router.GET(fmt.Sprintf("%s/build-info", cfg.HTTP.BasePath), buildInfoHandler)
	router.GET(fmt.Sprintf("%s/diagnostics", cfg.HTTP.BasePath), authAnyMiddleware(cfg.Auth.BearerToken, cfg.ControlPlane.BearerToken), func(c *gin.Context) {
		apiresponse.Item(c, http.StatusOK, buildDiagnosticsView(cfg, client != nil, runtime))
	})

	if client != nil {
		platform := router.Group(fmt.Sprintf("%s/platform", cfg.HTTP.BasePath))
		platform.Use(authMiddleware(cfg.Auth.BearerToken))
		{
			registerResourceYAMLRoutes(platform, client, actions)
			registerCustomResourceRoutes(platform, client, actions)
			registerPortForwardRoutes(platform, newPortForwardRegistry(cfg.Kubernetes, kubernetesPortForwardStarter(client)), actions)
			registerHelmRoutes(platform, client, actions)
			registerPodStreamRoutes(platform, client)
			registerPodTerminalRoutes(platform, client, actions)
			platform.GET("/summary", func(c *gin.Context) {
				apiresponse.Item(c, http.StatusOK, client.Summary(c.Request.Context()))
			})
			platform.GET("/namespaces", func(c *gin.Context) {
				items, err := client.ListNamespaces(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/infrastructure/nodes", func(c *gin.Context) {
				items, err := client.ListNodes(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/infrastructure/nodes/:name/detail", func(c *gin.Context) {
				item, err := client.GetNodeDetail(c.Request.Context(), c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/pods", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				items, err := client.ListPods(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/pods/:name/detail", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				item, err := client.GetPodDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/pods/:name/logs", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				tailLines := int64(parseLimit(c.Query("tailLines"), 200))
				sinceSeconds := int64(parseLimit(c.Query("sinceSeconds"), 0))
				previous := c.Query("previous") == "true"
				item, err := client.GetPodLogs(c.Request.Context(), namespace, c.Param("name"), c.Query("container"), tailLines, sinceSeconds, previous)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/pods/:name/yaml", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				item, err := client.GetPodYAML(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.POST("/workloads/pods/:name/exec", actions.Require(actionPlatformPodsExec), func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				var req execPodRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Command == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "command is required")
					return
				}
				item, err := client.ExecPod(c.Request.Context(), namespace, c.Param("name"), req.Container, req.Command, req.TimeoutSeconds)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/deployments", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				items, err := client.ListDeployments(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/deployments/:name/detail", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				item, err := client.GetDeploymentDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/deployments/:name/yaml", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				item, err := client.GetDeploymentYAML(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/deployments/:name/rollout-status", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				item, err := client.GetDeploymentRolloutStatus(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/deployments/:name/rollouts", func(c *gin.Context) {
				namespace := c.DefaultQuery("namespace", "default")
				items, err := client.ListDeploymentRolloutHistory(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/statefulsets", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListStatefulSets(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/statefulsets/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetStatefulSetDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/statefulsets/:name/yaml", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetStatefulSetYAML(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/daemonsets", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListDaemonSets(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/daemonsets/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetDaemonSetDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/daemonsets/:name/yaml", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetDaemonSetYAML(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/jobs", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListJobs(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/jobs/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetJobDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/jobs/:name/yaml", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetJobYAML(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/cronjobs", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListCronJobs(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/replicasets", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListReplicaSets(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/cronjobs/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetCronJobDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/workloads/cronjobs/:name/yaml", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetCronJobYAML(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/configuration/configmaps", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListConfigMaps(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/secrets", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListSecrets(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/hpas", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListHorizontalPodAutoscalers(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/poddisruptionbudgets", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListPodDisruptionBudgets(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/serviceaccounts", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListServiceAccounts(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/serviceaccounts/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetServiceAccountDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/access-control/roles", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListRoles(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/roles/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetRoleDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/access-control/rolebindings", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListRoleBindings(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/rolebindings/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetRoleBindingDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/network/services", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListServices(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/ingresses", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListIngresses(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/endpointslices", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListEndpointSlices(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/networkpolicies", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListNetworkPolicies(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/gatewayclasses", func(c *gin.Context) {
				items, err := client.ListGatewayClasses(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/gateways", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListGateways(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/httproutes", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListHTTPRoutes(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/backendtlspolicies", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListBackendTLSPolicies(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/grpcroutes", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListGRPCRoutes(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/referencegrants", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListReferenceGrants(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/storage/persistentvolumeclaims", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListPersistentVolumeClaims(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/storage/persistentvolumes", func(c *gin.Context) {
				items, err := client.ListPersistentVolumes(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/storage/storageclasses", func(c *gin.Context) {
				items, err := client.ListStorageClasses(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/network/ingressclasses", func(c *gin.Context) {
				items, err := client.ListIngressClasses(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/priorityclasses", func(c *gin.Context) {
				items, err := client.ListPriorityClasses(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/runtimeclasses", func(c *gin.Context) {
				items, err := client.ListRuntimeClasses(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/clusterroles", func(c *gin.Context) {
				items, err := client.ListClusterRoles(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/clusterroles/:name/detail", func(c *gin.Context) {
				item, err := client.GetClusterRoleDetail(c.Request.Context(), c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/access-control/clusterrolebindings", func(c *gin.Context) {
				items, err := client.ListClusterRoleBindings(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/access-control/clusterrolebindings/:name/detail", func(c *gin.Context) {
				item, err := client.GetClusterRoleBindingDetail(c.Request.Context(), c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/configuration/mutatingwebhookconfigurations", func(c *gin.Context) {
				items, err := client.ListMutatingWebhookConfigurations(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/validatingwebhookconfigurations", func(c *gin.Context) {
				items, err := client.ListValidatingWebhookConfigurations(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/resourcequotas", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListResourceQuotas(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/limitranges", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListLimitRanges(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/configuration/leases", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListLeases(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/workloads/replicationcontrollers", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListReplicationControllers(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/extensions/crds", func(c *gin.Context) {
				items, err := client.ListCRDs(c.Request.Context())
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/helm/releases", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListHelmReleases(c.Request.Context(), namespace)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/helm/releases/:name/detail", func(c *gin.Context) {
				namespace := c.Query("namespace")
				item, err := client.GetHelmReleaseDetail(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/helm/releases/:name/history", func(c *gin.Context) {
				namespace := c.Query("namespace")
				items, err := client.ListHelmReleaseHistory(c.Request.Context(), namespace, c.Param("name"))
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.GET("/helm/releases/:name/values", func(c *gin.Context) {
				namespace := c.Query("namespace")
				revision := c.Query("revision")
				item, err := client.GetHelmReleaseValues(c.Request.Context(), namespace, c.Param("name"), revision)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			platform.GET("/events", func(c *gin.Context) {
				namespace := c.Query("namespace")
				limit := parseLimit(c.Query("limit"), 20)
				items, err := client.ListClusterEvents(c.Request.Context(), namespace, limit)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Items(c, http.StatusOK, items)
			})
			platform.POST("/actions/deployments/restart", actions.Require(actionPlatformDeploymentRestart), func(c *gin.Context) {
				var req restartDeploymentRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace and name are required")
					return
				}
				if err := client.RestartDeployment(c.Request.Context(), req.Namespace, req.Name); err != nil {
					writeError(c, err)
					return
				}
				apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
			})
			platform.POST("/actions/deployments/scale", actions.Require(actionPlatformDeploymentScale), func(c *gin.Context) {
				var req scaleDeploymentRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace, name, and replicas are required")
					return
				}
				if req.Replicas < 0 {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "replicas must be greater than or equal to zero")
					return
				}
				if err := client.ScaleDeployment(c.Request.Context(), req.Namespace, req.Name, req.Replicas); err != nil {
					writeError(c, err)
					return
				}
				apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
			})
			platform.POST("/actions/statefulsets/restart", actions.Require(actionPlatformStatefulSetRestart), func(c *gin.Context) {
				var req restartStatefulSetRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace and name are required")
					return
				}
				if err := client.RestartStatefulSet(c.Request.Context(), req.Namespace, req.Name); err != nil {
					writeError(c, err)
					return
				}
				apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
			})
			platform.POST("/actions/statefulsets/scale", actions.Require(actionPlatformStatefulSetScale), func(c *gin.Context) {
				var req scaleStatefulSetRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace, name, and replicas are required")
					return
				}
				if req.Replicas < 0 {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "replicas must be greater than or equal to zero")
					return
				}
				if err := client.ScaleStatefulSet(c.Request.Context(), req.Namespace, req.Name, req.Replicas); err != nil {
					writeError(c, err)
					return
				}
				apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
			})
			platform.POST("/actions/daemonsets/restart", actions.Require(actionPlatformDaemonSetRestart), func(c *gin.Context) {
				var req restartDaemonSetRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace and name are required")
					return
				}
				if err := client.RestartDaemonSet(c.Request.Context(), req.Namespace, req.Name); err != nil {
					writeError(c, err)
					return
				}
				apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
			})
			platform.POST("/actions/deployments/image", actions.Require(actionPlatformDeploymentImage), func(c *gin.Context) {
				var req updateDeploymentImageRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" || req.Image == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace, name, and image are required")
					return
				}
				containerName, previousImage, err := client.UpdateDeploymentImage(c.Request.Context(), req.Namespace, req.Name, req.ContainerName, req.Image)
				if err != nil {
					writeError(c, err)
					return
				}
				apiresponse.Item(c, http.StatusOK, gin.H{
					"containerName": containerName,
					"previousImage": previousImage,
				})
			})
			platform.POST("/actions/deployments/rollback", actions.Require(actionPlatformDeploymentRollback), func(c *gin.Context) {
				var req rollbackDeploymentRequest
				if err := c.ShouldBindJSON(&req); err != nil || req.Namespace == "" || req.Name == "" || req.Revision == "" {
					apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace, name, and revision are required")
					return
				}
				if err := client.RollbackDeployment(c.Request.Context(), req.Namespace, req.Name, req.Revision); err != nil {
					writeError(c, err)
					return
				}
				apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
			})
		}
	}

	if runtime != nil {
		runtimeGroup := router.Group(fmt.Sprintf("%s/runtime", cfg.HTTP.BasePath))
		runtimeGroup.Use(authAnyMiddleware(cfg.Auth.BearerToken, cfg.ControlPlane.BearerToken))
		{
			runtimeGroup.GET("/execution-tasks", func(c *gin.Context) {
				apiresponse.Items(c, http.StatusOK, runtime.ListActiveTasks())
			})
			runtimeGroup.GET("/execution-tasks/:taskID", func(c *gin.Context) {
				item, ok := runtime.GetActiveTask(c.Param("taskID"))
				if !ok {
					apiresponse.Error(c, http.StatusNotFound, "not_found", "runtime execution task not found")
					return
				}
				apiresponse.Item(c, http.StatusOK, item)
			})
			runtimeGroup.POST("/execution-tasks/:taskID/cancel", actions.Require(actionRuntimeExecutionTaskCancel), func(c *gin.Context) {
				var req cancelRuntimeTaskRequest
				_ = c.ShouldBindJSON(&req)
				if !runtime.CancelActiveTask(c.Param("taskID"), req.Reason) {
					apiresponse.Error(c, http.StatusNotFound, "not_found", "runtime execution task not found")
					return
				}
				apiresponse.JSON(c, http.StatusAccepted, gin.H{"status": "canceling"})
			})
			if metrics, ok := runtime.(RuntimeMetricsController); ok {
				runtimeGroup.GET("/metrics", func(c *gin.Context) {
					apiresponse.Item(c, http.StatusOK, metrics.MetricsSnapshot())
				})
			}
		}
	}

	registerDockerRuntimeRoutes(router, cfg, logger, actions)

	logger.Info("agent server configured",
		zap.String("addr", cfg.HTTP.Addr),
		zap.String("base_path", cfg.HTTP.BasePath),
		zap.String("cluster_id", cfg.Kubernetes.ID),
		zap.String("version", buildinfo.Current().Version),
		zap.String("commit", buildinfo.Current().Commit),
	)

	return &Server{httpServer: &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
	}}
}

func buildDiagnosticsView(cfg cfgpkg.Config, kubernetesClientAvailable bool, runtime RuntimeTaskController) diagnosticsView {
	view := diagnosticsView{
		Build: buildinfo.Current(),
		App: diagnosticsAppView{
			Name: cfg.App.Name,
			Env:  cfg.App.Env,
		},
		HTTP: diagnosticsHTTPView{
			AddrConfigured:      strings.TrimSpace(cfg.HTTP.Addr) != "",
			BasePath:            cfg.HTTP.BasePath,
			AllowedOriginsCount: len(cfg.HTTP.AllowedOrigins),
		},
		Security: diagnosticsSecurityView{
			AllowedActionsCount: len(cfg.Security.AllowedActions),
			AuditFileConfigured: strings.TrimSpace(cfg.Audit.FilePath) != "",
		},
		Kubernetes: diagnosticsKubernetesView{
			Enabled:              cfg.Kubernetes.Enabled,
			ClientAvailable:      kubernetesClientAvailable,
			ID:                   cfg.Kubernetes.ID,
			Name:                 cfg.Kubernetes.Name,
			Region:               cfg.Kubernetes.Region,
			Environment:          cfg.Kubernetes.Environment,
			ContextConfigured:    strings.TrimSpace(cfg.Kubernetes.Context) != "",
			KubeconfigConfigured: strings.TrimSpace(cfg.Kubernetes.Kubeconfig) != "",
			KubeconfigDataLoaded: strings.TrimSpace(cfg.Kubernetes.KubeconfigData) != "",
			LabelsCount:          len(cfg.Kubernetes.Labels),
		},
		Capabilities: buildDiagnosticsCapabilitiesView(cfg, kubernetesClientAvailable),
		ControlPlane: diagnosticsControlPlaneView{
			Enabled:                   cfg.ControlPlane.Enabled,
			BaseURLConfigured:         strings.TrimSpace(cfg.ControlPlane.BaseURL) != "",
			AgentID:                   cfg.ControlPlane.AgentID,
			RuntimeEndpointConfigured: strings.TrimSpace(cfg.ControlPlane.RuntimeEndpoint) != "",
			MaxConcurrency:            cfg.ControlPlane.MaxConcurrency,
			PollInterval:              durationString(cfg.ControlPlane.PollInterval),
			DefaultTimeout:            durationString(cfg.ControlPlane.DefaultTimeout),
			ProviderKinds:             append([]string(nil), cfg.ControlPlane.ProviderKinds...),
			WorkspaceRootConfigured:   strings.TrimSpace(cfg.ControlPlane.WorkspaceRoot) != "",
			Docker: diagnosticsDockerRunnerView{
				Enabled:               cfg.ControlPlane.Docker.Enabled,
				WorkerIDConfigured:    strings.TrimSpace(cfg.ControlPlane.Docker.WorkerID) != "",
				HostCount:             len(cfg.ControlPlane.Docker.HostIDs),
				OperationKinds:        append([]string(nil), cfg.ControlPlane.Docker.OperationKinds...),
				ComposeRootConfigured: strings.TrimSpace(cfg.ControlPlane.Docker.ComposeRoot) != "",
				PollInterval:          durationString(cfg.ControlPlane.Docker.PollInterval),
			},
			AgentRuntime: diagnosticsAgentRuntimeView{
				Enabled:                 cfg.ControlPlane.AgentRuntime.Enabled,
				WorkerIDConfigured:      strings.TrimSpace(cfg.ControlPlane.AgentRuntime.WorkerID) != "",
				ProviderIDs:             append([]string(nil), cfg.ControlPlane.AgentRuntime.ProviderIDs...),
				ProviderKinds:           append([]string(nil), cfg.ControlPlane.AgentRuntime.ProviderKinds...),
				ProviderCount:           len(cfg.ControlPlane.AgentRuntime.Providers),
				HermesCommandConfigured: strings.TrimSpace(cfg.ControlPlane.AgentRuntime.HermesCommand) != "",
				WorkspaceRootConfigured: strings.TrimSpace(cfg.ControlPlane.AgentRuntime.WorkspaceRoot) != "",
				PollInterval:            durationString(cfg.ControlPlane.AgentRuntime.PollInterval),
			},
		},
		Runtime: diagnosticsRuntimeView{
			ControllerAvailable: runtime != nil,
		},
	}
	view.ControlPlane.CallbackRetry.MaxAttempts = cfg.ControlPlane.CallbackRetry.MaxAttempts
	view.ControlPlane.CallbackRetry.Backoff = durationString(cfg.ControlPlane.CallbackRetry.Backoff)
	if runtime != nil {
		view.Runtime.ActiveTasks = len(runtime.ListActiveTasks())
	}
	if metrics, ok := runtime.(RuntimeMetricsController); ok {
		snapshot := metrics.MetricsSnapshot()
		view.Runtime.ActiveTasks = snapshot.ActiveTasks
		view.Runtime.MetricsAvailable = true
		view.Metrics = &snapshot
	}
	return view
}

var managedAgentDiagnosticCapabilityKeys = []string{
	"cluster.inventory",
	"workload.read",
	"network.inventory",
	"port.forward",
	"pod.logs",
	"pod.exec",
	"workload.mutations",
	"helm.releases",
}

func buildDiagnosticsCapabilitiesView(cfg cfgpkg.Config, kubernetesClientAvailable bool) diagnosticsCapabilitiesView {
	items := make([]diagnosticsCapabilityItem, 0, len(managedAgentDiagnosticCapabilityKeys))
	if !cfg.Kubernetes.Enabled {
		for _, key := range managedAgentDiagnosticCapabilityKeys {
			items = append(items, diagnosticsCapabilityItem{Key: key, Status: "unsupported", Reason: "kubernetes runtime is disabled"})
		}
		return diagnosticsCapabilitiesView{
			Mode:         "agent",
			Status:       "degraded",
			RequiredKeys: append([]string(nil), managedAgentDiagnosticCapabilityKeys...),
			DegradedKeys: append([]string(nil), managedAgentDiagnosticCapabilityKeys...),
			Items:        items,
			Message:      "Managed-agent Kubernetes capabilities are unavailable because Kubernetes runtime is disabled.",
		}
	}
	if !kubernetesClientAvailable {
		for _, key := range managedAgentDiagnosticCapabilityKeys {
			items = append(items, diagnosticsCapabilityItem{Key: key, Status: "unsupported", Reason: "kubernetes client is not available"})
		}
		return diagnosticsCapabilitiesView{
			Mode:         "agent",
			Status:       "degraded",
			RequiredKeys: append([]string(nil), managedAgentDiagnosticCapabilityKeys...),
			DegradedKeys: append([]string(nil), managedAgentDiagnosticCapabilityKeys...),
			Items:        items,
			Message:      "Managed-agent Kubernetes capabilities are unavailable because Kubernetes client initialization failed or was not configured.",
		}
	}

	items = append(items,
		diagnosticsCapabilityItem{Key: "cluster.inventory", Status: "available"},
		diagnosticsCapabilityItem{Key: "workload.read", Status: "available"},
		diagnosticsCapabilityItem{Key: "network.inventory", Status: "available"},
		guardedCapabilityItem("port.forward",
			cfg.Security.AllowedActions,
			"port-forward actions are not fully allowlisted",
			actionPlatformPortForwardsCreate,
			actionPlatformPortForwardsTunnel,
			actionPlatformPortForwardsDelete,
		),
		diagnosticsCapabilityItem{Key: "pod.logs", Status: "available"},
		guardedCapabilityItem("pod.exec",
			cfg.Security.AllowedActions,
			"pod exec action is not allowlisted",
			actionPlatformPodsExec,
		),
		guardedCapabilityItem("workload.mutations",
			cfg.Security.AllowedActions,
			"workload mutation actions are not fully allowlisted",
			actionPlatformDeploymentRestart,
			actionPlatformDeploymentScale,
			actionPlatformDeploymentRollback,
			actionPlatformStatefulSetRestart,
			actionPlatformStatefulSetScale,
			actionPlatformDaemonSetRestart,
		),
		guardedCapabilityItem("helm.releases",
			cfg.Security.AllowedActions,
			"Helm release mutation actions are not fully allowlisted",
			actionPlatformHelmReleaseInstall,
			actionPlatformHelmReleaseValuesUpdate,
			actionPlatformHelmReleaseDelete,
		),
	)

	view := diagnosticsCapabilitiesView{
		Mode:         "agent",
		Status:       "available",
		RequiredKeys: append([]string(nil), managedAgentDiagnosticCapabilityKeys...),
		Items:        items,
		Message:      "All managed-agent required capabilities are available.",
	}
	for _, item := range items {
		if item.Status == "available" {
			view.AvailableKeys = append(view.AvailableKeys, item.Key)
			continue
		}
		view.DegradedKeys = append(view.DegradedKeys, item.Key)
	}
	if len(view.DegradedKeys) > 0 {
		view.Status = "degraded"
		view.Message = "One or more managed-agent capabilities are partial or unsupported by current Agent configuration."
	}
	return view
}

func guardedCapabilityItem(key string, allowedActions []string, reason string, actions ...string) diagnosticsCapabilityItem {
	for _, action := range actions {
		if !securityActionAllowed(allowedActions, action) {
			return diagnosticsCapabilityItem{Key: key, Status: "partial", Reason: reason}
		}
	}
	return diagnosticsCapabilityItem{Key: key, Status: "available"}
}

func securityActionAllowed(allowedActions []string, action string) bool {
	normalized := normalizeAction(action)
	if normalized == "" {
		return false
	}
	for _, allowed := range allowedActions {
		switch normalizeAction(allowed) {
		case "*", normalized:
			return true
		}
	}
	return false
}

func durationString(value time.Duration) string {
	if value <= 0 {
		return ""
	}
	return value.String()
}

func (s *Server) Run() error {
	err := s.httpServer.ListenAndServe()
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func authMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}
		if c.GetHeader("Authorization") != fmt.Sprintf("Bearer %s", token) {
			apiresponse.Error(c, http.StatusUnauthorized, "unauthorized", "invalid agent token")
			c.Abort()
			return
		}
		c.Next()
	}
}

func authAnyMiddleware(tokens ...string) gin.HandlerFunc {
	allowed := allowedAuthTokens(tokens...)
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.Next()
			return
		}
		if requestHasAnyBearerToken(c, allowed) {
			c.Next()
			return
		}
		apiresponse.Error(c, http.StatusUnauthorized, "unauthorized", "invalid agent token")
		c.Abort()
	}
}

func authRequiredAnyMiddleware(tokens ...string) gin.HandlerFunc {
	allowed := allowedAuthTokens(tokens...)
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			apiresponse.Error(c, http.StatusUnauthorized, "unauthorized", "agent token is required")
			c.Abort()
			return
		}
		if requestHasAnyBearerToken(c, allowed) {
			c.Next()
			return
		}
		apiresponse.Error(c, http.StatusUnauthorized, "unauthorized", "invalid agent token")
		c.Abort()
	}
}

func allowedAuthTokens(tokens ...string) []string {
	allowed := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if trimmed := strings.TrimSpace(token); trimmed != "" {
			allowed = append(allowed, trimmed)
		}
	}
	return allowed
}

func requestHasAnyBearerToken(c *gin.Context, allowed []string) bool {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	for _, token := range allowed {
		if header == fmt.Sprintf("Bearer %s", token) {
			return true
		}
	}
	return false
}

func writeError(c *gin.Context, _ error) {
	apiresponse.Error(c, http.StatusBadGateway, "cluster_unavailable", "cluster request failed")
}

func parseLimit(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}
