// Package conformance verifies that a plugin implements kapi's plugin
// protocol v1.
//
// It is a black-box driver: point it at an installed plugin directory and it
// exercises the same contract the kapi host relies on — the manifest, the
// standard binary verbs, and whichever of the three transport modes the
// manifest declares. Nothing here reads plugin source, so the plugin may be
// written in any language.
//
//	report, err := conformance.Run(ctx, conformance.Suite{Dir: "/path/to/myplugin"})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Print(report.Text())
//	if !report.OK() {
//		os.Exit(1)
//	}
//
// From a Go test, [RunT] fails the test once per failing required check:
//
//	func TestProtocolConformance(t *testing.T) {
//		conformance.RunT(t, conformance.Suite{Dir: pluginDir})
//	}
//
// # Scope and stability
//
// This package is the published, versioned contract for out-of-tree plugin
// repositories: they depend on a released neokapi module and self-report
// conformance in their own CI, rather than living inside this repository. The
// exported API is therefore treated as stable — checks may be added, and a
// check's Detail text may change, but check IDs are not renamed or repurposed
// within a protocol version.
//
// [ProtocolVersion] names the protocol revision the suite validates. A plugin
// that passes every required check for a given ProtocolVersion is conformant
// for that revision.
//
// # Safety
//
// The suite never invokes a plugin capability that could have side effects on
// its own initiative. It runs only the standard read-only verbs (version,
// doctor, mcp-server, daemon) plus deliberately invalid arguments whose sole
// job is to prove the plugin rejects them. Real work — running a declared
// command, reading a document through a declared format, segmenting text — is
// opt-in via the *Probe fields on [Suite], because only the plugin author
// knows which of their capabilities are safe to exercise.
package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// ProtocolVersion is the plugin-protocol revision this suite validates.
// It tracks the spec published at
// web/docs/contribute/implementation/engine/plugin-protocol-v1.md.
const ProtocolVersion = "1"

// DefaultTimeout is the per-check budget applied when [Suite.Timeout] is zero.
// It is generous on purpose: a failing check should report a real failure rather
// than an impatient one.
const DefaultTimeout = 60 * time.Second

// DefaultStartupTimeout is the handshake budget applied when neither
// [Suite.StartupTimeout] nor the manifest's daemon.startup_timeout_seconds says
// otherwise. It matches the host's own default wait, and is generous on purpose:
// a JVM or Python daemon can take seconds to reach its handshake.
const DefaultStartupTimeout = 30 * time.Second

// DefaultShutdownGrace is the post-teardown budget applied when
// [Suite.ShutdownGrace] is zero. It mirrors the host's own grace period, widened
// a little because a JVM-hosted daemon unwinds more slowly than a Go one.
const DefaultShutdownGrace = 5 * time.Second

// ErrStartupTimeout is the cause reported by modeC.handshake when a daemon did
// not print its handshake line inside the startup budget.
//
// It is a distinct failure mode on purpose. "The daemon never announced itself"
// says nothing about whether the plugin speaks the protocol — it is the one
// Mode-C outcome that a slow or heavily loaded machine can produce on its own —
// whereas every other handshake failure is a real protocol violation the plugin
// committed. A caller that must tell them apart (a flaky-CI triage, a retry
// policy) does so with errors.Is(res.Err, conformance.ErrStartupTimeout);
// conformance itself is unaffected, since either way the required check failed.
var ErrStartupTimeout = errors.New("the daemon printed no handshake line within the startup budget")

// Mode identifies a plugin transport mode as defined by the protocol.
type Mode string

const (
	// ModeAny marks a check that applies to every plugin regardless of the
	// transports it declares.
	ModeAny Mode = ""

	// ModeA is the one-shot subprocess transport used for commands.
	ModeA Mode = "A"

	// ModeB is the session-subprocess transport used for MCP tools.
	ModeB Mode = "B"

	// ModeC is the daemon-over-local-socket transport used for formats,
	// flow tools, segmenters, and source connectors.
	ModeC Mode = "C"
)

// Status is the outcome of a single check.
type Status string

const (
	// Pass means the plugin satisfies the check.
	Pass Status = "pass"

	// Fail means the plugin violates the check. A failing check whose
	// Required field is set makes the plugin non-conformant.
	Fail Status = "fail"

	// Skip means the check does not apply — the plugin does not declare the
	// transport, the check needs an opt-in probe, or the platform does not
	// support the transport. A skip is never a failure.
	Skip Status = "skip"
)

// Check describes one conformance check independently of any run. [Checks]
// returns the full set, which makes the suite's coverage reviewable (and
// diffable in CI) without executing anything.
type Check struct {
	// ID is the stable identifier, formatted "<group>.<name>" — for example
	// "manifest.parse" or "modeC.handshake". IDs are never renamed or
	// repurposed within a protocol version, so they are safe to reference
	// from Suite.Only, Suite.Skip, and external dashboards.
	ID string

	// Title is a one-line human description of what is being checked.
	Title string

	// Mode is the transport the check belongs to, or ModeAny when it applies
	// to every plugin.
	Mode Mode

	// Required reports whether failing this check makes the plugin
	// non-conformant. A non-required (advisory) failure is reported but does
	// not clear Report.OK — it flags something the host tolerates today but
	// that a well-behaved plugin should fix.
	Required bool

	// Why states, in one line, what breaks in kapi when the check fails. It
	// exists so a failure is actionable without opening the spec.
	Why string
}

// Group returns the portion of the ID before the first dot — the value
// Suite.Only and Suite.Skip match against when given a group name.
func (c Check) Group() string {
	if i := strings.IndexByte(c.ID, '.'); i >= 0 {
		return c.ID[:i]
	}
	return c.ID
}

// Result is the outcome of running one [Check].
type Result struct {
	Check

	// Status is the outcome.
	Status Status

	// Detail is always set: what was observed, or why the check was skipped.
	Detail string

	// Err carries the underlying cause for a failure, when there is one.
	Err error

	// Elapsed is how long the check took.
	Elapsed time.Duration
}

// CommandProbe opts a Mode-A plugin into having one declared command actually
// invoked. Only the plugin author knows which of their commands is safe to run
// unattended, so the suite never picks one on its own.
type CommandProbe struct {
	// Command is the top-level command name as declared in
	// capabilities.commands[].name.
	Command string

	// Args are passed after the command name, exactly as kapi would forward a
	// user's arguments.
	Args []string

	// WantExit is the exit code the plugin is expected to produce.
	WantExit int

	// WantStdout, when non-empty, must appear in the command's stdout.
	WantStdout string
}

// FormatProbe opts a Mode-C plugin into one real read through a declared
// format. The suite writes Input to a temporary file and hands the daemon that
// path (the protocol prefers paths over inline bytes so the plugin can resolve
// companion files), then requires at least MinBlocks translatable blocks back.
type FormatProbe struct {
	// Format is the format name as declared in capabilities.formats[].name.
	Format string

	// Input is the document to read.
	Input []byte

	// Filename is the basename the input is staged under, so extension-based
	// sniffing inside the plugin sees a plausible name. Defaults to
	// "conformance-probe.txt".
	Filename string

	// SourceLocale is the BCP-47 source locale sent in the process header.
	// Defaults to "en".
	SourceLocale string

	// MinBlocks is the minimum number of translatable blocks the read must
	// produce. Defaults to 1.
	MinBlocks int
}

// SegmentProbe opts a Mode-C plugin into one real call to a declared
// segmentation engine.
type SegmentProbe struct {
	// Engine is the engine name as declared in capabilities.segmenters[].name.
	Engine string

	// Text is the text to segment.
	Text string

	// Locale is the BCP-47 locale. Defaults to "en".
	Locale string

	// Params are engine-specific parameters.
	Params map[string]string

	// MinBoundaries is the minimum number of interior boundaries expected.
	// Defaults to 1.
	MinBoundaries int
}

// Suite configures one conformance run.
type Suite struct {
	// Dir is the plugin's install directory — the one holding manifest.json.
	// Required.
	Dir string

	// Env adds "KEY=VALUE" entries to the environment of every plugin process
	// the suite spawns, on top of the host environment and the protocol's own
	// KAPI_PLUGIN_* variables.
	//
	// The host environment arrives scrubbed of provider API keys, exactly as it
	// does under kapi (core/credentials/providerenv). Naming one here overrides
	// that — which is the point: a caller writing the variable down is granting
	// it, where a plugin inheriting it was never granted anything.
	Env []string

	// Timeout bounds the work each individual check does. Zero means
	// DefaultTimeout.
	//
	// It does not bound the two waits the suite spends on the daemon's own
	// lifecycle — for its handshake at startup, and for its exit at teardown.
	// Those have their own budgets, StartupTimeout and ShutdownGrace, and a
	// check that owns one is given its Timeout *plus* that wait. Folding all
	// three into one knob means shortening either wait starves the other: a
	// caller trimming Timeout to shorten a deliberate teardown wait would also
	// cut the startup budget, and every negative case would then collapse into
	// a startup timeout on a loaded machine instead of reporting the protocol
	// violation it was written to catch.
	Timeout time.Duration

	// StartupTimeout bounds how long a Mode-C daemon may take to print its
	// handshake line. Zero means the manifest's
	// daemon.startup_timeout_seconds, or DefaultStartupTimeout when the
	// manifest does not declare one.
	//
	// Shorten it only to wait out a daemon that is expected never to announce
	// itself; a plugin that announces something invalid does so promptly, so it
	// wants the generous default however impatient the rest of the run is.
	StartupTimeout time.Duration

	// ShutdownGrace bounds how long a daemon may take to exit after the
	// Shutdown RPC or SIGTERM. Zero means DefaultShutdownGrace.
	ShutdownGrace time.Duration

	// Only, when non-empty, restricts the run to checks whose ID or group
	// appears in the list. Everything else is omitted from the report
	// entirely (not reported as skipped).
	Only []string

	// Skip lists check IDs or groups to report as skipped without running.
	Skip []string

	// CommandProbe optionally exercises one declared Mode-A command.
	CommandProbe *CommandProbe

	// FormatProbe optionally exercises one declared Mode-C format.
	FormatProbe *FormatProbe

	// SegmentProbe optionally exercises one declared Mode-C segmenter.
	SegmentProbe *SegmentProbe

	// Logf, when set, receives a trace line per check. Nil discards them.
	Logf func(format string, args ...any)
}

func (s Suite) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return DefaultTimeout
}

// shutdownGrace resolves the teardown budget. Unlike the startup budget it does
// not consult the manifest: the protocol lets a plugin declare how long it needs
// to *start*, not how long it may take to stop when the host wants its process
// back.
func (s Suite) shutdownGrace() time.Duration {
	if s.ShutdownGrace > 0 {
		return s.ShutdownGrace
	}
	return DefaultShutdownGrace
}

func (s Suite) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

// selected reports whether a check should run, be skipped by request, or be
// omitted from the report.
func (s Suite) selected(c Check) (run bool, omit bool) {
	matches := func(list []string) bool {
		return slices.Contains(list, c.ID) || slices.Contains(list, c.Group())
	}
	if len(s.Only) > 0 && !matches(s.Only) {
		return false, true
	}
	if matches(s.Skip) {
		return false, false
	}
	return true, false
}

// Report is the outcome of a conformance run.
type Report struct {
	// ProtocolVersion is the revision the run validated.
	ProtocolVersion string `json:"protocol_version"`

	// Plugin and Version identify the plugin, as read from its manifest.
	// Both are empty when the manifest could not be parsed.
	Plugin  string `json:"plugin"`
	Version string `json:"version"`

	// Dir is the directory that was checked.
	Dir string `json:"dir"`

	// Modes lists the transports the manifest declares, in A, B, C order.
	Modes []Mode `json:"modes"`

	// Results holds one entry per check that ran or was skipped, in
	// execution order.
	Results []Result `json:"results"`

	// GeneratedAt is when the run started, and Elapsed how long it took.
	GeneratedAt time.Time     `json:"generated_at"`
	Elapsed     time.Duration `json:"elapsed"`
}

// OK reports whether the plugin is conformant: every required check that ran
// passed. Advisory failures and skips do not clear it.
func (r *Report) OK() bool { return len(r.Failures()) == 0 }

// Failures returns the required checks that failed — the reasons the plugin is
// not conformant.
func (r *Report) Failures() []Result { return r.filter(Fail, true) }

// Advisories returns the non-required checks that failed. These do not break
// conformance, but each one names something a well-behaved plugin should fix.
func (r *Report) Advisories() []Result { return r.filter(Fail, false) }

// Skipped returns the checks that did not apply.
func (r *Report) Skipped() []Result {
	out := []Result{}
	for _, res := range r.Results {
		if res.Status == Skip {
			out = append(out, res)
		}
	}
	return out
}

// Passed returns the checks that passed.
func (r *Report) Passed() []Result {
	out := []Result{}
	for _, res := range r.Results {
		if res.Status == Pass {
			out = append(out, res)
		}
	}
	return out
}

func (r *Report) filter(st Status, required bool) []Result {
	out := []Result{}
	for _, res := range r.Results {
		if res.Status == st && res.Required == required {
			out = append(out, res)
		}
	}
	return out
}

// Result returns the result for a check ID.
func (r *Report) Result(id string) (Result, bool) {
	for _, res := range r.Results {
		if res.ID == id {
			return res, true
		}
	}
	return Result{}, false
}

// Err returns a single error summarising every required failure, or nil when
// the plugin is conformant. It exists so a caller can bubble the outcome up an
// error-returning call chain without walking Failures itself.
func (r *Report) Err() error {
	fails := r.Failures()
	if len(fails) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(fails))
	for _, f := range fails {
		msgs = append(msgs, fmt.Sprintf("%s: %s", f.ID, f.Detail))
	}
	return fmt.Errorf("plugin protocol v%s: %d required check(s) failed: %s",
		r.ProtocolVersion, len(fails), strings.Join(msgs, "; "))
}

// Run executes the suite against the plugin in s.Dir.
//
// A failing check is reported in the returned Report, not as an error: the
// error return is reserved for a suite that could not run at all (an empty or
// unreadable Dir). Callers gate on Report.OK.
//
//nolint:contextcheck // the nil-ctx guard is an API courtesy for non-test callers; ctx is otherwise threaded into every check
func Run(ctx context.Context, s Suite) (*Report, error) {
	if s.Dir == "" {
		return nil, errors.New("conformance: Suite.Dir is required")
	}
	abs, err := filepath.Abs(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("conformance: resolve Dir %q: %w", s.Dir, err)
	}
	s.Dir = abs
	if ctx == nil {
		ctx = context.Background()
	}

	started := time.Now()
	rep := &Report{
		ProtocolVersion: ProtocolVersion,
		Dir:             s.Dir,
		GeneratedAt:     started,
		Results:         []Result{},
		Modes:           []Mode{},
	}

	r := &runner{suite: s, report: rep}
	defer r.cleanup()

	// Resolve the manifest and binary up front, before any check runs. The
	// manifest checks then *report* on that resolution rather than being the
	// only thing that performs it — otherwise a filtered run (Suite.Only:
	// {"modeC"}) would have no manifest to check against and every selected
	// check would skip.
	r.resolve()

	for _, entry := range checkRegistry() {
		run, omit := s.selected(entry.Check)
		if omit {
			continue
		}
		if !run {
			rep.Results = append(rep.Results, Result{
				Check: entry.Check, Status: Skip, Detail: "skipped by Suite.Skip",
			})
			continue
		}
		res := r.exec(ctx, entry)
		rep.Results = append(rep.Results, res)
		s.logf("%-34s %-4s %s", res.ID, res.Status, res.Detail)
	}

	rep.Elapsed = time.Since(started)
	return rep, nil
}

// registryEntry pairs a Check's metadata with its implementation.
type registryEntry struct {
	Check
	run func(ctx context.Context, r *runner) (Status, string, error)

	// lifecycleWait, when set, is how long the check spends waiting on the
	// daemon's own lifecycle rather than doing work of its own — waiting for the
	// handshake, or for the process to exit. The per-check deadline is
	// Suite.Timeout plus this, so the check's own budget stays authoritative:
	// whichever of the two waits expires, the reported reason is the specific
	// one and not a generic per-check timeout.
	lifecycleWait func(r *runner) time.Duration
}

// checkRegistry returns every check in execution order. Order is load-bearing:
// manifest checks run first because everything downstream reads the parsed
// manifest, and the Mode-C teardown checks run last because they stop the
// daemon the other Mode-C checks share.
func checkRegistry() []registryEntry {
	var out []registryEntry
	out = append(out, manifestChecks()...)
	out = append(out, binaryChecks()...)
	out = append(out, modeAChecks()...)
	out = append(out, modeBChecks()...)
	out = append(out, modeCChecks()...)
	return out
}

// Checks returns every check the suite can run, in execution order. It reads
// no files and spawns no processes.
func Checks() []Check {
	entries := checkRegistry()
	out := make([]Check, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Check)
	}
	return out
}

// runner carries the state shared across checks in one run.
type runner struct {
	suite  Suite
	report *Report

	// raw is the manifest bytes, and readErr why they could not be read.
	raw     []byte
	readErr error

	// man is the decoded manifest. It is populated whenever the document is
	// well-formed JSON, even if strict validation rejected it — so the
	// individual manifest checks can name the specific rule broken instead of
	// every one of them skipping behind manifest.parse.
	man *manifest.Manifest

	// parseErr is the strict manifest.Parse error (nil when the manifest is
	// fully valid), and decodeErr the lenient JSON decode error.
	parseErr  error
	decodeErr error

	// binPath is the resolved plugin executable, and binErr why it did not
	// resolve. Checks that execute the plugin require binPath.
	binPath string
	binErr  error

	// binMode is the resolved executable's permission bits, for reporting.
	binMode os.FileMode

	// daemon is the Mode-C session, nil until modeC.handshake succeeds.
	daemon *daemonSession

	// startupBudgetExpired is the startup budget that ran out, set only when
	// modeC.handshake failed because the daemon never announced itself. It lets
	// the dependent checks skip with that specific cause instead of a generic
	// "the daemon did not start", which reads as a protocol failure.
	startupBudgetExpired time.Duration
}

// resolve reads and decodes the manifest and locates the plugin binary. It
// records every failure rather than returning one, so each manifest check can
// report its own specific finding.
func (r *runner) resolve() {
	r.raw, r.readErr = os.ReadFile(manifestPath(r.suite.Dir))
	if r.readErr != nil {
		return
	}

	// Strict first: manifest.Parse applies the same validation the host does.
	man, err := manifest.Parse(r.raw)
	r.parseErr = err
	if err == nil {
		r.man = man
	} else {
		// Fall back to a plain decode so the structural checks still have data
		// to work with. A document that is not even well-formed JSON leaves man
		// nil and every dependent check skips behind manifest.parse.
		var lenient manifest.Manifest
		if decErr := json.Unmarshal(r.raw, &lenient); decErr != nil {
			r.decodeErr = decErr
			return
		}
		r.man = &lenient
	}

	r.report.Plugin = r.man.Plugin
	r.report.Version = r.man.Version
	r.report.Modes = declaredModes(r.man)
	r.resolveBinary()
}

// resolveBinary applies the protocol's rules for manifest.binary: relative to
// the plugin directory, inside it, a regular file, and executable.
func (r *runner) resolveBinary() {
	rel := r.man.Binary
	switch {
	case rel == "":
		r.binErr = errors.New("manifest declares no binary")
		return
	case filepath.IsAbs(rel):
		r.binErr = fmt.Errorf("binary %q is absolute; it must be relative to the plugin directory", rel)
		return
	}
	path := filepath.Join(r.suite.Dir, filepath.FromSlash(rel))
	if !withinDir(r.suite.Dir, path) {
		r.binErr = fmt.Errorf("binary %q escapes the plugin directory", rel)
		return
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		r.binErr = fmt.Errorf("binary %q: %w", rel, err)
		return
	case info.IsDir():
		r.binErr = fmt.Errorf("binary %q is a directory", rel)
		return
	case info.Mode().Perm()&0o111 == 0:
		r.binErr = fmt.Errorf("binary %q is not executable (mode %v)", rel, info.Mode().Perm())
		return
	}
	r.binPath, r.binMode = path, info.Mode().Perm()
}

// exec runs one check with its own timeout and converts a panic in a check
// body into a failure, so one misbehaving probe cannot abort the run.
func (r *runner) exec(ctx context.Context, e registryEntry) (res Result) {
	res = Result{Check: e.Check}
	start := time.Now()
	defer func() {
		res.Elapsed = time.Since(start)
		if rec := recover(); rec != nil {
			res.Status = Fail
			res.Detail = fmt.Sprintf("check panicked: %v", rec)
			res.Err = fmt.Errorf("panic: %v", rec)
		}
	}()

	cctx, cancel := context.WithTimeout(ctx, r.budget(e))
	defer cancel()

	status, detail, err := e.run(cctx, r)
	res.Status, res.Detail, res.Err = status, detail, err
	if res.Detail == "" && res.Err != nil {
		res.Detail = res.Err.Error()
	}
	return res
}

// budget is the wall-clock deadline for one check: the per-check working budget
// plus whichever daemon-lifecycle wait the check owns. The sum is what keeps the
// two independent — a check that must sit through a 30-second handshake is not
// also asked to do its own work inside that window.
func (r *runner) budget(e registryEntry) time.Duration {
	d := r.suite.timeout()
	if e.lifecycleWait != nil {
		d += e.lifecycleWait(r)
	}
	return d
}

// cleanup tears down anything a check left running.
func (r *runner) cleanup() {
	if r.daemon != nil {
		r.daemon.close()
		r.daemon = nil
	}
}

// requireManifest is the guard every post-manifest check uses: without a
// parsed manifest there is nothing to check against.
func (r *runner) requireManifest() (Status, string, error) {
	if r.man == nil {
		return Skip, "manifest did not parse, so there is nothing to check against", nil
	}
	return "", "", nil
}

// requireBinary guards checks that need to execute the plugin.
func (r *runner) requireBinary() (Status, string, error) {
	if st, d, err := r.requireManifest(); st != "" {
		return st, d, err
	}
	if r.binPath == "" {
		return Skip, "plugin binary did not resolve, so there is nothing to execute", nil
	}
	return "", "", nil
}

// modeCSupported reports whether Mode C can be exercised on this platform.
// The transport is a Unix-domain socket, so it is POSIX-only today — the same
// limitation the host's daemon pool enforces.
func modeCSupported() bool { return runtime.GOOS != "windows" }
