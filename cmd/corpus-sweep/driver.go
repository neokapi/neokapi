package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Driver is the corpus-sweep orchestrator. It never parses a corpus file
// itself — for every file it spawns exactly one worker subprocess (Tika
// ForkParser doctrine) and watches its exit status, wall clock, and RSS, so a
// permahang or evil-OOM kills a throwaway child rather than the orchestrator.
type Driver struct {
	RepoRoot string        // workspace root (resolved by findRepoRoot)
	Formats  []string      // format ids to sweep
	Timeout  time.Duration // per-file wall-clock budget (0 disables → no HANG)
	RSSCap   int64         // per-file RSS cap in bytes (0 disables → no OOM)

	// WorkerArgv is the argv prefix used to spawn a worker; "--format <id>
	// --file <path>" is appended per file. Production sets [exe, "--worker"];
	// the test re-execs the test binary with a helper env.
	WorkerArgv []string
	WorkerEnv  []string // extra env appended to os.Environ() for the worker

	// Promote, when true (default), copies safety failures (CRASH/HANG/OOM,
	// any tier) and Tier B round-trip drifts into the Go fuzz seed dir under
	// PromoteDir, recording a suggested (not applied) origin:bug manifest line.
	Promote    bool
	PromoteDir string // root for promoted seeds; defaults to RepoRoot

	Stderr io.Writer // progress/diagnostics (defaults to os.Stderr)
}

// fileOutcome is one swept file's result.
type fileOutcome struct {
	Format     string `json:"format"`
	File       string `json:"file"` // repo-relative
	Tier       string `json:"tier"`
	Class      string `json:"class"`
	Detail     string `json:"detail,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// promotion records a crasher promoted to the fuzz seed dir + the suggested
// (not applied) corpus.yaml manifest line.
type promotion struct {
	Class        string `json:"class"`
	SourceFile   string `json:"source_file"` // repo-relative
	SeedFile     string `json:"seed_file"`   // repo-relative target
	SHA256       string `json:"sha256"`
	ManifestYAML string `json:"manifest_yaml"` // suggested origin:bug entry
}

// formatReport aggregates one format's sweep.
type formatReport struct {
	Format     string         `json:"format"`
	TierBEmpty bool           `json:"tier_b_empty"`
	Counts     map[string]int `json:"counts"`
	Files      []fileOutcome  `json:"files"`
	Promotions []promotion    `json:"promotions,omitempty"`
}

// sweepReport is the machine-readable artifact consumed by
// scripts/format-ops/record-sweep.mjs (ledger watermarks + run record).
type sweepReport struct {
	GeneratedAt     string                    `json:"generated_at"`
	CorpusRoot      string                    `json:"corpus_root"` // "" when no Tier B fetched
	Timeout         string                    `json:"timeout"`
	RSSCapMB        int                       `json:"rss_cap_mb"`
	TierBEmptyAll   bool                      `json:"tier_b_empty_all"`
	PerFormatCounts map[string]map[string]int `json:"per_format_counts"`
	Totals          map[string]int            `json:"totals"`
	OutputSHA       string                    `json:"output_sha"`
	Formats         []formatReport            `json:"formats"`
}

// Run executes the sweep over d.Formats and returns the aggregate report.
func (d *Driver) Run() (sweepReport, error) {
	rep := sweepReport{
		GeneratedAt:     time.Now().UTC().Format("2006-01-02"),
		Timeout:         d.Timeout.String(),
		RSSCapMB:        int(d.RSSCap >> 20),
		PerFormatCounts: map[string]map[string]int{},
		Totals:          map[string]int{},
		TierBEmptyAll:   true,
	}
	for _, c := range allClasses {
		rep.Totals[string(c)] = 0
	}
	// CorpusRoot is informational — a present fetched corpus is reported even
	// when no format declares Tier B entries yet.
	if root, err := findCorpusRoot(d.RepoRoot); err == nil {
		rep.CorpusRoot = root
	}

	for _, f := range d.Formats {
		fr, err := d.sweepFormat(f)
		if err != nil {
			return rep, err
		}
		rep.Formats = append(rep.Formats, fr)
		rep.PerFormatCounts[f] = fr.Counts
		if !fr.TierBEmpty {
			rep.TierBEmptyAll = false
		}
		for k, v := range fr.Counts {
			rep.Totals[k] += v
		}
	}
	rep.OutputSHA = outputSHA(rep.Formats)
	return rep, nil
}

func (d *Driver) sweepFormat(formatID string) (formatReport, error) {
	fr := formatReport{Format: formatID, Counts: map[string]int{}}
	for _, c := range allClasses {
		fr.Counts[string(c)] = 0
	}
	files, tierBEmpty, err := enumerate(d.RepoRoot, formatID)
	if err != nil {
		return fr, err
	}
	fr.TierBEmpty = tierBEmpty
	for _, cf := range files {
		out := d.runOne(formatID, cf)
		fr.Counts[out.Class]++
		fr.Files = append(fr.Files, out)
		// Promote safety failures always; promote round-trip drift only for
		// Tier B wild files (a Tier A exemplar drift is a known generic-pipeline
		// limitation, not a fresh crasher to capture).
		promote := isSafetyFailure(out.Class) || (isRoundtripDrift(out.Class) && cf.Tier == "B")
		if d.Promote && promote {
			p, perr := d.promote(formatID, cf, out)
			if perr != nil {
				fmt.Fprintf(d.stderr(), "corpus-sweep: promote %s failed: %v\n", cf.RelPath, perr)
				continue
			}
			fr.Promotions = append(fr.Promotions, p)
		}
	}
	return fr, nil
}

// runOne spawns one worker subprocess for a single file and watches it. The
// worker's emitted JSON is authoritative on a clean exit; a wall-clock or RSS
// kill overrides it as HANG/OOM; a missing result line (the worker died) is
// CRASH.
func (d *Driver) runOne(formatID string, cf corpusFile) fileOutcome {
	out := fileOutcome{Format: formatID, File: cf.RelPath, Tier: cf.Tier}

	argv := append(append([]string{}, d.WorkerArgv...), "--format", formatID, "--file", cf.AbsPath)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), d.WorkerEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		out.Class = string(Crash)
		out.Detail = "spawn worker: " + err.Error()
		return out
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	killed := "" // "HANG" | "OOM" once a kill is issued

	for {
		select {
		case werr := <-done:
			out.DurationMs = time.Since(start).Milliseconds()
			switch killed {
			case "HANG":
				out.Class = string(Hang)
				out.Detail = fmt.Sprintf("killed: wall-clock timeout %s", d.Timeout)
			case "OOM":
				out.Class = string(OOM)
				out.Detail = fmt.Sprintf("killed: RSS exceeded %d MiB", d.RSSCap>>20)
			default:
				d.classifyExit(&out, werr, stdout.Bytes(), stderr.String())
			}
			return out
		case <-ticker.C:
			if d.Timeout > 0 && time.Since(start) > d.Timeout {
				killed = "HANG"
				_ = cmd.Process.Kill()
				continue
			}
			if d.RSSCap > 0 {
				if rss, ok := processRSS(cmd.Process.Pid); ok && rss > d.RSSCap {
					killed = "OOM"
					_ = cmd.Process.Kill()
				}
			}
		}
	}
}

// classifyExit derives the outcome from a cleanly-finished (or non-zero)
// worker: prefer the emitted JSON line; absent it, a non-zero/zero exit with no
// line is CRASH.
func (d *Driver) classifyExit(out *fileOutcome, werr error, stdout []byte, stderr string) {
	if res, ok := lastWorkerResult(stdout); ok {
		out.Class = res.Class
		out.Detail = res.Detail
		return
	}
	out.Class = string(Crash)
	if werr != nil {
		out.Detail = fmt.Sprintf("worker exited without a result line: %v; stderr: %s", werr, truncate(stderr, 200))
	} else {
		out.Detail = "worker exited 0 without a result line"
	}
}

// lastWorkerResult parses the last JSON object line on the worker's stdout.
func lastWorkerResult(stdout []byte) (workerResult, bool) {
	lines := strings.Split(strings.TrimRight(string(stdout), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || line[0] != '{' {
			continue
		}
		var res workerResult
		if err := json.Unmarshal([]byte(line), &res); err == nil && res.Class != "" {
			return res, true
		}
	}
	return workerResult{}, false
}

// processRSS returns the resident set size (bytes) of pid via `ps`, which is
// portable across darwin and linux. Best-effort: when ps is unavailable or the
// field cannot be parsed, ok is false and the RSS cap is simply not enforced
// for that sample.
func processRSS(pid int) (int64, bool) {
	cmd := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	kb, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, false
	}
	return kb * 1024, true // ps reports RSS in KiB
}

// promote copies a crasher into the Go fuzz seed dir (as a `go test fuzz v1`
// seed file) and builds a suggested origin:bug manifest line. Manifests are
// never mutated — the line is printed/recorded for the maintainer to apply.
func (d *Driver) promote(formatID string, cf corpusFile, out fileOutcome) (promotion, error) {
	data, err := os.ReadFile(cf.AbsPath)
	if err != nil {
		return promotion{}, err
	}
	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])
	fuzzName := "FuzzRead" + titleFormat(formatID)
	seedName := "sweep_" + shaHex[:12]
	relSeed := filepath.ToSlash(filepath.Join("core", "formats", formatID, "testdata", "fuzz", fuzzName, seedName))

	root := d.PromoteDir
	if root == "" {
		root = d.RepoRoot
	}
	absSeed := filepath.Join(root, filepath.FromSlash(relSeed))
	if err := os.MkdirAll(filepath.Dir(absSeed), 0o755); err != nil {
		return promotion{}, err
	}
	// Go fuzz seed-corpus encoding: a header line + one []byte literal.
	seed := fmt.Sprintf("go test fuzz v1\n[]byte(%q)\n", string(data))
	if err := os.WriteFile(absSeed, []byte(seed), 0o644); err != nil {
		return promotion{}, err
	}

	manifestYAML := fmt.Sprintf(
		"  - path: %s\n    tier: A\n    origin: bug\n    sha256: %s\n    size: %d\n"+
			"    license: Apache-2.0\n    redistributable: true\n    notes: %q",
		relSeed, shaHex, len(data),
		fmt.Sprintf("corpus-sweep %s promotion (file a bug; review minimization): %s", out.Class, truncate(out.Detail, 120)),
	)
	return promotion{
		Class:        out.Class,
		SourceFile:   cf.RelPath,
		SeedFile:     relSeed,
		SHA256:       shaHex,
		ManifestYAML: manifestYAML,
	}, nil
}

func (d *Driver) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

// outputSHA is the deterministic evidence hash over the per-file
// classifications (format/file/tier/class only — excluding timing and free-text
// detail so re-runs hash identically). It is the C3 floor's green-sweep record.
func outputSHA(reports []formatReport) string {
	type row struct {
		Format, File, Tier, Class string
	}
	var rows []row
	for _, fr := range reports {
		for _, f := range fr.Files {
			rows = append(rows, row{f.Format, f.File, f.Tier, f.Class})
		}
	}
	b, _ := json.Marshal(rows)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// titleFormat upper-cases the first rune of a (lowercase) format id, matching
// the existing FuzzRead<Format> seed-dir naming (FuzzReadJson, FuzzReadHtml).
func titleFormat(id string) string {
	if id == "" {
		return ""
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
