// Command main regenerates host/i18n/commands.json. Run via the go:generate
// directive in cli/i18ngen/doc.go (cwd is the cli/i18ngen package directory)
// or pass an explicit output path as the first argument.
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli/i18ngen"
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
