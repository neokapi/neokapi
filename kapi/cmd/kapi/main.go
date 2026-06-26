package main

import (
	"os"

	"github.com/neokapi/neokapi/cli"
)

func main() {
	// Multi-call ("busybox") dispatch: when the kapi binary is invoked through a
	// kgrep / ksed / kcat / kconv / kdiff symlink, run that toolbox utility as a
	// standalone root instead of the full kapi command tree. One binary, the
	// extra names.
	if root := cli.BusyboxRoot(app, os.Args[0]); root != nil {
		cli.Run(root, app.Shutdown)
		return
	}
	cli.Run(rootCmd, app.Shutdown)
}
