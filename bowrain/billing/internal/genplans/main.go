// Command genplans writes the landing-page plan catalog (the Go-sourced plan
// FACTS) to a committed JSON file. It is invoked only by `go generate ./...` via
// the directive in billing/gen.go — never as a runtime subcommand. The bytes it
// writes are exactly what billing.MarshalLandingPlanCatalog produces, which the
// drift test in the billing package guards against staleness.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/billing"
)

func main() {
	out := flag.String("out", "", "path to write the plan catalog JSON")
	flag.Parse()
	if *out == "" {
		log.Fatal("genplans: -out is required")
	}

	data, err := billing.MarshalLandingPlanCatalog()
	if err != nil {
		log.Fatalf("genplans: marshal catalog: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("genplans: create output dir: %v", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatalf("genplans: write %s: %v", *out, err)
	}
}
