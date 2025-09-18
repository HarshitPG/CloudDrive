package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"backend/internal/storage"
	"backend/internal/worker"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	st, err := storage.NewMinioStorage()
	if err != nil {
		log.Fatalf("storage init failed: %v", err)
	}

	gc := worker.NewGCWorker(st)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go gc.Run(ctx)

	log.Println("GC worker started")

	<-ctx.Done()
	log.Println("Shutting down GC worker...")
	_ = gc.Close()
	log.Println("GC worker exited")
}
