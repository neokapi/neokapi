package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/gokapi/gokapi/bowrain/apps/web"
	"github.com/gokapi/gokapi/bowrain/server"
	pb "github.com/gokapi/gokapi/bowrain/proto/v1"
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
	flag.StringVar(&cfg.OIDCIssuerURL, "oidc-issuer-url", cfg.OIDCIssuerURL, "OIDC issuer URL")
	flag.StringVar(&cfg.OIDCClientID, "oidc-client-id", cfg.OIDCClientID, "OIDC OAuth client ID")
	flag.StringVar(&cfg.OIDCClientSecret, "oidc-client-secret", cfg.OIDCClientSecret, "OIDC OAuth client secret")
	flag.StringVar(&cfg.WebUIDir, "web-ui-dir", cfg.WebUIDir, "Path to built web UI static files")
	flag.Parse()

	// Allow environment variable overrides.
	if envPort := os.Getenv("BOWRAIN_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			cfg.Port = p
		}
	}
	if envHost := os.Getenv("BOWRAIN_HOST"); envHost != "" {
		cfg.Host = envHost
	}
	if envDataDir := os.Getenv("BOWRAIN_DATA_DIR"); envDataDir != "" {
		cfg.DataDir = envDataDir
	}
	if envStore := os.Getenv("BOWRAIN_STORE"); envStore != "" {
		cfg.StorePath = envStore
	}
	if envGRPC := os.Getenv("BOWRAIN_GRPC_PORT"); envGRPC != "" {
		if p, err := strconv.Atoi(envGRPC); err == nil {
			grpcPort = p
		}
	}
	if envJWT := os.Getenv("BOWRAIN_JWT_SECRET"); envJWT != "" {
		cfg.JWTSecret = envJWT
	}
	if envIssuer := os.Getenv("BOWRAIN_OIDC_ISSUER_URL"); envIssuer != "" {
		cfg.OIDCIssuerURL = envIssuer
	}
	if envClientID := os.Getenv("BOWRAIN_OIDC_CLIENT_ID"); envClientID != "" {
		cfg.OIDCClientID = envClientID
	}
	if envSecret := os.Getenv("BOWRAIN_OIDC_CLIENT_SECRET"); envSecret != "" {
		cfg.OIDCClientSecret = envSecret
	}
	if envPublic := os.Getenv("BOWRAIN_OIDC_PUBLIC_URL"); envPublic != "" {
		cfg.OIDCPublicURL = envPublic
	}
	if envSMTPHost := os.Getenv("BOWRAIN_SMTP_HOST"); envSMTPHost != "" {
		cfg.SMTPHost = envSMTPHost
	}
	if envSMTPFrom := os.Getenv("BOWRAIN_SMTP_FROM"); envSMTPFrom != "" {
		cfg.SMTPFrom = envSMTPFrom
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

			// Build gRPC server options with auth interceptors when JWT is configured.
			var opts []grpc.ServerOption
			if cfg.JWTSecret != "" {
				opts = append(opts,
					grpc.UnaryInterceptor(server.GRPCAuthUnaryInterceptor(cfg.JWTSecret)),
					grpc.StreamInterceptor(server.GRPCAuthStreamInterceptor(cfg.JWTSecret)),
				)
			}

			grpcSrv := grpc.NewServer(opts...)
			pb.RegisterGokapiServiceServer(grpcSrv, server.NewGRPCServer(srv))
			pb.RegisterEditorServiceServer(grpcSrv, server.NewEditorGRPCServer(srv))
			log.Printf("Starting gRPC server on %s", grpcAddr)
			if err := grpcSrv.Serve(lis); err != nil {
				log.Fatalf("gRPC server failed: %v", err)
			}
		}()
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting Bowrain server on %s", addr)
	if err := srv.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
