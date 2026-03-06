package main

import "os"

func main() {
	err := rootCmd.Execute()
	app.Shutdown()
	if err != nil {
		os.Exit(1)
	}
}
