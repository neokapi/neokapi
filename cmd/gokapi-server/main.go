package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
)

func main() {
	cfg := DefaultServerConfig()

	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "Address to bind to")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory for temporary files")
	flag.Parse()

	// Allow environment variable overrides.
	if envPort := os.Getenv("GOKAPI_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			cfg.Port = p
		}
	}
	if envHost := os.Getenv("GOKAPI_HOST"); envHost != "" {
		cfg.Host = envHost
	}
	if envDataDir := os.Getenv("GOKAPI_DATA_DIR"); envDataDir != "" {
		cfg.DataDir = envDataDir
	}

	srv := NewServer(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting gokapi server on %s", addr)
	if err := srv.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
