package bootstrap

import (
	"context"
	"fmt"

	agentapi "github.com/opensoha/soha-agent/internal/agent/api"
	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	loggerpkg "github.com/opensoha/soha-agent/internal/agent/logger"
	runnerpkg "github.com/opensoha/soha-agent/internal/agent/runner"
	"go.uber.org/zap"
)

type App struct {
	Config cfgpkg.Config
	Logger *zap.Logger
	Server *agentapi.Server
	Runner *runnerpkg.Runner
	cancel context.CancelFunc
}

func New(ctx context.Context) (*App, error) {
	lifecycleCtx, cancel := context.WithCancel(ctx)
	cfg, err := cfgpkg.Load()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load agent config: %w", err)
	}

	logger, err := loggerpkg.New(cfg.Logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build agent logger: %w", err)
	}

	var client *k8sagent.Client
	if cfg.Kubernetes.Enabled {
		client, err = k8sagent.New(cfg.Kubernetes)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("build kubernetes client: %w", err)
		}
	} else {
		logger.Info("agent kubernetes client disabled; platform proxy routes will be unavailable")
	}

	runner := runnerpkg.New(cfg.ControlPlane, logger)
	runner.Start(lifecycleCtx)
	server := agentapi.New(cfg, logger, client, runner)
	return &App{Config: cfg, Logger: logger, Server: server, Runner: runner, cancel: cancel}, nil
}

func (a *App) Run() error {
	return a.Server.Run()
}

func (a *App) Shutdown(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.Server != nil {
		if err := a.Server.Shutdown(ctx); err != nil {
			return err
		}
	}
	if a.Logger != nil {
		_ = a.Logger.Sync()
	}
	return nil
}
