package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/apps/bowrain/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

// parseProjectArg scans command-line arguments for a .kaz file path.
// It skips flags (args starting with "-") and returns the first .kaz
// path found, resolved to an absolute path. Returns "" if none found.
func parseProjectArg(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(arg), ".kaz") {
			abs, err := filepath.Abs(arg)
			if err != nil {
				return arg
			}
			return abs
		}
	}
	return ""
}

func main() {
	// Create the backend without plugins first so the window can appear
	// immediately. Plugin loading (which may start a JVM subprocess) is
	// deferred to the background.
	appService := backend.NewAppWithoutPlugins()

	// Check if a .kaz project file was passed as a CLI argument.
	if path := parseProjectArg(os.Args[1:]); path != "" {
		appService.SetInitialProjectPath(path)
	}

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

	// Load plugins in the background after the window is ready.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		go appService.LoadPlugins()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
