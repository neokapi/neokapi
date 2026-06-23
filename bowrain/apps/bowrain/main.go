package main

import (
	"embed"
	"log/slog"
	"os"
	"strings"

	// Register bowrain recipe extension decoders so the desktop app can
	// load and validate *.kapi recipes that include bowrain-specific
	// blocks (server, hooks, automations, assets, brand_voice).
	_ "github.com/neokapi/neokapi/bowrain/plugin/schema"

	"github.com/neokapi/neokapi/bowrain/apps/bowrain/backend"
	"github.com/neokapi/neokapi/core/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	// Create the backend without plugins first so the window can appear
	// immediately. Plugin loading (which may start a JVM subprocess) is
	// deferred to the background.
	appService := backend.NewAppWithoutPlugins()

	app := application.New(application.Options{
		Name: "Bowrain",
		Icon: appIcon,
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Store app reference so the service can use dialogs and events.
	appService.SetApplication(app)

	// Wire the in-app updater (appcast feed for the current channel, 6h
	// background check). No-op until a real signing key is committed; never
	// blocks startup.
	backend.InitUpdater(app)

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          version.WindowTitle("Bowrain"),
		Width:          1280,
		Height:         800,
		EnableFileDrop: true,
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInsetUnified,
			InvisibleTitleBarHeight: 50,
		},
	})

	// Forward dropped files to the frontend via a custom event.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		app.Event.Emit("files-dropped", files)
	})

	// Handle bowrain: URLs from the OS protocol handler.
	// Two formats:
	//   bowrain://auth/callback?...  → OIDC auth callback (unchanged)
	//   bowrain:https://server/...   → deep link to web URL
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		rawURL := event.Context().URL()
		if rawURL == "" {
			return
		}

		// Bring the app to the foreground.
		application.InvokeSync(func() {
			app.Show()
			win.Focus()
		})

		switch {
		case strings.HasPrefix(rawURL, "bowrain://auth/"):
			// OIDC auth callback — unchanged.
			go appService.HandleAuthURL(rawURL)
		case strings.HasPrefix(rawURL, "bowrain:"):
			// Deep link: strip "bowrain:" prefix, rest is a web URL.
			webURL := strings.TrimPrefix(rawURL, "bowrain:")
			go appService.HandleDeepLink(webURL)
		default:
			slog.Info("bowrain: unrecognized URL:", "value", rawURL)
		}
	})

	// Load plugins in the background after the window is ready.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		go appService.LoadPlugins()
	})

	if err := app.Run(); err != nil {
		slog.Error("bowrain: fatal error", "error", err)
		os.Exit(1)
	}
}
