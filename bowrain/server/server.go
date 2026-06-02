package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	slogecho "github.com/samber/slog-echo"
	"google.golang.org/grpc"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/formats"
	coreg "github.com/neokapi/neokapi/core/graph"
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
	"github.com/neokapi/neokapi/bowrain/event"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/mailer"
	"github.com/neokapi/neokapi/bowrain/observe"
	mcpserver "github.com/neokapi/neokapi/bowrain/server/mcp"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
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

	// wsStores manages per-workspace TM and terminology stores.
	wsStores *workspaceStores

	// CredentialStore manages AI provider credentials.
	CredentialStore *credentials.Store

	// EmailSender sends raw transactional emails. Prefer Mailer for
	// template-rendered messages. Kept for backward compatibility with tests.
	// Nil if email is not configured.
	EmailSender EmailSenderI

	// Mailer renders branded email templates and dispatches them via
	// EmailSender. Nil when email sending is not configured.
	Mailer *mailer.Mailer

	// KeycloakAdmin writes through identity changes (email, etc.) to Keycloak
	// via its Admin API. Nil when Config.KeycloakAdminURL is unset, in which
	// case Bowrain-managed email change is unavailable.
	KeycloakAdmin *auth.KeycloakAdminClient

	// collabHub manages collaborative editing WebSocket rooms.
	collabHub *collabHub

	// notificationHub manages per-user WebSocket connections for real-time notifications.
	notificationHub *notificationHub

	// JobStore persists translation job state. Nil when job system is not configured.
	JobStore jobs.JobStore

	// JobQueue enqueues and dequeues translation job IDs. Nil when job system is not configured.
	JobQueue jobs.Queue

	// QuotaStore tracks AI token usage per workspace. Nil when quota tracking is not configured.
	QuotaStore jobs.QuotaStore

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

	// BrandStore manages brand voice profiles. Nil when not configured.
	BrandStore corebrand.BrandStore

	// GraphStore provides graph-based concept management. Nil when not configured.
	GraphStore coreg.GraphStore

	// graphSyncer keeps the graph in sync with content events. Nil when graph is not configured.
	graphSyncer *platgraph.GraphSyncer

	// AuditLogger persists all events to the audit_log table. Nil when not configured.
	AuditLogger *event.AuditLogger

	// AuditRetention prunes audit rows past the retention window. Nil when disabled.
	AuditRetention *event.AuditRetentionCleaner

	// SIEMExporter forwards events to an external SIEM/log sink. Nil when disabled.
	SIEMExporter *event.SIEMExporter

	// mcpServer is the MCP protocol server for brand voice. Nil when brand store is not configured.
	mcpServer *mcpserver.MCPServer

	// ReviewQueueStore persists entity/term extraction review items. Nil when not configured.
	ReviewQueueStore *bstore.ReviewQueueStore

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

	// stepCompletionTracker monitors async automation steps. Nil when not configured.
	stepCompletionTracker *event.StepCompletionTracker

	// runHub manages SSE connections for live automation run updates. Always initialized.
	runHub *automationRunHub

	// changeRelay fans out platform events to attached web (SSE) and desktop
	// (gRPC WatchProject) clients so no view shows stale state on an external
	// change. Always initialized.
	changeRelay *event.ChangeRelay

	// SyncCache is the optional Redis hash cache for sync diff engine (Bowrain AD-009).
	SyncCache bowsync.HashCache

	// ExtractionJobStore persists extraction job state. Nil when job system is not configured.
	ExtractionJobStore jobs.ExtractionJobStore

	// ExtractionQueue enqueues extraction job IDs. Nil when not configured.
	ExtractionQueue jobs.Queue

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
}

// NewServer creates a new Server with the given configuration.
// createEventBus selects the event bus backend based on configuration (Bowrain AD-012).
func createEventBus(cfg Config) platev.EventBus {
	if cfg.ServiceBusConnection != "" {
		bus, err := event.NewServiceBusEventBus(cfg.ServiceBusConnection)
		if err != nil {
			slog.Warn("failed to create Service Bus event bus, falling back to in-memory", "error", err)
			return event.NewChannelEventBus()
		}
		slog.Info("event bus configured", "backend", "azure-service-bus")
		return bus
	}
	if cfg.NATSURL != "" {
		bus, err := event.NewNATSEventBus(cfg.NATSURL)
		if err != nil {
			slog.Warn("failed to create NATS event bus, falling back to in-memory", "error", err)
			return event.NewChannelEventBus()
		}
		slog.Info("event bus configured", "backend", "nats-jetstream")
		return bus
	}
	slog.Info("event bus configured", "backend", "in-memory")
	return event.NewChannelEventBus()
}

func NewServer(cfg Config) *Server {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	connReg := platconn.NewRegistry()
	connector.RegisterAll(connReg, formatReg)

	s := &Server{
		Config:          cfg,
		FormatRegistry:  formatReg,
		ToolRegistry:    toolReg,
		ConnectorReg:    connReg,
		EventBus:        createEventBus(cfg),
		wsStores:        newWorkspaceStores(),
		collabHub:       newCollabHub(),
		notificationHub: newNotificationHub(),
		pulseCache:      newPulseCache(),
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

	// Initialize Keycloak Admin client (used for write-through email change).
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
			s.KeycloakAdmin = client
		}
	}

	// Initialize stores from PostgreSQL DatabaseURL.
	if cfg.DatabaseURL != "" {
		var pg *pgStores
		var err error
		if cfg.DatabaseAuth == "azure" {
			pg, err = openPostgresStoresAzure(cfg.DatabaseURL, cfg.AzureClientID)
		} else {
			pg, err = openPostgresStores(cfg.DatabaseURL)
		}
		if err != nil {
			slog.Warn("failed to open PostgreSQL stores", "error", err)
		} else {
			s.ContentStore = pg.Content
			s.Services = service.NewServices(pg.Content, connReg, formatReg, toolReg)
			s.JobStore = pg.Job
			s.ExtractionJobStore = pg.Extraction
			s.QuotaStore = pg.Quota
			s.wsStores.pgDB = pg.DB
			pgSQL := pg.DB.DB // embedded *sql.DB
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
			s.NotificationStore = bstore.NewNotificationStore(pgSQL)
			s.ActivityStore = bstore.NewActivityStore(pgSQL)
			s.TaskStore = bstore.NewTaskStore(pgSQL)
			s.AutomationRunStore = bstore.NewAutomationRunStore(pgSQL)
			s.PreferenceStore = bstore.NewPreferenceStore(pgSQL)
			s.DigestStore = bstore.NewDigestStore(pgSQL)
			s.BrandStore = pg.Brand
			s.GraphStore = pg.GraphStore
			s.AgentStore = pg.Agent
			s.BillingStore = pg.Billing
			if cfg.JWTSecret != "" {
				s.AuthStore = pg.Auth
				s.Services.Auth = service.NewAuthService(pg.Auth, cfg.JWTSecret)
			}
		}
	}

	// Initialize job queue if Service Bus or NATS is configured.
	switch {
	case cfg.ServiceBusConnection != "":
		q, err := jobs.NewServiceBusQueue(cfg.ServiceBusConnection, "translation-jobs")
		if err != nil {
			slog.Warn("failed to connect to Service Bus queue", "error", err)
		} else {
			s.JobQueue = q
		}
	case cfg.NATSURL != "":
		q, err := jobs.NewNATSQueue(cfg.NATSURL)
		if err != nil {
			slog.Warn("failed to connect to NATS queue", "error", err)
		} else {
			s.JobQueue = q
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

	// Wire up automation engine with run manager (Bowrain AD-013).
	s.runHub = newAutomationRunHub()

	runManager := event.NewAutomationRunManager(s.AutomationRunStore, s.executeAutomationAction)
	s.AutomationEngine = event.NewAutomationEngine(s.EventBus, runManager.Execute)
	s.registerDefaultAutomations()

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

		// Wire immediate email delivery for high-priority notifications.
		if s.Mailer != nil && s.AuthStore != nil {
			s.NotificationDispatcher.SetMailer(event.NewMailerAdapter(s.Mailer, s.AuthStore))
		}

		// Wire quiet hours enforcement for push/email suppression.
		if s.DigestStore != nil {
			s.NotificationDispatcher.SetDigestStore(s.DigestStore)
		}
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

	// Initialize MCP server for brand voice + agent tools when stores are available.
	if s.BrandStore != nil {
		mcpCfg := mcpserver.Config{
			JWTSecret:     cfg.JWTSecret,
			OIDCIssuerURL: cfg.OIDCIssuerURL,
			PublicURL:     cfg.OIDCPublicURL,
		}
		var mcpOpts []mcpserver.Option
		if s.wsStores != nil {
			mcpOpts = append(mcpOpts,
				mcpserver.WithTMResolver(&tmResolverAdapter{ws: s.wsStores}),
				mcpserver.WithTBResolver(&tbResolverAdapter{ws: s.wsStores}),
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
		if s.ToolRegistry != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithToolRegistry(s.ToolRegistry))
		}
		if s.PostHogClient != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithEventTracker(&eventTrackerAdapter{client: s.PostHogClient}))
		}
		ms, err := mcpserver.NewMCPServerWithStore(s.BrandStore, s.ContentStore, mcpCfg, mcpOpts...)
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
		case "queue":
			// Queue mode: agent processing is handled by the worker.
			// API server enqueues jobs to Service Bus and subscribes to Redis pub/sub.
			if err := s.setupAgentQueue(cfg); err != nil {
				slog.Warn("failed to initialize agent queue mode", "error", err)
			} else {
				slog.Info("agent mode configured", "mode", "queue")
			}
		case "docker", "aca":
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
				s.WebhookHandler.SetPlanSyncer(&planSyncAdapter{authStore: s.AuthStore})
			}
			// PostHog wiring deferred to after PostHog init below.
		}
		slog.Info("Stripe billing enabled")
	}

	// Initialize PostHog client (Bowrain AD-018).
	if cfg.PostHogAPIKey != "" {
		host := cfg.PostHogHost
		if host == "" {
			host = "https://us.i.posthog.com"
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

	// Initialize admin OIDC verifier (Bowrain AD-018).
	if cfg.AdminOIDCIssuerURL != "" && cfg.AdminOIDCClientID != "" {
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

// SetupRoutes registers all API routes on the Echo instance.
func (s *Server) SetupRoutes(e *echo.Echo) {
	// Middleware — order matters:
	// 1. Request ID (propagate/generate correlation ID)
	// 2. Structured request logging (slog-echo, includes request_id)
	// 3. Prometheus metrics
	// 4. Recovery, body limit, CORS
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
	e.Use(observe.MetricsMiddleware())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("50M"))
	e.Use(middleware.CORSWithConfig(s.corsConfig()))

	// Prometheus metrics endpoint (no auth).
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// pprof endpoints are served on a separate localhost-only listener
	// (see observe.StartPprofServer in main.go).

	// API v1 routes
	v1 := e.Group("/api/v1")

	// Public endpoints (no auth).
	v1.GET("/health", s.HandleHealth)
	v1.GET("/ready", s.HandleReady)
	v1.GET("/info", s.HandleInfo)
	v1.GET("/badges/:proj", s.HandleProjectBadge)

	// Pulse public activity dashboard (Bowrain AD-017).
	// No auth required — access gated by workspace/project dashboard_visibility.
	v1.GET("/pulse", s.HandlePulseFrontPage)
	if s.AuthStore != nil {
		pulseGroup := v1.Group("/pulse/:workspace")
		pulseGroup.Use(PulseAccessMiddleware(s.Config.JWTSecret, s.AuthStore))
		pulseGroup.GET("", s.HandlePulseOverview)
		pulseGroup.GET("/projects", s.HandlePulseProjects)
		pulseGroup.GET("/activity/heatmap", s.HandlePulseActivityHeatmap)
		pulseGroup.GET("/activity", s.HandlePulseActivity)
		pulseGroup.GET("/leaderboard", s.HandlePulseLeaderboard)
		pulseGroup.GET("/terms", s.HandlePulseTerms)
		pulseGroup.GET("/terms/:cid", s.HandlePulseTermDetail)

		// Project-scoped routes also enforce project-level visibility.
		pulseProjectGroup := pulseGroup.Group("", PulseProjectAccessMiddleware(s.ContentStore))
		pulseProjectGroup.GET("/projects/:id", s.HandlePulseProjectDetail)
		pulseProjectGroup.GET("/projects/:id/lang/:locale", s.HandlePulseLocaleDetail)
	}

	// Authenticated mode: auth routes, protected endpoints, workspace management.
	if s.Config.JWTSecret != "" {
		// Anonymous project creation (no auth required).
		v1.POST("/projects/anonymous", s.HandleCreateAnonymousProject)

		// Public auth routes (no token required)
		authGroup := v1.Group("/auth")
		authGroup.POST("/device/start", s.HandleDeviceAuthStart)
		authGroup.POST("/device/poll", s.HandleDeviceAuthPoll)
		authGroup.POST("/refresh", s.HandleTokenRefresh)
		authGroup.GET("/login", s.HandleAuthLogin)
		authGroup.GET("/callback", s.HandleAuthCallback)
		authGroup.POST("/callback", s.HandleAuthCallback)
		authGroup.GET("/desktop/login", s.HandleDesktopLogin)
		authGroup.GET("/desktop/callback", s.HandleDesktopCallback)

		// Device verification page (user opens in browser)
		authGroup.GET("/device/verify", s.HandleAuthCallback)
		authGroup.POST("/device/verify", func(c echo.Context) error {
			return s.handleDeviceVerification(c, c.FormValue("user_code"))
		})
		authGroup.GET("/device/callback", s.HandleDeviceAuthCallback)

		// Back-channel logout (called server-to-server by Keycloak, unauthenticated)
		authGroup.POST("/backchannel-logout", s.HandleBackChannelLogout)

		// Slug availability check — public; reused by onboarding and the
		// profile rename form. Format and reservation rules enforced here
		// rather than the client.
		authGroup.GET("/check-slug", s.HandleCheckSlug)

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

		// Project claim and invite acceptance (auth required, no workspace).
		jwtProtected := v1.Group("")
		jwtProtected.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		jwtProtected.POST("/projects/claim", s.HandleClaimProject)
		jwtProtected.POST("/join/:code", s.HandleAcceptInvite)

		// Flat sync routes for unclaimed projects (claim-token or JWT auth).
		// Bowrain AD-011: /api/v1/projects/:id/sync/:ref/*
		if s.AuthStore != nil {
			syncRateLimit := RateLimitSyncPush(10, 3)
			flatSyncGroup := v1.Group("/projects/:id/sync/:ref")
			flatSyncGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			// Resolve permissions (claim-token grant, project membership, or the
			// project's workspace role) so the sync handlers' permission checks
			// enforce rather than fail open.
			flatSyncGroup.Use(s.ProjectAccessMiddleware())
			flatSyncGroup.GET("/pull", s.HandleSyncPull)
			flatSyncGroup.GET("/blocks", s.HandleSyncGetBlocks)
			flatSyncGroup.GET("/status", s.HandleSyncPushStatus)
			flatSyncGroup.POST("/push/init", s.HandleSyncPushInit)
			flatSyncGroup.POST("/push/diff", s.HandleSyncPushDiff)
			flatSyncGroup.POST("/push/commit", s.HandleSyncPushCommit, syncRateLimit)
			flatSyncGroup.PUT("/push/chunks/:uploadId/:chunkIndex", s.HandleSyncProxyChunkUpload)
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
			wsSpecific.Use(WeeklyAllocationMiddleware(s.BillingStore))
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

		// Invite routes (workspace-scoped, admin/owner only).
		wsSpecific.POST("/invites", s.HandleCreateInvite)
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

		s.registerWorkspaceContentRoutes(wsSpecific)

		// @bravo agent routes (Bowrain AD-016) with QuotaGuard for credit-consuming operations.
		s.registerBravoRoutes(wsSpecific)

		// Billing routes (Bowrain AD-018, workspace-scoped)
		billingGroup := wsSpecific.Group("/billing")
		billingGroup.GET("", s.HandleGetBilling)
		billingGroup.GET("/usage", s.HandleGetBillingUsage)
		billingGroup.GET("/model-usage", s.HandleGetBillingModelUsage)
		billingGroup.POST("/checkout", s.HandleCreateCheckout)
		billingGroup.POST("/portal", s.HandleCreatePortal)
		billingGroup.GET("/invoices", s.HandleGetInvoices)
		billingGroup.POST("/buy-credits", s.HandleBuyCredits)
	}

	// Stripe webhook (no auth, signature-verified) (Bowrain AD-018).
	e.POST("/api/webhooks/stripe", s.HandleStripeWebhook)

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
		adminMiddleware = billing.AdminGuard(s.AdminVerifier, accessVerifier)
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
	if mountAdmin {
		adminGroup := e.Group("/api/admin", adminMiddleware)
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
		adminGroup.GET("/events", s.HandleAdminListEvents)
		adminGroup.GET("/upsells", s.HandleAdminGetUpsells)
		adminGroup.GET("/overrides", s.HandleAdminListOverrides)
		adminGroup.GET("/slug-reservations", s.HandleAdminListSlugReservations)
		adminGroup.POST("/slug-reservations/release", s.HandleAdminReleaseSlugReservation)
	}

	// MCP server (brand voice resources, tools, prompts via Streamable HTTP).
	if s.mcpServer != nil {
		s.mcpServer.RegisterRoutes(e)
	}

	// Web UI static file serving (development and E2E only).
	// A single handler serves static files first and falls back to index.html
	// for SPA client-side routing. Using two separate handlers (e.Static + e.GET)
	// would conflict because Echo overwrites the first GET /* with the second.
	if s.Config.WebUIDir != "" || s.Config.PulseUIDir != "" {
		e.GET("/*", func(c echo.Context) error {
			// Host-based routing: serve Pulse SPA for pulse.* subdomain.
			host := c.Request().Host
			if s.Config.PulseUIDir != "" && strings.HasPrefix(host, "pulse.") {
				return serveSPAFile(c, s.Config.PulseUIDir)
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
func serveSPAFile(c echo.Context, dir string) error {
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
func (s *Server) registerWorkspaceContentRoutes(g *echo.Group) {
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

	// TM CRUD — Bowrain AD-011: /:ws/translation-memory
	g.GET("/translation-memory", s.HandleGetTMEntries)
	g.GET("/translation-memory/count", s.HandleGetTMCount)
	g.POST("/translation-memory", s.HandleAddTMEntry)
	g.PUT("/translation-memory/:eid", s.HandleUpdateTMEntry)
	g.DELETE("/translation-memory/:eid", s.HandleDeleteTMEntry)

	// Terminology CRUD — Bowrain AD-011: /:ws/terms
	g.GET("/terms", s.HandleGetTerms)
	g.GET("/terms/count", s.HandleGetTermCount)
	g.POST("/terms", s.HandleAddConcept)
	g.PUT("/terms/:cid", s.HandleUpdateConcept)
	g.DELETE("/terms/:cid", s.HandleDeleteConcept)
	g.POST("/terms/import/csv", s.HandleImportTermsCSV)
	g.POST("/terms/import/json", s.HandleImportTermsJSON)
	g.GET("/terms/export/json", s.HandleExportTermsJSON)

	// Provider configs — Bowrain AD-011: /:ws/providers
	g.GET("/providers", s.HandleListProviderConfigs)
	g.POST("/providers", s.HandleSaveProviderConfig)
	g.DELETE("/providers/:id", s.HandleDeleteProviderConfig)
	g.POST("/providers/test", s.HandleTestProviderConfig)

	// Connectors — Bowrain AD-011: /:ws/connectors (moved from public)
	g.GET("/connectors", s.HandleListActiveConnectors)
	g.POST("/connectors", s.HandleAddConnector)
	g.DELETE("/connectors/:id", s.HandleRemoveConnector)
	g.GET("/connectors/:id/status", s.HandleConnectorStatus)
	g.POST("/connectors/:id/fetch", s.HandleFetch)
	g.POST("/connectors/:id/publish", s.HandlePublish)

	// Brand profiles — Bowrain AD-011: /:ws/brand-profiles
	g.GET("/brand-profiles", s.HandleListBrandProfiles)
	g.POST("/brand-profiles", s.HandleCreateBrandProfile)
	g.GET("/brand-profiles/:id", s.HandleGetBrandProfile)
	g.PUT("/brand-profiles/:id", s.HandleUpdateBrandProfile)
	g.DELETE("/brand-profiles/:id", s.HandleDeleteBrandProfile)
	g.POST("/brand-profiles/:id/check", s.HandleCheckBrandVoice)
	g.POST("/brand-profiles/from-starter", s.HandleCreateFromStarter)
	g.GET("/brand-profiles/suggested-rules", s.HandleGetSuggestedRules)
	g.GET("/brand-profiles/:id/candidates", s.HandleListCandidates)
	g.POST("/brand-profiles/:id/promote-rule", s.HandlePromoteSuggestedRule)
	g.POST("/brand-profiles/:id/demote-rule", s.HandleDemoteSuggestedRule)
	g.POST("/brand-profiles/:id/reject-rule", s.HandleRejectSuggestedRule)
	g.POST("/brand-profiles/:id/evaluate-rule", s.HandleEvaluateRulePromotion)
	g.GET("/brand-profiles/starter-packs", s.HandleListStarterPacks)

	// Translation jobs — Bowrain AD-011: /:ws/jobs
	g.POST("/jobs/translate", s.HandleCreateTranslationJob)
	g.GET("/jobs", s.HandleListJobs)
	g.GET("/jobs/:id", s.HandleGetJob)
	g.DELETE("/jobs/:id", s.HandleDeleteJob)
	g.GET("/ai-usage", s.HandleGetAIUsage)

	// Graph — Bowrain AD-011: /:ws/graph
	g.GET("/graph/concepts", s.HandleGetConceptHierarchy)
	g.GET("/graph/nodes/:nodeId/neighbors", s.HandleGetGraphNeighbors)
	g.GET("/graph/nodes/:nodeId/edges", s.HandleGetGraphEdges)
	g.GET("/graph/shortest-path", s.HandleGetShortestPath)

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

	// Tasks — Bowrain AD-011: /:ws/tasks (no more /my/tasks, use ?assignee_id=me)
	g.GET("/tasks", s.HandleListTasks)
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

	// Project members — Bowrain AD-011: /:ws/:id/members
	g.GET("/:id/members", s.HandleListProjectMembers)
	g.POST("/:id/members", s.HandleAddProjectMember)
	g.PUT("/:id/members/:uid", s.HandleUpdateProjectMember)
	g.DELETE("/:id/members/:uid", s.HandleRemoveProjectMember)

	// Project settings — Bowrain AD-011: /:ws/:id/settings
	g.GET("/:id/settings/extraction", s.HandleGetExtractionSettings)
	g.PUT("/:id/settings/extraction", s.HandleUpdateExtractionSettings)

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

	// Items — Bowrain AD-011: /:ws/:id/items/:ref
	g.GET("/:id/items/:ref", s.HandleGetFileBlocks) // list items
	g.POST("/:id/items/:ref", s.HandleUploadFiles)
	g.DELETE("/:id/items/:ref", s.HandleRemoveFile) // ?item=path/to/file

	// Blocks — Bowrain AD-011: /:ws/:id/blocks/:ref
	g.GET("/:id/blocks/:ref", s.HandleGetFileBlocks)
	g.PUT("/:id/blocks/:ref/:bid", s.HandleUpdateBlockTarget)
	g.PUT("/:id/blocks/:ref/:bid/runs", s.HandleUpdateBlockTargetRuns)
	g.PUT("/:id/blocks/:ref/:bid/status", s.HandleSetBlockStatus)
	g.GET("/:id/blocks/:ref/:bid/history", s.HandleGetBlockHistory)
	g.POST("/:id/blocks/:ref/:bid/rollback", s.HandleRollbackBlock)
	g.POST("/:id/revert", s.HandleRevertBatch)
	g.POST("/:id/restore", s.HandleRestoreToPoint)
	g.GET("/:id/blocks/:ref/:bid/notes", s.HandleListBlockNotes)
	g.POST("/:id/blocks/:ref/:bid/notes", s.HandleAddBlockNote)
	g.DELETE("/:id/blocks/:ref/:bid/notes/:nid", s.HandleDeleteBlockNote)
	g.GET("/:id/blocks/:ref/:bid/tm-matches", s.HandleLookupTMForBlock)
	g.GET("/:id/blocks/:ref/:bid/term-matches", s.HandleLookupTermsForBlock)
	g.GET("/:id/blocks/:ref/:bid/html", s.HandleRenderBlockHTML)

	// Entities on blocks — Bowrain AD-011: /:ws/:id/blocks/:ref/:bid/entities
	g.POST("/:id/blocks/:ref/:bid/entities", s.HandleCreateEntity)
	g.PUT("/:id/blocks/:ref/:bid/entities/:idx", s.HandleUpdateEntity)
	g.DELETE("/:id/blocks/:ref/:bid/entities/:idx", s.HandleDeleteEntity)
	g.POST("/:id/blocks/:ref/:bid/entities/:idx/promote", s.HandlePromoteEntity)

	// Actions — Bowrain AD-011: /:ws/:id/actions/:ref/<verb>
	g.POST("/:id/actions/:ref/pseudo-translate", s.HandlePseudoTranslate)
	g.POST("/:id/actions/:ref/ai-translate", s.HandleAITranslate, billing.QuotaGuard(s.BillingStore, s.billingGuardEvent()))
	g.POST("/:id/actions/:ref/tm-translate", s.HandleTMTranslate)
	g.POST("/:id/actions/:ref/export", s.HandleExportTranslatedFile)
	g.POST("/:id/actions/:ref/qa-check", s.HandleQACheckFile)
	g.POST("/:id/actions/:ref/qa-check-block", s.HandleQACheckBlock)

	// Preview and word count — Bowrain AD-011: /:ws/:id/preview/:ref, /:ws/:id/word-count/:ref
	g.GET("/:id/preview/:ref", s.HandleRenderDocumentPreview)
	g.GET("/:id/word-count/:ref", s.HandleGetWordCount)

	// Dashboard — Bowrain AD-011: /:ws/:id/dashboard/:ref
	g.GET("/:id/dashboard/:ref", s.HandleGetTranslationDashboard)

	// Sync — Bowrain AD-011: /:ws/:id/sync/:ref
	g.GET("/:id/sync/:ref/pull", s.HandleSyncPull)
	g.GET("/:id/sync/:ref/blocks", s.HandleSyncGetBlocks)
	g.GET("/:id/sync/:ref/status", s.HandleSyncPushStatus)
	g.POST("/:id/sync/:ref/push/init", s.HandleSyncPushInit)
	g.POST("/:id/sync/:ref/push/diff", s.HandleSyncPushDiff)
	g.POST("/:id/sync/:ref/push/commit", s.HandleSyncPushCommit, syncRateLimit)
	g.PUT("/:id/sync/:ref/push/chunks/:uploadId/:chunkIndex", s.HandleSyncProxyChunkUpload)
	g.POST("/:id/sync/:ref/translate", s.HandleCreateProjectTranslationJob)

	// Collections — Bowrain AD-011: /:ws/:id/collections/:ref
	g.GET("/:id/collections/:ref", s.HandleListCollections)
	g.POST("/:id/collections/:ref", s.HandleCreateCollection)
	g.GET("/:id/collections/:ref/:cid", s.HandleGetCollection)
	g.PUT("/:id/collections/:ref/:cid", s.HandleUpdateCollection)
	g.DELETE("/:id/collections/:ref/:cid", s.HandleDeleteCollection)
	g.POST("/:id/collections/:ref/:cid/items", s.HandleUploadToCollection)

	// Assets — Bowrain AD-011: /:ws/:id/assets/:ref
	g.POST("/:id/assets/:ref/upload-url", s.HandleAssetUploadURL)
	g.GET("/:id/assets/:ref", s.HandleListAssets)
	g.POST("/:id/assets/:ref", s.HandleCreateAsset)
	g.GET("/:id/assets/:ref/:aid", s.HandleGetAsset)
	g.DELETE("/:id/assets/:ref/:aid", s.HandleDeleteAsset)
	g.POST("/:id/assets/:ref/:aid/variants/upload-url", s.HandleVariantUploadURL)
	g.GET("/:id/assets/:ref/:aid/variants", s.HandleListVariants)
	g.POST("/:id/assets/:ref/:aid/variants", s.HandleCreateVariant)

	// Review queue — Bowrain AD-011: /:ws/:id/review-queue/:ref
	g.GET("/:id/review-queue/:ref", s.HandleListReviewQueue)
	g.GET("/:id/review-queue/:ref/:itemId", s.HandleGetReviewQueueItem)
	g.POST("/:id/review-queue/:ref/:itemId/decide", s.HandleDecideReviewItem)
	g.POST("/:id/review-queue/:ref/:itemId/assign", s.HandleAssignReviewItem)
	g.POST("/:id/review-queue/:ref/:itemId/split", s.HandleSplitReviewItem)
	g.POST("/:id/review-queue/:ref/batch-decide", s.HandleBatchDecideReviewItems)
	g.POST("/:id/review-queue/:ref/sync", s.HandleSyncReviewDecisions)

	// Brand voice — Bowrain AD-011: /:ws/:id/brand-voice/:ref
	g.GET("/:id/brand-voice/:ref/scores", s.HandleGetBrandVoiceScores)
	g.GET("/:id/brand-voice/:ref/scores/:locale", s.HandleGetBrandVoiceScoresByLocale)
	g.GET("/:id/brand-voice/:ref/trends", s.HandleGetBrandVoiceTrends)
	g.GET("/:id/brand-voice/:ref/drift", s.HandleGetBrandVoiceDrift)
	g.POST("/:id/brand-voice/:ref/drift-check", s.HandleRunBrandVoiceDriftCheck)
	g.POST("/:id/brand-voice/:ref/corrections", s.HandleCreateBrandVoiceCorrection)

	// Collab — Bowrain AD-011: /:ws/:id/collab/:ref
	g.GET("/:id/collab/:ref", s.HandleCollabWebSocket)
}

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

	// When no gRPC server is configured, use Echo's built-in listener.
	if s.GRPCServer == nil {
		return e.Start(addr)
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
		Addr:      addr,
		Handler:   handler,
		Protocols: protocols,
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
	if s.DailyDigestWorker != nil {
		s.DailyDigestWorker.Close()
	}
	if s.WeeklyDigestWorker != nil {
		s.WeeklyDigestWorker.Close()
	}
	if s.deadlineChecker != nil {
		s.deadlineChecker.Close()
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

// corsConfig builds a CORS middleware configuration. When a fixed
// OIDCPublicURL is configured (production), only that origin is allowed.
// Otherwise, the middleware dynamically allows the request's own origin
// (same-origin requests only).
func (s *Server) corsConfig() middleware.CORSConfig {
	cfg := middleware.CORSConfig{
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-Requested-With"},
	}

	if s.Config.OIDCPublicURL != "" {
		// Use the OIDC public URL's origin (scheme + host) as the allowed origin.
		if u, err := url.Parse(s.Config.OIDCPublicURL); err == nil && u.Host != "" {
			origin := u.Scheme + "://" + u.Host
			cfg.AllowOrigins = []string{origin}
			return cfg
		}
	}

	// Dynamic: allow only requests whose Origin matches the server's own host.
	cfg.AllowOriginFunc = func(origin string) (bool, error) {
		u, err := url.Parse(origin)
		if err != nil {
			return false, nil
		}
		// Allow localhost origins in development.
		if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
			return true, nil
		}
		return false, nil
	}
	return cfg
}
