package main

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	// Blank-import the lightweight bowrain schema package so the desktop
	// app validates bowrain recipes (server, hooks, automations, assets,
	// brand_voice, per-item collection/base/assets/asset_max_size). This
	// pulls in extension decoders only — no heavy CLI / connector code.
	_ "github.com/neokapi/neokapi/bowrain/plugin/schema"

	"github.com/neokapi/neokapi/kapi-desktop/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	appService := backend.NewApp()

	app := application.New(application.Options{
		Name: "Kapi",
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

	appService.SetApplication(app)

	// Wire the in-app updater (appcast feed for the current channel). No-op
	// until a real signing key is committed; never blocks startup.
	backend.InitUpdater(app)

	// --- Application menu ---
	//
	// The whole menu is rebuilt by buildAppMenu so the Recent Projects submenu
	// always reflects the current recent-projects list. The menu is set once
	// here, again once the app has started, and on every "recent:changed"
	// event the backend emits when a project is opened/created/cleared.
	app.Menu.SetApplicationMenu(buildAppMenu(app, appService))

	// Rebuild the menu whenever the recent-projects list changes so newly
	// opened projects appear under File → Recent Projects without a restart.
	app.Event.On("recent:changed", func(*application.CustomEvent) {
		application.InvokeSync(func() {
			app.Menu.SetApplicationMenu(buildAppMenu(app, appService))
		})
	})

	// --- Window ---

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Kapi",
		Width:          1280,
		Height:         800,
		EnableFileDrop: true,
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInsetUnified,
			InvisibleTitleBarHeight: 50,
		},
	})

	// Forward dropped files to the frontend.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		app.Event.Emit("files-dropped", files)
	})

	// Handle .kapi files opened from Finder (double-click, or second instance).
	// macOS sends the file path(s) to the running instance via this event.
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(event *application.ApplicationEvent) {
		files := event.Context().OpenedFiles()
		if len(files) == 0 {
			return
		}
		// Bring window to front and open each file as a new tab.
		application.InvokeSync(func() {
			win.Focus()
		})
		for _, filePath := range files {
			go func(path string) {
				tab, err := appService.OpenProject(path)
				if err != nil {
					slog.Info("open file", "error", err)
					return
				}
				app.Event.Emit("open-project-tab", tab)
			}(filePath)
		}
	})

	// Load plugins in the background after all services have started, and
	// rebuild the menu once the app is fully up so Recent Projects reflects the
	// persisted list even on the very first frame.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		application.InvokeSync(func() {
			app.Menu.SetApplicationMenu(buildAppMenu(app, appService))
		})
		go appService.LoadPlugins()
	})

	if err := app.Run(); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}

// buildAppMenu constructs the full native application menu. It is called at
// startup and rebuilt whenever the recent-projects list changes, so the
// Recent Projects submenu always mirrors the live list from appService.
func buildAppMenu(app *application.App, appService *backend.App) *application.Menu {
	menu := application.NewMenu()

	// App menu (macOS standard)
	menu.AddRole(application.AppMenu)

	// File menu
	fileMenu := menu.AddSubmenu("File")
	fileMenu.Add("New Project").
		SetAccelerator("CmdOrCtrl+N").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("menu:new-project", nil)
		})
	fileMenu.Add("Open...").
		SetAccelerator("CmdOrCtrl+O").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("menu:open-project", nil)
		})
	fileMenu.AddSeparator()

	// Recent Projects submenu — populated from the live recent store.
	recentMenu := fileMenu.AddSubmenu("Recent Projects")
	home, _ := os.UserHomeDir()
	recents := appService.ListRecentFiles()
	for _, recent := range recents {
		r := recent // capture
		// Format: ~/path (Name) — include filename if not "project.kapi".
		dir := filepath.Dir(r.Path)
		if home != "" && strings.HasPrefix(dir, home) {
			dir = "~" + dir[len(home):]
		}
		var label string
		if filepath.Base(r.Path) != "project.kapi" {
			label = dir + "/" + filepath.Base(r.Path) + " (" + r.Name + ")"
		} else {
			label = dir + " (" + r.Name + ")"
		}
		recentMenu.Add(label).
			SetTooltip(r.Path).
			OnClick(func(ctx *application.Context) {
				app.Event.Emit("menu:open-recent", r.Path)
			})
	}
	if len(recents) == 0 {
		recentMenu.Add("No Recent Projects").SetEnabled(false)
	} else {
		recentMenu.AddSeparator()
		recentMenu.Add("Clear Recent Projects").OnClick(func(ctx *application.Context) {
			appService.ClearRecentFiles()
		})
	}

	fileMenu.AddSeparator()
	fileMenu.Add("Save").
		SetAccelerator("CmdOrCtrl+S").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("menu:save-project", nil)
		})
	fileMenu.Add("Save As...").
		SetAccelerator("CmdOrCtrl+Shift+S").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("menu:save-project-as", nil)
		})

	// In-app updater: check → download → verify → swap → relaunch.
	fileMenu.AddSeparator()
	fileMenu.Add("Check for Updates…").
		OnClick(func(ctx *application.Context) {
			backend.CheckForUpdatesNow(app)
		})

	// Edit menu (macOS standard)
	menu.AddRole(application.EditMenu)

	// View menu
	menu.AddRole(application.ViewMenu)

	// Window menu (macOS standard)
	menu.AddRole(application.WindowMenu)

	// Help menu (macOS standard)
	menu.AddRole(application.HelpMenu)

	return menu
}
