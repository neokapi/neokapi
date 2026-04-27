package commands

import (
	"github.com/neokapi/neokapi/cli"
)

// app is set during plugin registration and read by every bowrain command.
// It is populated via the AppInitializer hook registered in init.go's
// init() function so each command body can read app.FormatReg /
// app.PluginLoader without dragging the App through every closure.
var app *cli.App
