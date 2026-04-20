// Command kapi-i18n-gen regenerates the builtin capability manifest used
// by the i18n extraction pipeline. Driven by //go:generate in
// core/i18n/doc.go; CI runs the same generator and fails on a dirty diff
// against core/i18n/builtins/.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/i18n/gen"
)

func main() {
	out := flag.String("out", "core/i18n/builtins", "output directory (relative to repo root or absolute)")
	flag.Parse()
	if err := gen.Generate(*out); err != nil {
		fmt.Fprintf(os.Stderr, "kapi-i18n-gen: %v\n", err)
		os.Exit(1)
	}
}
