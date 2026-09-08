// Command kapimart-absorb runs the record absorb over a project and reports
// what it learned.
//
// It exists because the absorb is otherwise only reachable as a side effect of
// `kapi up`, which also converges — and a converge run would rewrite the very
// target files the absorb is meant to read. This calls the seed directly, so
// the committed record is read and nothing else moves.
//
// Driven by gen/regenerate.sh; not part of any build.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/host"
)

func main() { os.Exit(run()) }

// run holds the body so the deferred Shutdown closes the app's stores on every
// exit path; os.Exit inside it would skip the defer and leave them open.
func run() int {
	project := flag.String("project", "", "path to the project recipe or its directory")
	flag.Parse()

	if *project == "" {
		fmt.Fprintln(os.Stderr, "kapimart-absorb: -project is required")
		return 2
	}

	a := &host.App{}
	// The absorb returns early with no format registry, having done nothing and
	// reported nothing — the failure mode this whole command exists to avoid.
	a.InitRegistries()
	defer a.Shutdown()

	res, err := a.SeedProjectContext(context.Background(), *project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapimart-absorb: %v\n", err)
		return 1
	}

	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapimart-absorb: %v\n", err)
		return 1
	}
	fmt.Println(string(out))

	if res.Record.Learned == 0 && res.Record.Reconciled == 0 {
		fmt.Fprintln(os.Stderr,
			"kapimart-absorb: the record taught the memory nothing: every pair was already answered, "+
				"already stamped, or refused. The seed corpus must be empty and the store rebuilt before this runs.")
		return 1
	}
	return 0
}
