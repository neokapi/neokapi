package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

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

	// --- Application menu ---

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

	// Recent Projects submenu — populated dynamically from the recent store.
	recentMenu := fileMenu.AddSubmenu("Recent Projects")
	home, _ := os.UserHomeDir()
	for _, recent := range appService.ListRecentFiles() {
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
	if len(appService.ListRecentFiles()) == 0 {
		recentMenu.Add("No Recent Projects").SetEnabled(false)
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

	// Edit menu (macOS standard)
	menu.AddRole(application.EditMenu)

	// View menu
	menu.AddRole(application.ViewMenu)

	// Window menu (macOS standard)
	menu.AddRole(application.WindowMenu)

	// Help menu
	menu.AddRole(application.HelpMenu)

	app.Menu.SetApplicationMenu(menu)

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
					log.Printf("open file: %v", err)
					return
				}
				app.Event.Emit("open-project-tab", tab)
			}(filePath)
		}
	})

	// Signal the frontend that the backend is ready, then load plugins.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		app.Event.Emit("app-ready", nil)
		go appService.LoadPlugins()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
