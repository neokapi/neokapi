// Command kapi-i18n-catalogs compiles the committed translated JSON catalogs
// named by the dogfood recipe into the gettext MO files that core/i18n and
// host/i18n embed. The MO is build output: it is regenerated before anything
// compiles those packages, and it is not in version control.
//
// It links no part of core/i18n, so there is no bootstrap cycle — the
// compiler builds and runs against an empty catalogs directory.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/i18n/gen"
)

func main() {
	root := flag.String("root", ".", "repository root: the directory holding kapi.yaml")
	flag.Parse()
	if _, err := gen.CompileCatalogs(*root, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "kapi-i18n-catalogs: %v\n", err)
		os.Exit(1)
	}
}
