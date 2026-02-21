package server

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/bowrain/connector"
	"github.com/gokapi/gokapi/bowrain/credentials"
	"github.com/gokapi/gokapi/bowrain/event"
	"github.com/gokapi/gokapi/bowrain/service"
	"github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/registry"
	libtools "github.com/gokapi/gokapi/core/tools"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Server is the REST API server for gokapi.
type Server struct {
	Config         ServerConfig
	FormatRegistry *registry.FormatRegistry
	ToolRegistry   *registry.ToolRegistry
	ConnectorReg   *connector.Registry
	ContentStore   store.ContentStore
	Services       *service.Services
	AuthStore      auth.AuthStore
	EventBus       *event.ChannelEventBus
	Echo           *echo.Echo

	// wsStores manages per-workspace TM and terminology stores.
	wsStores *workspaceStores

	// CredentialStore manages AI provider credentials.
	CredentialStore *credentials.Store

	// EmailSender sends transactional emails (invitations, etc.).
	// Nil if SMTP is not configured.
	EmailSender EmailSenderI

	// collabHub manages collaborative editing WebSocket rooms.
	collabHub *collabHub

	// WebUIFS is an optional embedded filesystem for serving the web UI.
	// When set, it takes precedence over Config.WebUIDir.
	WebUIFS fs.FS
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	connReg := connector.NewRegistry()
	connector.RegisterAll(connReg, formatReg)

	s := &Server{
		Config:         cfg,
		FormatRegistry: formatReg,
		ToolRegistry:   toolReg,
		ConnectorReg:   connReg,
		EventBus:       event.NewChannelEventBus(),
		wsStores:       newWorkspaceStores(cfg.DataDir),
		collabHub:      newCollabHub(),
	}

	// Initialize credential store.
	s.CredentialStore = credentials.NewStore(credentials.DefaultPath())

	// Initialize email sender if SMTP is configured.
	if cfg.SMTPHost != "" && cfg.SMTPFrom != "" {
		s.EmailSender = &SMTPSender{Host: cfg.SMTPHost, From: cfg.SMTPFrom}
	}

	// Initialize content store if a store path is configured.
	if cfg.StorePath != "" {
		cs, err := store.NewSQLiteStore(cfg.StorePath)
		if err != nil {
			log.Printf("WARNING: failed to open content store at %s: %v", cfg.StorePath, err)
		} else {
			s.ContentStore = cs
			s.Services = service.NewServices(cs, connReg, formatReg, toolReg)
		}
	}

	// Initialize auth store when authentication is configured.
	if cfg.JWTSecret != "" && cfg.StorePath != "" {
		authDBPath := cfg.StorePath + ".auth"
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

		// Protected auth routes (require valid token)
		authProtected := authGroup.Group("")
		authProtected.Use(AuthMiddleware(s.Config.JWTSecret))
		authProtected.GET("/me", s.HandleAuthMe)
		authProtected.POST("/logout", s.HandleAuthLogout)

		// JWT-protected routes: project CRUD, blocks, versions, changes.
		jwtProtected := v1.Group("")
		jwtProtected.Use(AuthMiddleware(s.Config.JWTSecret))
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
		}

		// Workspace endpoints (require auth + workspace membership)
		wsGroup := v1.Group("/workspaces")
		wsGroup.Use(AuthMiddleware(s.Config.JWTSecret))
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

		s.registerWorkspaceContentRoutes(wsSpecific)
	}

	// Web UI static file serving
	if s.WebUIFS != nil {
		e.GET("/*", s.serveEmbeddedUI)
	} else if s.Config.WebUIDir != "" {
		e.Static("/", s.Config.WebUIDir)
		// SPA fallback: serve index.html for non-API routes
		e.GET("/*", func(c echo.Context) error {
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

	// Editor project routes
	g.POST("/editor/projects", s.HandleCreateEditorProject)
	g.GET("/editor/projects", s.HandleListEditorProjects)
	g.GET("/editor/projects/:pid", s.HandleGetEditorProject)
	g.DELETE("/editor/projects/:pid", s.HandleDeleteEditorProject)

	// File management
	g.POST("/editor/projects/:pid/files", s.HandleUploadFiles)
	g.DELETE("/editor/projects/:pid/files/:fname", s.HandleRemoveFile)

	// Block editing
	g.GET("/editor/projects/:pid/files/:fname/blocks", s.HandleGetFileBlocks)
	g.PUT("/editor/projects/:pid/blocks/:bid", s.HandleUpdateBlockTarget)
	g.PUT("/editor/projects/:pid/blocks/:bid/coded", s.HandleUpdateBlockTargetCoded)

	// Translation operations
	g.POST("/editor/projects/:pid/files/:fname/pseudo", s.HandlePseudoTranslate)
	g.POST("/editor/projects/:pid/files/:fname/ai-translate", s.HandleAITranslate)
	g.POST("/editor/projects/:pid/files/:fname/tm-translate", s.HandleTMTranslate)
	g.GET("/editor/projects/:pid/files/:fname/wordcount", s.HandleGetWordCount)
	g.POST("/editor/projects/:pid/files/:fname/export", s.HandleExportTranslatedFile)

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

	// Collaborative editing WebSocket
	g.GET("/editor/projects/:pid/collab/:fname", s.HandleCollabWebSocket)

	// Provider configs (workspace-level)
	g.GET("/providers", s.HandleListProviderConfigs)
	g.POST("/providers", s.HandleSaveProviderConfig)
	g.DELETE("/providers/:id", s.HandleDeleteProviderConfig)
	g.POST("/providers/test", s.HandleTestProviderConfig)
}

// Start initializes the Echo server and starts listening.
func (s *Server) Start(addr string) error {
	e := echo.New()
	e.HideBanner = true
	s.Echo = e

	s.SetupRoutes(e)

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)
	}

	return e.Start(addr)
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

// serveEmbeddedUI serves static files from the embedded WebUIFS filesystem.
// If the requested file is not found, it falls back to index.html for SPA routing.
func (s *Server) serveEmbeddedUI(c echo.Context) error {
	reqPath := c.Param("*")
	if reqPath == "" {
		reqPath = "index.html"
	}

	// Try to open the requested file.
	f, err := s.WebUIFS.Open(reqPath)
	if err == nil {
		defer f.Close()
		info, statErr := f.Stat()
		if statErr == nil && !info.IsDir() {
			contentType := mime.TypeByExtension(path.Ext(reqPath))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			// Read the file into a seeker for ServeContent.
			rs, ok := f.(io.ReadSeeker)
			if ok {
				http.ServeContent(c.Response(), c.Request(), info.Name(), info.ModTime(), rs)
				return nil
			}
			// Fallback: read all and stream.
			data, readErr := io.ReadAll(f)
			if readErr != nil {
				return readErr
			}
			return c.Blob(http.StatusOK, contentType, data)
		}
	}

	// SPA fallback: serve index.html for unmatched routes.
	indexFile, err := s.WebUIFS.Open("index.html")
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "web UI not found")
	}
	defer indexFile.Close()

	data, err := io.ReadAll(indexFile)
	if err != nil {
		return err
	}

	// Only fallback for navigation requests (not missing assets).
	if isAssetPath(reqPath) {
		return echo.NewHTTPError(http.StatusNotFound)
	}

	return c.HTMLBlob(http.StatusOK, data)
}

// isAssetPath returns true if the path looks like a static asset request
// (has a file extension like .js, .css, .png) rather than an SPA route.
func isAssetPath(p string) bool {
	ext := path.Ext(p)
	return ext != "" && ext != ".html" && !strings.HasSuffix(p, "/")
}
