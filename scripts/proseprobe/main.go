// Command proseprobe scores the Prose maturity axis. The rungs, the naming
// contract and the three outcomes are defined in docs/internals/format-maturity.md
// §2.8.
//
// A rung is awarded only on evidence: a test named TestProseP<n>_<subject> that
// `go test -json` reports as passing. The probe finds those tests in every
// module go.work names and in each plugin module under plugins/, runs them,
// and records every rung as met, not met, or did not run. A skipped test, a
// package that failed to build and a test that produced no result all count as
// did not run, and none of them awards a rung. A missing lower rung caps the
// level.
//
// Every run also scores the canary subject in ./canary, whose tests are built
// to be refused. The canary must score P0 with exactly the outcomes its tests
// were written to produce. When it does not, the classification is broken, the
// run is invalid, and no report is written.
//
// Usage:
//
//	go run ./scripts/proseprobe                 # report on stdout
//	go run ./scripts/proseprobe -o report.json  # report in a file
//
// Exit status: 0 for a valid report, 1 for an invalid run, 2 for an error that
// stopped the probe before it could score anything.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, ExecGo))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, goRun GoRunner) int {
	fs := flag.NewFlagSet("proseprobe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "repository root (default: the nearest directory above the working directory that holds go.work)")
	tags := fs.String("tags", "fts5", "build tags for workspace modules, comma-separated in one flag")
	timeout := fs.String("timeout", "10m", "go test -timeout for each module")
	out := fs.String("o", "", "write the report to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *root == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "proseprobe: %v\n", err)
			return 2
		}
		found, err := findWorkspaceRoot(wd)
		if err != nil {
			fmt.Fprintf(stderr, "proseprobe: %v\n", err)
			return 2
		}
		*root = found
	}

	rep, problems, err := Probe(ctx, Options{Root: *root, Tags: *tags, Timeout: *timeout, Run: goRun})
	if err != nil {
		fmt.Fprintf(stderr, "proseprobe: %v\n", err)
		return 2
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(stderr, "proseprobe: %s\n", p)
		}
		fmt.Fprintln(stderr, "proseprobe: invalid run; no report written")
		return 1
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "proseprobe: %v\n", err)
		return 2
	}
	data = append(data, '\n')
	if *out == "" {
		_, err = stdout.Write(data)
	} else {
		err = os.WriteFile(*out, data, 0o644)
	}
	if err != nil {
		fmt.Fprintf(stderr, "proseprobe: %v\n", err)
		return 2
	}
	fmt.Fprintln(stderr, summary(rep))
	return 0
}

// summary is the one line a caller sees on a valid run: the canary's score
// and how the subjects fell.
func summary(rep *Report) string {
	levels := map[string]int{}
	presence := map[Presence]int{}
	tests := 0
	for _, m := range rep.Modules {
		tests += m.Tests
	}
	for _, s := range rep.Subjects {
		presence[s.Presence]++
		if s.Level != "" {
			levels[s.Level]++
		}
	}
	return fmt.Sprintf("proseprobe: canary %s; %d rung tests in %d modules; %d subjects: P0 %d, P1 %d, P2 %d, P3 %d, P4 %d; absent %d, did not run %d",
		rep.Canary.Level, tests, len(rep.Modules), len(rep.Subjects),
		levels["P0"], levels["P1"], levels["P2"], levels["P3"], levels["P4"],
		presence[Absent], presence[PresenceUnproven])
}

func findWorkspaceRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.work above the working directory; pass -root")
		}
		dir = parent
	}
}
