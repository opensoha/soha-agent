package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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

func New(cfg cfgpkg.Config, logger *zap.Logger, client *k8sagent.Client, runtime RuntimeTaskController) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(fmt.Sprintf("disable trusted proxies: %v", err))
	}
	router.Use(gin.Recovery())
	router.Use(apiMiddleware.RequestID())
	auditSink := newActionAuditSink(cfg.Audit, logger)
	actions := newActionPolicy(cfg.Security, logger, auditSink)
	registerSystemRoutes(router, cfg, client, runtime)
	registerPlatformRoutes(router, cfg, client, actions)
	registerRuntimeRoutes(router, cfg, runtime, actions)
	registerDockerRuntimeRoutes(router, cfg, logger, actions)
	registerOutpostRoutes(router, cfg, runtime)

	logger.Info("agent server configured",
		zap.String("addr", cfg.HTTP.Addr),
		zap.String("base_path", cfg.HTTP.BasePath),
		zap.String("cluster_id", cfg.Kubernetes.ID),
		zap.Bool("authentication_enabled", len(allowedAuthTokens(cfg.Auth.BearerToken)) > 0),
		zap.Int("allowed_actions", len(actions.allowed)),
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

func registerPlatformRoutes(router *gin.Engine, cfg cfgpkg.Config, client *k8sagent.Client, actions actionPolicy) {
	if client == nil {
		return
	}
	platform := router.Group(fmt.Sprintf("%s/platform", cfg.HTTP.BasePath))
	platform.Use(authMiddleware(cfg.Auth.BearerToken))
	registerResourceYAMLRoutes(platform, client, actions)
	registerResourceCreationRoutes(platform, client, actions)
	registerCustomResourceRoutes(platform, client, actions)
	origins := newWebSocketOriginPolicy(cfg.HTTP.AllowedOrigins, cfg.Auth.BearerToken)
	registerPortForwardRoutes(
		platform,
		newPortForwardRegistry(cfg.Kubernetes, kubernetesPortForwardStarter(client)),
		actions,
		origins,
	)
	registerHelmRoutes(platform, client, actions)
	registerPodStreamRoutes(platform, client)
	registerPodTerminalRoutes(platform, client, actions, origins)
	registerPlatformInventoryRoutes(platform, client)
	registerPlatformWorkloadRoutes(platform, client, actions)
	registerPlatformConfigurationRoutes(platform, client)
	registerPlatformRBACRoutes(platform, client)
	registerPlatformNetworkRoutes(platform, client)
	registerPlatformStorageRoutes(platform, client)
	registerPlatformHelmReadRoutes(platform, client)
	registerPlatformWorkloadMutationRoutes(platform, client, actions)
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
