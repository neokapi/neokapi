package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/gokapi/gokapi/apps/web"
	"github.com/gokapi/gokapi/internal/server"
	pb "github.com/gokapi/gokapi/proto/v1"
	"google.golang.org/grpc"
)

func main() {
	cfg := server.DefaultServerConfig()

	var grpcPort int
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
	flag.IntVar(&grpcPort, "grpc-port", 0, "gRPC port (0 to disable)")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "Address to bind to")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory for temporary files")
	flag.StringVar(&cfg.StorePath, "store", cfg.StorePath, "Path to SQLite content store database")
	flag.StringVar(&cfg.JWTSecret, "jwt-secret", cfg.JWTSecret, "JWT signing secret")
	flag.StringVar(&cfg.DexIssuerURL, "dex-issuer-url", cfg.DexIssuerURL, "Dex OIDC issuer URL")
	flag.StringVar(&cfg.DexClientID, "dex-client-id", cfg.DexClientID, "Dex OAuth client ID")
	flag.StringVar(&cfg.DexClientSecret, "dex-client-secret", cfg.DexClientSecret, "Dex OAuth client secret")
	flag.StringVar(&cfg.WebUIDir, "web-ui-dir", cfg.WebUIDir, "Path to built web UI static files")
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
	if envStore := os.Getenv("GOKAPI_STORE"); envStore != "" {
		cfg.StorePath = envStore
	}
	if envGRPC := os.Getenv("GOKAPI_GRPC_PORT"); envGRPC != "" {
		if p, err := strconv.Atoi(envGRPC); err == nil {
			grpcPort = p
		}
	}
	if envJWT := os.Getenv("GOKAPI_JWT_SECRET"); envJWT != "" {
		cfg.JWTSecret = envJWT
	}
	if envIssuer := os.Getenv("GOKAPI_DEX_ISSUER_URL"); envIssuer != "" {
		cfg.DexIssuerURL = envIssuer
	}
	if envClientID := os.Getenv("GOKAPI_DEX_CLIENT_ID"); envClientID != "" {
		cfg.DexClientID = envClientID
	}
	if envSecret := os.Getenv("GOKAPI_DEX_CLIENT_SECRET"); envSecret != "" {
		cfg.DexClientSecret = envSecret
	}

	srv := server.NewServer(cfg)

	// Serve embedded web UI.
	webFS, _ := fs.Sub(web.Assets, "dist")
	srv.WebUIFS = webFS

	// Start gRPC server if a port is configured.
	if grpcPort > 0 {
		grpcAddr := fmt.Sprintf("%s:%d", cfg.Host, grpcPort)
		go func() {
			lis, err := net.Listen("tcp", grpcAddr)
			if err != nil {
				log.Fatalf("gRPC listen failed: %v", err)
			}
			grpcSrv := grpc.NewServer()
			pb.RegisterGokapiServiceServer(grpcSrv, server.NewGRPCServer(srv))
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
