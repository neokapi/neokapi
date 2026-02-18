package main

import (
	"embed"
	"log"

	"github.com/gokapi/gokapi/bowrain/apps/bowrain/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create the backend without plugins first so the window can appear
	// immediately. Plugin loading (which may start a JVM subprocess) is
	// deferred to the background.
	appService := backend.NewAppWithoutPlugins()

	app := application.New(application.Options{
		Name: "Bowrain",
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

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Bowrain",
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

	// Handle bowrain:// URLs from the OS protocol handler (used for OIDC auth callback).
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		authURL := event.Context().URL()
		if authURL != "" {
			// Bring the app to the foreground. Show/Focus are Cocoa calls that
			// must run on the main thread; the event handler runs in a goroutine.
			application.InvokeSync(func() {
				app.Show()
				win.Focus()
			})
			go appService.HandleAuthURL(authURL)
		}
	})

	// Load plugins in the background after the window is ready.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		go appService.LoadPlugins()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
