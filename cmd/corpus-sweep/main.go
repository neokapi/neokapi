// Command corpus-sweep is the out-of-band Tier B corpus-sweep harness
// (issue #848; docs/internals/format-ops.md §3 ritual 8 "corpus-sweep";
// docs/internals/format-maturity.md §2.5 C3). It executes wild corpus files
// through read→write→read and classifies each into the
// OK / OK_ROUNDTRIP / EXPECTED_REJECT / CRASH / HANG / OOM / ROUNDTRIP_DRIFT
// taxonomy, one file per subprocess, with wall-clock and RSS caps (the Tika
// ForkParser doctrine — a permahang or evil-OOM kills a throwaway child, never
// the orchestrator).
//
// Two modes:
//
//	corpus-sweep --worker --format <id> --file <path>
//	    Classify one file in this (child) process; emit a one-line JSON result.
//
//	corpus-sweep [--formats <id,id,…|all>] [--timeout 10s] [--rss-mb 1024]
//	             [--report <path>] [--no-promote] [--json]
//	    Driver: enumerate each format's Tier B (or Tier A smoke-fallback) files
//	    from its corpus.yaml, spawn one worker per file, aggregate the taxonomy.
//
// The driver prints a human-readable taxonomy table; with --report it also
// writes the machine report consumed by scripts/format-ops/record-sweep.mjs to
// update the corpus-sweep ritual's ledger watermarks.
//
// Until `make fetch-corpus` has a format-corpus-vN release, no format declares
// Tier B files; the driver then sweeps each format's committed Tier A testdata
// as a smoke corpus and reports "Tier B empty" honestly.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

func main() {
	// Mode selector: the worker is chosen by a leading --worker so the rest of
	// the args are the worker's own flags.
	if len(os.Args) > 1 && (os.Args[1] == "--worker" || os.Args[1] == "-worker") {
		os.Exit(runWorkerArgs(os.Args[2:]))
	}
	os.Exit(runDriverArgs(os.Args[1:]))
}

func runDriverArgs(args []string) int {
	fs := flag.NewFlagSet("corpus-sweep", flag.ContinueOnError)
	var (
		formatsArg = fs.String("formats", "all", `comma-separated format ids, or "all" (every format with a corpus.yaml)`)
		timeout    = fs.Duration("timeout", 10*time.Second, "per-file wall-clock budget (kill → HANG); 0 disables")
		rssMB      = fs.Int("rss-mb", 1024, "per-file RSS cap in MiB (kill → OOM); 0 disables")
		reportPath = fs.String("report", "", "write the machine-readable JSON report to this path")
		noPromote  = fs.Bool("no-promote", false, "do not copy crashers into the fuzz seed dir")
		asJSON     = fs.Bool("json", false, "print the full JSON report to stdout instead of the table")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpus-sweep: %v\n", err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpus-sweep: cannot resolve own path: %v\n", err)
		return 1
	}

	formatsList, err := resolveFormats(repoRoot, *formatsArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpus-sweep: %v\n", err)
		return 1
	}
	if len(formatsList) == 0 {
		fmt.Fprintln(os.Stderr, "corpus-sweep: no formats to sweep")
		return 1
	}

	d := &Driver{
		RepoRoot:   repoRoot,
		Formats:    formatsList,
		Timeout:    *timeout,
		RSSCap:     int64(*rssMB) << 20,
		WorkerArgv: []string{exe, "--worker"},
		Promote:    !*noPromote,
		PromoteDir: repoRoot,
		Stderr:     os.Stderr,
	}

	rep, err := d.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpus-sweep: %v\n", err)
		return 1
	}

	if *reportPath != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(*reportPath, append(b, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "corpus-sweep: write report: %v\n", err)
			return 1
		}
	}

	if *asJSON {
		b, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(b))
	} else {
		printReport(os.Stdout, rep)
	}

	// Only the hard safety failures (CRASH/HANG/OOM) break the run; round-trip
	// drift is advisory (recorded for the ritual's count-delta check).
	if rep.Totals[string(Crash)]+rep.Totals[string(Hang)]+rep.Totals[string(OOM)] > 0 {
		return 3
	}
	return 0
}

// resolveFormats expands the --formats argument into a concrete list.
func resolveFormats(repoRoot, arg string) ([]string, error) {
	if strings.TrimSpace(arg) == "all" {
		return manifestFormats(repoRoot)
	}
	var out []string
	for _, f := range strings.Split(arg, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out, nil
}

// printReport renders the human-readable taxonomy table.
func printReport(w *os.File, rep sweepReport) {
	fmt.Fprintf(w, "corpus-sweep %s — timeout %s, RSS cap %d MiB\n", rep.GeneratedAt, rep.Timeout, rep.RSSCapMB)
	if rep.CorpusRoot == "" {
		fmt.Fprintln(w, "Tier B corpus: none fetched (run `make fetch-corpus`); sweeping Tier A committed testdata as a smoke corpus.")
	} else {
		fmt.Fprintf(w, "Tier B corpus root: %s\n", rep.CorpusRoot)
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FORMAT\tTIER\tOK\tOK_RT\tEXP_REJ\tDRIFT\tCRASH\tHANG\tOOM\tFILES")
	for _, fr := range rep.Formats {
		tier := "B"
		if fr.TierBEmpty {
			tier = "A*"
		}
		total := 0
		for _, c := range allClasses {
			total += fr.Counts[string(c)]
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			fr.Format, tier,
			fr.Counts[string(OK)], fr.Counts[string(OKRoundTrip)], fr.Counts[string(ExpectedReject)],
			fr.Counts[string(RoundtripDrift)], fr.Counts[string(Crash)], fr.Counts[string(Hang)], fr.Counts[string(OOM)],
			total)
	}
	fmt.Fprintf(tw, "TOTAL\t\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		rep.Totals[string(OK)], rep.Totals[string(OKRoundTrip)], rep.Totals[string(ExpectedReject)],
		rep.Totals[string(RoundtripDrift)], rep.Totals[string(Crash)], rep.Totals[string(Hang)], rep.Totals[string(OOM)],
		sumCounts(rep.Totals))
	tw.Flush()
	fmt.Fprintf(w, "\n(TIER A* = Tier B empty, smoke over committed Tier A testdata)\n")

	// Promotions + suggested manifest lines.
	for _, fr := range rep.Formats {
		for _, p := range fr.Promotions {
			fmt.Fprintf(w, "\nPROMOTED (%s) %s → %s\n", p.Class, p.SourceFile, p.SeedFile)
			fmt.Fprintf(w, "suggested corpus.yaml entry (NOT applied; add by hand):\n%s\n", p.ManifestYAML)
		}
	}
	fmt.Fprintf(w, "\noutput_sha: %s\n", rep.OutputSHA)
}

func sumCounts(counts map[string]int) int {
	total := 0
	for _, c := range allClasses {
		total += counts[string(c)]
	}
	return total
}
