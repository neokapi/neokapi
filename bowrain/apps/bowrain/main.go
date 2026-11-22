package main

import (
	"embed"
	"log/slog"
	"os"
	"strings"

	// Register bowrain recipe extension decoders so the desktop app can
	// load and validate *.kapi recipes that include bowrain-specific
	// blocks (server, hooks, automations, assets, brand_voice).
	_ "github.com/neokapi/neokapi/host/venue/schema"

	"github.com/neokapi/neokapi/bowrain/apps/bowrain/backend"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/desktopclose"
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
		// Every quit asks here first: Quit in the app menu, Cmd+Q, the Dock,
		// a logout, a signal, the updater, and app.Quit() on Windows and
		// Linux. Saves the page still has waiting hold the quit until the
		// page has flushed them.
		ShouldQuit: func() bool {
			return backend.GuardClose(appService, desktopclose.RouteQuit)
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

	// The window's close button, Close Window (Cmd+W) and win.Close() all reach
	// WindowClosing, from the macOS windowShouldClose, the Windows WM_CLOSE
	// and the Linux delete-event. The hook holds the close while the page has
	// saves waiting; the page flushes them and confirms.
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if !backend.GuardClose(appService, desktopclose.RouteWindow) {
			event.Cancel()
		}
	})
	backend.SetClosers(appService, win.Close, app.Quit)
	// When the page does not acknowledge a held close in time (a render error,
	// a crashed web content process), a native dialog asks instead, so the app
	// can always be closed and never drops an edit without asking.
	backend.SetCloseFallback(appService, func(desktopclose.Route, []desktopclose.Blocker) bool {
		answer := make(chan bool, 1)
		dialog := app.Dialog.Question().
			SetTitle("Changes Not Yet Saved").
			SetMessage("Some changes have not reached the server, and the window did not answer.")
		dialog.AddButton("Close Without Saving").OnClick(func() { answer <- true })
		cancel := dialog.AddButton("Cancel").OnClick(func() { answer <- false })
		dialog.SetDefaultButton(cancel)
		dialog.SetCancelButton(cancel)
		dialog.Show()
		return <-answer
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
			// OIDC auth callback.
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

	// The first SIGTERM or SIGINT asks like any quit; Wails v3.1.0 stops
	// listening after it, so the second one ends the process here.
	desktopclose.ExitOnSecondSignal(os.Exit)

	if err := app.Run(); err != nil {
		slog.Error("bowrain: fatal error", "error", err)
		os.Exit(1)
	}
}
