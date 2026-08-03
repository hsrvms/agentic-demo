package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/worker"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "error", err)
		os.Exit(1)
	}

	w, err := worker.New(*cfg)
	if err != nil {
		logger.Error("worker setup", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := w.Close(); err != nil {
			logger.Error("worker cleanup", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)
	<-quit

	logger.Info("received shutdown signal")
	if err := w.Shutdown(); err != nil {
		logger.Error("worker shutdown", "error", err)
	}
}
