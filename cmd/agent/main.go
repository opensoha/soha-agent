package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentbootstrap "github.com/opensoha/soha-agent/internal/agent/bootstrap"
	"github.com/opensoha/soha-agent/internal/agent/buildinfo"
	"go.uber.org/zap"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if shouldPrintVersion(args) {
		fmt.Println(buildinfo.Human())
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := agentbootstrap.New(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bootstrap soha agent: %v\n", err)
		return 1
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- application.Run()
	}()

	application.Logger.Info("soha agent started")

	exitCode := 0
	select {
	case <-ctx.Done():
		stop()
	case err := <-runErr:
		if err != nil {
			application.Logger.Error("agent server exited with error", zap.Error(err))
			exitCode = 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		application.Logger.Error("agent graceful shutdown failed", zap.Error(err))
		return 1
	}

	return exitCode
}

func shouldPrintVersion(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "version", "--version", "-version", "-v":
		return true
	default:
		return false
	}
}
