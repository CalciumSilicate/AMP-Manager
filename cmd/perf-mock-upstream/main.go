package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"ampmanager/internal/perf/mockupstream"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := mockupstream.ConfigFromEnv()
	log.Printf("perf mock upstream listening on %s with %d model(s)", cfg.Addr, len(cfg.Models))

	server := mockupstream.New(cfg)
	if err := server.ListenAndServe(ctx); err != nil {
		log.Fatalf("perf mock upstream failed: %v", err)
	}
}
