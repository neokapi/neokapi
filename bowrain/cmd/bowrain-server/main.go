package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	// Register the AWS Bedrock AI provider ("bedrock") in the aiprovider registry,
	// so BOWRAIN_PLATFORM_PROVIDER=bedrock resolves for platform-path translation.
	_ "github.com/neokapi/neokapi/bowrain/ai/bedrock"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/cmd/internal/boot"
	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/observe"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/neokapi/neokapi/bowrain/server"
	"github.com/neokapi/neokapi/core/version"
)

func main() {
	// Structured logging — bridges existing log.Printf calls through slog.
	observe.SetupLogger(
		os.Getenv("BOWRAIN_LOG_FORMAT"),
		os.Getenv("BOWRAIN_LOG_LEVEL"),
	)

	// Error tracking (no-op without SENTRY_DSN). Flushed at the end of run(),
	// which returns before main's os.Exit so buffered events are not lost.
	observe.InitSentryFromEnv("server", version.Version+"+"+version.Commit)

	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// guardSecretsKey enforces that a multi-tenant Postgres deployment configures a
// secrets key. crypto.NewCipher("") is a nil, pass-through cipher, so an ABSENT
// key silently stores every tenant's AI provider key and connector secret in
// PLAINTEXT in Postgres (only an INVALID key aborts at startup). On a billed SaaS
// deployment (STRIPE_SECRET_KEY set) this is a hard startup failure; on a
// self-hosted Postgres deployment it is a loud error-level warning for operators
// who knowingly accept plaintext-at-rest. Non-Postgres (dev/SQLite) is exempt.
func guardSecretsKey(secretsKey, databaseURL string) error {
	if secretsKey != "" {
		return nil
	}
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return nil
	}
	if billing.IsStripeSecretKey(os.Getenv("STRIPE_SECRET_KEY")) {
		return errors.New("BOWRAIN_SECRETS_KEY is required on a billed multi-tenant deployment: " +
			"without it every workspace's AI provider key and connector secret is stored in plaintext in Postgres")
	}
	slog.Error("BOWRAIN_SECRETS_KEY is not set: workspace AI provider keys and connector secrets " +
		"will be stored UNENCRYPTED in Postgres; set a 32-byte base64 key before storing tenant secrets")
	return nil
}

// acceptStripeValue returns the environment value for a Stripe setting when it
// looks like the Stripe identifier it should be, and "" otherwise — so an
// unprovisioned or mistyped value degrades to "this deployment does not sell",
// never to a half-configured billing surface. The distinction between the
// Terraform placeholder and a genuinely wrong value is worth making in the logs:
// the first is an expected state before provisioning, the second is an operator
// error.
func acceptStripeValue(envVar string, valid func(string) bool, want string) string {
	v := os.Getenv(envVar)
	switch {
	case v == "":
		return ""
	case valid(v):
		return v
	case billing.IsPlaceholder(v):
		slog.Info("stripe: "+envVar+" is still the provisioning placeholder; billing stays off until it is filled in",
			"env", envVar)
		return ""
	default:
		slog.Warn("stripe: ignoring "+envVar+": not a Stripe identifier",
			"env", envVar, "expected_prefix", want)
		return ""
	}
}

func run() error {
	// Flush buffered Sentry events before the process exits (main calls os.Exit
	// after run returns, which would skip a main-level defer).
	defer observe.FlushSentry(2 * time.Second)

	cfg := server.DefaultConfig()

	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "Address to bind to")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Directory for temporary files")
	flag.StringVar(&cfg.DatabaseURL, "database-url", cfg.DatabaseURL, "PostgreSQL connection string (postgres://...)")
	flag.StringVar(&cfg.JWTSecret, "jwt-secret", cfg.JWTSecret, "JWT signing secret")
	flag.StringVar(&cfg.OIDCIssuerURL, "oidc-issuer-url", cfg.OIDCIssuerURL, "OIDC issuer URL")
	flag.StringVar(&cfg.OIDCClientID, "oidc-client-id", cfg.OIDCClientID, "OIDC OAuth client ID")
	flag.StringVar(&cfg.OIDCClientSecret, "oidc-client-secret", cfg.OIDCClientSecret, "OIDC OAuth client secret")
	flag.StringVar(&cfg.WebUIDir, "web-ui-dir", cfg.WebUIDir, "Path to built web UI static files")
	allowInsecureDev := flag.Bool("allow-insecure-dev", false,
		"Allow starting without BOWRAIN_JWT_SECRET / BOWRAIN_DATABASE_URL, and approve device authorizations without an identity provider (local development only; also BOWRAIN_ALLOW_INSECURE_DEV=1)")
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
	if envSecretsKey := os.Getenv("BOWRAIN_SECRETS_KEY"); envSecretsKey != "" {
		cfg.SecretsKey = envSecretsKey
	}
	if _, err := crypto.NewCipher(cfg.SecretsKey); err != nil {
		return fmt.Errorf("invalid BOWRAIN_SECRETS_KEY: %w", err)
	}
	if err := guardSecretsKey(cfg.SecretsKey, cfg.DatabaseURL); err != nil {
		return err
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
	// Marketing landing origin (e.g. https://bowrain.cloud). Allowlisted for
	// the credentialed cross-origin whoami fetch that renders the signed-in CTA.
	if v := os.Getenv("BOWRAIN_PUBLIC_SITE_URL"); v != "" {
		cfg.PublicSiteURL = v
	}
	// Identity provider selection: "keycloak" (default; self-host/dev) or
	// "cognito" (hosted prod). Steers the IdentityAdmin write-through and the
	// logout URL; the OIDC verify path is issuer-driven regardless.
	if v := strings.TrimSpace(os.Getenv("BOWRAIN_AUTH_PROVIDER")); v != "" {
		cfg.AuthProvider = strings.ToLower(v)
	}
	if cfg.AuthProvider == "" {
		cfg.AuthProvider = server.AuthProviderKeycloak
	}
	if cfg.AuthProvider != server.AuthProviderKeycloak && cfg.AuthProvider != server.AuthProviderCognito {
		return fmt.Errorf("invalid BOWRAIN_AUTH_PROVIDER %q (want %q or %q)",
			cfg.AuthProvider, server.AuthProviderKeycloak, server.AuthProviderCognito)
	}
	// Opt-out of verified-email enforcement (default: enforce). See Config docs.
	if v := os.Getenv("BOWRAIN_ALLOW_UNVERIFIED_EMAIL"); v != "" {
		cfg.AllowUnverifiedEmail, _ = strconv.ParseBool(v)
	}
	// Self-service passkey management (default: off / ship dark). Enable once the
	// Cognito WebAuthn RP ID + upstream-token retention are verified in prod.
	if v := os.Getenv("BOWRAIN_PASSKEYS_ENABLED"); v != "" {
		cfg.PasskeysEnabled, _ = strconv.ParseBool(v)
	}
	// Force Secure cookies (set true in TLS-fronted prod). See Config docs.
	if v := os.Getenv("BOWRAIN_FORCE_SECURE_COOKIES"); v != "" {
		cfg.ForceSecureCookies, _ = strconv.ParseBool(v)
	}
	if v := os.Getenv("BOWRAIN_KEYCLOAK_ADMIN_URL"); v != "" {
		cfg.KeycloakAdminURL = v
	}
	if v := os.Getenv("BOWRAIN_KEYCLOAK_REALM"); v != "" {
		cfg.KeycloakRealm = v
	}
	if v := os.Getenv("BOWRAIN_KEYCLOAK_ADMIN_CLIENT_ID"); v != "" {
		cfg.KeycloakAdminClientID = v
	}
	if v := os.Getenv("BOWRAIN_KEYCLOAK_ADMIN_CLIENT_SECRET"); v != "" {
		cfg.KeycloakAdminClientSecret = v
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
	if envRedis := os.Getenv("BOWRAIN_REDIS_URL"); envRedis != "" {
		cfg.RedisURL = envRedis
	}
	if envRedisPassword := os.Getenv("BOWRAIN_REDIS_PASSWORD"); envRedisPassword != "" {
		cfg.RedisPassword = envRedisPassword
	}
	// Platform AI provider (mirrors the worker): the server-chosen backend for
	// synchronous platform-path editor translations. On the hosted cloud this is
	// "bedrock" with a Bedrock inference-profile model and no API key (the ECS
	// task role authenticates).
	if v := os.Getenv("BOWRAIN_PLATFORM_PROVIDER"); v != "" {
		cfg.PlatformProvider = v
	}
	if v := os.Getenv("BOWRAIN_PLATFORM_MODEL"); v != "" {
		cfg.PlatformModel = v
	}
	if v := os.Getenv("BOWRAIN_PLATFORM_BASE_URL"); v != "" {
		cfg.PlatformBaseURL = v
	}
	if envWebUI := os.Getenv("BOWRAIN_WEB_UI_DIR"); envWebUI != "" {
		cfg.WebUIDir = envWebUI
	}
	// Public Pulse dashboard surface (default: unmounted). See Config docs.
	if v := os.Getenv("BOWRAIN_PULSE_ENABLED"); v != "" {
		cfg.PulseEnabled, _ = strconv.ParseBool(v)
	}
	if v := os.Getenv("BOWRAIN_MAX_PUSH_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxPushBytes = n
		}
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

	// Billing (Bowrain AD-018).
	// Stripe settings are accepted only when they look like Stripe identifiers.
	// Terraform seeds these SSM parameters as "CHANGEME" (epic 002), and a
	// placeholder that reads as "configured" would put the deployment in the worst
	// state available: billing advertised, every Stripe call failing. Rejecting it
	// here means an unprovisioned deployment simply has no checkout surface.
	cfg.StripeSecretKey = acceptStripeValue("STRIPE_SECRET_KEY", billing.IsStripeSecretKey, "sk_… or rk_…")
	cfg.StripeWebhookSecret = acceptStripeValue("STRIPE_WEBHOOK_SECRET", billing.IsStripeWebhookSecret, "whsec_…")
	cfg.StripeProPriceID = acceptStripeValue("STRIPE_PRO_PRICE_ID", billing.IsStripePriceID, "price_…")
	cfg.StripeTeamPriceID = acceptStripeValue("STRIPE_TEAM_PRICE_ID", billing.IsStripePriceID, "price_…")
	cfg.StripeCreditPriceID = acceptStripeValue("STRIPE_CREDIT_PRICE_ID", billing.IsStripePriceID, "price_…")
	if v := os.Getenv("POSTHOG_API_KEY"); v != "" {
		cfg.PostHogAPIKey = v
	}
	if v := os.Getenv("POSTHOG_HOST"); v != "" {
		cfg.PostHogHost = v
	}

	// Admin control plane (Bowrain AD-018).
	if v := os.Getenv("BOWRAIN_ADMIN_OIDC_ISSUER_URL"); v != "" {
		cfg.AdminOIDCIssuerURL = v
	}
	if v := os.Getenv("BOWRAIN_ADMIN_OIDC_CLIENT_ID"); v != "" {
		cfg.AdminOIDCClientID = v
	}
	if v := os.Getenv("BOWRAIN_ADMIN_OIDC_CLIENT_SECRET"); v != "" {
		cfg.AdminOIDCClientSecret = v
	}
	if v := os.Getenv("BOWRAIN_AUDIT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AuditRetentionDays = n
		}
	}
	if v := os.Getenv("BOWRAIN_AUDIT_SIEM_WEBHOOK_URL"); v != "" {
		cfg.AuditSIEMWebhookURL = v
	}
	// GitHub App (forge delivery, app mode). The key arrives as PEM text or,
	// more conveniently under systemd/compose, as a file path.
	cfg.GitHubAppID = os.Getenv("GITHUB_APP_ID")
	cfg.GitHubAppWebhookSecret = os.Getenv("GITHUB_APP_WEBHOOK_SECRET")
	if v := os.Getenv("GITHUB_APP_PRIVATE_KEY"); v != "" {
		cfg.GitHubAppPrivateKey = v
	} else if v := os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"); v != "" {
		pemBytes, err := os.ReadFile(v)
		if err != nil {
			slog.Error("GITHUB_APP_PRIVATE_KEY_FILE unreadable — GitHub App disabled", "error", err)
		} else {
			cfg.GitHubAppPrivateKey = string(pemBytes)
		}
	}

	// Fail-fast boot validation. A malformed database URL is always fatal;
	// MISSING JWT secret / database URL are fatal unless the insecure-dev
	// escape hatch is explicitly set (a typo'd env var must never silently
	// ship a server with no auth routes or no stores).
	if cfg.DatabaseURL != "" {
		if err := boot.ValidatePostgresURL("BOWRAIN_DATABASE_URL (or -database-url)", cfg.DatabaseURL); err != nil {
			return err
		}
	}
	insecureDev := *allowInsecureDev || boot.AllowInsecureDevFromEnv()
	if missing := boot.MissingServerConfig(cfg.JWTSecret, cfg.DatabaseURL); len(missing) > 0 {
		if !insecureDev {
			return fmt.Errorf("refusing to start (set --allow-insecure-dev or %s=1 to override for local development): %w",
				boot.InsecureDevEnv, errors.Join(missing...))
		}
		for _, m := range missing {
			slog.Warn("INSECURE DEV MODE: starting despite missing configuration", "issue", m.Error())
		}
	}
	// Direct device approval belongs to the same escape hatch: it authorizes a
	// device for whatever identity the request names, so it rides on the one
	// explicit "this is a development box" signal rather than on any inference
	// from how OIDC happens to be configured.
	cfg.AllowInsecureDeviceAuth = insecureDev

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
	srv.GRPCServer = grpcSrv

	// Start pprof on a separate localhost-only listener (if enabled).
	pprofShutdown := observe.StartPprofServer(context.Background())

	// Graceful shutdown: start server in a goroutine, wait for signal.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server listen: %w", err)
	case <-ctx.Done():
	}

	slog.Info("shutdown signal received, draining connections...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if pprofShutdown != nil {
		_ = pprofShutdown(shutdownCtx)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("forced shutdown: %w", err)
	}
	slog.Info("server stopped")
	return nil
}
