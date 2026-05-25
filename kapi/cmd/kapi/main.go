package main

import (
	"os"

	"github.com/neokapi/neokapi/cli"
)

func main() {
	// Multi-call ("busybox") dispatch: when the kapi binary is invoked through a
	// kgrep / ksed / kcat symlink, run that toolbox utility as a standalone root
	// instead of the full kapi command tree. One binary, three extra names.
	if root := cli.BusyboxRoot(app, os.Args[0]); root != nil {
		cli.Run(root, app.Shutdown)
		return
	}
	cli.Run(rootCmd, app.Shutdown)
}
