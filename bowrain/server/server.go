package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/bowrain/connector"
	"github.com/gokapi/gokapi/bowrain/credentials"
	"github.com/gokapi/gokapi/bowrain/event"
	"github.com/gokapi/gokapi/bowrain/jobs"
	"github.com/gokapi/gokapi/bowrain/mailer"
	"github.com/gokapi/gokapi/bowrain/service"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/registry"
	libtools "github.com/gokapi/gokapi/core/tools"
	platconn "github.com/gokapi/gokapi/platform/connector"
	"github.com/gokapi/gokapi/platform/store"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

// Server is the REST API server for gokapi.
type Server struct {
	Config         ServerConfig
	FormatRegistry *registry.FormatRegistry
	ToolRegistry   *registry.ToolRegistry
	ConnectorReg   *platconn.Registry
	ContentStore   store.ContentStore
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

	// ReviewQueueStore persists entity/term extraction review items. Nil when not configured.
	ReviewQueueStore *bstore.ReviewQueueStore

	// NotificationStore persists user notifications. Nil when not configured.
	NotificationStore *bstore.NotificationStore

	// ExtractionJobStore persists extraction job state. Nil when job system is not configured.
	ExtractionJobStore jobs.ExtractionJobStore

	// ExtractionQueue enqueues extraction job IDs. Nil when not configured.
	ExtractionQueue jobs.Queue
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
		Config:         cfg,
		FormatRegistry: formatReg,
		ToolRegistry:   toolReg,
		ConnectorReg:   connReg,
		EventBus:       event.NewChannelEventBus(),
		wsStores:       newWorkspaceStores(cfg.DataDir),
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

	// Initialize stores based on DatabaseURL or StorePath.
	dbURL := cfg.DatabaseURL
	if dbURL == "" && cfg.StorePath != "" {
		dbURL = "sqlite:///" + cfg.StorePath
	}

	if strings.HasPrefix(dbURL, "postgres://") || strings.HasPrefix(dbURL, "postgresql://") {
		var pg *pgStores
		var err error
		if cfg.DatabaseAuth == "azure" {
			pg, err = openPostgresStoresAzure(dbURL, cfg.AzureClientID)
		} else {
			pg, err = openPostgresStores(dbURL)
		}
		if err != nil {
			log.Printf("WARNING: failed to open PostgreSQL stores: %v", err)
		} else {
			s.ContentStore = pg.Content
			s.Services = service.NewServices(pg.Content, connReg, formatReg, toolReg)
			s.JobStore = pg.Job
			s.QuotaStore = pg.Quota
			// In PostgreSQL (SaaS) mode, TM and termbase use the shared PG pool.
			s.wsStores.pgDB = pg.DB
			// Wire up automation, review queue, and notification stores for PG.
			pgSQL := pg.DB.DB // embedded *sql.DB
			s.AutomationRuleStore = event.NewPostgresRuleStore(pgSQL)
			s.ReviewQueueStore = bstore.NewPostgresReviewQueueStore(pgSQL)
			s.NotificationStore = bstore.NewPostgresNotificationStore(pgSQL)
			if cfg.JWTSecret != "" {
				s.AuthStore = pg.Auth
				s.Services.Auth = service.NewAuthService(pg.Auth, cfg.JWTSecret)
			}
		}
	} else if cfg.StorePath != "" || strings.HasPrefix(dbURL, "sqlite:///") {
		storePath := cfg.StorePath
		if storePath == "" {
			storePath = strings.TrimPrefix(dbURL, "sqlite:///")
		}
		cs, err := bstore.NewSQLiteStore(storePath)
		if err != nil {
			log.Printf("WARNING: failed to open content store at %s: %v", storePath, err)
		} else {
			s.ContentStore = cs
			s.Services = service.NewServices(cs, connReg, formatReg, toolReg)
			s.AutomationRuleStore = event.NewSQLiteRuleStore(cs.DB())
			s.ReviewQueueStore = bstore.NewReviewQueueStore(cs.DB())
			s.NotificationStore = bstore.NewNotificationStore(cs.DB())
		}

		// Initialize auth store when authentication is configured.
		if cfg.JWTSecret != "" {
			authDBPath := storePath + ".auth"
			as, err := auth.NewSQLiteAuthStore(authDBPath)
			if err != nil {
				log.Printf("WARNING: failed to open auth store at %s: %v", authDBPath, err)
			} else {
				s.AuthStore = as
				if s.Services != nil {
					s.Services.Auth = service.NewAuthService(as, cfg.JWTSecret)
				}
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

	// Wire up automation engine.
	s.AutomationEngine = event.NewAutomationEngine(s.EventBus, s.executeAutomationAction)
	s.registerDefaultAutomations()

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
	v1.GET("/config", s.HandleConfig)
	v1.GET("/info", s.HandleInfo)
	v1.GET("/formats", s.HandleListFormats)
	v1.GET("/tools", s.HandleListTools)
	v1.GET("/locales", s.HandleGetKnownLocales)

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
		if s.AuthStore != nil {
			syncGroup := v1.Group("/projects/:id/sync")
			syncGroup.Use(ClaimOrAuthMiddleware(s.Config.JWTSecret, s.AuthStore))
			syncGroup.POST("/push", s.HandleSyncPush)
			syncGroup.GET("/pull", s.HandleSyncPull)
			syncGroup.GET("/blocks", s.HandleSyncGetBlocks)
			syncGroup.POST("/translate", s.HandleCreateProjectTranslationJob)
			syncGroup.GET("/status", s.HandleSyncPushStatus)
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

	// Sync routes (workspace-scoped, used by kapi CLI with workspace config)
	g.POST("/projects/:id/sync/push", s.HandleSyncPush)
	g.GET("/projects/:id/sync/pull", s.HandleSyncPull)
	g.GET("/projects/:id/sync/blocks", s.HandleSyncGetBlocks)
	g.GET("/projects/:id/sync/status", s.HandleSyncPushStatus)

	// Editor project routes
	g.POST("/editor/projects", s.HandleCreateEditorProject)
	g.GET("/editor/projects", s.HandleListEditorProjects)
	g.GET("/editor/projects/:pid", s.HandleGetEditorProject)
	g.DELETE("/editor/projects/:pid", s.HandleDeleteEditorProject)

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

	// Entity annotations (block-scoped, AD-022)
	g.POST("/editor/projects/:pid/blocks/:bid/entities", s.HandleCreateEntity)
	g.PUT("/editor/projects/:pid/blocks/:bid/entities/:idx", s.HandleUpdateEntity)
	g.DELETE("/editor/projects/:pid/blocks/:bid/entities/:idx", s.HandleDeleteEntity)
	g.POST("/editor/projects/:pid/blocks/:bid/entities/:idx/promote", s.HandlePromoteEntity)

	// Extraction settings (project-scoped, AD-022)
	g.GET("/projects/:id/settings/extraction", s.HandleGetExtractionSettings)
	g.PUT("/projects/:id/settings/extraction", s.HandleUpdateExtractionSettings)

	// Stream management (project-scoped)
	g.GET("/projects/:id/streams", s.HandleListStreams)
	g.POST("/projects/:id/streams", s.HandleCreateStream)
	g.GET("/projects/:id/streams/:stream", s.HandleGetStream)
	g.PATCH("/projects/:id/streams/:stream", s.HandleUpdateStream)
	g.DELETE("/projects/:id/streams/:stream", s.HandleArchiveStream)
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 &&
			strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
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
