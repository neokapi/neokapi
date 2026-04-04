package main

import "github.com/neokapi/neokapi/cli"

func main() {
	cli.Run(rootCmd, app.Shutdown)
}
