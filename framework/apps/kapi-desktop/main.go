package main

import (
	"embed"
	"log"

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
		Name: "Kapi Desktop",
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
		Title:          "Kapi Desktop",
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

	// Load plugins in the background after the window is ready.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		go appService.LoadPlugins()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
