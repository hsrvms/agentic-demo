package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	srv, err := server.New(*cfg)
	if err != nil {
		log.Fatalf("server setup error: %v", err)
	}
	defer func() {
		if err := srv.Close(); err != nil {
			log.Printf("server cleanup error: %v", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Start(":3000")
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)
	select {
	case err := <-serverErr:
		if err != nil {
			log.Printf("server error: %v", err)
		}
		return
	case <-quit:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
