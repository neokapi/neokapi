// Command main regenerates cli/i18n/commands.json. Run via the go:generate
// directive in cli/i18n/doc.go (cwd is the cli/i18n package directory) or
// pass an explicit output path as the first argument.
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli/i18n/gen"
)

func main() {
	out := "commands.json"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := gen.Generate(out); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
