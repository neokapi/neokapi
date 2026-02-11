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

	"github.com/gokapi/gokapi/core/auth"
	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/credentials"
	"github.com/gokapi/gokapi/core/event"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/service"
	"github.com/gokapi/gokapi/core/store"
	"github.com/gokapi/gokapi/formats"
	libtools "github.com/gokapi/gokapi/lib/tools"
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

	// EditorStore manages in-memory translation editor sessions.
	EditorStore *EditorStore

	// CredentialStore manages AI provider credentials.
	CredentialStore *credentials.Store

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
		EditorStore:    NewEditorStore(50),
	}

	// Initialize credential store.
	s.CredentialStore = credentials.NewStore(credentials.DefaultPath())

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

	// Initialize auth store for multi-user mode.
	if !cfg.LocalMode && cfg.JWTSecret != "" && cfg.StorePath != "" {
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
	e.Use(middleware.CORS())

	// API v1 routes
	v1 := e.Group("/api/v1")
	v1.GET("/health", s.HandleHealth)
	v1.GET("/config", s.HandleConfig)
	v1.GET("/formats", s.HandleListFormats)
	v1.GET("/tools", s.HandleListTools)
	v1.GET("/locales", s.HandleGetKnownLocales)

	// Project endpoints (backward-compatible flat routes)
	v1.POST("/projects", s.HandleCreateProject)
	v1.GET("/projects", s.HandleListProjects)
	v1.GET("/projects/:id", s.HandleGetProject)
	v1.PUT("/projects/:id", s.HandleUpdateProject)
	v1.DELETE("/projects/:id", s.HandleDeleteProject)
	v1.POST("/projects/:id/blocks", s.HandleStoreBlocks)
	v1.GET("/projects/:id/blocks", s.HandleGetBlocks)
	v1.POST("/projects/:id/versions", s.HandleCreateVersion)
	v1.GET("/projects/:id/versions", s.HandleListVersions)

	// Connector endpoints
	v1.GET("/connectors/types", s.HandleListConnectorTypes)
	v1.GET("/connectors", s.HandleListActiveConnectors)
	v1.POST("/connectors", s.HandleAddConnector)
	v1.DELETE("/connectors/:id", s.HandleRemoveConnector)
	v1.GET("/connectors/:id/status", s.HandleSyncStatus)
	v1.POST("/pull", s.HandlePull)
	v1.POST("/push", s.HandlePush)

	// Auth endpoints (only for multi-user mode)
	if !s.Config.LocalMode && s.Config.JWTSecret != "" {
		// Public auth routes (no token required)
		authGroup := v1.Group("/auth")
		authGroup.POST("/device/start", s.HandleDeviceAuthStart)
		authGroup.POST("/device/poll", s.HandleDeviceAuthPoll)
		authGroup.GET("/callback", s.HandleAuthCallback)
		authGroup.POST("/callback", s.HandleAuthCallback)

		// Device verification page (user opens in browser)
		authGroup.GET("/device/verify", s.HandleAuthCallback)
		authGroup.POST("/device/verify", func(c echo.Context) error {
			return s.handleDeviceVerification(c, c.FormValue("user_code"))
		})

		// Protected auth routes (require valid token)
		authProtected := authGroup.Group("")
		authProtected.Use(AuthMiddleware(s.Config.JWTSecret))
		authProtected.GET("/me", s.HandleAuthMe)
		authProtected.POST("/logout", s.HandleAuthLogout)

		// Workspace endpoints (require auth + workspace membership)
		wsGroup := v1.Group("/workspaces")
		wsGroup.Use(AuthMiddleware(s.Config.JWTSecret))
		wsGroup.POST("", s.HandleCreateWorkspace)
		wsGroup.GET("", s.HandleListWorkspaces)

		// Workspace-specific routes also check membership
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

		// Workspace-scoped project routes
		wsSpecific.GET("/projects", s.HandleListWorkspaceProjects)
		wsSpecific.POST("/projects", s.HandleCreateWorkspaceProject)

		// Editor project routes (in-memory translation projects)
		wsSpecific.POST("/editor/projects", s.HandleCreateEditorProject)
		wsSpecific.GET("/editor/projects", s.HandleListEditorProjects)
		wsSpecific.GET("/editor/projects/:pid", s.HandleGetEditorProject)
		wsSpecific.DELETE("/editor/projects/:pid", s.HandleDeleteEditorProject)

		// File management
		wsSpecific.POST("/editor/projects/:pid/files", s.HandleUploadFiles)
		wsSpecific.DELETE("/editor/projects/:pid/files/:fname", s.HandleRemoveFile)

		// Block editing
		wsSpecific.GET("/editor/projects/:pid/files/:fname/blocks", s.HandleGetFileBlocks)
		wsSpecific.PUT("/editor/projects/:pid/blocks/:bid", s.HandleUpdateBlockTarget)
		wsSpecific.PUT("/editor/projects/:pid/blocks/:bid/coded", s.HandleUpdateBlockTargetCoded)

		// Translation operations
		wsSpecific.POST("/editor/projects/:pid/files/:fname/pseudo", s.HandlePseudoTranslate)
		wsSpecific.POST("/editor/projects/:pid/files/:fname/ai-translate", s.HandleAITranslate)
		wsSpecific.POST("/editor/projects/:pid/files/:fname/tm-translate", s.HandleTMTranslate)
		wsSpecific.GET("/editor/projects/:pid/files/:fname/wordcount", s.HandleGetWordCount)
		wsSpecific.POST("/editor/projects/:pid/files/:fname/export", s.HandleExportTranslatedFile)

		// Block-level TM and term lookup
		wsSpecific.GET("/editor/projects/:pid/blocks/:bid/tm-lookup", s.HandleLookupTMForBlock)
		wsSpecific.GET("/editor/projects/:pid/blocks/:bid/term-lookup", s.HandleLookupTermsForBlock)

		// TM CRUD (workspace-scoped)
		wsSpecific.GET("/tm", s.HandleGetTMEntries)
		wsSpecific.GET("/tm/count", s.HandleGetTMCount)
		wsSpecific.POST("/tm", s.HandleAddTMEntry)
		wsSpecific.PUT("/tm/:eid", s.HandleUpdateTMEntry)
		wsSpecific.DELETE("/tm/:eid", s.HandleDeleteTMEntry)

		// Terminology CRUD (workspace-scoped)
		wsSpecific.GET("/terms", s.HandleGetTerms)
		wsSpecific.GET("/terms/count", s.HandleGetTermCount)
		wsSpecific.POST("/terms", s.HandleAddConcept)
		wsSpecific.PUT("/terms/:cid", s.HandleUpdateConcept)
		wsSpecific.DELETE("/terms/:cid", s.HandleDeleteConcept)
		wsSpecific.POST("/terms/import/csv", s.HandleImportTermsCSV)
		wsSpecific.POST("/terms/import/json", s.HandleImportTermsJSON)
		wsSpecific.GET("/terms/export/json", s.HandleExportTermsJSON)

		// Provider configs (workspace-level)
		wsSpecific.GET("/providers", s.HandleListProviderConfigs)
		wsSpecific.POST("/providers", s.HandleSaveProviderConfig)
		wsSpecific.DELETE("/providers/:id", s.HandleDeleteProviderConfig)
		wsSpecific.POST("/providers/test", s.HandleTestProviderConfig)
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
