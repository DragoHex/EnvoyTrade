package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "configs/testbroker_config.json", "Path to testbroker configuration JSON")
	portFlag := flag.Int("port", 0, "Override HTTP port (defaults to port in config)")
	flag.Parse()

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", *configPath, err)
	}

	if *portFlag > 0 {
		cfg.Port = *portFlag
	}

	// Initialize server
	server, err := NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Trap termination signals
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Server stopped with error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down test broker...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
	log.Println("Test broker shutdown complete.")
}
