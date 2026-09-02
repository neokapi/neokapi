package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bowsync "github.com/neokapi/neokapi/bowrain/sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	slogecho "github.com/samber/slog-echo"
	"google.golang.org/grpc"

	"github.com/neokapi/neokapi/core/formats"
	coreg "github.com/neokapi/neokapi/core/graph"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	corestorage "github.com/neokapi/neokapi/core/storage"
	libtools "github.com/neokapi/neokapi/core/tools"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/connector"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/forge"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/mailer"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/resilience"
	mcpserver "github.com/neokapi/neokapi/bowrain/server/mcp"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	coreblockstore "github.com/neokapi/neokapi/core/blockstore"
)

// Server is the REST API server for neokapi.
type Server struct {
	Config         Config
	FormatRegistry *registry.FormatRegistry
	ToolRegistry   *registry.ToolRegistry
	ConnectorReg   *platconn.Registry
	ContentStore   store.ContentStore
	BlobStore      corestorage.BlobStore
	Services       *service.Services
	AuthStore      auth.AuthStore
	EventBus       platev.EventBus
	Echo           *echo.Echo

	// wsStores manages per-workspace content memory and terminology stores.
	wsStores *workspaceStores

	// CredentialStore manages AI provider credentials.
	CredentialStore *credentials.Store

	// ProviderStore holds workspace-scoped AI provider configs in Postgres
	// (Epic 004), sealed at rest under BOWRAIN_SECRETS_KEY. It is the
	// multi-tenant, keychain-free replacement for CredentialStore on the
	// /:ws/providers surface. Nil until PostgreSQL stores are initialized.
	ProviderStore *bstore.ProviderConfigStore

	// ConnectorConfigStore persists workspace-scoped connector configurations
	// (WordPress, Figma, HubSpot, …) so they survive a restart, with credential
	// fields sealed at rest under BOWRAIN_SECRETS_KEY. The ConnectorService only
	// holds live instances in memory; on boot rehydrateConnectors reads this
	// store and re-instantiates each connector. Nil until PostgreSQL stores are
	// initialized (the in-memory/desktop path runs connectors live-only).
	ConnectorConfigStore *bstore.ConnectorConfigStore

	// ForgeInstallationStore records which workspace each GitHub App
	// installation belongs to. One registered app serves every workspace on a
	// shared instance and its JWT can mint a token for any installation, so this
	// store is what makes the post-install setup endpoints answerable: an
	// installation a workspace has not claimed is invisible to it. Nil until
	// PostgreSQL stores are initialized (the in-memory/desktop path is
	// single-tenant and binds repositories directly).
	ForgeInstallationStore *bstore.ForgeInstallationStore

	// GitHubApp authenticates forge delivery as an installed GitHub App
	// (nil unless the GITHUB_APP_* config is complete).
	GitHubApp *forge.GitHubApp

	// EmailSender sends raw transactional emails. Prefer Mailer for
	// template-rendered messages. Kept for backward compatibility with tests.
	// Nil if email is not configured.
	EmailSender EmailSenderI

	// Mailer renders branded email templates and dispatches them via
	// EmailSender. Nil when email sending is not configured.
	Mailer *mailer.Mailer

	// IdentityAdmin writes through identity changes (email, etc.) to the upstream
	// IdP via the provider-neutral IdentityAdmin port. Nil when no admin client
	// is configured, in which case Bowrain-managed email change is unavailable.
	// The concrete implementation is chosen by Config.AuthProvider: the Keycloak
	// service-account admin client (self-host/dev) or the Cognito
	// AdminUpdateUserAttributes adapter (hosted prod) — the call sites are
	// unaware of which is in play.
	IdentityAdmin auth.IdentityAdmin

	// CredentialManager is the provider-neutral seam for a user's self-service
	// credential management (passkeys). Like IdentityAdmin its concrete
	// implementation is chosen by Config.AuthProvider — Cognito relays the
	// WebAuthn ceremony in-app (hosted prod), Keycloak hands off to its account
	// console (self-host). Nil when OIDC is not configured, in which case the
	// account-security endpoints return 503.
	CredentialManager auth.CredentialManager

	// secretsCipher seals secret values at rest (connector credentials, provider
	// keys, and the short-lived elevated Cognito access token stashed in the
	// session store for passkey management). Nil when no secrets key is
	// configured (pass-through, dev only).
	secretsCipher *crypto.Cipher

	// collabHub manages collaborative editing WebSocket rooms.
	collabHub *collabHub

	// posthogDemand caches PostHog locale-demand snapshots per (project,
	// range) — the PostHog query API is rate-limited.
	posthogDemand *posthogDemandCache

	// notificationHub manages per-user WebSocket connections for real-time notifications.
	notificationHub *notificationHub

	// JobStore persists translation job state. Nil when job system is not configured.
	JobStore jobs.JobStore

	// JobQueue enqueues and dequeues translation job IDs. Nil when job system is not configured.
	JobQueue jobs.Queue

	// QuotaStore tracks AI token usage per workspace. Nil when quota tracking is not configured.
	QuotaStore jobs.QuotaStore

	// SweepStore persists model recommendation sweep measurements (measured
	// steerability). Nil when the job system is not configured.
	SweepStore jobs.ModelSweepStore

	// GRPCServer is an optional gRPC server multiplexed on the same port.
	// When set, gRPC requests (HTTP/2 with Content-Type: application/grpc)
	// are routed to this server. When nil, gRPC is not available.
	GRPCServer *grpc.Server

	// httpServer holds the underlying *http.Server for graceful shutdown.
	// Set by Start() when gRPC multiplexing is active (h2c mode).
	httpServer *http.Server

	// AutomationEngine evaluates automation rules on events. Nil when event system is not wired up.
	AutomationEngine *event.AutomationEngine

	// AutomationRuleStore persists automation rules. Nil when not configured.
	AutomationRuleStore *event.RuleStore

	// FlowDefStore persists project flow definitions (Bowrain AD-013). Nil when not configured.
	FlowDefStore *bstore.FlowDefStore

	// SessionStore holds ephemeral auth states (device codes, OIDC states).
	// Backed by Redis when configured, otherwise in-memory.
	SessionStore SessionStateStore

	// VoiceStore manages voice profiles. Nil when not configured.
	VoiceStore coreprofile.Store

	// KnowledgeStore persists the governance and collaboration layer of the
	// brand knowledge graph (AD-021): markets, observations, comments, concept
	// revisions, and change-sets with reviews and pilots. Nil when not configured.
	KnowledgeStore knowledge.Store

	// GraphStore provides graph-based concept management. Nil when not configured.
	GraphStore coreg.GraphStore

	// graphSyncer keeps the graph in sync with content events. Nil when graph is not configured.
	graphSyncer *platgraph.GraphSyncer

	// conceptScanBudget bounds the content walk the context-graph read falls
	// back to when the graph holds no record of a concept. Zero uses
	// defaultConceptScanBudget.
	conceptScanBudget time.Duration

	// AuditLogger persists all events to the audit_log table. Nil when not configured.
	AuditLogger *event.AuditLogger

	// AuditRetention prunes audit rows past the retention window. Nil when disabled.
	AuditRetention *event.AuditRetentionCleaner

	// SIEMExporter forwards events to an external SIEM/log sink. Nil when disabled.
	SIEMExporter *event.SIEMExporter

	// mcpServer is the MCP protocol server for voice. Nil when voice store is not configured.
	mcpServer *mcpserver.MCPServer

	// ReviewQueueStore persists entity/term extraction review items. Nil when not configured.
	ReviewQueueStore *bstore.ReviewQueueStore

	// SourceProposalStore persists back-to-source review proposals (RV-F): a
	// reviewer's proposed source-text fix, approved by a source owner (which
	// re-drafts every locale) or rejected. Nil when not configured.
	SourceProposalStore *bstore.SourceProposalStore

	// NotificationStore persists user notifications. Nil when not configured.
	NotificationStore *bstore.NotificationStore

	// ActivityStore persists activity feed entries. Nil when not configured.
	ActivityStore *bstore.ActivityStore

	// TaskStore persists human tasks. Nil when not configured.
	TaskStore *bstore.TaskStore

	// PreferenceStore persists notification preferences. Nil when not configured.
	PreferenceStore *bstore.PreferenceStore

	// DigestStore persists digest settings and state. Nil when not configured.
	DigestStore *bstore.DigestStore

	// DailyDigestWorker sends daily digest emails. Nil when not configured.
	DailyDigestWorker *event.DigestWorker
	// WeeklyDigestWorker sends weekly digest emails. Nil when not configured.
	WeeklyDigestWorker *event.DigestWorker

	// ActivityRecorder subscribes to events and records activities. Nil when not configured.
	ActivityRecorder *event.ActivityRecorder

	// NotificationDispatcher routes events to user notifications. Nil when not configured.
	NotificationDispatcher *event.NotificationDispatcher

	// deadlineChecker periodically scans for tasks approaching their deadline. Nil when not configured.
	deadlineChecker *event.DeadlineChecker

	// progressTracker detects translation progress milestones. Nil when not configured.
	progressTracker *event.ProgressTracker

	// pushCompletionTracker monitors automation jobs per push and emits push.automations.completed. Nil when not configured.
	pushCompletionTracker *event.PushCompletionTracker

	// AutomationRunStore persists automation runs, steps, and logs (Bowrain AD-013). Nil when not configured.
	AutomationRunStore *bstore.AutomationRunStore

	// ConvergenceRunStore persists server-side convergence runs and their event
	// streams (strategy 2026-07-kapi-up doc 03). Nil when not configured.
	ConvergenceRunStore *bstore.ConvergenceRunStore

	// convergence drives server-side convergence runs (the venue-neutral loop
	// wired with block-store + job-queue IO) and fans their events to SSE
	// subscribers. Nil when the run store is not configured.
	convergence *convergenceOrchestrator

	// unclaimedPurger periodically removes expired anonymous projects — their
	// unclaimed auth records and orphaned content-store projects. Nil when auth
	// / content stores are not configured.
	unclaimedPurger *jobs.UnclaimedProjectPurger

	// stepCompletionTracker monitors async automation steps. Nil when not configured.
	stepCompletionTracker *event.StepCompletionTracker

	// runHub manages SSE connections for live automation run updates. Always initialized.
	runHub *automationRunHub

	// changeRelay fans out platform events to attached web (SSE) and desktop
	// (gRPC WatchProject) clients so no view shows stale state on an external
	// change. Always initialized.
	changeRelay *event.ChangeRelay

	// actions counts the automation actions running in the background, so
	// Shutdown can wait for them instead of pulling their stores out from
	// under them.
	actions sync.WaitGroup

	// SyncCache is the optional Redis hash cache for sync diff engine (Bowrain AD-009).
	SyncCache bowsync.HashCache

	// ExtractionJobStore persists extraction job state. Nil when job system is not configured.
	ExtractionJobStore jobs.ExtractionJobStore

	// ExtractionQueue enqueues extraction job IDs. Nil when not configured.
	ExtractionQueue jobs.Queue

	// RecipeChangeStore holds recipe fields an approval wants set, waiting for
	// a pull to write them into a working tree. Nil when not configured.
	RecipeChangeStore RecipeChangeStore

	// ContextScanStore persists AI brand-scan job state (epic 016). Nil when not configured.
	ContextScanStore jobs.ContextScanJobStore

	// ContextScanQueue enqueues brand-scan job IDs. Nil when not configured.
	ContextScanQueue jobs.Queue

	// dashboardCache caches translation dashboard stats per project/stream.
	dashboardCache sync.Map // map[string]*dashboardCacheEntry

	// pulseCache caches Pulse public dashboard responses with TTL-based expiry.
	pulseCache *pulseCache

	// AgentStore persists @bravo agent conversations, messages, and config (Bowrain AD-016).
	// Nil when agent system is not configured.
	AgentStore platagent.AgentStore

	// AgentService orchestrates @bravo agent lifecycle (Bowrain AD-016).
	// Nil when agent system is not configured.
	AgentService *service.AgentService

	// BillingStore persists subscription and credit data (Bowrain AD-018).
	// Nil when billing is not configured.
	BillingStore billing.BillingStore

	// PlatformConfig is the instance-wide settings service (ctrl-managed): AI
	// provider/model, signups gate, maintenance, workspace defaults, global
	// feature toggles. Always non-nil (falls back to env Defaults when the DB
	// store is unavailable), so callers never nil-check.
	PlatformConfig *platformconfig.Service

	// StripeClient manages Stripe API interactions (Bowrain AD-018).
	// Nil when STRIPE_SECRET_KEY is not set.
	StripeClient *billing.StripeClient

	// PostHogClient captures product analytics events (Bowrain AD-018).
	// Nil when POSTHOG_API_KEY is not set.
	PostHogClient *analytics.PostHogClient

	// BillingHooks provides billing integration points for AI operations.
	// Nil-safe: all methods are no-ops on a nil receiver.
	BillingHooks *billing.UsageHooks

	// WebhookHandler processes Stripe webhook events (Bowrain AD-018).
	// Nil when Stripe is not configured.
	WebhookHandler *billing.WebhookHandler

	// AdminVerifier validates ID tokens from the admin OIDC realm (Bowrain AD-018).
	// Nil when admin OIDC is not configured.
	AdminVerifier *oidc.IDTokenVerifier

	// AdminAccessVerifier validates access tokens from the admin OIDC realm.
	// Keycloak access tokens use aud="account" so the standard ID-token
	// verifier rejects them. This verifier skips the audience check.
	AdminAccessVerifier *oidc.IDTokenVerifier

	// claimEmailLimiter caps outbound claim emails per client IP per hour so an
	// unauthenticated caller cannot use anonymous project creation as an
	// email-spam relay (see HandleCreateAnonymousProject). Initialized in
	// SetupRoutes from BOWRAIN_RL_CLAIM_EMAIL_PER_HOUR.
	claimEmailLimiter *hourlyIPLimiter
}

// NewServer creates a new Server with the given configuration.
// createEventBus selects the event bus backend based on configuration (Bowrain AD-012).
func createEventBus(cfg Config) platev.EventBus {
	if os.Getenv("BOWRAIN_EVENT_BACKEND") == "redis" && cfg.RedisURL != "" {
		bus, err := event.NewRedisEventBus(cfg.RedisURL, cfg.RedisPassword)
		if err != nil {
			slog.Warn("failed to create Redis event bus, falling back to in-memory", "error", err)
			return event.NewChannelEventBus()
		}
		slog.Info("event bus configured", "backend", "redis-streams")
		return bus
	}
	slog.Info("event bus configured", "backend", "in-memory")
	return event.NewChannelEventBus()
}

// platformConfigDefaults derives the instance-wide settings bootstrap defaults
// from process config, so an un-provisioned instance behaves exactly as before
// the platform_config store existed (DB values override these at runtime).
func platformConfigDefaults(cfg Config) platformconfig.Defaults {
	return platformconfig.Defaults{
		AIProvider:     cfg.PlatformProvider,
		AIDefaultModel: cfg.PlatformModel,
		AIBaseURL:      cfg.PlatformBaseURL,
		SignupsOpen:    true, // signups open unless an admin closes them
		DefaultPlan:    string(billing.PlanPro),
		TrialDays:      billing.DefaultTrialDays,
	}
}

func NewServer(cfg Config) *Server {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	connReg := platconn.NewRegistry()
	connector.RegisterServer(connReg, formatReg)

	// GitHub App for forge delivery: when configured, forge connectors may use
	// `auth: app` (per-installation tokens, no stored credentials) and the
	// app-level webhook endpoint accepts pushes for every installation. Wired
	// before connector rehydration so persisted app-mode configs come back.
	var githubApp *forge.GitHubApp
	if cfg.GitHubAppID != "" || cfg.GitHubAppPrivateKey != "" || cfg.GitHubAppWebhookSecret != "" {
		app, err := forge.NewGitHubApp(cfg.GitHubAppID, cfg.GitHubAppPrivateKey, cfg.GitHubAppWebhookSecret)
		if err != nil {
			// A half-configured app must be loud: silently ignoring it would
			// strand every auth:app connector.
			slog.Error("github app disabled (incomplete or invalid GITHUB_APP_* config)", "error", err)
		} else {
			githubApp = app
			connector.RegisterForgeApp(connReg, formatReg, app)
			slog.Info("github app enabled for forge delivery", "app_id", cfg.GitHubAppID)
		}
	}

	s := &Server{
		Config:          cfg,
		GitHubApp:       githubApp,
		FormatRegistry:  formatReg,
		ToolRegistry:    toolReg,
		ConnectorReg:    connReg,
		EventBus:        createEventBus(cfg),
		wsStores:        newWorkspaceStores(),
		collabHub:       newCollabHub(),
		notificationHub: newNotificationHub(),
		pulseCache:      newPulseCache(),
		posthogDemand:   newPostHogDemandCache(),
	}

	// Build the at-rest secrets cipher once (also seals the short-lived elevated
	// access token for passkey step-up). An invalid/empty key is pass-through.
	if cipher, cerr := crypto.NewCipher(cfg.SecretsKey); cerr != nil {
		slog.Error("invalid secrets key; secret values stored unencrypted", "error", cerr)
	} else {
		s.secretsCipher = cipher
	}

	// Initialize session state store (Redis or in-memory).
	if cfg.RedisURL != "" {
		rs, err := NewRedisSessionStore(cfg.RedisURL, cfg.RedisPassword)
		if err != nil {
			slog.Warn("failed to connect to Redis for session store, falling back to in-memory", "error", err)
			s.SessionStore = NewMemorySessionStore()
		} else {
			s.SessionStore = rs
			slog.Info("session store configured", "backend", "redis", "redis_url", cfg.RedisURL)
		}

		// Wire Redis hash cache for sync diff engine (Bowrain AD-009).
		redisOpts, err := redis.ParseURL(cfg.RedisURL)
		if err == nil {
			if cfg.RedisPassword != "" {
				redisOpts.Password = cfg.RedisPassword
			}
			redisClient := redis.NewClient(redisOpts)
			s.SyncCache = bowsync.NewRedisHashCache(redisClient, 30*time.Minute)
			slog.Info("sync hash cache configured", "backend", "redis")
		}
	} else {
		s.SessionStore = NewMemorySessionStore()
	}

	// Initialize credential store.
	s.CredentialStore = credentials.NewStore(credentials.DefaultPath())

	// Initialize email sender and mailer.
	s.initMailer(cfg)

	// Initialize the IdentityAdmin write-through client (email change) for the
	// configured provider. Either may be absent, which just disables
	// Bowrain-managed email change.
	switch cfg.AuthProvider {
	case AuthProviderCognito:
		// Cognito uses the ambient task role (no secrets) and derives the pool
		// ID + region from the OIDC issuer URL the server is already wired to.
		adminCfg, err := auth.CognitoConfigFromIssuer(cfg.OIDCIssuerURL)
		if err != nil {
			slog.Warn("cognito admin client disabled", "error", err)
		} else if client, err := auth.NewCognitoAdminClient(context.Background(), adminCfg); err != nil {
			slog.Warn("cognito admin client disabled", "error", err)
		} else {
			s.IdentityAdmin = client
			slog.Info("identity admin: cognito", "user_pool", adminCfg.UserPoolID, "region", adminCfg.Region)
		}
	default: // "keycloak" (and the empty default)
		if cfg.KeycloakAdminURL != "" {
			realm := cfg.KeycloakRealm
			if realm == "" {
				realm = "bowrain"
			}
			client, err := auth.NewKeycloakAdminClient(auth.KeycloakAdminConfig{
				BaseURL:      cfg.KeycloakAdminURL,
				Realm:        realm,
				ClientID:     cfg.KeycloakAdminClientID,
				ClientSecret: cfg.KeycloakAdminClientSecret,
			})
			if err != nil {
				slog.Warn("keycloak admin client disabled", "error", err)
			} else {
				s.IdentityAdmin = client
			}
		}
	}

	// Initialize the provider-neutral CredentialManager (self-service passkey
	// management) for the configured provider — the parallel of IdentityAdmin.
	// Cognito relays the WebAuthn ceremony in-app; Keycloak hands off to its
	// account console. Gated behind PasskeysEnabled so it ships dark (nil →
	// account-security endpoints report 503 → the web Security card hides) until
	// the RP ID + upstream-token retention are verified in the environment.
	if cfg.PasskeysEnabled {
		switch cfg.AuthProvider {
		case AuthProviderCognito:
			if adminCfg, err := auth.CognitoConfigFromIssuer(cfg.OIDCIssuerURL); err != nil {
				slog.Warn("cognito credential manager disabled", "error", err)
			} else if cm, err := auth.NewCognitoCredentialManager(context.Background(), adminCfg); err != nil {
				slog.Warn("cognito credential manager disabled", "error", err)
			} else {
				s.CredentialManager = cm
				slog.Info("credential manager: cognito (in-app passkeys)", "region", adminCfg.Region)
			}
		default: // "keycloak" (and the empty default)
			if cfg.OIDCIssuerURL != "" {
				// Prefer the browser-facing URL for the account-console deep link.
				issuer := cfg.OIDCPublicURL
				if issuer == "" {
					issuer = cfg.OIDCIssuerURL
				}
				s.CredentialManager = auth.NewKeycloakCredentialManager(issuer)
			}
		}
	}

	// Initialize stores from PostgreSQL DatabaseURL.
	if cfg.DatabaseURL != "" {
		pg, err := openPostgresStores(cfg.DatabaseURL)
		if err != nil {
			slog.Warn("failed to open PostgreSQL stores", "error", err)
		} else {
			s.ContentStore = pg.Content
			// Encrypt secret columns (connector credentials, AI provider keys) at
			// rest when a key is configured (the cipher was built above and also
			// seals the provider_configs store below).
			secretsCipher := s.secretsCipher
			if pgStore, ok := pg.Content.(*bstore.PostgresStore); ok {
				pgStore.SetSecretsCipher(secretsCipher)
			}
			s.Services = service.NewServices(pg.Content, connReg, formatReg, toolReg)
			s.JobStore = pg.Job
			s.ExtractionJobStore = pg.Extraction
			s.ContextScanStore = pg.ContextScan
			if pgStore, ok := pg.Content.(*bstore.PostgresStore); ok {
				s.RecipeChangeStore = pgStore
			}
			s.QuotaStore = pg.Quota
			s.SweepStore = pg.Sweep
			s.wsStores.pgDB = pg.DB
			pgSQL := pg.DB.DB // embedded *sql.DB
			// Workspace-scoped AI provider configs (Epic 004), sealed at rest with
			// the same cipher. Backs the /:ws/providers handlers.
			s.ProviderStore = bstore.NewProviderConfigStore(pgSQL, secretsCipher)
			// Workspace-scoped connector configs, sealed at rest with the same
			// cipher. Backs the /:ws/connectors handlers and boot rehydration.
			s.ConnectorConfigStore = bstore.NewConnectorConfigStore(pgSQL, secretsCipher)
			// GitHub App installation ownership — the tenancy record the
			// post-install setup endpoints gate on.
			s.ForgeInstallationStore = bstore.NewForgeInstallationStore(pgSQL)
			s.AuditLogger = event.NewAuditLogger(pgSQL, s.EventBus)
			if cfg.AuditRetentionDays > 0 {
				s.AuditRetention = event.NewAuditRetentionCleaner(
					s.AuditLogger, time.Duration(cfg.AuditRetentionDays)*24*time.Hour, 24*time.Hour)
			}
			if cfg.AuditSIEMWebhookURL != "" {
				s.SIEMExporter = event.NewSIEMExporter(s.EventBus, &event.HTTPSink{URL: cfg.AuditSIEMWebhookURL})
			}
			s.AutomationRuleStore = event.NewRuleStore(pgSQL)
			s.FlowDefStore = bstore.NewFlowDefStore(pgSQL)
			s.ReviewQueueStore = bstore.NewReviewQueueStore(pgSQL)
			s.SourceProposalStore = bstore.NewSourceProposalStore(pgSQL)
			s.NotificationStore = bstore.NewNotificationStore(pgSQL)
			s.ActivityStore = bstore.NewActivityStore(pgSQL)
			s.TaskStore = bstore.NewTaskStore(pgSQL)
			s.AutomationRunStore = bstore.NewAutomationRunStore(pgSQL)
			s.ConvergenceRunStore = bstore.NewConvergenceRunStore(pgSQL)
			s.PreferenceStore = bstore.NewPreferenceStore(pgSQL)
			s.DigestStore = bstore.NewDigestStore(pgSQL)
			s.VoiceStore = pg.Brand
			s.KnowledgeStore = pg.Knowledge
			s.GraphStore = pg.GraphStore
			s.AgentStore = pg.Agent
			s.BillingStore = pg.Billing
			s.PlatformConfig = platformconfig.NewService(pg.PlatformConfig, platformConfigDefaults(cfg))
			if cfg.JWTSecret != "" {
				s.AuthStore = pg.Auth
				s.Services.Auth = service.NewAuthService(pg.Auth, cfg.JWTSecret)
			}
		}
	}

	// Always provide a platform settings service; env-only (no persistence) when
	// no DB store initialized, so every reader can call it unconditionally.
	if s.PlatformConfig == nil {
		s.PlatformConfig = platformconfig.NewService(nil, platformConfigDefaults(cfg))
	}
	if err := s.PlatformConfig.Refresh(context.Background()); err != nil {
		slog.Warn("platform_config: initial load failed (serving env defaults)", "error", err)
	}
	// Apply the admin's circuit-breaker thresholds. A failed load above leaves
	// this at the compiled-in defaults, which is the correct fallback: breakers
	// must work on an un-provisioned instance, not wait for configuration.
	resilience.Default().Configure(s.PlatformConfig.ResilienceOverrides())

	// Initialize job queues if SQS is configured. The extraction queue feeds
	// the auto-extract automation (AD-013/AD-015): triggerAutoExtract enqueues
	// here and bowrain-worker's extraction worker consumes.
	switch {
	case jobs.SQSConfigured():
		sqsOpts := jobs.SQSOptionsFromEnv()
		if q, err := jobs.NewSQSQueue(context.Background(), sqsOpts, jobs.SQSTranslationQueue); err != nil {
			slog.Warn("failed to connect to SQS translation queue", "error", err)
		} else {
			s.JobQueue = q
		}
		if eq, err := jobs.NewSQSQueue(context.Background(), sqsOpts, jobs.SQSExtractionQueue); err != nil {
			slog.Warn("failed to connect to SQS extraction queue", "error", err)
		} else {
			s.ExtractionQueue = eq
		}
		if bq, err := jobs.NewSQSQueue(context.Background(), sqsOpts, jobs.SQSContextScanQueue); err != nil {
			slog.Warn("failed to connect to SQS brand-scan queue", "error", err)
		} else {
			s.ContextScanQueue = bq
		}
	}

	// Initialize blob storage (Bowrain AD-007).
	s.initBlobStore(cfg)

	// Wrap ContentStore with EventEmittingStore so all mutations publish events.
	if s.ContentStore != nil {
		s.ContentStore = event.NewEventEmittingStore(s.ContentStore, s.EventBus)
		// Update Services to use the wrapped store.
		if s.Services != nil {
			s.Services = service.NewServices(s.ContentStore, connReg, formatReg, toolReg)
			if s.AuthStore != nil && cfg.JWTSecret != "" {
				s.Services.Auth = service.NewAuthService(s.AuthStore, cfg.JWTSecret)
			}
		}
	}

	// Rehydrate persisted connectors into the (now-final) ConnectorService so
	// remote connectors and their credentials survive a restart. Fail-soft per
	// connector; never aborts boot. Startup wiring — no request context exists.
	s.rehydrateConnectors(context.Background())

	// Wire up automation engine with run manager (Bowrain AD-013).
	s.runHub = newAutomationRunHub()

	runManager := event.NewAutomationRunManager(s.AutomationRunStore, s.executeAutomationAction)
	s.AutomationEngine = event.NewAutomationEngine(s.EventBus, runManager.Execute)
	s.registerDefaultAutomations()

	// Server-side convergence (strategy 2026-07-kapi-up doc 03): the run engine
	// plus the on-push policy that replaces the retired auto-translate-on-push
	// automation. A completed push starts a convergence run for on-push
	// projects; manual projects converge only on demand (kapi up / REST).
	if s.ConvergenceRunStore != nil {
		s.convergence = newConvergenceOrchestrator(s)
		// Reconcile zombie runs left 'running' by a crash/restart before
		// accepting new work, so the one-run guard is never blocked by a dead
		// row (F3). Startup wiring — no request context exists yet.
		s.convergence.SweepInterruptedRuns(context.Background())
		s.subscribeConvergeOnPush()
		s.subscribeForgeDelivery()
		// Governed review → delivery continuation (RV-B): when a project's review
		// queue empties, review.completed starts a completing run so approved
		// content ships with no extra user action. Durable, like the two above.
		s.subscribeReviewCompletion()
		// Review loop fan-out to translations (RV-E): when a term/concept is marked
		// forbidden or a brand rule is promoted, re-check existing reviewed targets
		// and pull the violating ones back into the review queue. Durable, and
		// deliberately does not start a run — it only re-queues; the human (or bulk
		// approve-passing) re-drives completion through the path wired above.
		s.subscribeReviewRecheck()
	}

	// Unclaimed-project purge (epic 003 item 6): periodically remove expired
	// anonymous projects — both the unclaimed auth records
	// (auth.PurgeExpiredUnclaimed, which nothing else calls) and the orphaned
	// content-store projects the anonymous-create flow leaves behind
	// (handlers_claim.go). Claimed projects are guarded. Same ticker/goroutine
	// lifecycle as the convergence sweep and the other periodic cleaners; the
	// deletions run through the event-emitting content store wired above.
	if s.AuthStore != nil && s.ContentStore != nil && s.wsStores.pgDB != nil {
		s.unclaimedPurger = jobs.NewUnclaimedProjectPurger(jobs.UnclaimedPurgeConfig{
			Lister:   jobs.NewPgUnclaimedLister(s.wsStores.pgDB),
			Auth:     s.AuthStore,
			Projects: s.ContentStore,
			Interval: time.Hour,
		})
	}

	// Wire up activity recorder (Bowrain AD-014).
	if s.ActivityStore != nil {
		s.ActivityRecorder = event.NewActivityRecorder(s.ActivityStore, s.EventBus)
	}

	// Wire up the unified change-event relay. It attaches to the bus per
	// instance (SubscribeAll) and forwards events to locally-connected web
	// (SSE) and desktop (gRPC WatchProject) clients. The resolver lets
	// workspace-scoped SSE clients receive events for any of their projects.
	if s.EventBus != nil {
		var resolver event.ProjectWorkspaceResolver
		if s.ContentStore != nil {
			resolver = &contentStoreWorkspaceResolver{store: s.ContentStore}
		}
		s.changeRelay = event.NewChangeRelay(s.EventBus, resolver)

		// Fan-out: when instance-wide platform config changes in ctrl (on any
		// server/worker), every instance reloads its cached settings so the
		// change propagates cluster-wide without a redeploy. Subscribe (not
		// SubscribeGroup) so all instances react. Fine-to-miss: this only
		// refreshes a cache that is loaded fresh at startup and re-read on the
		// next change event — no state advances here, so an event lost during a
		// rollover cannot strand work.
		if s.PlatformConfig != nil && s.PlatformConfig.Persistent() {
			s.EventBus.Subscribe(platev.EventPlatformConfigChanged, func(platev.Event) {
				if err := s.PlatformConfig.Refresh(context.Background()); err != nil {
					slog.Warn("platform_config: reload after change event failed", "error", err)
					return
				}
				// Re-apply breaker thresholds from the new config. This is the
				// path that makes the ctrl kill switch immediate on every
				// instance rather than a redeploy away.
				resilience.Default().Configure(s.PlatformConfig.ResilienceOverrides())
			})
		}
	}

	// Wire up notification dispatcher (Bowrain AD-014).
	if s.NotificationStore != nil {
		// targetFn resolves which users should receive a project event notification.
		// It queries workspace members (excluding the actor who triggered the event).
		var targetFn event.NotificationTarget
		if s.AuthStore != nil {
			targetFn = s.resolveNotificationTargets
		}
		s.NotificationDispatcher = event.NewNotificationDispatcher(
			s.EventBus, s.NotificationStore, s.PreferenceStore, s, targetFn)

		// Wire immediate email delivery for high-priority notifications, and
		// the assignment email for tasks marked high or urgent.
		if s.Mailer != nil && s.AuthStore != nil {
			s.NotificationDispatcher.SetMailer(
				event.NewMailerAdapter(s.Mailer, s.AuthStore, cfg.AppPublicURL))
			// Tasks are keyed by workspace id and notification preferences by
			// slug, so the assignment email needs the lookup between them.
			s.NotificationDispatcher.SetWorkspaceSlugResolver(
				func(ctx context.Context, workspaceID string) (string, error) {
					ws, err := s.AuthStore.GetWorkspace(ctx, workspaceID)
					if err != nil {
						return "", err
					}
					if ws == nil {
						return "", nil
					}
					return ws.Slug, nil
				})
		}

		// Wire quiet hours enforcement for push/email suppression.
		if s.DigestStore != nil {
			s.NotificationDispatcher.SetDigestStore(s.DigestStore)
		}

		// A failed job summons the person waiting on it. Separate from the
		// dispatcher's generic event map on purpose: that map fans out to every
		// member of the job's project, and a failure goes to whoever asked for
		// the work. See job_failure_summons.go.
		s.subscribeJobFailures()
	}

	// Wire up digest workers (Bowrain AD-014).
	if s.DigestStore != nil && s.Mailer != nil {
		// resolveEmail converts a user ID to their email address.
		var resolveEmail event.UserEmailResolver
		if s.AuthStore != nil {
			resolveEmail = func(ctx context.Context, userID string) (string, error) {
				u, err := s.AuthStore.GetUser(ctx, userID)
				if err != nil {
					return "", err
				}
				return u.Email, nil
			}
		}

		// Daily digest runs every hour, checking for users due for daily digest.
		s.DailyDigestWorker = event.NewDigestWorker(
			s.NotificationStore, s.DigestStore, s.Mailer, resolveEmail,
			bstore.DigestDaily, 1*time.Hour,
		)
		s.DailyDigestWorker.Start()

		// Weekly digest runs every 6 hours, checking for users due for weekly digest.
		s.WeeklyDigestWorker = event.NewDigestWorker(
			s.NotificationStore, s.DigestStore, s.Mailer, resolveEmail,
			bstore.DigestWeekly, 6*time.Hour,
		)
		s.WeeklyDigestWorker.Start()
	}

	// Wire up deadline checker (Bowrain AD-014).
	if s.TaskStore != nil && s.NotificationDispatcher != nil {
		s.deadlineChecker = event.NewDeadlineChecker(s.TaskStore, s.NotificationDispatcher, 1*time.Hour)
		s.deadlineChecker.Start()
	}

	// Wire up progress milestone tracker (Bowrain AD-014).
	if s.ContentStore != nil && s.NotificationDispatcher != nil {
		s.progressTracker = event.NewProgressTracker(s.ContentStore, s.NotificationDispatcher, s.EventBus)
	}

	// Wire up push completion tracker (Bowrain AD-014).
	if s.EventBus != nil && s.JobStore != nil {
		s.pushCompletionTracker = event.NewPushCompletionTracker(
			s.EventBus, s.JobStore, s.ExtractionJobStore, s.ContentStore,
		)
		if s.AutomationRunStore != nil {
			s.pushCompletionTracker.SetRunStore(s.AutomationRunStore)
		}
	}

	// Wire up step completion tracker (Bowrain AD-013).
	if s.AutomationRunStore != nil && s.JobStore != nil {
		s.stepCompletionTracker = event.NewStepCompletionTracker(
			s.AutomationRunStore, s.JobStore, s.ExtractionJobStore,
		)
		if s.BillingHooks != nil {
			s.stepCompletionTracker.SetBillingHooks(s.BillingHooks)
		}
		if pgQuota, ok := s.QuotaStore.(*jobs.QuotaStoreDB); ok {
			s.stepCompletionTracker.SetQuotaStore(pgQuota)
		}
	}

	// Wire up run retention cleaner (Bowrain AD-013): delete runs older than 30 days, check daily.
	if s.AutomationRunStore != nil {
		_ = event.NewRunRetentionCleaner(s.AutomationRunStore, 30*24*time.Hour, 24*time.Hour)
	}

	// Wire up graph sync if graph store is available.
	if s.GraphStore != nil {
		s.graphSyncer = platgraph.NewGraphSyncer(s.GraphStore, s.EventBus)
	}
	// The context graph is rebuilt from what a push landed, so it stays in step
	// with the content the same way `kapi up` keeps a project's own graph in
	// step with its block cache.
	s.subscribeContextGraphOnPush()

	// Initialize MCP server for voice + agent tools when stores are available.
	if s.VoiceStore != nil {
		mcpCfg := mcpserver.Config{
			JWTSecret:     cfg.JWTSecret,
			OIDCIssuerURL: cfg.OIDCIssuerURL,
			// The RFC 9728 resource identifier is this server's own address,
			// which is what mcpserver.Config.PublicURL documents. It was being
			// given OIDCPublicURL — the identity provider's — so the metadata
			// announced the IdP as the protected resource. The authorization
			// server is named separately, from OIDCIssuerURL, just below it.
			PublicURL: cfg.AppPublicURL,
		}
		var mcpOpts []mcpserver.Option
		if s.wsStores != nil {
			mcpOpts = append(mcpOpts,
				mcpserver.WithMemoryResolver(&memoryResolverAdapter{ws: s.wsStores}),
				mcpserver.WithTermsResolver(&tbResolverAdapter{ws: s.wsStores}),
			)
		}
		if s.Services != nil && s.Services.Connector != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithConnectorResolver(s.Services.Connector))
		}
		// Enforce workspace membership on the workspace-scoped MCP tools so a
		// client-supplied workspace_id can't be used to reach another tenant.
		if s.AuthStore != nil && cfg.JWTSecret != "" {
			mcpOpts = append(mcpOpts, mcpserver.WithMembershipChecker(&mcpMembershipAdapter{auth: s.AuthStore}))
		}
		// Base rung of the voice resolution ladder: the workspace default
		// profile, resolved from the workspace record when no more-specific
		// (project/stream/collection) binding is bound on a scoring call.
		if s.AuthStore != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithWorkspaceDefault(&mcpWorkspaceDefaultAdapter{auth: s.AuthStore}))
		}
		if s.ToolRegistry != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithToolRegistry(s.ToolRegistry))
		}
		if s.PostHogClient != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithEventTracker(&eventTrackerAdapter{client: s.PostHogClient}))
		}
		ms, err := mcpserver.NewMCPServerWithStore(s.VoiceStore, s.ContentStore, mcpCfg, mcpOpts...)
		if err != nil {
			slog.Warn("failed to initialize MCP server", "error", err)
		} else {
			s.mcpServer = ms
		}
	}

	// Initialize agent service (Bowrain AD-016).
	if s.AgentStore != nil {
		s.AgentService = service.NewAgentService(s.AgentStore, s.EventBus)

		switch cfg.AgentRuntime {
		case "docker":
			// Direct mode: API server manages containers directly.
			if pool := s.buildAgentPool(); pool != nil {
				s.AgentService.SetPool(pool)
				slog.Info("agent pool initialized", "runtime", cfg.AgentRuntime)
			}
		case "":
			// No runtime — mock mode.
		default:
			slog.Warn("unknown agent runtime", "runtime", cfg.AgentRuntime)
		}
	}

	// Initialize Stripe client (Bowrain AD-018).
	if cfg.StripeSecretKey != "" {
		s.StripeClient = billing.NewStripeClient(cfg.StripeSecretKey)
		if cfg.StripeWebhookSecret != "" && s.BillingStore != nil {
			s.WebhookHandler = billing.NewWebhookHandler(s.BillingStore, cfg.StripeWebhookSecret)
			// Wire plan syncer so webhooks update workspace.plan.
			if s.AuthStore != nil {
				s.WebhookHandler.SetPlanSyncer(s.planSyncer())
			}
			// PostHog wiring deferred to after PostHog init below.
		}
		slog.Info("Stripe billing enabled")
	}

	// Initialize PostHog client (Bowrain AD-018).
	if cfg.PostHogAPIKey != "" {
		host := cfg.PostHogHost
		if host == "" {
			host = analytics.DefaultHost
		}
		phClient, err := analytics.NewPostHogClient(cfg.PostHogAPIKey, host)
		if err != nil {
			slog.Warn("failed to init PostHog client, analytics disabled", "error", err)
		} else {
			s.PostHogClient = phClient
			slog.Info("PostHog analytics enabled")
		}
	}

	// Wire PostHog to webhook handler now that both are initialized.
	if s.PostHogClient != nil && s.WebhookHandler != nil {
		s.WebhookHandler.SetEventTracker(s.PostHogClient)
	}

	// Wire PostHog into the service layer so domain events (flow runs,
	// connector publishes, workspace/member/project lifecycle) emit regardless
	// of the transport that invoked them (epic 018 workstream D).
	if s.PostHogClient != nil && s.Services != nil {
		s.Services.SetEventTracker(s.PostHogClient)
	}

	// Initialize admin OIDC verifier (Bowrain AD-018).
	if cfg.AdminOIDCIssuerURL != "" && cfg.AdminOIDCClientID != "" {
		// Startup wiring — no request context exists yet.
		ctx := context.Background()
		verifier, err := auth.NewOIDCVerifier(ctx, cfg.AdminOIDCIssuerURL, cfg.AdminOIDCClientID)
		if err != nil {
			slog.Warn("failed to init admin OIDC verifier, admin API disabled", "error", err)
		} else {
			s.AdminVerifier = verifier
			// Access token verifier skips audience check (Keycloak uses aud="account").
			accessVerifier, err := auth.NewOIDCAccessTokenVerifier(ctx, cfg.AdminOIDCIssuerURL)
			if err != nil {
				slog.Warn("failed to init admin access token verifier", "error", err)
			} else {
				s.AdminAccessVerifier = accessVerifier
			}
			slog.Info("admin OIDC verifier enabled", "issuer", cfg.AdminOIDCIssuerURL)
		}
	}

	// Build billing hooks for credit deduction + Stripe meter reporting (Bowrain AD-018).
	// Must be after Stripe client init so StripeClient is available.
	if s.BillingStore != nil {
		// Build billing notifier for email notifications.
		var notifier *billing.BillingNotifier
		if s.EmailSender != nil {
			notifier = &billing.BillingNotifier{
				Sender: s.EmailSender,
				Store:  s.BillingStore,
			}
		}

		// Wire notifier to webhook handler.
		if s.WebhookHandler != nil && notifier != nil {
			s.WebhookHandler.SetNotifier(notifier)
		}

		billingHooks := &billing.UsageHooks{
			Store:    s.BillingStore,
			Stripe:   s.StripeClient, // may be nil; hooks handle that
			Notifier: notifier,
		}

		// Wire owner email resolver for credit threshold notifications.
		if s.AuthStore != nil {
			resolver := &ownerEmailResolver{authStore: s.AuthStore}
			billingHooks.GetOwnerEmail = resolver.GetOwnerEmail
		}

		s.BillingHooks = billingHooks

		if s.AgentService != nil {
			s.AgentService.SetBillingHooks(billingHooks)
		}
	}

	return s
}

// configureIPExtractor pins echo's RealIP resolution to the client address
// appended by our trusted reverse proxy, closing the X-Forwarded-For spoofing
// hole that would otherwise let a client forge its per-IP identity.
//
// Deployment assumption: the server runs behind EXACTLY ONE trusted reverse
// proxy (Caddy on the box, or a cloud ALB). That proxy overwrites/append the
// real client IP as the right-most X-Forwarded-For hop; anything to its left is
// client-controlled and untrusted. echo.ExtractIPFromXFFHeader walks the header
// right-to-left, skipping trusted hops, and returns the first UNtrusted IP —
// i.e. exactly the address the proxy appended.
//
// Trust set: loopback + link-local + private ranges (echo defaults — covers a
// same-box Caddy or an ALB on a private subnet) plus any extra CIDRs from
// BOWRAIN_TRUSTED_PROXIES (comma/space-separated), for a proxy that presents a
// non-private source address. Invalid CIDRs are logged and skipped.
func configureIPExtractor(e *echo.Echo) {
	opts := []echo.TrustOption{
		echo.TrustLoopback(true),
		echo.TrustLinkLocal(true),
		echo.TrustPrivateNet(true),
	}
	for _, cidr := range parseTrustedProxyCIDRs(os.Getenv("BOWRAIN_TRUSTED_PROXIES")) {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			slog.Warn("ignoring invalid BOWRAIN_TRUSTED_PROXIES entry", "value", cidr, "error", err)
			continue
		}
		// Reject a default-route (/0) trust range. Trusting 0.0.0.0/0 or ::/0
		// makes EVERY hop trusted, so ExtractIPFromXFFHeader returns the
		// left-most (fully client-controlled) X-Forwarded-For entry — reopening
		// the XFF-spoofing hole this extractor exists to close (rate-limit key
		// forgery, audit-IP forgery, /metrics bypass). This is the one
		// misconfiguration that fails OPEN, so drop it with a loud warning.
		if ones, _ := ipNet.Mask.Size(); ones == 0 {
			slog.Warn("ignoring default-route BOWRAIN_TRUSTED_PROXIES entry (would trust all hops and reopen XFF spoofing)",
				"value", cidr)
			continue
		}
		opts = append(opts, echo.TrustIPRange(ipNet))
	}
	e.IPExtractor = echo.ExtractIPFromXFFHeader(opts...)
}

// parseTrustedProxyCIDRs splits a comma/whitespace-separated list of CIDR
// ranges, dropping empty tokens.
func parseTrustedProxyCIDRs(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// SetupRoutes registers all API routes on the Echo instance.
func (s *Server) SetupRoutes(e *echo.Echo) {
	// Trust the client IP only as far as the reverse proxy in front of us.
	// Every per-IP control below (login/refresh/device/anonymous/AI rate
	// limits, the claim-email cap, the /metrics private-IP gate) keys on
	// c.RealIP(); without a configured IPExtractor echo would trust the
	// leftmost, client-controlled X-Forwarded-For / X-Real-IP value, so any of
	// them is bypassable by spoofing that header. Configuring the extractor
	// pins RealIP to the address our trusted proxy appended (the right-most XFF
	// hop), which the client cannot forge.
	configureIPExtractor(e)

	// Single, consistent error handler: unified ErrorResponse envelope, a
	// "reference" (request ID) on every error, no internal-detail leakage on
	// 5xx, and the Sentry capture seam. Replaces Echo's default handler.
	e.HTTPErrorHandler = s.httpErrorHandler

	// Middleware — order matters:
	// 1. Request ID (propagate/generate correlation ID)
	// 2. Structured request logging (slog-echo, includes request_id)
	// 3. Prometheus metrics
	// 4. Sentry transaction — after the request id it tags itself with, and
	//    before Recover so a recovered panic is still recorded on it
	// 5. Recovery, body limit, CORS
	// 6. Security response headers
	e.Use(observe.RequestIDMiddleware())
	e.Use(slogecho.NewWithConfig(slog.Default(), slogecho.Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
		WithRequestID:    true,
		Filters: []slogecho.Filter{
			slogecho.IgnorePath("/api/v1/health", "/metrics"),
		},
	}))
	// Response-size histograms are opted in for the two payload-hot endpoints
	// (workspace project list, translation dashboard) so payload-shape
	// regressions there show up in Prometheus alongside the duration series.
	e.Use(observe.MetricsMiddleware(
		"/api/v1/:ws/projects",
		"/api/v1/:ws/:id/dashboard/:ref",
	))
	// One Sentry transaction per request. sentry-go does not auto-instrument, so
	// without this the traces sample rate the deployment sets samples nothing —
	// see observe/tracing.go. No-op when Sentry is unconfigured.
	e.Use(observe.TracingMiddleware())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("50M"))
	e.Use(middleware.CORSWithConfig(s.corsConfig()))
	// Baseline response headers (nosniff, frame policy, referrer policy, HSTS
	// over TLS, and a content policy for this server's own responses). Runs
	// after CORS so a preflight carries them too, and before every handler, so a
	// handler that needs a different policy — the document previews, the SPA in
	// development — overrides it rather than adding it.
	e.Use(securityHeaders())

	// Per-IP throttle knobs (env-overridable) used across the abuse-prone
	// public and pre-auth routes below.
	rl := loadRateLimitDefaults()
	s.claimEmailLimiter = newHourlyIPLimiter(rl.ClaimEmailPerHour)

	// Prometheus metrics endpoint. Gated (not public): a bearer token when
	// BOWRAIN_METRICS_TOKEN is set, otherwise restricted to loopback/private
	// source IPs so an in-cluster scraper works while external clients (via the
	// ingress, presenting a public RealIP) are refused.
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()),
		MetricsAccessMiddleware(os.Getenv("BOWRAIN_METRICS_TOKEN")))

	// pprof endpoints are served on a separate localhost-only listener
	// (see observe.StartPprofServer in main.go).

	// API v1 routes
	v1 := e.Group("/api/v1")

	// Public endpoints (no auth).
	v1.GET("/health", s.HandleHealth)
	v1.GET("/ready", s.HandleReady)
	v1.GET("/info", s.HandleInfo)
	v1.GET("/config", s.HandleGetPublicConfig)

	// Public per-project ship-status feed (D-3): the hosted twin of `kapi status
	// --ship --emit ship.json`, byte-shape-identical so @neokapi/i18n-react's
	// loadShipStatus consumes it without a second code path. No auth — a language
	// picker on a public site reads it — and off unless the project opts in
	// (ShipFeedProperty), because a public feed is a disclosure. `?stream=`
	// selects a stream; the project's default otherwise.
	v1.GET("/projects/:id/ship.json", s.HandlePublicShipManifest)

	// Pulse public activity dashboard (Bowrain AD-017). Unmounted by default —
	// the platform's public surface is on-brand authoring, not l10n
	// gamification. BOWRAIN_PULSE_ENABLED=true re-mounts the routes (and their
	// access middleware); the handlers and cache stay in the tree so the OSS
	// program can revive the surface.
	// No auth required when mounted — access gated by workspace/project
	// dashboard_visibility.
	if s.Config.PulseEnabled {
		s.registerPulseRoutes(v1)
	}
	// Authenticated mode: auth routes, protected endpoints, workspace management.
	if s.Config.JWTSecret != "" {
		// Anonymous project creation (no auth required) — IP-throttled: it
		// writes a project + claim token (and optionally sends an email) with no
		// credential, so it is a prime abuse target.
		v1.POST("/projects/anonymous", s.HandleCreateAnonymousProject,
			RateLimitByIP(rl.AnonPerMin, rl.AnonBurst))

		// Shared per-IP throttle for the unauthenticated / pre-auth token
		// endpoints (login, refresh, and every leg of the device flow — start,
		// verify, poll). One bucket across them so a client cannot fan a
		// brute-force / token-mint flood across endpoints; verify and poll are
		// in it because together they are what turns a guessed user code into a
		// session. OIDC redirect callbacks are intentionally excluded — they
		// carry provider state and must not be dropped.
		authThrottle := newIPThrottle(rl.AuthPerMin, rl.AuthBurst)
		authLimit := authThrottle.middleware(nil)

		// The device poll is a loop, not a one-shot: the CLI calls it every few
		// seconds until the browser leg finishes. A throttled poll therefore has
		// to say "wait longer" in the device flow's own vocabulary (RFC 8628
		// slow_down); the generic throttle error reads as fatal to the client,
		// which would abandon a sign-in that was going fine. Same bucket, so the
		// throttle still holds — only the way it answers changes.
		pollLimit := authThrottle.middleware(func(c echo.Context) error {
			c.Response().Header().Set("Retry-After", "5")
			return apiErr(c, http.StatusTooManyRequests, "slow_down")
		})

		// Public auth routes (no token required)
		authGroup := v1.Group("/auth")
		authGroup.POST("/device/start", s.HandleDeviceAuthStart, authLimit)
		authGroup.POST("/device/poll", s.HandleDeviceAuthPoll, pollLimit)
		authGroup.POST("/refresh", s.HandleTokenRefresh, authLimit)
		authGroup.GET("/login", s.HandleAuthLogin, authLimit)
		authGroup.GET("/callback", s.HandleAuthCallback)
		authGroup.POST("/callback", s.HandleAuthCallback)
		authGroup.GET("/desktop/login", s.HandleDesktopLogin)
		authGroup.GET("/desktop/callback", s.HandleDesktopCallback)

		// Device verification page (user opens in browser)
		authGroup.GET("/device/verify", s.HandleAuthCallback, authLimit)
		authGroup.POST("/device/verify", func(c echo.Context) error {
			return s.handleDeviceVerification(c, c.FormValue("user_code"))
		}, authLimit)
		authGroup.GET("/device/callback", s.HandleDeviceAuthCallback)

		if s.Config.AllowInsecureDeviceAuth {
			slog.Warn("device authorization mounted with INSECURE direct approval: " +
				"/device/verify approves a pending device for the identity the request names, " +
				"with no identity-provider check. Do NOT use in production.")
		}

		// Back-channel logout (called server-to-server by Keycloak, unauthenticated)
		authGroup.POST("/backchannel-logout", s.HandleBackChannelLogout)

		// Slug availability check — public; reused by onboarding and the
		// profile rename form. Format and reservation rules enforced here
		// rather than the client.
		authGroup.GET("/check-slug", s.HandleCheckSlug)

		// Display-only identity probe — intentionally OUTSIDE the auth-required
		// group: it resolves the session cookie itself and always answers 200
		// ({authenticated:false} when there is none). The same-site marketing
		// landing reads it cross-origin (credentialed CORS) to render the
		// signed-in CTA. It returns only display fields — never a token.
		authGroup.GET("/whoami", s.HandleAuthWhoami)

		// Email-change confirmation — token-authenticated, intentionally
		// outside the JWT-protected group so the link works in any browser
		// session (incl. one without an active Bowrain login).
		authGroup.POST("/email/confirm", s.HandleConfirmEmailChange)

		// Protected auth routes (require valid token)
		authProtected := authGroup.Group("")
		authProtected.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		authProtected.GET("/me", s.HandleAuthMe)
		authProtected.GET("/me/onboarding", s.HandleGetOnboarding)
		authProtected.POST("/me/onboarding", s.HandleCompleteOnboarding)
		authProtected.POST("/me/email", s.HandleRequestEmailChange)
		authProtected.POST("/logout", s.HandleAuthLogout)
		authProtected.POST("/token/exchange", s.HandleTokenExchange)

		// Account self-service (Security section): passkey management via the
		// provider-neutral CredentialManager. Cookie-authenticated (CSRF gate on
		// the POST/DELETE routes is enforced by AuthMiddleware); state-changing
		// IdP calls run server-side with a user-scoped access token minted on
		// demand from the retained upstream refresh token — no IdP token reaches
		// the browser.
		accountGroup := v1.Group("/account")
		accountGroup.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		accountGroup.GET("/security", s.HandleAccountSecurity)
		// Step-up elevation for credential management (top-level browser GET
		// navigations — cookie-authenticated, CSRF-exempt).
		accountGroup.GET("/security/elevate", s.HandleSecurityElevate)
		accountGroup.GET("/security/elevate/callback", s.HandleSecurityElevateCallback)
		accountGroup.GET("/passkeys", s.HandleListPasskeys)
		accountGroup.POST("/passkeys/register/start", s.HandleRegisterStart)
		accountGroup.POST("/passkeys/register/finish", s.HandleRegisterFinish)
		accountGroup.DELETE("/passkeys/:id", s.HandleDeletePasskey)

		// Per-IP throttles for invite and AI-consuming routes (env-overridable).
		// Shared instances so all invite routes / all AI routes each draw from a
		// single per-IP bucket.
		inviteLimit := RateLimitByIP(rl.InvitePerMin, rl.InviteBurst)
		aiLimit := RateLimitByIP(rl.AIPerMin, rl.AIBurst)

		// Project claim and invite acceptance (auth required, no workspace).
		jwtProtected := v1.Group("")
		jwtProtected.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		jwtProtected.POST("/projects/claim", s.HandleClaimProject)
		jwtProtected.POST("/join/:code", s.HandleAcceptInvite, inviteLimit)

		// Flat sync routes for unclaimed projects (claim-token or JWT auth).
		// Bowrain AD-011: /api/v1/projects/:id/sync/:ref/*
		if s.AuthStore != nil {
			syncRateLimit := RateLimitSyncPush(10, 3)
			// Chunk uploads need their own, more generous per-project bucket: one
			// push streams many 2 MiB chunks, so the commit cap would throttle a
			// legitimate large push. Without any cap an authenticated client can
			// upload chunks that are never committed (no manifest ⇒ the worker's
			// sweep never sees them) and grow blob storage unbounded; this bounds
			// the rate. The durable backstop is a bucket TTL in bowrain-infra.
			chunkRateLimit := RateLimitSyncPush(600, 120)
			flatSyncGroup := v1.Group("/projects/:id/sync/:ref")
			flatSyncGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			// Resolve permissions (claim-token grant, project membership, or the
			// project's workspace role) so the sync handlers' permission checks
			// enforce rather than fail open.
			flatSyncGroup.Use(s.ProjectAccessMiddleware())
			flatSyncGroup.GET("/pull", s.HandleSyncPull)
			flatSyncGroup.GET("/ref", s.HandleSyncRef)
			flatSyncGroup.GET("/blocks", s.HandleSyncGetBlocks)
			flatSyncGroup.GET("/status", s.HandleSyncPushStatus)
			flatSyncGroup.GET("/tree", s.HandleSyncTree)
			flatSyncGroup.GET("/blobs/:key", s.HandleBlobDownload)
			flatSyncGroup.POST("/push/init", s.HandleSyncPushInit)
			flatSyncGroup.POST("/push/uploads", s.HandleSyncPushUploads)
			flatSyncGroup.POST("/push/commit", s.HandleSyncPushCommit, syncRateLimit)
			flatSyncGroup.PUT("/push/chunks/:uploadId/:chunkIndex", s.HandleSyncProxyChunkUpload, chunkRateLimit)
			// Retired with the protocol it belonged to, and still routed so an
			// old plugin is told to upgrade instead of reading a bare 404.
			flatSyncGroup.POST("/push/diff", s.HandleSyncPushDiffRetired)

			// Flat project-scoped convergence + settings for unclaimed projects:
			// /api/v1/projects/:id/convergence/... and /projects/:id/settings.
			flatProjectGroup := v1.Group("/projects")
			flatProjectGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			flatProjectGroup.Use(s.ProjectAccessMiddleware())
			s.registerConvergenceRoutes(flatProjectGroup)
		}

		// Workspace collection routes: list and create (require auth).
		// Bowrain AD-011: /api/v1/workspaces (collection noun for list/create)
		wsCollectionGroup := v1.Group("/workspaces")
		wsCollectionGroup.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		wsCollectionGroup.POST("", s.HandleCreateWorkspace)
		wsCollectionGroup.GET("", s.HandleListWorkspaces)

		// Workspace-specific routes: bare slug at /:ws (require auth + membership).
		// Bowrain AD-011: /api/v1/:ws (bare workspace slug)
		wsSpecific := v1.Group("/:ws")
		wsSpecific.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		if s.AuthStore != nil {
			wsSpecific.Use(WorkspaceAccessMiddleware(s.AuthStore))
			wsSpecific.Use(MonthlyAllocationMiddleware(s.BillingStore))
			// Resolve effective feature overrides (instance-wide global flags
			// under per-workspace admin grants) so PlanGuard and client-surfaced
			// entitlements honor both.
			wsSpecific.Use(FeatureOverridesMiddleware(s.BillingStore, s.PlatformConfig.GlobalFeatures))
		}
		wsSpecific.GET("", s.HandleGetWorkspace)
		wsSpecific.PUT("", s.HandleUpdateWorkspace)
		wsSpecific.PATCH("", s.HandleUpdateWorkspace)
		wsSpecific.DELETE("", s.HandleDeleteWorkspace)
		wsSpecific.GET("/members", s.HandleListMembers)
		wsSpecific.POST("/members", s.HandleAddMember)
		wsSpecific.PUT("/members/:uid/role", s.HandleUpdateMemberRole)
		wsSpecific.DELETE("/members/:uid", s.HandleRemoveMember)

		// Unified change-event stream (SSE) — keeps web views fresh on any
		// external change (other users, kapi push, connector sync, automations).
		// Optional ?project=<id> narrows the stream to one project.
		wsSpecific.GET("/events", s.HandleWorkspaceEventsSSE)

		// Invite routes (workspace-scoped, admin/owner only). Creation is
		// IP-throttled — it can trigger an outbound invite email per call.
		wsSpecific.POST("/invites", s.HandleCreateInvite, inviteLimit)
		wsSpecific.GET("/invites", s.HandleListInvites)
		wsSpecific.DELETE("/invites/:id", s.HandleDeleteInvite)

		// Role template routes (workspace-scoped, admin/owner only for mutations).
		wsSpecific.GET("/roles", s.HandleListRoleTemplates)
		wsSpecific.POST("/roles", s.HandleCreateRoleTemplate)
		wsSpecific.PUT("/roles/:rid", s.HandleUpdateRoleTemplate)
		wsSpecific.DELETE("/roles/:rid", s.HandleDeleteRoleTemplate)

		// Governance: groups, deny rules, role overrides, separation-of-duties
		// (workspace-scoped; mutations admin/owner only).
		wsSpecific.GET("/groups", s.HandleListGroups)
		wsSpecific.POST("/groups", s.HandleCreateGroup)
		wsSpecific.DELETE("/groups/:gid", s.HandleDeleteGroup)
		wsSpecific.GET("/groups/:gid/members", s.HandleListGroupMembers)
		wsSpecific.POST("/groups/:gid/members", s.HandleAddGroupMember)
		wsSpecific.DELETE("/groups/:gid/members/:uid", s.HandleRemoveGroupMember)
		wsSpecific.GET("/groups/:gid/bindings", s.HandleListGroupBindings)
		wsSpecific.POST("/groups/:gid/bindings", s.HandleAddGroupBinding)
		wsSpecific.DELETE("/groups/:gid/bindings/:bid", s.HandleRemoveGroupBinding)
		wsSpecific.GET("/deny-rules", s.HandleListDenyRules)
		wsSpecific.POST("/deny-rules", s.HandleCreateDenyRule)
		wsSpecific.DELETE("/deny-rules/:rid", s.HandleDeleteDenyRule)
		wsSpecific.GET("/role-overrides", s.HandleListRoleOverrides)
		wsSpecific.PUT("/role-overrides/:role", s.HandleSetRoleOverride)
		wsSpecific.GET("/sod", s.HandleGetSoD)
		wsSpecific.PUT("/sod", s.HandleSetSoD)

		// API token routes (workspace-scoped, requires Pro+ plan).
		tokenGroup := wsSpecific.Group("/tokens")
		tokenGroup.Use(billing.PlanGuard(billing.FeatureAPIAccess))
		tokenGroup.POST("", s.HandleCreateToken)
		tokenGroup.GET("", s.HandleListTokens)
		tokenGroup.DELETE("/:id", s.HandleDeleteToken)

		s.registerWorkspaceContentRoutes(wsSpecific, aiLimit)

		// @bravo agent routes (Bowrain AD-016) with QuotaGuard for credit-consuming operations.
		s.registerBravoRoutes(wsSpecific, aiLimit)

		// Billing routes (Bowrain AD-018, workspace-scoped)
		billingGroup := wsSpecific.Group("/billing")
		billingGroup.GET("", s.HandleGetBilling)
		billingGroup.GET("/plans", s.HandleListPlans)
		billingGroup.GET("/usage", s.HandleGetBillingUsage)
		billingGroup.GET("/model-usage", s.HandleGetBillingModelUsage)
		billingGroup.GET("/invoices", s.HandleGetInvoices)
		// The three POSTs each reach out to Stripe and mint an object (a Checkout
		// session, a portal session, and — on first checkout — a Customer that is
		// only persisted later by the webhook). An owner looping any of them would
		// accumulate orphaned Stripe objects and API cost, so throttle the
		// money-moving verbs. The GETs above power the billing page and stay
		// unthrottled.
		billingGroup.POST("/checkout", s.HandleCreateCheckout, aiLimit)
		billingGroup.POST("/portal", s.HandleCreatePortal, aiLimit)
		billingGroup.POST("/buy-credits", s.HandleBuyCredits, aiLimit)
	}

	// Stripe webhook (no auth, signature-verified) (Bowrain AD-018).
	e.POST("/api/webhooks/stripe", s.HandleStripeWebhook)
	// Forge push webhooks (GitHub/GitLab). Unauthenticated like the Stripe
	// hook: verified by the connector's webhook secret, not a session.
	e.POST("/api/webhooks/forge/:configID", s.HandleForgeWebhook)
	// The app-level variant: one endpoint for every repository the GitHub App
	// is installed on, verified with the app's webhook secret.
	e.POST("/api/webhooks/github-app", s.HandleGitHubAppWebhook)

	// Admin routes (admin realm auth) (Bowrain AD-018).
	//
	// Authorization model:
	//   - When an admin-realm OIDC verifier is configured, gate every admin
	//     route with AdminGuard (validates an admin-realm token).
	//   - Otherwise the routes are NOT mounted. The plain user-JWT fallback is
	//     only used when explicitly opted in via AllowInsecureAdminAuth (local
	//     dev) — because that fallback performs NO admin-role check and would
	//     otherwise let any regular-user JWT/session reach impersonation,
	//     credit-grant, and plan-management handlers (privilege escalation).
	//     Critically, AdminVerifier==nil && JWTSecret!="" is the normal
	//     production state for user auth and also the state after a transient
	//     admin-OIDC discovery failure, so failing open there is unacceptable.
	mountAdmin := false
	var adminMiddleware echo.MiddlewareFunc
	switch {
	case s.AdminVerifier != nil:
		accessVerifier := s.AdminAccessVerifier
		if accessVerifier == nil {
			accessVerifier = s.AdminVerifier
		}
		// Accept either the admin BFF session cookie (with CSRF) or an admin-realm
		// Bearer token. The cookie path is the ctrl SPA's default; the Bearer
		// fallback keeps any token-based admin client working.
		adminMiddleware = s.adminAuthMiddleware(billing.AdminGuard(s.AdminVerifier, accessVerifier))
		mountAdmin = true
	case s.Config.AllowInsecureAdminAuth && s.Config.JWTSecret != "":
		slog.Warn("admin API mounted with INSECURE user-JWT auth (no admin-role check); " +
			"set ADMIN_OIDC_ISSUER_URL to enable AdminGuard. Do NOT use in production.")
		adminMiddleware = AuthMiddleware(s.Config.JWTSecret, s.AuthStore)
		mountAdmin = true
	default:
		if s.Config.JWTSecret != "" {
			slog.Info("admin API disabled: no admin OIDC verifier configured " +
				"(set ADMIN_OIDC_ISSUER_URL, or AllowInsecureAdminAuth for local dev)")
		}
	}
	// Admin BFF auth (Bowrain AD-018): the ctrl SPA runs the admin-realm OIDC
	// flow in the browser and POSTs the resulting id_token here; the server
	// verifies it and sets an HttpOnly admin session cookie (no admin token in
	// the browser). These establish/clear the session, so they are
	// unauthenticated and registered OUTSIDE the admin guard. Only wired when an
	// admin OIDC verifier exists to verify the exchanged token.
	if s.AdminVerifier != nil {
		e.POST("/api/admin/auth/exchange", s.HandleAdminAuthExchange)
		e.POST("/api/admin/auth/logout", s.HandleAdminAuthLogout)
	}

	if mountAdmin {
		adminGroup := e.Group("/api/admin", adminMiddleware)
		adminGroup.GET("/auth/me", s.HandleAdminAuthMe)
		adminGroup.GET("/workspaces", s.HandleAdminListWorkspaces)
		adminGroup.GET("/workspaces/:id", s.HandleAdminGetWorkspace)
		adminGroup.PUT("/workspaces/:id/plan", s.HandleAdminUpdatePlan)
		adminGroup.POST("/workspaces/:id/credits", s.HandleAdminGrantCredits)
		adminGroup.GET("/workspaces/:id/feature-overrides", s.HandleAdminGetFeatureOverrides)
		adminGroup.PUT("/workspaces/:id/feature-overrides", s.HandleAdminSetFeatureOverrides)
		adminGroup.GET("/workspaces/:id/notes", s.HandleAdminGetNotes)
		adminGroup.POST("/workspaces/:id/notes", s.HandleAdminAddNote)
		adminGroup.GET("/workspaces/:id/ledger", s.HandleAdminGetLedger)
		adminGroup.GET("/workspaces/:id/model-usage", s.HandleAdminGetModelUsage)
		adminGroup.POST("/workspaces/:id/impersonate", s.HandleAdminImpersonate)
		adminGroup.POST("/workspaces/:id/members", s.HandleAdminAddMember)
		adminGroup.GET("/users", s.HandleAdminListUsers)
		adminGroup.GET("/users/:id", s.HandleAdminGetUser)
		adminGroup.GET("/metrics", s.HandleAdminGetMetrics)
		adminGroup.GET("/health", s.HandleAdminHealth)
		adminGroup.GET("/events", s.HandleAdminListEvents)
		adminGroup.GET("/upsells", s.HandleAdminGetUpsells)
		adminGroup.GET("/overrides", s.HandleAdminListOverrides)
		adminGroup.GET("/slug-reservations", s.HandleAdminListSlugReservations)
		adminGroup.POST("/slug-reservations/release", s.HandleAdminReleaseSlugReservation)

		// Instance-wide platform configuration (AI provider/models, signups gate,
		// maintenance banner, new-workspace defaults, global feature flags).
		adminGroup.GET("/platform", s.HandleAdminGetPlatformConfig)
		adminGroup.PUT("/platform", s.HandleAdminUpdatePlatformConfig)
	}

	// MCP server (voice resources, tools, prompts via Streamable HTTP).
	if s.mcpServer != nil {
		s.mcpServer.RegisterRoutes(e)
	}

	// Web UI static file serving (development and E2E only).
	// A single handler serves static files first and falls back to index.html
	// for SPA client-side routing. Using two separate handlers (e.Static + e.GET)
	// would conflict because Echo overwrites the first GET /* with the second.
	pulseUIDir := ""
	if s.Config.PulseEnabled {
		pulseUIDir = s.Config.PulseUIDir
	}
	if s.Config.WebUIDir != "" || pulseUIDir != "" {
		e.GET("/*", func(c echo.Context) error {
			// Host-based routing: serve Pulse SPA for pulse.* subdomain
			// (only when the Pulse surface is enabled).
			host := c.Request().Host
			if pulseUIDir != "" && strings.HasPrefix(host, "pulse.") {
				return serveSPAFile(c, pulseUIDir)
			}
			// Default: serve main web UI.
			if s.Config.WebUIDir != "" {
				return serveSPAFile(c, s.Config.WebUIDir)
			}
			return c.String(http.StatusNotFound, "not found")
		})
	}
}

// serveSPAFile serves a static file from the given directory, falling back to index.html
// for SPA client-side routing.
//
// This path exists only in development and E2E; production builds the SPAs in
// CI and serves them from the CDN, which owns their content policy. The
// server's own policy is cleared here so the document behaves the same in both
// places — see clearCSPForSPA.
func serveSPAFile(c echo.Context, dir string) error {
	clearCSPForSPA(c)

	reqPath := c.Param("*")
	if reqPath == "" {
		reqPath = "index.html"
	}
	// Resolve both base and target to absolute paths to prevent path traversal.
	baseDir, err := filepath.Abs(dir)
	if err != nil {
		return c.String(http.StatusNotFound, "not found")
	}
	filePath := filepath.Join(baseDir, filepath.Clean(reqPath))
	if !strings.HasPrefix(filePath, baseDir+string(filepath.Separator)) && filePath != baseDir {
		return c.String(http.StatusNotFound, "not found")
	}
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		return c.File(filePath)
	}
	return c.File(filepath.Join(baseDir, "index.html"))
}

// registerWorkspaceContentRoutes registers all workspace-scoped content routes
// on the given route group (mounted at /:ws).
//
// Bowrain AD-011 URL patterns:
//   - Workspace-level: /:ws/translation-memory, /:ws/terms, /:ws/providers, etc.
//   - Project collection: /:ws/projects (list/create)
//   - Project-specific: /:ws/:id (bare slug, get/update/delete)
//   - Ref-scoped content: /:ws/:id/blocks/:ref, /:ws/:id/sync/:ref, etc.
//
// Note: During migration, handlers still extract project ID via c.Param("pid")
// or c.Param("id"). Slug-to-ID resolution middleware will be added separately.
//
// aiLimit is a per-IP throttle applied to the AI-consuming routes (ai-translate,
// voice check) so a single client cannot drive unbounded provider spend.
func (s *Server) registerWorkspaceContentRoutes(g *echo.Group, aiLimit echo.MiddlewareFunc) {
	// Apply project-level permission resolution for routes with :pid or :id params.
	// The middleware is a no-op when no project ID is present (workspace-scoped routes).
	if s.AuthStore != nil {
		g.Use(s.ProjectAccessMiddleware())
	}
	// Narrow permissions based on API token scopes (Layer 2).
	g.Use(ScopeRestrictionMiddleware())
	// Narrow permissions based on session grants for @bravo/MCP (Layer 3).
	if s.SessionStore != nil {
		g.Use(SessionGrantMiddleware(s.SessionStore))
	}

	// -----------------------------------------------------------------------
	// Workspace-level resources (no project context)
	// -----------------------------------------------------------------------

	// content memory CRUD — Bowrain AD-011: /:ws/translation-memory
	g.GET("/translation-memory", s.HandleGetMemoryEntries)
	g.GET("/translation-memory/count", s.HandleGetMemoryCount)
	g.POST("/translation-memory", s.HandleAddMemoryEntry)
	g.PUT("/translation-memory/:eid", s.HandleUpdateMemoryEntry)
	g.DELETE("/translation-memory/:eid", s.HandleDeleteMemoryEntry)
	g.POST("/translation-memory/bulk-delete", s.HandleBulkDeleteMemoryEntries)

	// Brand knowledge graph — Bowrain AD-021. The concept API (/:ws/concepts)
	// is the workspace terminology surface; it replaces the former /:ws/terms
	// routes, and every consumer (web, desktop, Pulse, MCP) uses it. Change-sets
	// (/:ws/changesets) carry the governed-edit lifecycle.
	s.registerConceptRoutes(g)
	s.registerChangesetRoutes(g)

	// Provider configs — Bowrain AD-011: /:ws/providers
	g.GET("/providers", s.HandleListProviderConfigs)
	g.POST("/providers", s.HandleSaveProviderConfig)
	g.DELETE("/providers/:id", s.HandleDeleteProviderConfig)
	g.POST("/providers/test", s.HandleTestProviderConfig)

	// Connectors — Bowrain AD-011: /:ws/connectors (workspace-scoped, not public)
	g.GET("/connectors", s.HandleListActiveConnectors)
	// GitHub App post-install setup: mint the state that ties an installation
	// to this workspace, redeem it on the way back, then list the
	// installation's repositories and bind one to a project (creating an
	// auth:app forge connector). Everything below the claim requires the
	// workspace to already own the installation.
	g.GET("/github/setup-state", s.HandleGitHubSetupState)
	g.POST("/github/installations/:installationID/claim", s.HandleClaimInstallation)
	g.GET("/github/installations/:installationID/repositories", s.HandleListInstallationRepos)
	g.POST("/github/installations/:installationID/repositories", s.HandleBindInstallationRepo)
	g.GET("/github/installations/:installationID/repositories/:owner/:name/detect", s.HandleDetectInstallationRepo)
	g.POST("/connectors", s.HandleAddConnector)
	g.PUT("/connectors/:id", s.HandleUpdateConnector)
	g.DELETE("/connectors/:id", s.HandleRemoveConnector)
	g.GET("/connectors/status", s.HandleConnectorStatusBatch)
	g.GET("/connectors/:id/status", s.HandleConnectorStatus)
	g.GET("/connectors/:id/content", s.HandleConnectorContent)
	g.POST("/connectors/:id/fetch", s.HandleFetch)
	g.POST("/connectors/:id/publish", s.HandlePublish)

	// Governance profiles — the points the workspace's content occupies in the
	// context space, aggregated read-only from collections, voices and
	// vocabulary: /:ws/profiles
	g.GET("/profiles", s.HandleListContextProfiles)

	// Channel-slug equivalence the workspace observed between projects and
	// proposes without resolving: /:ws/context/channel-proposals. The judge
	// write settles a proposal — accepted or dismissed — and still rewrites no
	// project's slug.
	g.GET("/context/channel-proposals", s.HandleListChannelAliasProposals)
	g.POST("/context/channel-proposals/judge", s.HandleJudgeChannelAliasProposal)

	// The read surface over the workspace context graph the push writes.
	s.registerContextGraphRoutes(g)

	// Brand profiles — Bowrain AD-011: /:ws/voice-profiles
	g.GET("/voice-profiles", s.HandleListVoiceProfiles)
	g.POST("/voice-profiles", s.HandleCreateVoiceProfile)
	g.GET("/voice-profiles/:id", s.HandleGetVoiceProfile)
	g.PUT("/voice-profiles/:id", s.HandleUpdateVoiceProfile)
	g.DELETE("/voice-profiles/:id", s.HandleDeleteVoiceProfile)
	g.POST("/voice-profiles/:id/check", s.HandleCheckVoice, aiLimit)
	g.POST("/voice-profiles/from-starter", s.HandleCreateFromStarter)
	g.POST("/voice-profiles/upsert", s.HandleUpsertVoiceProfile)
	g.GET("/voice-profiles/suggested-rules", s.HandleGetSuggestedRules)
	g.GET("/voice-profiles/:id/candidates", s.HandleListCandidates)
	g.POST("/voice-profiles/:id/promote-rule", s.HandlePromoteSuggestedRule)
	g.POST("/voice-profiles/:id/demote-rule", s.HandleDemoteSuggestedRule)
	g.POST("/voice-profiles/:id/reject-rule", s.HandleRejectSuggestedRule)
	g.POST("/voice-profiles/:id/evaluate-rule", s.HandleEvaluateRulePromotion)
	g.GET("/voice-profiles/starter-packs", s.HandleListStarterPacks)

	// Recipe changes an approval is waiting to put in a working tree. An
	// approved axis is not a coordinate until kapi.yaml says so and a push
	// carries it, so these three endpoints are the hand-off: propose, read what
	// is pending, settle it once a pull has taken it.
	g.POST("/projects/:id/axes", s.HandleApproveAxis)
	g.GET("/projects/:id/recipe-changes", s.HandleListPendingRecipeChanges)
	g.POST("/projects/:id/recipe-changes/:changeID/applied", s.HandleMarkRecipeChangeApplied)

	// Workspace voice compliance rollup — the all-surfaces board aggregating
	// every project's stored scores/trends into one matrix: /:ws/voice/rollup
	g.GET("/voice/rollup", s.HandleGetVoiceRollup)

	// AI brand onboarding scans — epic 016: /:ws/context-scans. The scan
	// endpoint burns platform credits (QuotaGuard + the handler's 402
	// pre-check); the draft tester is deterministic (aiLimit only).
	g.POST("/context-scans/uploads", s.HandleContextScanUploads)
	g.POST("/context-scans", s.HandleCreateContextScan, aiLimit, billing.QuotaGuard(s.BillingStore))
	g.GET("/context-scans/:id", s.HandleGetContextScan)
	g.POST("/context-scans/:id/approve", s.HandleApproveContextScan)
	g.POST("/context-scans/check-draft", s.HandleCheckVoiceDraft, aiLimit)

	// Translation jobs — Bowrain AD-011: /:ws/jobs
	g.POST("/jobs/translate", s.HandleCreateTranslationJob)
	g.GET("/jobs", s.HandleListJobs)
	g.GET("/jobs/:id", s.HandleGetJob)
	g.DELETE("/jobs/:id", s.HandleDeleteJob)
	g.GET("/ai-usage", s.HandleGetAIUsage)

	// Notifications — Bowrain AD-011: /:ws/notifications
	g.GET("/notifications", s.HandleListNotifications)
	g.POST("/notifications/:nid/read", s.HandleMarkNotificationRead)
	g.POST("/notifications/read-all", s.HandleMarkAllNotificationsRead)
	g.DELETE("/notifications/:nid", s.HandleDeleteNotification)
	g.GET("/notifications/ws", s.HandleNotificationWebSocket)
	g.GET("/notification-preferences", s.HandleGetNotificationPreferences)
	g.PUT("/notification-preferences", s.HandleUpdateNotificationPreferences)
	g.GET("/digest-settings", s.HandleGetDigestSettings)
	g.PUT("/digest-settings", s.HandleUpdateDigestSettings)

	// Activities — Bowrain AD-011: /:ws/activities
	g.GET("/activities", s.HandleListActivities)
	g.POST("/activities/seen", s.HandleMarkActivitiesSeen)

	// Loop rollup — workspace-home aggregate (latest convergence run +
	// ship-state rollup): /:ws/loop-rollup
	g.GET("/loop-rollup", s.HandleWorkspaceLoopRollup)

	// Tasks — Bowrain AD-011: /:ws/tasks (no more /my/tasks, use ?assignee_id=me)
	g.GET("/tasks", s.HandleListTasks)
	g.GET("/tasks/counts", s.HandleGetTaskCounts)
	g.POST("/tasks", s.HandleCreateTask)
	g.GET("/tasks/:taskId", s.HandleGetTask)
	g.PATCH("/tasks/:taskId", s.HandleUpdateTask)
	g.DELETE("/tasks/:taskId", s.HandleDeleteTask)
	g.POST("/tasks/:taskId/assign", s.HandleAssignTask)
	g.POST("/tasks/:taskId/complete", s.HandleCompleteTask)
	g.POST("/tasks/:taskId/cancel", s.HandleCancelTask)

	// Workspace audit log — Bowrain AD-011: /:ws/audit-log
	g.GET("/audit-log", s.HandleListWorkspaceAuditLog)
	g.GET("/audit-log/verify", s.HandleVerifyWorkspaceAuditChain)

	// Archived projects — Bowrain AD-011: /:ws/archived-projects
	g.GET("/archived-projects", s.HandleListArchivedProjects)

	// -----------------------------------------------------------------------
	// Project collection routes: /:ws/projects
	// -----------------------------------------------------------------------

	g.GET("/projects", s.HandleListWorkspaceProjects)
	g.POST("/projects", s.HandleCreateWorkspaceProject)

	// -----------------------------------------------------------------------
	// Project-specific routes: /:ws/:id
	// Bowrain AD-011: bare project slug, no /p/ prefix
	// -----------------------------------------------------------------------

	// Project CRUD
	g.GET("/:id", s.HandleGetEditorProject)
	g.PUT("/:id", s.HandleUpdateEditorProject)
	g.PATCH("/:id", s.HandleUpdateEditorProject)
	g.DELETE("/:id", s.HandleDeleteEditorProject)
	g.POST("/:id/restore", s.HandleRestoreProject)
	g.DELETE("/:id/permanent", s.HandlePermanentlyDeleteProject)

	// What the caller may do here, so a surface offers only what the server
	// will accept.
	g.GET("/:id/permissions", s.HandleGetCallerPermissions)

	// Project members — Bowrain AD-011: /:ws/:id/members
	g.GET("/:id/members", s.HandleListProjectMembers)
	g.POST("/:id/members", s.HandleAddProjectMember)
	g.PUT("/:id/members/:uid", s.HandleUpdateProjectMember)
	g.DELETE("/:id/members/:uid", s.HandleRemoveProjectMember)

	// Presence — /:ws/:id/presence. Reports the caller's editing focus; the
	// change relay fans it out to project watchers over the /:ws/events SSE
	// stream (per-cursor presence uses the Yjs awareness channel).
	g.POST("/:id/presence", s.HandleUpdatePresence)

	// Project settings — Bowrain AD-011: /:ws/:id/settings
	g.GET("/:id/settings/extraction", s.HandleGetExtractionSettings)
	g.PUT("/:id/settings/extraction", s.HandleUpdateExtractionSettings)

	// Measured steerability — model recommendation sweeps: /:ws/:id/model-recommendations
	g.GET("/:id/model-recommendations", s.HandleGetModelRecommendations)
	g.POST("/:id/model-recommendations/refresh", s.HandleRefreshModelRecommendations)

	// Project audit log — Bowrain AD-011: /:ws/:id/audit-log
	g.GET("/:id/audit-log", s.HandleListAuditLog)

	// Automations — Bowrain AD-011: /:ws/:id/automations
	g.GET("/:id/automations", s.HandleListAutomationRules)
	g.POST("/:id/automations", s.HandleCreateAutomationRule)
	g.PUT("/:id/automations/:ruleId", s.HandleUpdateAutomationRule)
	g.DELETE("/:id/automations/:ruleId", s.HandleDeleteAutomationRule)
	g.PATCH("/:id/automations/:ruleId/toggle", s.HandleToggleAutomationRule)
	g.GET("/:id/automations/events", s.HandleListAutomationEvents)
	g.GET("/:id/automations/history", s.HandleListAutomationHistory)

	// Flow definitions — Bowrain AD-013: /:ws/:id/flows
	// Server-side, project-scoped pipeline graphs that automation run_flow
	// actions reference. Built-in flows are merged into the listing; project
	// flows are persisted in the FlowDefStore.
	g.GET("/:id/flows", s.HandleListFlowDefinitions)
	g.POST("/:id/flows", s.HandleCreateFlowDefinition)
	g.GET("/:id/flows/:flowId", s.HandleGetFlowDefinition)
	g.PUT("/:id/flows/:flowId", s.HandleUpdateFlowDefinition)
	g.DELETE("/:id/flows/:flowId", s.HandleDeleteFlowDefinition)

	// Automation runs — Bowrain AD-011: /:ws/:id/automations/runs (nested)
	g.GET("/:id/automations/runs", s.HandleListAutomationRuns)
	g.GET("/:id/automations/runs/:runId", s.HandleGetAutomationRun)
	g.GET("/:id/automations/runs/:runId/steps", s.HandleListAutomationRunSteps)
	g.GET("/:id/automations/runs/:runId/steps/:stepId/logs", s.HandleListStepLogs)
	g.POST("/:id/automations/runs/:runId/cancel", s.HandleCancelAutomationRun)
	g.GET("/:id/automations/runs/:runId/events", s.HandleAutomationRunSSE)

	// Stream management — Bowrain AD-011: /:ws/:id/streams
	g.GET("/:id/streams", s.HandleListStreams)
	g.POST("/:id/streams", s.HandleCreateStream)
	g.GET("/:id/streams/:stream", s.HandleGetStream)
	g.PATCH("/:id/streams/:stream", s.HandleUpdateStream)
	g.DELETE("/:id/streams/:stream", s.HandleArchiveStream)
	g.POST("/:id/streams/:stream/restore", s.HandleRestoreStream)
	g.POST("/:id/streams/:stream/merge", s.HandleMergeStream)
	g.GET("/:id/streams/:stream/diff", s.HandleDiffStream)
	g.POST("/:id/streams/:stream/lock", s.HandleLockStream)
	g.POST("/:id/streams/:stream/unlock", s.HandleUnlockStream)

	// Tags — Bowrain AD-011: /:ws/:id/tags (peer to streams)
	g.GET("/:id/tags", s.HandleListProjectTags)
	g.POST("/:id/tags", s.HandleCreateStreamTag)
	g.GET("/:id/tags/:tag", s.HandleGetStreamTag)
	g.DELETE("/:id/tags/:tag", s.HandleDeleteStreamTag)

	// Refs — Bowrain AD-011: /:ws/:id/refs (unified streams + tags listing)
	g.GET("/:id/refs", s.HandleListProjectRefs)

	// -----------------------------------------------------------------------
	// Ref-scoped content routes: /:ws/:id/<resource>/:ref
	// Bowrain AD-011: resource-first ref pattern (GitHub-style)
	// -----------------------------------------------------------------------

	syncRateLimit := RateLimitSyncPush(10, 3) // 10 pushes/min, burst of 3
	// Chunk uploads stream many 2 MiB blobs per push, so they draw on their own
	// more generous per-project bucket (see the flat sync group above).
	chunkRateLimit := RateLimitSyncPush(600, 120)

	// Items — Bowrain AD-011: /:ws/:id/items/:ref
	g.GET("/:id/items/:ref", s.HandleGetFileBlocks) // list items
	// One item's metadata. Item names carry slashes, so the name is a query
	// parameter and the route needs its own segment beside the list route.
	g.GET("/:id/items/:ref/one", s.HandleGetItem) // ?item=path/to/file
	g.POST("/:id/items/:ref", s.HandleUploadFiles)
	g.DELETE("/:id/items/:ref", s.HandleRemoveFile) // ?item=path/to/file

	// Blocks — Bowrain AD-011: /:ws/:id/blocks/:ref
	g.GET("/:id/blocks/:ref", s.HandleGetFileBlocks) // ?item=&locale=&status=&q=&translatable=
	g.GET("/:id/blocks/:ref/counts", s.HandleGetBlockCounts)
	g.POST("/:id/blocks/:ref/bulk-review", s.HandleBulkReviewBlocks)
	g.POST("/:id/blocks/:ref/bulk-apply-memory", s.HandleBulkApplyMemory)
	g.GET("/:id/blocks/:ref/:bid", s.HandleGetBlock)
	g.PUT("/:id/blocks/:ref/:bid", s.HandleUpdateBlockTarget)
	g.PUT("/:id/blocks/:ref/:bid/runs", s.HandleUpdateBlockTargetRuns)
	g.PUT("/:id/blocks/:ref/:bid/status", s.HandleSetBlockStatus)
	g.PUT("/:id/blocks/:ref/:bid/review", s.HandleReviewBlock)
	// Bulk approve every passing draft in one action, then continue the loop to
	// delivery (RV-D). Distinct path segment from /:id/review-queue below.
	g.POST("/:id/review/approve-passing", s.HandleApprovePassing)
	g.GET("/:id/blocks/:ref/:bid/history", s.HandleGetBlockHistory)
	g.POST("/:id/blocks/:ref/:bid/rollback", s.HandleRollbackBlock)
	g.POST("/:id/revert", s.HandleRevertBatch)
	g.POST("/:id/restore", s.HandleRestoreToPoint)
	g.GET("/:id/blocks/:ref/:bid/notes", s.HandleListBlockNotes)
	g.POST("/:id/blocks/:ref/:bid/notes", s.HandleAddBlockNote)
	g.DELETE("/:id/blocks/:ref/:bid/notes/:nid", s.HandleDeleteBlockNote)
	g.GET("/:id/blocks/:ref/:bid/tm-matches", s.HandleLookupMemoryForBlock)
	g.GET("/:id/blocks/:ref/:bid/term-matches", s.HandleLookupTermsForBlock)
	// The context a reviewer decides in, for one unit: what governs it, what
	// surrounds it, what was decided about it, what the checks found.
	g.GET("/:id/blocks/:ref/:bid/review-context", s.HandleGetReviewContext)
	g.GET("/:id/blocks/:ref/:bid/html", s.HandleRenderBlockHTML)

	// Entities on blocks — Bowrain AD-011: /:ws/:id/blocks/:ref/:bid/entities
	g.POST("/:id/blocks/:ref/:bid/entities", s.HandleCreateEntity)
	g.PUT("/:id/blocks/:ref/:bid/entities/:idx", s.HandleUpdateEntity)
	g.DELETE("/:id/blocks/:ref/:bid/entities/:idx", s.HandleDeleteEntity)
	g.POST("/:id/blocks/:ref/:bid/entities/:idx/promote", s.HandlePromoteEntity)
	g.POST("/:id/blocks/:ref/:bid/entities/:idx/promote-to-concept", s.HandlePromoteEntityToConcept)

	// Actions — Bowrain AD-011: /:ws/:id/actions/:ref/<verb>
	g.POST("/:id/actions/:ref/pseudo-translate", s.HandlePseudoTranslate)
	// ai-translate enforces the credit gate INSIDE the handler via
	// billing.GuardSyncCredits (not QuotaGuard middleware) so it can exempt
	// bring-your-own-key requests, which the middleware cannot see in the body.
	// A per-IP AI throttle (003) still fronts it so a client cannot burst spend.
	g.POST("/:id/actions/:ref/ai-translate", s.HandleAITranslate, aiLimit)
	g.POST("/:id/actions/:ref/tm-translate", s.HandleMemoryTranslate)
	g.POST("/:id/actions/:ref/export", s.HandleExportTranslatedFile)
	g.POST("/:id/actions/:ref/qa-check", s.HandleQACheckFile)
	g.POST("/:id/actions/:ref/qa-check-block", s.HandleQACheckBlock)
	g.POST("/:id/actions/:ref/term-enforce", s.HandleTermEnforce)

	// Preview and word count — Bowrain AD-011: /:ws/:id/preview/:ref, /:ws/:id/word-count/:ref
	g.GET("/:id/preview/:ref", s.HandleRenderDocumentPreview)
	g.GET("/:id/word-count/:ref", s.HandleGetWordCount)

	// Dashboard — Bowrain AD-011: /:ws/:id/dashboard/:ref
	g.GET("/:id/dashboard/:ref", s.HandleGetTranslationDashboard)

	// Sync — Bowrain AD-011: /:ws/:id/sync/:ref
	g.GET("/:id/sync/:ref/pull", s.HandleSyncPull)
	g.GET("/:id/sync/:ref/ref", s.HandleSyncRef)
	g.GET("/:id/sync/:ref/blocks", s.HandleSyncGetBlocks)
	g.GET("/:id/sync/:ref/status", s.HandleSyncPushStatus)
	g.GET("/:id/sync/:ref/tree", s.HandleSyncTree)
	g.GET("/:id/sync/:ref/blobs/:key", s.HandleBlobDownload)
	g.POST("/:id/sync/:ref/push/init", s.HandleSyncPushInit)
	g.POST("/:id/sync/:ref/push/uploads", s.HandleSyncPushUploads)
	g.POST("/:id/sync/:ref/push/commit", s.HandleSyncPushCommit, syncRateLimit)
	g.PUT("/:id/sync/:ref/push/chunks/:uploadId/:chunkIndex", s.HandleSyncProxyChunkUpload, chunkRateLimit)
	g.POST("/:id/sync/:ref/push/diff", s.HandleSyncPushDiffRetired)
	g.POST("/:id/sync/:ref/translate", s.HandleCreateProjectTranslationJob)

	// PostHog locale-demand connector — /:ws/:id/connectors/posthog
	// Per-project analytics connector config (secret stored sealed, masked on
	// read) plus the cached demand snapshot endpoint.
	g.GET("/:id/connectors/posthog", s.HandleGetPostHogConfig)
	g.PUT("/:id/connectors/posthog", s.HandleSavePostHogConfig)
	g.DELETE("/:id/connectors/posthog", s.HandleDeletePostHogConfig)
	g.GET("/:id/connectors/posthog/demand", s.HandlePostHogDemand)

	// Collections — Bowrain AD-011: /:ws/:id/collections/:ref
	g.GET("/:id/collections/:ref", s.HandleListCollections)
	g.POST("/:id/collections/:ref", s.HandleCreateCollection)
	g.GET("/:id/collections/:ref/:cid", s.HandleGetCollection)
	g.PUT("/:id/collections/:ref/:cid", s.HandleUpdateCollection)
	g.DELETE("/:id/collections/:ref/:cid", s.HandleDeleteCollection)
	g.POST("/:id/collections/:ref/:cid/items", s.HandleUploadToCollection)
	// The story index the collection's preview host publishes, read here so the
	// application's connect-src stays 'self' and CORS stops being the
	// customer's problem — see handlers_preview_index.go.
	g.GET("/:id/collections/:ref/:cid/preview/index", s.HandlePreviewIndex)

	// Assets — Bowrain AD-011: /:ws/:id/assets/:ref
	g.POST("/:id/assets/:ref/upload-url", s.HandleAssetUploadURL)
	g.GET("/:id/assets/:ref", s.HandleListAssets)
	g.POST("/:id/assets/:ref", s.HandleCreateAsset)
	g.GET("/:id/assets/:ref/:aid", s.HandleGetAsset)
	g.DELETE("/:id/assets/:ref/:aid", s.HandleDeleteAsset)
	g.POST("/:id/assets/:ref/:aid/variants/upload-url", s.HandleVariantUploadURL)
	g.GET("/:id/assets/:ref/:aid/variants", s.HandleListVariants)
	g.POST("/:id/assets/:ref/:aid/variants", s.HandleCreateVariant)

	// Pending translation review — the session's server-side queue.
	g.GET("/:id/pending-review/:ref", s.HandleListPendingReview)

	// Review queue — Bowrain AD-011: /:ws/:id/review-queue/:ref
	g.GET("/:id/review-queue/:ref", s.HandleListReviewQueue)
	g.GET("/:id/review-queue/:ref/:itemId", s.HandleGetReviewQueueItem)
	g.POST("/:id/review-queue/:ref/:itemId/decide", s.HandleDecideReviewItem)
	g.POST("/:id/review-queue/:ref/:itemId/assign", s.HandleAssignReviewItem)
	g.POST("/:id/review-queue/:ref/:itemId/split", s.HandleSplitReviewItem)
	g.POST("/:id/review-queue/:ref/batch-decide", s.HandleBatchDecideReviewItems)
	g.POST("/:id/review-queue/:ref/sync", s.HandleSyncReviewDecisions)

	// Back-to-source review (RV-F): a reviewer proposes a source-text fix; a
	// source owner (PermEditSource) approves it (applies + re-drafts every locale)
	// or rejects it. /:ws/:id/source-proposals
	g.GET("/:id/source-proposals", s.HandleListSourceProposals)
	g.POST("/:id/source-proposals", s.HandleCreateSourceProposal)
	g.POST("/:id/source-proposals/:pid/decide", s.HandleDecideSourceProposal)

	// Voice — Bowrain AD-011: /:ws/:id/voice/:ref
	g.GET("/:id/voice/:ref/scores", s.HandleGetVoiceScores)
	g.GET("/:id/voice/:ref/scores/:locale", s.HandleGetVoiceScoresByLocale)
	g.GET("/:id/voice/:ref/trends", s.HandleGetVoiceTrends)
	g.GET("/:id/voice/:ref/drift", s.HandleGetVoiceDrift)
	g.POST("/:id/voice/:ref/drift-check", s.HandleRunVoiceDriftCheck)
	g.POST("/:id/voice/:ref/corrections", s.HandleCreateVoiceCorrection)

	// Collab — Bowrain AD-011: /:ws/:id/collab/:ref
	g.GET("/:id/collab/:ref", s.HandleCollabWebSocket)

	// Convergence runs (strategy 2026-07-kapi-up doc 03) — project-scoped, not
	// ref-scoped: /:ws/:id/convergence/runs
	s.registerConvergenceRoutes(g)
}

// registerConvergenceRoutes wires the project-scoped convergence-run endpoints
// (start/list/get/cancel/events SSE) and the project-settings PATCH onto a
// group whose path already carries the project :id param. Shared between the
// workspace group (/:ws/:id/...) and the flat unclaimed group
// (/projects/:id/...) so both client route styles reach the same handlers.
func (s *Server) registerConvergenceRoutes(g *echo.Group) {
	g.GET("/:id/convergence/estimate", s.HandleConvergenceEstimate)
	g.POST("/:id/convergence/runs", s.HandleStartConvergenceRun)
	g.GET("/:id/convergence/runs", s.HandleListConvergenceRuns)
	g.GET("/:id/convergence/runs/:runID", s.HandleGetConvergenceRun)
	g.POST("/:id/convergence/runs/:runID/cancel", s.HandleCancelConvergenceRun)
	g.GET("/:id/convergence/runs/:runID/events", s.HandleConvergenceRunSSE)
	g.PATCH("/:id/settings", s.HandleUpdateProjectSettings)
}

// readHeaderTimeout bounds the header-read phase so a slow client cannot hold a
// connection open indefinitely (Slowloris). Request bodies can be large — block
// pushes — and stream over slow links, so only the header deadline is set,
// never a whole-request ReadTimeout. Both listeners below use it: a bound that
// applies to one of two ways into the same server is not a bound.
const readHeaderTimeout = 30 * time.Second

// Start initializes the Echo server and starts listening.
// When GRPCServer is set, gRPC and HTTP are multiplexed on the same port
// using h2c (cleartext HTTP/2). Requests with Content-Type: application/grpc
// are routed to the gRPC server; all others go to Echo.
func (s *Server) Start(addr string) error {
	e := echo.New()
	e.HideBanner = true
	s.Echo = e

	s.SetupRoutes(e)

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)
	}

	// When no gRPC server is configured, serve Echo from a server built here
	// rather than from e.Start, which constructs an http.Server with the
	// zero-value — that is, absent — header timeout. Both listeners answer the
	// same internet and need the same bound; this one only lacked it because
	// nothing along its path had occasion to set one.
	if s.GRPCServer == nil {
		srv := &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: readHeaderTimeout,
		}
		// Recorded for Shutdown, which prefers httpServer over Echo's own —
		// e.Shutdown would stop the server Echo built, not this one.
		s.httpServer = srv
		slog.Info("starting Bowrain server", "addr", addr, "mode", "HTTP")
		return e.StartServer(srv)
	}

	// Multiplex gRPC and HTTP on the same port via h2c.
	// Only check Content-Type (not ProtoMajor) because cloud platforms like
	// Azure Container Apps terminate TLS and may forward as HTTP/1.1 internally.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			s.GRPCServer.ServeHTTP(w, r)
		} else {
			e.ServeHTTP(w, r)
		}
	})

	// Serve HTTP/1.1 and cleartext HTTP/2 (h2c with prior knowledge, as used by
	// gRPC clients) on the same listener via the standard library's protocol
	// negotiation (Go 1.24+), replacing the deprecated
	// golang.org/x/net/http2/h2c handler.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	s.httpServer = srv
	slog.Info("starting Bowrain server", "addr", addr, "mode", "HTTP+gRPC")
	return srv.ListenAndServe()
}

// Shutdown gracefully shuts down the server and all background resources.
// The shutdown proceeds in four phases:
//  1. Stop accepting new work (HTTP + gRPC listeners)
//  2. Stop background workers (digest, deadline, progress, etc.)
//  3. Close event infrastructure (automation engine, audit logger, event bus)
//  4. Close data connections (stores, queues, analytics)
func (s *Server) Shutdown(ctx context.Context) error {
	var firstErr error
	collectErr := func(name string, err error) {
		if err != nil {
			slog.Error("shutdown error", "component", name, "error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", name, err)
			}
		}
	}

	// Phase 1: stop accepting new connections.
	slog.Info("shutdown phase 1: stopping listeners")
	if s.GRPCServer != nil {
		s.GRPCServer.GracefulStop()
	}
	if s.httpServer != nil {
		collectErr("http-server", s.httpServer.Shutdown(ctx))
	} else if s.Echo != nil {
		collectErr("echo", s.Echo.Shutdown(ctx))
	}

	// Phase 2: stop background workers. These are safe to call on nil receivers
	// because each worker's Close checks internal state.
	slog.Info("shutdown phase 2: stopping background workers")
	// Automation actions first: they write through the stores the workers
	// below and the phases after this one are about to close, and each is
	// already bounded by its own timeout.
	s.awaitActions(ctx)
	if s.DailyDigestWorker != nil {
		s.DailyDigestWorker.Close()
	}
	if s.WeeklyDigestWorker != nil {
		s.WeeklyDigestWorker.Close()
	}
	if s.deadlineChecker != nil {
		s.deadlineChecker.Close()
	}
	if s.unclaimedPurger != nil {
		s.unclaimedPurger.Close()
	}
	if s.progressTracker != nil {
		s.progressTracker.Close()
	}
	if s.pushCompletionTracker != nil {
		s.pushCompletionTracker.Close()
	}
	if s.stepCompletionTracker != nil {
		s.stepCompletionTracker.Close()
	}
	if s.ActivityRecorder != nil {
		s.ActivityRecorder.Close()
	}
	if s.changeRelay != nil {
		s.changeRelay.Close()
	}
	if s.NotificationDispatcher != nil {
		s.NotificationDispatcher.Close()
	}

	// Phase 3: close event infrastructure. Order matters — stop consumers
	// before closing the bus so in-flight events can drain.
	slog.Info("shutdown phase 3: closing event infrastructure")
	if s.AutomationEngine != nil {
		s.AutomationEngine.Close()
	}
	if s.AuditRetention != nil {
		s.AuditRetention.Close()
	}
	if s.SIEMExporter != nil {
		s.SIEMExporter.Close()
	}
	if s.AuditLogger != nil {
		s.AuditLogger.Close()
	}
	if s.graphSyncer != nil {
		s.graphSyncer.Close()
	}
	if s.EventBus != nil {
		s.EventBus.Close()
	}

	// Phase 4: close data connections and external clients.
	slog.Info("shutdown phase 4: closing data connections")
	if s.JobQueue != nil {
		collectErr("job-queue", s.JobQueue.Close())
	}
	if s.ExtractionQueue != nil {
		collectErr("extraction-queue", s.ExtractionQueue.Close())
	}
	if s.ContextScanQueue != nil {
		collectErr("brand-scan-queue", s.ContextScanQueue.Close())
	}
	if s.PostHogClient != nil {
		collectErr("posthog", s.PostHogClient.Close())
	}
	// Close Redis session store if the implementation supports it.
	if c, ok := s.SessionStore.(io.Closer); ok {
		collectErr("session-store", c.Close())
	}
	if s.ContentStore != nil {
		collectErr("content-store", s.ContentStore.Close())
	}
	if s.AuthStore != nil {
		collectErr("auth-store", s.AuthStore.Close())
	}

	return firstErr
}

// OpenBlockstore returns a `blockstore.Store` bound to the given
// project/stream on this Server's ContentStore — the in-process
// adapter used by automation actions and server-side flow execution.
// See AD-013 and #385 for the design.
func (s *Server) OpenBlockstore(projectID, stream string) (coreblockstore.Store, error) {
	if s.ContentStore == nil {
		return nil, errors.New("OpenBlockstore: ContentStore not configured")
	}
	return bwblockstore.Open(s.ContentStore, projectID, stream)
}

// GetEcho returns the underlying Echo instance. Useful for testing.
func (s *Server) GetEcho() *echo.Echo {
	if s.Echo == nil {
		s.Echo = echo.New()
		s.Echo.HideBanner = true
		s.SetupRoutes(s.Echo)
	}
	return s.Echo
}

// resolveNotificationTargets returns user IDs of workspace/project members
// who should receive a notification for a project event, excluding the actor.
func (s *Server) resolveNotificationTargets(ctx context.Context, projectID string, excludeActorID string) ([]string, error) {
	members, err := s.AuthStore.ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		if m.UserID != excludeActorID {
			userIDs = append(userIDs, m.UserID)
		}
	}
	return userIDs, nil
}

// requestBaseURL returns the base URL (scheme + host) for the current request,
// respecting X-Forwarded-Host and X-Forwarded-Proto headers set by reverse
// proxies. Falls back to the direct request host and scheme.
func requestBaseURL(c echo.Context) string {
	host := c.Request().Header.Get("X-Forwarded-Host")
	if host == "" {
		host = c.Request().Host
	}
	return fmt.Sprintf("%s://%s", c.Scheme(), host)
}

// corsConfig builds a CORS middleware configuration. In production only this
// app's own origin (AppPublicURL) and the marketing landing origin
// (PublicSiteURL) are allowed; in development the middleware dynamically allows
// any localhost origin plus the landing.
//
// Which of the two applies is decided by Config.DevMode and nothing else. It
// used to be decided by whether OIDCPublicURL happened to be set — an optional
// field naming the identity provider — so a production deployment that
// configured only the issuer URL, which the documentation described as
// supported, silently served credentialed CORS to any localhost origin.
//
// Credentials are enabled so the landing (a DIFFERENT but same-site origin) can
// read GET /api/v1/auth/whoami with the session cookie and render the signed-in
// CTA. Per the Fetch spec, credentialed CORS forbids a "*" origin, so the
// allowlist is kept to exactly the two trusted origins (app + landing). This is
// BFF-safe — whoami returns only display JSON, never a token — and CSRF-safe:
// the credentialed surface a cross-origin caller can reach is a read-only GET,
// while every state-changing cookie request still passes the CSRF gate in
// AuthMiddleware.
func (s *Server) corsConfig() middleware.CORSConfig {
	cfg := middleware.CORSConfig{
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-Requested-With"},
		// Expose the correlation ID so cross-origin clients (desktop app, CLI)
		// can read it off any response and attach it as the error "reference".
		ExposeHeaders: []string{observe.RequestIDHeader},
		// Allow the same-site landing to send the session cookie on its whoami
		// fetch. Never combined with a "*" origin (see origin allowlist below).
		AllowCredentials: true,
	}

	landingOrigin := originOf(s.Config.PublicSiteURL)

	if !s.Config.DevMode {
		// Production: a fixed allowlist of this app's own origin plus the
		// landing. Whether it ends up empty is not a reason to widen it — an
		// unset or unparsable AppPublicURL means no cross-origin caller is
		// credentialed, which breaks the landing's signed-in CTA and nothing
		// else. Same-origin requests never consult CORS.
		var origins []string
		if o := originOf(s.Config.AppPublicURL); o != "" {
			origins = append(origins, o)
		}
		if landingOrigin != "" {
			origins = append(origins, landingOrigin)
		}
		if len(origins) == 0 {
			slog.Warn("CORS: no cross-origin caller is allowed; set BOWRAIN_APP_PUBLIC_URL " +
				"(and BOWRAIN_PUBLIC_SITE_URL) if a different origin must reach this API")
		}
		cfg.AllowOrigins = origins
		return cfg
	}

	// Development: allow localhost origins and the configured landing origin.
	// Echo reflects only the matching origin — never "*" — so this stays
	// credential-safe. Reached only when DevMode was set deliberately.
	cfg.AllowOriginFunc = func(origin string) (bool, error) {
		u, err := url.Parse(origin)
		if err != nil {
			return false, nil
		}
		if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
			return true, nil
		}
		if landingOrigin != "" && origin == landingOrigin {
			return true, nil
		}
		return false, nil
	}
	return cfg
}

// originOf reduces a URL to its scheme://host origin (dropping any path, query,
// or fragment). Returns "" when the input is empty or unparsable — callers omit
// it from the allowlist rather than allow a malformed origin.
func originOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// wsOriginPatterns lists the origins allowed to open one of this server's
// WebSockets, in the host-pattern form websocket.AcceptOptions expects.
//
// Both sockets authenticate through AuthMiddleware, and an upgrade is a GET —
// a CSRF-safe method — so a browser that already holds the session cookie needs
// nothing else to connect. SameSite=Lax keeps a genuinely cross-site page out,
// but "same-site" is a registrable-domain relation, not an origin one: any
// sibling host under the same domain still sends the cookie. The origin check
// is what makes the socket same-origin, so it has to be a real allowlist.
//
// The policy is deliberately narrower than corsConfig's. CORS grants the
// landing origin one credentialed read (GET whoami, display JSON only); a
// socket is a different privilege — the notification stream, and join-and-write
// access to a collaboration room — and nothing off-origin needs it. So the
// landing is not here, and the app's own origin arrives via AppPublicURL rather
// than via the identity provider's URL, which never names this application.
//
// What is allowed:
//
//   - The origin the request itself arrived on. The library already accepts an
//     Origin matching r.Host; this adds the public host from X-Forwarded-Host
//     for deployments where the proxy rewrites Host, the same header
//     requestBaseURL trusts when it builds OIDC redirect URIs. A browser cannot
//     set that header on a WebSocket handshake, so it does not widen the
//     browser-driven surface this check exists to close.
//   - This app's own public host (AppPublicURL), for a deployment whose proxy
//     leaves Host alone and where the browser therefore arrives on a name the
//     library's Origin-equals-Host check would not match.
//   - Any localhost origin in development, mirroring corsConfig's dev branch,
//     so a Vite dev server on another port can connect. Development means
//     Config.DevMode, set deliberately — not "some unrelated field is empty".
//
// An absent Origin — every non-browser client, so the CLI and the Go SDK — is
// accepted by the library before these patterns are consulted.
func (s *Server) wsOriginPatterns(c echo.Context) []string {
	var patterns []string

	// Host patterns rather than scheme://host: a proxy that terminates TLS may
	// not set X-Forwarded-Proto, and a scheme guessed wrong here would reject
	// the app's own origin.
	if host := c.Request().Header.Get("X-Forwarded-Host"); host != "" {
		patterns = append(patterns, host)
	}

	// The app's own public host, for a deployment whose proxy does not rewrite
	// Host and where the browser therefore arrives on a name the library's
	// Origin-equals-Host check would not match.
	if h := hostOf(s.Config.AppPublicURL); h != "" {
		patterns = append(patterns, h)
	}

	// Development, gated exactly as corsConfig gates its dynamic branch — on
	// the explicit discriminator, not on whether some other field is populated.
	if s.Config.DevMode {
		patterns = append(patterns,
			"localhost", "localhost:*",
			"127.0.0.1", "127.0.0.1:*",
			"[::1]", "[::1]:*",
		)
	}

	return patterns
}

// hostOf returns the host[:port] of a URL, or "" if it has none. Host rather
// than origin because websocket.AcceptOptions matches on host patterns.
func hostOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
