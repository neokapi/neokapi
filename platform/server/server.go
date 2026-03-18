package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/connector"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/mailer"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/formats"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/registry"
	corestorage "github.com/neokapi/neokapi/core/storage"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	mcpserver "github.com/neokapi/neokapi/bowrain/server/mcp"
	libtools "github.com/neokapi/neokapi/core/tools"
	platconn "github.com/neokapi/neokapi/platform/connector"
	"github.com/neokapi/neokapi/platform/store"
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
	EventBus       *event.ChannelEventBus
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

	// ActivityRecorder subscribes to events and records activities. Nil when not configured.
	ActivityRecorder *event.ActivityRecorder

	// NotificationDispatcher routes events to user notifications. Nil when not configured.
	NotificationDispatcher *event.NotificationDispatcher

	// ExtractionJobStore persists extraction job state. Nil when job system is not configured.
	ExtractionJobStore jobs.ExtractionJobStore

	// ExtractionQueue enqueues extraction job IDs. Nil when not configured.
	ExtractionQueue jobs.Queue

	// dashboardCache caches translation dashboard stats per project/stream.
	dashboardCache sync.Map // map[string]*dashboardCacheEntry
}

// NewServer creates a new Server with the given configuration.
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
		EventBus:        event.NewChannelEventBus(),
		wsStores:        newWorkspaceStores(),
		collabHub:       newCollabHub(),
		notificationHub: newNotificationHub(),
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
			s.QuotaStore = pg.Quota
			s.wsStores.pgDB = pg.DB
			pgSQL := pg.DB.DB // embedded *sql.DB
			s.AuditLogger = event.NewAuditLogger(pgSQL, s.EventBus)
			s.AutomationRuleStore = event.NewPostgresRuleStore(pgSQL)
			s.ReviewQueueStore = bstore.NewPostgresReviewQueueStore(pgSQL)
			s.NotificationStore = bstore.NewPostgresNotificationStore(pgSQL)
			s.ActivityStore = bstore.NewPostgresActivityStore(pgSQL)
			s.TaskStore = bstore.NewPostgresTaskStore(pgSQL)
			s.PreferenceStore = bstore.NewPostgresPreferenceStore(pgSQL)
			s.BrandStore = pg.Brand
			s.GraphStore = pg.GraphStore
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

	// Wire up automation engine.
	s.AutomationEngine = event.NewAutomationEngine(s.EventBus, s.executeAutomationAction)
	s.registerDefaultAutomations()

	// Wire up activity recorder (AD-027).
	if s.ActivityStore != nil {
		s.ActivityRecorder = event.NewActivityRecorder(s.ActivityStore, s.EventBus)
	}

	// Wire up notification dispatcher (AD-027).
	if s.NotificationStore != nil {
		s.NotificationDispatcher = event.NewNotificationDispatcher(
			s.EventBus, s.NotificationStore, s.PreferenceStore, s, nil)
	}

	// Wire up graph sync if graph store is available.
	if s.GraphStore != nil {
		s.graphSyncer = platgraph.NewGraphSyncer(s.GraphStore, s.EventBus)
	}

	// Initialize MCP server for brand voice when brand store is available.
	if s.BrandStore != nil {
		mcpCfg := mcpserver.Config{
			JWTSecret:     cfg.JWTSecret,
			OIDCIssuerURL: cfg.OIDCIssuerURL,
			PublicURL:      cfg.OIDCPublicURL,
		}
		ms, err := mcpserver.NewMCPServer(s.BrandStore, mcpCfg)
		if err != nil {
			log.Printf("WARNING: failed to initialize MCP server: %v", err)
		} else {
			s.mcpServer = ms
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
	v1.GET("/health", s.HandleHealth)
	v1.GET("/ready", s.HandleReady)
	v1.GET("/config", s.HandleConfig)
	v1.GET("/info", s.HandleInfo)
	v1.GET("/formats", s.HandleListFormats)
	v1.GET("/tools", s.HandleListTools)
	v1.GET("/locales", s.HandleGetKnownLocales)

	// Brand voice starter packs (public, no auth required).
	v1.GET("/brand-voice/starter-packs", s.HandleListStarterPacks)

	// Connector endpoints (public).
	v1.GET("/connectors/types", s.HandleListConnectorTypes)
	v1.GET("/connectors", s.HandleListActiveConnectors)
	v1.POST("/connectors", s.HandleAddConnector)
	v1.DELETE("/connectors/:id", s.HandleRemoveConnector)
	v1.GET("/connectors/:id/status", s.HandleConnectorStatus)
	v1.POST("/fetch", s.HandleFetch)
	v1.POST("/publish", s.HandlePublish)

	// Public badge endpoint (shields.io-compatible, CDN-cacheable).
	v1.GET("/badges/projects/:id", s.HandleProjectBadge)

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

		// JWT-protected routes: project CRUD, blocks, versions, changes.
		jwtProtected := v1.Group("")
		jwtProtected.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		jwtProtected.POST("/projects", s.HandleCreateProject)
		jwtProtected.GET("/projects", s.HandleListProjects)
		jwtProtected.GET("/projects/:id", s.HandleGetProject)
		jwtProtected.PUT("/projects/:id", s.HandleUpdateProject)
		jwtProtected.DELETE("/projects/:id", s.HandleDeleteProject)
		jwtProtected.POST("/projects/:id/blocks", s.HandleStoreBlocks)
		jwtProtected.GET("/projects/:id/blocks", s.HandleGetBlocks)
		jwtProtected.POST("/projects/:id/versions", s.HandleCreateVersion)
		jwtProtected.GET("/projects/:id/versions", s.HandleListVersions)
		jwtProtected.GET("/projects/:id/changes", s.HandleGetChanges)
		jwtProtected.POST("/projects/claim", s.HandleClaimProject)
		jwtProtected.POST("/join/:code", s.HandleAcceptInvite)

		// Sync routes: accept either JWT or ClaimToken.
		// Register both legacy (flat) and stream-scoped routes.
		if s.AuthStore != nil {
			syncGroup := v1.Group("/projects/:id/sync")
			syncGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			syncGroup.POST("/push", s.HandleSyncPush)
			syncGroup.GET("/pull", s.HandleSyncPull)
			syncGroup.GET("/blocks", s.HandleSyncGetBlocks)
			syncGroup.POST("/translate", s.HandleCreateProjectTranslationJob)
			syncGroup.GET("/status", s.HandleSyncPushStatus)

			// Stream-scoped sync routes: /projects/:id/streams/:stream/sync/*
			streamSyncGroup := v1.Group("/projects/:id/streams/:stream/sync")
			streamSyncGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			streamSyncGroup.POST("/push", s.HandleSyncPush)
			streamSyncGroup.GET("/pull", s.HandleSyncPull)
			streamSyncGroup.GET("/blocks", s.HandleSyncGetBlocks)
			streamSyncGroup.POST("/translate", s.HandleCreateProjectTranslationJob)
			streamSyncGroup.GET("/status", s.HandleSyncPushStatus)
		}

		// Workspace endpoints (require auth + workspace membership)
		wsGroup := v1.Group("/workspaces")
		wsGroup.Use(AuthMiddleware(s.Config.JWTSecret, s.AuthStore))
		wsGroup.POST("", s.HandleCreateWorkspace)
		wsGroup.GET("", s.HandleListWorkspaces)

		// Workspace-specific routes with auth and membership checks
		wsSpecific := wsGroup.Group("/:ws")
		if s.AuthStore != nil {
			wsSpecific.Use(WorkspaceAccessMiddleware(s.AuthStore))
		}
		wsSpecific.GET("", s.HandleGetWorkspace)
		wsSpecific.PUT("", s.HandleUpdateWorkspace)
		wsSpecific.DELETE("", s.HandleDeleteWorkspace)
		wsSpecific.GET("/members", s.HandleListMembers)
		wsSpecific.POST("/members", s.HandleAddMember)
		wsSpecific.PUT("/members/:uid/role", s.HandleUpdateMemberRole)
		wsSpecific.DELETE("/members/:uid", s.HandleRemoveMember)

		// Invite routes (workspace-scoped, admin/owner only).
		wsSpecific.POST("/invites", s.HandleCreateInvite)
		wsSpecific.GET("/invites", s.HandleListInvites)
		wsSpecific.DELETE("/invites/:id", s.HandleDeleteInvite)

		// API token routes (workspace-scoped).
		wsSpecific.POST("/tokens", s.HandleCreateToken)
		wsSpecific.GET("/tokens", s.HandleListTokens)
		wsSpecific.DELETE("/tokens/:id", s.HandleDeleteToken)

		s.registerWorkspaceContentRoutes(wsSpecific)
	}

	// MCP server (brand voice resources, tools, prompts via Streamable HTTP).
	if s.mcpServer != nil {
		s.mcpServer.RegisterRoutes(e)
	}

	// Web UI static file serving (development and E2E only).
	// A single handler serves static files first and falls back to index.html
	// for SPA client-side routing. Using two separate handlers (e.Static + e.GET)
	// would conflict because Echo overwrites the first GET /* with the second.
	if s.Config.WebUIDir != "" {
		e.GET("/*", func(c echo.Context) error {
			reqPath := c.Param("*")
			if reqPath == "" {
				reqPath = "index.html"
			}
			filePath := filepath.Join(s.Config.WebUIDir, filepath.Clean(reqPath))
			if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
				return c.File(filePath)
			}
			return c.File(filepath.Join(s.Config.WebUIDir, "index.html"))
		})
	}
}

// registerWorkspaceContentRoutes registers all workspace-scoped content routes
// (editor projects, file management, block editing, translation, TM, terms, providers)
// on the given route group.
func (s *Server) registerWorkspaceContentRoutes(g *echo.Group) {
	// Workspace-scoped project routes
	g.GET("/projects", s.HandleListWorkspaceProjects)
	g.POST("/projects", s.HandleCreateWorkspaceProject)

	// Sync routes (workspace-scoped, used by bowrain CLI with workspace config)
	g.POST("/projects/:id/sync/push", s.HandleSyncPush)
	g.GET("/projects/:id/sync/pull", s.HandleSyncPull)
	g.GET("/projects/:id/sync/blocks", s.HandleSyncGetBlocks)
	g.GET("/projects/:id/sync/status", s.HandleSyncPushStatus)

	// Stream-scoped sync routes (workspace-scoped)
	g.POST("/projects/:id/streams/:stream/sync/push", s.HandleSyncPush)
	g.GET("/projects/:id/streams/:stream/sync/pull", s.HandleSyncPull)
	g.GET("/projects/:id/streams/:stream/sync/blocks", s.HandleSyncGetBlocks)
	g.GET("/projects/:id/streams/:stream/sync/status", s.HandleSyncPushStatus)

	// Asset management (AD-029)
	g.POST("/projects/:id/assets/upload-url", s.HandleAssetUploadURL)
	g.POST("/projects/:id/assets", s.HandleCreateAsset)
	g.GET("/projects/:id/assets", s.HandleListAssets)
	g.GET("/projects/:id/assets/:aid", s.HandleGetAsset)
	g.DELETE("/projects/:id/assets/:aid", s.HandleDeleteAsset)
	g.POST("/projects/:id/assets/:aid/variants/upload-url", s.HandleVariantUploadURL)
	g.POST("/projects/:id/assets/:aid/variants", s.HandleCreateVariant)
	g.GET("/projects/:id/assets/:aid/variants", s.HandleListVariants)

	// Editor project routes
	g.POST("/editor/projects", s.HandleCreateEditorProject)
	g.GET("/editor/projects", s.HandleListEditorProjects)
	g.GET("/editor/projects/:pid", s.HandleGetEditorProject)
	g.PUT("/editor/projects/:pid", s.HandleUpdateEditorProject)
	g.DELETE("/editor/projects/:pid", s.HandleDeleteEditorProject)
	g.POST("/editor/projects/:pid/restore", s.HandleRestoreProject)
	g.DELETE("/editor/projects/:pid/permanent", s.HandlePermanentlyDeleteProject)

	// Archived projects (bin)
	g.GET("/archived/projects", s.HandleListArchivedProjects)

	// Translation dashboard (project-scoped, cached)
	g.GET("/editor/projects/:pid/dashboard", s.HandleGetTranslationDashboard)

	// Collection management (project-scoped)
	g.GET("/editor/projects/:pid/collections", s.HandleListCollections)
	g.POST("/editor/projects/:pid/collections", s.HandleCreateCollection)
	g.GET("/editor/projects/:pid/collections/:cid", s.HandleGetCollection)
	g.PUT("/editor/projects/:pid/collections/:cid", s.HandleUpdateCollection)
	g.DELETE("/editor/projects/:pid/collections/:cid", s.HandleDeleteCollection)
	g.POST("/editor/projects/:pid/collections/:cid/files", s.HandleUploadToCollection)

	// File management
	g.POST("/editor/projects/:pid/files", s.HandleUploadFiles)
	g.DELETE("/editor/projects/:pid/file/*", s.HandleRemoveFile)

	// Block editing
	g.GET("/editor/projects/:pid/file-blocks/*", s.HandleGetFileBlocks)
	g.PUT("/editor/projects/:pid/blocks/:bid", s.HandleUpdateBlockTarget)
	g.PUT("/editor/projects/:pid/blocks/:bid/coded", s.HandleUpdateBlockTargetCoded)

	// Translation operations (pseudo + AI are automation/API-only, not exposed in editor UI)
	g.POST("/editor/projects/:pid/file-pseudo/*", s.HandlePseudoTranslate)
	g.POST("/editor/projects/:pid/file-ai-translate/*", s.HandleAITranslate)
	g.POST("/editor/projects/:pid/file-tm-translate/*", s.HandleTMTranslate)
	g.GET("/editor/projects/:pid/file-wordcount/*", s.HandleGetWordCount)
	g.POST("/editor/projects/:pid/file-export/*", s.HandleExportTranslatedFile)

	// QA checks
	g.POST("/editor/projects/:pid/blocks/:bid/qa-check", s.HandleQACheckBlock)
	g.POST("/editor/projects/:pid/file-qa-check/*", s.HandleQACheckFile)

	// Block history
	g.GET("/editor/projects/:pid/blocks/:bid/history", s.HandleGetBlockHistory)

	// Block notes
	g.POST("/editor/projects/:pid/blocks/:bid/notes", s.HandleAddBlockNote)
	g.GET("/editor/projects/:pid/blocks/:bid/notes", s.HandleListBlockNotes)
	g.DELETE("/editor/projects/:pid/blocks/:bid/notes/:nid", s.HandleDeleteBlockNote)

	// Block-level TM and term lookup
	g.GET("/editor/projects/:pid/blocks/:bid/tm-lookup", s.HandleLookupTMForBlock)
	g.GET("/editor/projects/:pid/blocks/:bid/term-lookup", s.HandleLookupTermsForBlock)

	// TM CRUD (workspace-scoped)
	g.GET("/tm", s.HandleGetTMEntries)
	g.GET("/tm/count", s.HandleGetTMCount)
	g.POST("/tm", s.HandleAddTMEntry)
	g.PUT("/tm/:eid", s.HandleUpdateTMEntry)
	g.DELETE("/tm/:eid", s.HandleDeleteTMEntry)

	// Terminology CRUD (workspace-scoped)
	g.GET("/terms", s.HandleGetTerms)
	g.GET("/terms/count", s.HandleGetTermCount)
	g.POST("/terms", s.HandleAddConcept)
	g.PUT("/terms/:cid", s.HandleUpdateConcept)
	g.DELETE("/terms/:cid", s.HandleDeleteConcept)
	g.POST("/terms/import/csv", s.HandleImportTermsCSV)
	g.POST("/terms/import/json", s.HandleImportTermsJSON)
	g.GET("/terms/export/json", s.HandleExportTermsJSON)

	// Preview rendering
	g.GET("/editor/projects/:pid/file-preview/*", s.HandleRenderDocumentPreview)
	g.GET("/editor/projects/:pid/blocks/:bid/html", s.HandleRenderBlockHTML)

	// Collaborative editing WebSocket
	g.GET("/editor/projects/:pid/collab/*", s.HandleCollabWebSocket)

	// Provider configs (workspace-level)
	g.GET("/providers", s.HandleListProviderConfigs)
	g.POST("/providers", s.HandleSaveProviderConfig)
	g.DELETE("/providers/:id", s.HandleDeleteProviderConfig)
	g.POST("/providers/test", s.HandleTestProviderConfig)

	// Translation jobs (async)
	g.POST("/jobs/translate", s.HandleCreateTranslationJob)
	g.GET("/jobs", s.HandleListJobs)
	g.GET("/jobs/:id", s.HandleGetJob)
	g.DELETE("/jobs/:id", s.HandleDeleteJob)
	g.GET("/ai/usage", s.HandleGetAIUsage)

	// Automation rules (project-scoped)
	g.GET("/projects/:id/automations", s.HandleListAutomationRules)
	g.POST("/projects/:id/automations", s.HandleCreateAutomationRule)
	g.PUT("/projects/:id/automations/:ruleId", s.HandleUpdateAutomationRule)
	g.DELETE("/projects/:id/automations/:ruleId", s.HandleDeleteAutomationRule)
	g.PATCH("/projects/:id/automations/:ruleId/toggle", s.HandleToggleAutomationRule)
	g.GET("/projects/:id/automations/events", s.HandleListAutomationEvents)
	g.GET("/projects/:id/automations/history", s.HandleListAutomationHistory)

	// Review queue (project-scoped, AD-022)
	g.GET("/projects/:id/review-queue", s.HandleListReviewQueue)
	g.GET("/projects/:id/review-queue/:itemId", s.HandleGetReviewQueueItem)
	g.POST("/projects/:id/review-queue/:itemId/decide", s.HandleDecideReviewItem)
	g.POST("/projects/:id/review-queue/:itemId/assign", s.HandleAssignReviewItem)
	g.POST("/projects/:id/review-queue/:itemId/split", s.HandleSplitReviewItem)
	g.POST("/projects/:id/review-queue/batch-decide", s.HandleBatchDecideReviewItems)
	g.POST("/projects/:id/review-queue/sync", s.HandleSyncReviewDecisions)

	// Notifications (user-scoped)
	g.GET("/notifications", s.HandleListNotifications)
	g.POST("/notifications/:nid/read", s.HandleMarkNotificationRead)
	g.POST("/notifications/read-all", s.HandleMarkAllNotificationsRead)
	g.DELETE("/notifications/:nid", s.HandleDeleteNotification)
	g.GET("/notifications/ws", s.HandleNotificationWebSocket)
	g.GET("/notifications/preferences", s.HandleGetNotificationPreferences)
	g.PUT("/notifications/preferences", s.HandleUpdateNotificationPreferences)

	// Activities (workspace-scoped, AD-027)
	g.GET("/activities", s.HandleListActivities)

	// Tasks (workspace-scoped, AD-027)
	g.GET("/tasks", s.HandleListTasks)
	g.POST("/tasks", s.HandleCreateTask)
	g.GET("/tasks/:taskId", s.HandleGetTask)
	g.PATCH("/tasks/:taskId", s.HandleUpdateTask)
	g.DELETE("/tasks/:taskId", s.HandleDeleteTask)
	g.POST("/tasks/:taskId/assign", s.HandleAssignTask)
	g.POST("/tasks/:taskId/complete", s.HandleCompleteTask)
	g.POST("/tasks/:taskId/cancel", s.HandleCancelTask)
	g.GET("/my/tasks", s.HandleListMyTasks)

	// Entity annotations (block-scoped, AD-022)
	g.POST("/editor/projects/:pid/blocks/:bid/entities", s.HandleCreateEntity)
	g.PUT("/editor/projects/:pid/blocks/:bid/entities/:idx", s.HandleUpdateEntity)
	g.DELETE("/editor/projects/:pid/blocks/:bid/entities/:idx", s.HandleDeleteEntity)
	g.POST("/editor/projects/:pid/blocks/:bid/entities/:idx/promote", s.HandlePromoteEntity)

	// Extraction settings (project-scoped, AD-022)
	g.GET("/projects/:id/settings/extraction", s.HandleGetExtractionSettings)
	g.PUT("/projects/:id/settings/extraction", s.HandleUpdateExtractionSettings)

	// Brand voice profiles (workspace-scoped)
	g.GET("/brand-profiles", s.HandleListBrandProfiles)
	g.POST("/brand-profiles", s.HandleCreateBrandProfile)
	g.GET("/brand-profiles/:id", s.HandleGetBrandProfile)
	g.PUT("/brand-profiles/:id", s.HandleUpdateBrandProfile)
	g.DELETE("/brand-profiles/:id", s.HandleDeleteBrandProfile)
	g.POST("/brand-profiles/:id/check", s.HandleCheckBrandVoice)
	g.POST("/brand-profiles/from-starter", s.HandleCreateFromStarter)
	g.GET("/brand-voice/suggested-rules", s.HandleGetSuggestedRules)

	// Brand voice scores and corrections (project-scoped)
	g.GET("/projects/:id/brand-voice/scores", s.HandleGetBrandVoiceScores)
	g.GET("/projects/:id/brand-voice/scores/:locale", s.HandleGetBrandVoiceScoresByLocale)
	g.GET("/projects/:id/brand-voice/trends", s.HandleGetBrandVoiceTrends)
	g.POST("/projects/:id/brand-voice/corrections", s.HandleCreateBrandVoiceCorrection)

	// Graph query endpoints (dashboard analytics)
	g.GET("/graph/concepts", s.HandleGetConceptHierarchy)
	g.GET("/graph/nodes/:nodeId/neighbors", s.HandleGetGraphNeighbors)
	g.GET("/graph/nodes/:nodeId/edges", s.HandleGetGraphEdges)
	g.GET("/graph/shortest-path", s.HandleGetShortestPath)

	// Audit log
	g.GET("/audit-log", s.HandleListWorkspaceAuditLog)
	g.GET("/projects/:id/audit-log", s.HandleListAuditLog)

	// Stream management (project-scoped)
	g.GET("/projects/:id/streams", s.HandleListStreams)
	g.POST("/projects/:id/streams", s.HandleCreateStream)
	g.GET("/projects/:id/streams/:stream", s.HandleGetStream)
	g.PATCH("/projects/:id/streams/:stream", s.HandleUpdateStream)
	g.DELETE("/projects/:id/streams/:stream", s.HandleArchiveStream)
	g.POST("/projects/:id/streams/:stream/restore", s.HandleRestoreStream)
	g.POST("/projects/:id/streams/:stream/merge", s.HandleMergeStream)
	g.GET("/projects/:id/streams/:stream/diff", s.HandleDiffStream)
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
