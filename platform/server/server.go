package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/connector"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/event"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/mailer"
	mcpserver "github.com/neokapi/neokapi/bowrain/server/mcp"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/formats"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/registry"
	corestorage "github.com/neokapi/neokapi/core/storage"
	libtools "github.com/neokapi/neokapi/core/tools"
	platagent "github.com/neokapi/neokapi/platform/agent"
	platconn "github.com/neokapi/neokapi/platform/connector"
	platev "github.com/neokapi/neokapi/platform/event"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

// Server is the REST API server for neokapi.
type Server struct {
	Config         ServerConfig
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

	// AutomationEngine evaluates automation rules on events. Nil when event system is not wired up.
	AutomationEngine *event.AutomationEngine

	// AutomationRuleStore persists automation rules. Nil when not configured.
	AutomationRuleStore *event.RuleStore

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

	// AutomationRunStore persists automation runs, steps, and logs (AD-035). Nil when not configured.
	AutomationRunStore *bstore.AutomationRunStore

	// stepCompletionTracker monitors async automation steps. Nil when not configured.
	stepCompletionTracker *event.StepCompletionTracker

	// runHub manages SSE connections for live automation run updates. Always initialized.
	runHub *automationRunHub

	// SyncCache is the optional Redis hash cache for sync diff engine (AD-038).
	SyncCache bowsync.HashCache


	// ExtractionJobStore persists extraction job state. Nil when job system is not configured.
	ExtractionJobStore jobs.ExtractionJobStore

	// ExtractionQueue enqueues extraction job IDs. Nil when not configured.
	ExtractionQueue jobs.Queue

	// dashboardCache caches translation dashboard stats per project/stream.
	dashboardCache sync.Map // map[string]*dashboardCacheEntry

	// pulseCache caches Pulse public dashboard responses with TTL-based expiry.
	pulseCache *pulseCache

	// AgentStore persists @bravo agent conversations, messages, and config (AD-028).
	// Nil when agent system is not configured.
	AgentStore platagent.AgentStore

	// AgentService orchestrates @bravo agent lifecycle (AD-028).
	// Nil when agent system is not configured.
	AgentService *service.AgentService

	// BillingStore persists subscription and credit data (AD-030).
	// Nil when billing is not configured.
	BillingStore billing.BillingStore

	// StripeClient manages Stripe API interactions (AD-030).
	// Nil when STRIPE_SECRET_KEY is not set.
	StripeClient *billing.StripeClient

	// PostHogClient captures product analytics events (AD-030).
	// Nil when POSTHOG_API_KEY is not set.
	PostHogClient *analytics.PostHogClient

	// WebhookHandler processes Stripe webhook events (AD-030).
	// Nil when Stripe is not configured.
	WebhookHandler *billing.WebhookHandler

	// AdminVerifier validates ID tokens from the admin OIDC realm (AD-030).
	// Nil when admin OIDC is not configured.
	AdminVerifier *oidc.IDTokenVerifier

	// AdminAccessVerifier validates access tokens from the admin OIDC realm.
	// Keycloak access tokens use aud="account" so the standard ID-token
	// verifier rejects them. This verifier skips the audience check.
	AdminAccessVerifier *oidc.IDTokenVerifier
}

// NewServer creates a new Server with the given configuration.
// createEventBus selects the event bus backend based on configuration (AD-036).
func createEventBus(cfg ServerConfig) platev.EventBus {
	if cfg.ServiceBusConnection != "" {
		bus, err := event.NewServiceBusEventBus(cfg.ServiceBusConnection)
		if err != nil {
			log.Printf("WARNING: failed to create Service Bus event bus: %v (falling back to in-memory)", err)
			return event.NewChannelEventBus()
		}
		log.Println("Using Azure Service Bus event bus")
		return bus
	}
	if cfg.NATSUrl != "" {
		bus, err := event.NewNATSEventBus(cfg.NATSUrl)
		if err != nil {
			log.Printf("WARNING: failed to create NATS event bus: %v (falling back to in-memory)", err)
			return event.NewChannelEventBus()
		}
		log.Println("Using NATS JetStream event bus")
		return bus
	}
	log.Println("Using in-memory event bus (single instance only)")
	return event.NewChannelEventBus()
}

func NewServer(cfg ServerConfig) *Server {
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
			log.Printf("WARNING: failed to connect to Redis for session store: %v (falling back to in-memory)", err)
			s.SessionStore = NewMemorySessionStore()
		} else {
			s.SessionStore = rs
			log.Printf("Using Redis session store at %s", cfg.RedisURL)
		}

		// Wire Redis hash cache for sync diff engine (AD-038).
		redisOpts, err := redis.ParseURL(cfg.RedisURL)
		if err == nil {
			if cfg.RedisPassword != "" {
				redisOpts.Password = cfg.RedisPassword
			}
			redisClient := redis.NewClient(redisOpts)
			s.SyncCache = bowsync.NewRedisHashCache(redisClient, 30*time.Minute)
			log.Printf("Using Redis sync hash cache")
		}
	} else {
		s.SessionStore = NewMemorySessionStore()
	}

	// Initialize credential store.
	s.CredentialStore = credentials.NewStore(credentials.DefaultPath())

	// Initialize email sender and mailer.
	s.initMailer(cfg)

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
			log.Printf("WARNING: failed to open PostgreSQL stores: %v", err)
		} else {
			s.ContentStore = pg.Content
			s.Services = service.NewServices(pg.Content, connReg, formatReg, toolReg)
			s.JobStore = pg.Job
			s.ExtractionJobStore = pg.Extraction
			s.QuotaStore = pg.Quota
			s.wsStores.pgDB = pg.DB
			pgSQL := pg.DB.DB // embedded *sql.DB
			s.AuditLogger = event.NewAuditLogger(pgSQL, s.EventBus)
			s.AutomationRuleStore = event.NewPostgresRuleStore(pgSQL)
			s.ReviewQueueStore = bstore.NewPostgresReviewQueueStore(pgSQL)
			s.NotificationStore = bstore.NewPostgresNotificationStore(pgSQL)
			s.ActivityStore = bstore.NewPostgresActivityStore(pgSQL)
			s.TaskStore = bstore.NewPostgresTaskStore(pgSQL)
			s.AutomationRunStore = bstore.NewAutomationRunStorePg(pgSQL)
			s.PreferenceStore = bstore.NewPostgresPreferenceStore(pgSQL)
			s.DigestStore = bstore.NewPostgresDigestStore(pgSQL)
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
			log.Printf("WARNING: failed to connect to Service Bus queue: %v", err)
		} else {
			s.JobQueue = q
		}
	case cfg.NATSUrl != "":
		q, err := jobs.NewNATSQueue(cfg.NATSUrl)
		if err != nil {
			log.Printf("WARNING: failed to connect to NATS queue: %v", err)
		} else {
			s.JobQueue = q
		}
	}

	// Initialize blob storage (AD-029).
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

	// Wire up automation engine with run manager (AD-035).
	s.runHub = newAutomationRunHub()

	runManager := event.NewAutomationRunManager(s.AutomationRunStore, s.executeAutomationAction)
	s.AutomationEngine = event.NewAutomationEngine(s.EventBus, runManager.Execute)
	s.registerDefaultAutomations()

	// Wire up activity recorder (AD-027).
	if s.ActivityStore != nil {
		s.ActivityRecorder = event.NewActivityRecorder(s.ActivityStore, s.EventBus)
	}

	// Wire up notification dispatcher (AD-027).
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

	// Wire up digest workers (AD-027).
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

	// Wire up deadline checker (AD-027).
	if s.TaskStore != nil && s.NotificationDispatcher != nil {
		s.deadlineChecker = event.NewDeadlineChecker(s.TaskStore, s.NotificationDispatcher, 1*time.Hour)
		s.deadlineChecker.Start()
	}

	// Wire up progress milestone tracker (AD-027).
	if s.ContentStore != nil && s.NotificationDispatcher != nil {
		s.progressTracker = event.NewProgressTracker(s.ContentStore, s.NotificationDispatcher, s.EventBus)
	}

	// Wire up push completion tracker (AD-034).
	if s.EventBus != nil && s.JobStore != nil {
		s.pushCompletionTracker = event.NewPushCompletionTracker(
			s.EventBus, s.JobStore, s.ExtractionJobStore, s.ContentStore,
		)
		if s.AutomationRunStore != nil {
			s.pushCompletionTracker.SetRunStore(s.AutomationRunStore)
		}
	}

	// Wire up step completion tracker (AD-035).
	if s.AutomationRunStore != nil && s.JobStore != nil {
		s.stepCompletionTracker = event.NewStepCompletionTracker(
			s.AutomationRunStore, s.JobStore, s.ExtractionJobStore,
		)
	}

	// Wire up run retention cleaner (AD-035): delete runs older than 30 days, check daily.
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
		if s.ToolRegistry != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithToolRegistry(s.ToolRegistry))
		}
		if s.PostHogClient != nil {
			mcpOpts = append(mcpOpts, mcpserver.WithEventTracker(&eventTrackerAdapter{client: s.PostHogClient}))
		}
		ms, err := mcpserver.NewMCPServerWithStore(s.BrandStore, s.ContentStore, mcpCfg, mcpOpts...)
		if err != nil {
			log.Printf("WARNING: failed to initialize MCP server: %v", err)
		} else {
			s.mcpServer = ms
		}
	}

	// Initialize agent service (AD-028).
	if s.AgentStore != nil {
		s.AgentService = service.NewAgentService(s.AgentStore, s.EventBus)

		switch cfg.AgentRuntime {
		case "queue":
			// Queue mode: agent processing is handled by the worker.
			// API server enqueues jobs to Service Bus and subscribes to Redis pub/sub.
			if err := s.setupAgentQueue(cfg); err != nil {
				log.Printf("WARNING: failed to initialize agent queue mode: %v", err)
			} else {
				log.Printf("Agent mode: queue (worker handles container lifecycle)")
			}
		case "docker", "aca":
			// Direct mode: API server manages containers directly.
			if pool := s.buildAgentPool(); pool != nil {
				s.AgentService.SetPool(pool)
				log.Printf("Agent pool initialized (runtime=%s)", cfg.AgentRuntime)
			}
		case "":
			// No runtime — mock mode.
		default:
			log.Printf("WARNING: unknown agent runtime %q", cfg.AgentRuntime)
		}
	}

	// Initialize Stripe client (AD-030).
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
		log.Printf("Stripe billing enabled")
	}

	// Initialize PostHog client (AD-030).
	if cfg.PostHogAPIKey != "" {
		host := cfg.PostHogHost
		if host == "" {
			host = "https://us.i.posthog.com"
		}
		phClient, err := analytics.NewPostHogClient(cfg.PostHogAPIKey, host)
		if err != nil {
			log.Printf("WARNING: failed to init PostHog client: %v (analytics disabled)", err)
		} else {
			s.PostHogClient = phClient
			log.Printf("PostHog analytics enabled")
		}
	}

	// Wire PostHog to webhook handler now that both are initialized.
	if s.PostHogClient != nil && s.WebhookHandler != nil {
		s.WebhookHandler.SetEventTracker(s.PostHogClient)
	}

	// Initialize admin OIDC verifier (AD-030).
	if cfg.AdminOIDCIssuerURL != "" && cfg.AdminOIDCClientID != "" {
		ctx := context.Background()
		verifier, err := auth.NewOIDCVerifier(ctx, cfg.AdminOIDCIssuerURL, cfg.AdminOIDCClientID)
		if err != nil {
			log.Printf("WARNING: failed to init admin OIDC verifier: %v (admin API disabled)", err)
		} else {
			s.AdminVerifier = verifier
			// Access token verifier skips audience check (Keycloak uses aud="account").
			accessVerifier, err := auth.NewOIDCAccessTokenVerifier(ctx, cfg.AdminOIDCIssuerURL)
			if err != nil {
				log.Printf("WARNING: failed to init admin access token verifier: %v", err)
			} else {
				s.AdminAccessVerifier = accessVerifier
			}
			log.Printf("Admin OIDC verifier enabled (issuer: %s)", cfg.AdminOIDCIssuerURL)
		}
	}

	// Build billing hooks for credit deduction + Stripe meter reporting (AD-030).
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

		if s.AgentService != nil {
			s.AgentService.SetBillingHooks(billingHooks)
		}
	}

	return s
}

// SetupRoutes registers all API routes on the Echo instance.
func (s *Server) SetupRoutes(e *echo.Echo) {
	// Middleware
	logger := log.New(os.Stdout, "", log.LstdFlags)
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod: true,
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logger.Printf("%s %s %d\n", v.Method, v.URI, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("50M"))
	e.Use(middleware.CORS())

	// API v1 routes
	v1 := e.Group("/api/v1")

	// Public endpoints (no auth).
	v1.GET("/health", s.HandleHealth)
	v1.GET("/ready", s.HandleReady)
	v1.GET("/info", s.HandleInfo)
	v1.GET("/badges/:proj", s.HandleProjectBadge)

	// Pulse public activity dashboard (AD-033).
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

		// Protected auth routes (require valid token)
		authProtected := authGroup.Group("")
		authProtected.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		authProtected.GET("/me", s.HandleAuthMe)
		authProtected.POST("/logout", s.HandleAuthLogout)
		authProtected.POST("/token/exchange", s.HandleTokenExchange)

		// Project claim and invite acceptance (auth required, no workspace).
		jwtProtected := v1.Group("")
		jwtProtected.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		jwtProtected.POST("/projects/claim", s.HandleClaimProject)
		jwtProtected.POST("/join/:code", s.HandleAcceptInvite)

		// Flat sync routes for unclaimed projects (claim-token or JWT auth).
		// AD-040: /api/v1/projects/:id/sync/:ref/*
		if s.AuthStore != nil {
			syncRateLimit := RateLimitSyncPush(10, 3)
			flatSyncGroup := v1.Group("/projects/:id/sync/:ref")
			flatSyncGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			flatSyncGroup.GET("/pull", s.HandleSyncPull)
			flatSyncGroup.GET("/blocks", s.HandleSyncGetBlocks)
			flatSyncGroup.GET("/status", s.HandleSyncPushStatus)
			flatSyncGroup.POST("/push/init", s.HandleSyncPushInit)
			flatSyncGroup.POST("/push/diff", s.HandleSyncPushDiff)
			flatSyncGroup.POST("/push/commit", s.HandleSyncPushCommit, syncRateLimit)
			flatSyncGroup.PUT("/push/chunks/:uploadId/:chunkIndex", s.HandleSyncProxyChunkUpload)
		}

		// Workspace collection routes: list and create (require auth).
		// AD-040: /api/v1/workspaces (collection noun for list/create)
		wsCollectionGroup := v1.Group("/workspaces")
		wsCollectionGroup.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		wsCollectionGroup.POST("", s.HandleCreateWorkspace)
		wsCollectionGroup.GET("", s.HandleListWorkspaces)

		// Workspace-specific routes: bare slug at /:ws (require auth + membership).
		// AD-040: /api/v1/:ws (bare workspace slug)
		wsSpecific := v1.Group("/:ws")
		wsSpecific.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		if s.AuthStore != nil {
			wsSpecific.Use(WorkspaceAccessMiddleware(s.AuthStore))
		}
		wsSpecific.GET("", s.HandleGetWorkspace)
		wsSpecific.PUT("", s.HandleUpdateWorkspace)
		wsSpecific.PATCH("", s.HandleUpdateWorkspace)
		wsSpecific.DELETE("", s.HandleDeleteWorkspace)
		wsSpecific.GET("/members", s.HandleListMembers)
		wsSpecific.POST("/members", s.HandleAddMember)
		wsSpecific.PUT("/members/:uid/role", s.HandleUpdateMemberRole)
		wsSpecific.DELETE("/members/:uid", s.HandleRemoveMember)

		// Invite routes (workspace-scoped, admin/owner only).
		wsSpecific.POST("/invites", s.HandleCreateInvite)
		wsSpecific.GET("/invites", s.HandleListInvites)
		wsSpecific.DELETE("/invites/:id", s.HandleDeleteInvite)

		// Role template routes (workspace-scoped, admin/owner only for mutations).
		wsSpecific.GET("/roles", s.HandleListRoleTemplates)
		wsSpecific.POST("/roles", s.HandleCreateRoleTemplate)
		wsSpecific.PUT("/roles/:rid", s.HandleUpdateRoleTemplate)
		wsSpecific.DELETE("/roles/:rid", s.HandleDeleteRoleTemplate)

		// API token routes (workspace-scoped, requires Pro+ plan).
		tokenGroup := wsSpecific.Group("/tokens")
		tokenGroup.Use(billing.PlanGuard(billing.FeatureAPIAccess))
		tokenGroup.POST("", s.HandleCreateToken)
		tokenGroup.GET("", s.HandleListTokens)
		tokenGroup.DELETE("/:id", s.HandleDeleteToken)

		s.registerWorkspaceContentRoutes(wsSpecific)

		// @bravo agent routes (AD-028) with QuotaGuard for credit-consuming operations.
		s.registerBravoRoutes(wsSpecific)

		// Billing routes (AD-030, workspace-scoped)
		billingGroup := wsSpecific.Group("/billing")
		billingGroup.GET("", s.HandleGetBilling)
		billingGroup.GET("/usage", s.HandleGetBillingUsage)
		billingGroup.POST("/checkout", s.HandleCreateCheckout)
		billingGroup.POST("/portal", s.HandleCreatePortal)
		billingGroup.GET("/invoices", s.HandleGetInvoices)
		billingGroup.POST("/buy-credits", s.HandleBuyCredits)
	}

	// Stripe webhook (no auth, signature-verified) (AD-030).
	e.POST("/api/webhooks/stripe", s.HandleStripeWebhook)

	// Admin routes (admin realm auth) (AD-030).
	// When no admin OIDC verifier is configured (dev/test), fall back to
	// JWT auth so admin routes remain accessible for operational tasks.
	if s.AdminVerifier != nil || s.Config.JWTSecret != "" {
		var adminMiddleware echo.MiddlewareFunc
		if s.AdminVerifier != nil {
			accessVerifier := s.AdminAccessVerifier
			if accessVerifier == nil {
				accessVerifier = s.AdminVerifier
			}
			adminMiddleware = billing.AdminGuard(s.AdminVerifier, accessVerifier)
		} else {
			adminMiddleware = AuthMiddleware(s.Config.JWTSecret, s.AuthStore)
		}
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
		adminGroup.POST("/workspaces/:id/impersonate", s.HandleAdminImpersonate)
		adminGroup.POST("/workspaces/:id/members", s.HandleAdminAddMember)
		adminGroup.GET("/users", s.HandleAdminListUsers)
		adminGroup.GET("/users/:id", s.HandleAdminGetUser)
		adminGroup.GET("/metrics", s.HandleAdminGetMetrics)
		adminGroup.GET("/events", s.HandleAdminListEvents)
		adminGroup.GET("/upsells", s.HandleAdminGetUpsells)
		adminGroup.GET("/overrides", s.HandleAdminListOverrides)
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
	filePath := filepath.Join(dir, filepath.Clean(reqPath))
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		return c.File(filePath)
	}
	return c.File(filepath.Join(dir, "index.html"))
}

// registerWorkspaceContentRoutes registers all workspace-scoped content routes
// on the given route group (mounted at /:ws).
//
// AD-040 URL patterns:
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
		g.Use(ProjectAccessMiddleware(s.AuthStore))
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

	// TM CRUD — AD-040: /:ws/translation-memory
	g.GET("/translation-memory", s.HandleGetTMEntries)
	g.GET("/translation-memory/count", s.HandleGetTMCount)
	g.POST("/translation-memory", s.HandleAddTMEntry)
	g.PUT("/translation-memory/:eid", s.HandleUpdateTMEntry)
	g.DELETE("/translation-memory/:eid", s.HandleDeleteTMEntry)

	// Terminology CRUD — AD-040: /:ws/terms
	g.GET("/terms", s.HandleGetTerms)
	g.GET("/terms/count", s.HandleGetTermCount)
	g.POST("/terms", s.HandleAddConcept)
	g.PUT("/terms/:cid", s.HandleUpdateConcept)
	g.DELETE("/terms/:cid", s.HandleDeleteConcept)
	g.POST("/terms/import/csv", s.HandleImportTermsCSV)
	g.POST("/terms/import/json", s.HandleImportTermsJSON)
	g.GET("/terms/export/json", s.HandleExportTermsJSON)

	// Provider configs — AD-040: /:ws/providers
	g.GET("/providers", s.HandleListProviderConfigs)
	g.POST("/providers", s.HandleSaveProviderConfig)
	g.DELETE("/providers/:id", s.HandleDeleteProviderConfig)
	g.POST("/providers/test", s.HandleTestProviderConfig)

	// Connectors — AD-040: /:ws/connectors (moved from public)
	g.GET("/connectors", s.HandleListActiveConnectors)
	g.POST("/connectors", s.HandleAddConnector)
	g.DELETE("/connectors/:id", s.HandleRemoveConnector)
	g.GET("/connectors/:id/status", s.HandleConnectorStatus)
	g.POST("/connectors/:id/fetch", s.HandleFetch)
	g.POST("/connectors/:id/publish", s.HandlePublish)

	// Brand profiles — AD-040: /:ws/brand-profiles
	g.GET("/brand-profiles", s.HandleListBrandProfiles)
	g.POST("/brand-profiles", s.HandleCreateBrandProfile)
	g.GET("/brand-profiles/:id", s.HandleGetBrandProfile)
	g.PUT("/brand-profiles/:id", s.HandleUpdateBrandProfile)
	g.DELETE("/brand-profiles/:id", s.HandleDeleteBrandProfile)
	g.POST("/brand-profiles/:id/check", s.HandleCheckBrandVoice)
	g.POST("/brand-profiles/from-starter", s.HandleCreateFromStarter)
	g.GET("/brand-profiles/suggested-rules", s.HandleGetSuggestedRules)
	g.GET("/brand-profiles/starter-packs", s.HandleListStarterPacks)

	// Translation jobs — AD-040: /:ws/jobs
	g.POST("/jobs/translate", s.HandleCreateTranslationJob)
	g.GET("/jobs", s.HandleListJobs)
	g.GET("/jobs/:id", s.HandleGetJob)
	g.DELETE("/jobs/:id", s.HandleDeleteJob)
	g.GET("/ai-usage", s.HandleGetAIUsage)

	// Graph — AD-040: /:ws/graph
	g.GET("/graph/concepts", s.HandleGetConceptHierarchy)
	g.GET("/graph/nodes/:nodeId/neighbors", s.HandleGetGraphNeighbors)
	g.GET("/graph/nodes/:nodeId/edges", s.HandleGetGraphEdges)
	g.GET("/graph/shortest-path", s.HandleGetShortestPath)

	// Notifications — AD-040: /:ws/notifications
	g.GET("/notifications", s.HandleListNotifications)
	g.POST("/notifications/:nid/read", s.HandleMarkNotificationRead)
	g.POST("/notifications/read-all", s.HandleMarkAllNotificationsRead)
	g.DELETE("/notifications/:nid", s.HandleDeleteNotification)
	g.GET("/notifications/ws", s.HandleNotificationWebSocket)
	g.GET("/notification-preferences", s.HandleGetNotificationPreferences)
	g.PUT("/notification-preferences", s.HandleUpdateNotificationPreferences)
	g.GET("/digest-settings", s.HandleGetDigestSettings)
	g.PUT("/digest-settings", s.HandleUpdateDigestSettings)

	// Activities — AD-040: /:ws/activities
	g.GET("/activities", s.HandleListActivities)
	g.POST("/activities/seen", s.HandleMarkActivitiesSeen)

	// Tasks — AD-040: /:ws/tasks (no more /my/tasks, use ?assignee_id=me)
	g.GET("/tasks", s.HandleListTasks)
	g.POST("/tasks", s.HandleCreateTask)
	g.GET("/tasks/:taskId", s.HandleGetTask)
	g.PATCH("/tasks/:taskId", s.HandleUpdateTask)
	g.DELETE("/tasks/:taskId", s.HandleDeleteTask)
	g.POST("/tasks/:taskId/assign", s.HandleAssignTask)
	g.POST("/tasks/:taskId/complete", s.HandleCompleteTask)
	g.POST("/tasks/:taskId/cancel", s.HandleCancelTask)

	// Workspace audit log — AD-040: /:ws/audit-log
	g.GET("/audit-log", s.HandleListWorkspaceAuditLog)

	// Archived projects — AD-040: /:ws/archived-projects
	g.GET("/archived-projects", s.HandleListArchivedProjects)

	// -----------------------------------------------------------------------
	// Project collection routes: /:ws/projects
	// -----------------------------------------------------------------------

	g.GET("/projects", s.HandleListWorkspaceProjects)
	g.POST("/projects", s.HandleCreateWorkspaceProject)

	// -----------------------------------------------------------------------
	// Project-specific routes: /:ws/:id
	// AD-040: bare project slug, no /p/ prefix
	// -----------------------------------------------------------------------

	// Project CRUD
	g.GET("/:id", s.HandleGetEditorProject)
	g.PUT("/:id", s.HandleUpdateEditorProject)
	g.PATCH("/:id", s.HandleUpdateEditorProject)
	g.DELETE("/:id", s.HandleDeleteEditorProject)
	g.POST("/:id/restore", s.HandleRestoreProject)
	g.DELETE("/:id/permanent", s.HandlePermanentlyDeleteProject)

	// Project members — AD-040: /:ws/:id/members
	g.GET("/:id/members", s.HandleListProjectMembers)
	g.POST("/:id/members", s.HandleAddProjectMember)
	g.PUT("/:id/members/:uid", s.HandleUpdateProjectMember)
	g.DELETE("/:id/members/:uid", s.HandleRemoveProjectMember)

	// Project settings — AD-040: /:ws/:id/settings
	g.GET("/:id/settings/extraction", s.HandleGetExtractionSettings)
	g.PUT("/:id/settings/extraction", s.HandleUpdateExtractionSettings)

	// Project audit log — AD-040: /:ws/:id/audit-log
	g.GET("/:id/audit-log", s.HandleListAuditLog)

	// Automations — AD-040: /:ws/:id/automations
	g.GET("/:id/automations", s.HandleListAutomationRules)
	g.POST("/:id/automations", s.HandleCreateAutomationRule)
	g.PUT("/:id/automations/:ruleId", s.HandleUpdateAutomationRule)
	g.DELETE("/:id/automations/:ruleId", s.HandleDeleteAutomationRule)
	g.PATCH("/:id/automations/:ruleId/toggle", s.HandleToggleAutomationRule)
	g.GET("/:id/automations/events", s.HandleListAutomationEvents)
	g.GET("/:id/automations/history", s.HandleListAutomationHistory)

	// Automation runs — AD-040: /:ws/:id/automations/runs (nested)
	g.GET("/:id/automations/runs", s.HandleListAutomationRuns)
	g.GET("/:id/automations/runs/:runId", s.HandleGetAutomationRun)
	g.GET("/:id/automations/runs/:runId/steps", s.HandleListAutomationRunSteps)
	g.GET("/:id/automations/runs/:runId/steps/:stepId/logs", s.HandleListStepLogs)
	g.POST("/:id/automations/runs/:runId/cancel", s.HandleCancelAutomationRun)
	g.GET("/:id/automations/runs/:runId/events", s.HandleAutomationRunSSE)

	// Stream management — AD-040: /:ws/:id/streams
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

	// Tags — AD-040: /:ws/:id/tags (peer to streams)
	g.GET("/:id/tags", s.HandleListProjectTags)
	g.POST("/:id/tags", s.HandleCreateStreamTag)
	g.GET("/:id/tags/:tag", s.HandleGetStreamTag)
	g.DELETE("/:id/tags/:tag", s.HandleDeleteStreamTag)

	// Refs — AD-040: /:ws/:id/refs (unified listing)
	g.GET("/:id/refs", s.HandleListProjectTags) // TODO: implement unified ref listing

	// -----------------------------------------------------------------------
	// Ref-scoped content routes: /:ws/:id/<resource>/:ref
	// AD-040: resource-first ref pattern (GitHub-style)
	// -----------------------------------------------------------------------

	syncRateLimit := RateLimitSyncPush(10, 3) // 10 pushes/min, burst of 3

	// Items — AD-040: /:ws/:id/items/:ref
	g.GET("/:id/items/:ref", s.HandleGetFileBlocks) // list items
	g.POST("/:id/items/:ref", s.HandleUploadFiles)
	g.DELETE("/:id/items/:ref", s.HandleRemoveFile) // ?item=path/to/file

	// Blocks — AD-040: /:ws/:id/blocks/:ref
	g.GET("/:id/blocks/:ref", s.HandleGetFileBlocks)
	g.PUT("/:id/blocks/:ref/:bid", s.HandleUpdateBlockTarget)
	g.PUT("/:id/blocks/:ref/:bid/coded", s.HandleUpdateBlockTargetCoded)
	g.GET("/:id/blocks/:ref/:bid/history", s.HandleGetBlockHistory)
	g.GET("/:id/blocks/:ref/:bid/notes", s.HandleListBlockNotes)
	g.POST("/:id/blocks/:ref/:bid/notes", s.HandleAddBlockNote)
	g.DELETE("/:id/blocks/:ref/:bid/notes/:nid", s.HandleDeleteBlockNote)
	g.GET("/:id/blocks/:ref/:bid/tm-matches", s.HandleLookupTMForBlock)
	g.GET("/:id/blocks/:ref/:bid/term-matches", s.HandleLookupTermsForBlock)
	g.GET("/:id/blocks/:ref/:bid/html", s.HandleRenderBlockHTML)

	// Entities on blocks — AD-040: /:ws/:id/blocks/:ref/:bid/entities
	g.POST("/:id/blocks/:ref/:bid/entities", s.HandleCreateEntity)
	g.PUT("/:id/blocks/:ref/:bid/entities/:idx", s.HandleUpdateEntity)
	g.DELETE("/:id/blocks/:ref/:bid/entities/:idx", s.HandleDeleteEntity)
	g.POST("/:id/blocks/:ref/:bid/entities/:idx/promote", s.HandlePromoteEntity)

	// Actions — AD-040: /:ws/:id/actions/:ref/<verb>
	g.POST("/:id/actions/:ref/pseudo-translate", s.HandlePseudoTranslate)
	g.POST("/:id/actions/:ref/ai-translate", s.HandleAITranslate)
	g.POST("/:id/actions/:ref/tm-translate", s.HandleTMTranslate)
	g.POST("/:id/actions/:ref/export", s.HandleExportTranslatedFile)
	g.POST("/:id/actions/:ref/qa-check", s.HandleQACheckFile)
	g.POST("/:id/actions/:ref/qa-check-block", s.HandleQACheckBlock)

	// Preview and word count — AD-040: /:ws/:id/preview/:ref, /:ws/:id/word-count/:ref
	g.GET("/:id/preview/:ref", s.HandleRenderDocumentPreview)
	g.GET("/:id/word-count/:ref", s.HandleGetWordCount)

	// Dashboard — AD-040: /:ws/:id/dashboard/:ref
	g.GET("/:id/dashboard/:ref", s.HandleGetTranslationDashboard)

	// Sync — AD-040: /:ws/:id/sync/:ref
	g.GET("/:id/sync/:ref/pull", s.HandleSyncPull)
	g.GET("/:id/sync/:ref/blocks", s.HandleSyncGetBlocks)
	g.GET("/:id/sync/:ref/status", s.HandleSyncPushStatus)
	g.POST("/:id/sync/:ref/push/init", s.HandleSyncPushInit)
	g.POST("/:id/sync/:ref/push/diff", s.HandleSyncPushDiff)
	g.POST("/:id/sync/:ref/push/commit", s.HandleSyncPushCommit, syncRateLimit)
	g.PUT("/:id/sync/:ref/push/chunks/:uploadId/:chunkIndex", s.HandleSyncProxyChunkUpload)
	g.POST("/:id/sync/:ref/translate", s.HandleCreateProjectTranslationJob)

	// Collections — AD-040: /:ws/:id/collections/:ref
	g.GET("/:id/collections/:ref", s.HandleListCollections)
	g.POST("/:id/collections/:ref", s.HandleCreateCollection)
	g.GET("/:id/collections/:ref/:cid", s.HandleGetCollection)
	g.PUT("/:id/collections/:ref/:cid", s.HandleUpdateCollection)
	g.DELETE("/:id/collections/:ref/:cid", s.HandleDeleteCollection)
	g.POST("/:id/collections/:ref/:cid/items", s.HandleUploadToCollection)

	// Assets — AD-040: /:ws/:id/assets/:ref
	g.POST("/:id/assets/:ref/upload-url", s.HandleAssetUploadURL)
	g.GET("/:id/assets/:ref", s.HandleListAssets)
	g.POST("/:id/assets/:ref", s.HandleCreateAsset)
	g.GET("/:id/assets/:ref/:aid", s.HandleGetAsset)
	g.DELETE("/:id/assets/:ref/:aid", s.HandleDeleteAsset)
	g.POST("/:id/assets/:ref/:aid/variants/upload-url", s.HandleVariantUploadURL)
	g.GET("/:id/assets/:ref/:aid/variants", s.HandleListVariants)
	g.POST("/:id/assets/:ref/:aid/variants", s.HandleCreateVariant)

	// Review queue — AD-040: /:ws/:id/review-queue/:ref
	g.GET("/:id/review-queue/:ref", s.HandleListReviewQueue)
	g.GET("/:id/review-queue/:ref/:itemId", s.HandleGetReviewQueueItem)
	g.POST("/:id/review-queue/:ref/:itemId/decide", s.HandleDecideReviewItem)
	g.POST("/:id/review-queue/:ref/:itemId/assign", s.HandleAssignReviewItem)
	g.POST("/:id/review-queue/:ref/:itemId/split", s.HandleSplitReviewItem)
	g.POST("/:id/review-queue/:ref/batch-decide", s.HandleBatchDecideReviewItems)
	g.POST("/:id/review-queue/:ref/sync", s.HandleSyncReviewDecisions)

	// Brand voice — AD-040: /:ws/:id/brand-voice/:ref
	g.GET("/:id/brand-voice/:ref/scores", s.HandleGetBrandVoiceScores)
	g.GET("/:id/brand-voice/:ref/scores/:locale", s.HandleGetBrandVoiceScoresByLocale)
	g.GET("/:id/brand-voice/:ref/trends", s.HandleGetBrandVoiceTrends)
	g.POST("/:id/brand-voice/:ref/corrections", s.HandleCreateBrandVoiceCorrection)

	// Collab — AD-040: /:ws/:id/collab/:ref
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

	h2s := &http2.Server{}
	srv := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(handler, h2s),
	}
	log.Printf("Starting Bowrain server on %s (HTTP + gRPC)", addr)
	return srv.ListenAndServe()
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
