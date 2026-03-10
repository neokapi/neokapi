package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	pb "github.com/gokapi/gokapi/bowrain/proto/v1"
	"github.com/gokapi/gokapi/bowrain/server"
	"google.golang.org/grpc"
)

func main() {
	cfg := server.DefaultServerConfig()

	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
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
	if envDBURL := os.Getenv("BOWRAIN_DATABASE_URL"); envDBURL != "" {
		cfg.DatabaseURL = envDBURL
	}
	if envDBAuth := os.Getenv("BOWRAIN_DATABASE_AUTH"); envDBAuth != "" {
		cfg.DatabaseAuth = envDBAuth
	}
	if envClientID := os.Getenv("AZURE_CLIENT_ID"); envClientID != "" {
		cfg.AzureClientID = envClientID
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
	if envSMTPUser := os.Getenv("BOWRAIN_SMTP_USERNAME"); envSMTPUser != "" {
		cfg.SMTPUsername = envSMTPUser
	}
	if envSMTPPass := os.Getenv("BOWRAIN_SMTP_PASSWORD"); envSMTPPass != "" {
		cfg.SMTPPassword = envSMTPPass
	}
	if envSMTPTLS := os.Getenv("BOWRAIN_SMTP_USE_TLS"); envSMTPTLS == "true" || envSMTPTLS == "1" {
		cfg.SMTPUseTLS = true
	}
	if envResend := os.Getenv("BOWRAIN_RESEND_API_KEY"); envResend != "" {
		cfg.ResendAPIKey = envResend
	}
	if envSB := os.Getenv("BOWRAIN_SERVICE_BUS_CONNECTION"); envSB != "" {
		cfg.ServiceBusConnection = envSB
	}
	if envNATS := os.Getenv("BOWRAIN_NATS_URL"); envNATS != "" {
		cfg.NATSUrl = envNATS
	}
	if envRedis := os.Getenv("BOWRAIN_REDIS_URL"); envRedis != "" {
		cfg.RedisURL = envRedis
	}
	if envRedisPassword := os.Getenv("BOWRAIN_REDIS_PASSWORD"); envRedisPassword != "" {
		cfg.RedisPassword = envRedisPassword
	}
	if envWebUI := os.Getenv("BOWRAIN_WEB_UI_DIR"); envWebUI != "" {
		cfg.WebUIDir = envWebUI
	}

	srv := server.NewServer(cfg)

	// Build gRPC server with auth interceptors when JWT is configured.
	var grpcOpts []grpc.ServerOption
	if cfg.JWTSecret != "" {
		grpcOpts = append(grpcOpts,
			grpc.UnaryInterceptor(server.GRPCAuthUnaryInterceptor(cfg.JWTSecret)),
			grpc.StreamInterceptor(server.GRPCAuthStreamInterceptor(cfg.JWTSecret)),
		)
	}
	grpcSrv := grpc.NewServer(grpcOpts...)
	pb.RegisterGokapiServiceServer(grpcSrv, server.NewGRPCServer(srv))
	pb.RegisterEditorServiceServer(grpcSrv, server.NewEditorGRPCServer(srv))
	srv.GRPCServer = grpcSrv

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if err := srv.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
