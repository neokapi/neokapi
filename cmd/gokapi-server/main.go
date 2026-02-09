package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	pb "github.com/gokapi/gokapi/proto/v1"
	"google.golang.org/grpc"
)

func main() {
	cfg := DefaultServerConfig()

	var grpcPort int
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
	flag.IntVar(&grpcPort, "grpc-port", 0, "gRPC port (0 to disable)")
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
	if envGRPC := os.Getenv("GOKAPI_GRPC_PORT"); envGRPC != "" {
		if p, err := strconv.Atoi(envGRPC); err == nil {
			grpcPort = p
		}
	}

	srv := NewServer(cfg)

	// Start gRPC server if a port is configured.
	if grpcPort > 0 {
		grpcAddr := fmt.Sprintf("%s:%d", cfg.Host, grpcPort)
		go func() {
			lis, err := net.Listen("tcp", grpcAddr)
			if err != nil {
				log.Fatalf("gRPC listen failed: %v", err)
			}
			grpcSrv := grpc.NewServer()
			pb.RegisterGokapiServiceServer(grpcSrv, NewGRPCServer(srv))
			log.Printf("Starting gRPC server on %s", grpcAddr)
			if err := grpcSrv.Serve(lis); err != nil {
				log.Fatalf("gRPC server failed: %v", err)
			}
		}()
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting gokapi REST server on %s", addr)
	if err := srv.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
