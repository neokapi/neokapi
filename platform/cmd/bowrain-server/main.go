package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/neokapi/neokapi/bowrain/server"
	"google.golang.org/grpc"
)

func main() {
	// Handle subcommands before flag parsing.
	if len(os.Args) > 1 && os.Args[0] != "-" {
		switch os.Args[1] {
		case "seed-service-token":
			dbURL := os.Getenv("BOWRAIN_DATABASE_URL")
			dbAuth := os.Getenv("BOWRAIN_DATABASE_AUTH")
			azureClientID := os.Getenv("AZURE_CLIENT_ID")
			workspace := os.Getenv("BOWRAIN_SERVICE_WORKSPACE")
			seedServiceToken(dbURL, dbAuth, azureClientID, workspace)
			return
		case "seed-project":
			seedProject(seedProjectConfig{
				DatabaseURL:     os.Getenv("BOWRAIN_DATABASE_URL"),
				DatabaseAuth:    os.Getenv("BOWRAIN_DATABASE_AUTH"),
				AzureClientID:   os.Getenv("AZURE_CLIENT_ID"),
				WorkspaceSlug:   os.Getenv("BOWRAIN_SERVICE_WORKSPACE"),
				ProjectName:     os.Getenv("BOWRAIN_PROJECT_NAME"),
				SourceLanguage:  os.Getenv("BOWRAIN_SOURCE_LANGUAGE"),
				TargetLanguages: os.Getenv("BOWRAIN_TARGET_LANGUAGES"),
			})
			return
		}
	}

	cfg := server.DefaultServerConfig()

	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "Address to bind to")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory for temporary files")
	flag.StringVar(&cfg.DatabaseURL, "database-url", cfg.DatabaseURL, "PostgreSQL connection string (postgres://...)")
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

	// Agent (@bravo) configuration.
	if v := os.Getenv("BOWRAIN_AGENT_RUNTIME"); v != "" {
		cfg.AgentRuntime = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_IMAGE"); v != "" {
		cfg.AgentImage = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.AgentMaxConcurrent = n
		}
	}
	if v := os.Getenv("BOWRAIN_AGENT_DOCKER_HOST"); v != "" {
		cfg.AgentDockerHost = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_DOCKER_NETWORK"); v != "" {
		cfg.AgentDockerNetwork = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_ACA_SUBSCRIPTION"); v != "" {
		cfg.AgentACASubscription = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_ACA_RESOURCE_GROUP"); v != "" {
		cfg.AgentACAResourceGroup = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_ACA_ENVIRONMENT_ID"); v != "" {
		cfg.AgentACAEnvironmentID = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_ACA_LOCATION"); v != "" {
		cfg.AgentACALocation = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_MODEL_PROVIDER"); v != "" {
		cfg.AgentModelProvider = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_MODEL_NAME"); v != "" {
		cfg.AgentModelName = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_MODEL_API_BASE"); v != "" {
		cfg.AgentModelAPIBase = v
	}
	if v := os.Getenv("BOWRAIN_AGENT_MODEL_API_KEY"); v != "" {
		cfg.AgentModelAPIKey = v
	}

	// Billing (AD-030).
	if v := os.Getenv("STRIPE_SECRET_KEY"); v != "" {
		cfg.StripeSecretKey = v
	}
	if v := os.Getenv("STRIPE_WEBHOOK_SECRET"); v != "" {
		cfg.StripeWebhookSecret = v
	}
	if v := os.Getenv("STRIPE_PRO_PRICE_ID"); v != "" {
		cfg.StripeProPriceID = v
	}
	if v := os.Getenv("STRIPE_TEAM_PRICE_ID"); v != "" {
		cfg.StripeTeamPriceID = v
	}
	if v := os.Getenv("STRIPE_CREDIT_PRICE_ID"); v != "" {
		cfg.StripeCreditPriceID = v
	}
	if v := os.Getenv("POSTHOG_API_KEY"); v != "" {
		cfg.PostHogAPIKey = v
	}
	if v := os.Getenv("POSTHOG_HOST"); v != "" {
		cfg.PostHogHost = v
	}

	// Admin control plane (AD-030).
	if v := os.Getenv("BOWRAIN_ADMIN_OIDC_ISSUER_URL"); v != "" {
		cfg.AdminOIDCIssuerURL = v
	}
	if v := os.Getenv("BOWRAIN_ADMIN_OIDC_CLIENT_ID"); v != "" {
		cfg.AdminOIDCClientID = v
	}
	if v := os.Getenv("BOWRAIN_ADMIN_OIDC_CLIENT_SECRET"); v != "" {
		cfg.AdminOIDCClientSecret = v
	}

	// Validate that DatabaseURL is a PostgreSQL connection string.
	if cfg.DatabaseURL != "" && !strings.HasPrefix(cfg.DatabaseURL, "postgres://") && !strings.HasPrefix(cfg.DatabaseURL, "postgresql://") {
		log.Fatalf("Invalid -database-url: must start with postgres:// or postgresql://")
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
	pb.RegisterNeokapiServiceServer(grpcSrv, server.NewGRPCServer(srv))
	pb.RegisterEditorServiceServer(grpcSrv, server.NewEditorGRPCServer(srv))
	srv.GRPCServer = grpcSrv

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if err := srv.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
