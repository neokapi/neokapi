package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/muesli/reflow/wordwrap"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/pluginhost"
	"golang.org/x/term"
)

// doctorResult is the outcome of diagnosing a single plugin.
type doctorResult struct {
	Healthy bool
	Summary string   // one-line status detail
	Checks  []string // per-check lines for the verbose report
	Output  string   // captured self-check stdout/stderr (verbose only)
}

// DiagnosePlugin runs the baseline + self-check diagnostics for one plugin:
//  1. the binary file exists and is a regular file,
//  2. `<binary> version` reports a version matching the manifest,
//  3. if the manifest declares a self-check, `<binary> doctor` exits 0.
func DiagnosePlugin(ctx context.Context, p *pluginhost.Plugin) doctorResult {
	res := doctorResult{Healthy: true}

	if st, err := os.Stat(p.BinaryPath); err != nil || st.IsDir() {
		res.Healthy = false
		res.Summary = "binary missing: " + p.BinaryPath
		res.Checks = append(res.Checks, "✗ binary present ("+p.BinaryPath+")")
		return res
	}
	res.Checks = append(res.Checks, "✓ binary present")

	// The version probe is an integrity hint, not a health gate: it runs across
	// every installed plugin — including third-party ones whose `version`
	// subcommand may print extra text or be absent — so a mismatch is surfaced
	// as a warning, never as "unhealthy". Unhealthy is reserved for a missing
	// binary or a failing declared self-check.
	out, err := RunVersionProbe(ctx, p.BinaryPath)
	actual := strings.TrimSpace(string(out))
	switch {
	case err != nil:
		res.Checks = append(res.Checks, fmt.Sprintf("⚠ version probe failed: %v", err))
	case actual != p.Manifest.Version:
		res.Checks = append(res.Checks, fmt.Sprintf("⚠ version probe %q ≠ manifest %q", actual, p.Manifest.Version))
	default:
		res.Checks = append(res.Checks, "✓ version matches manifest ("+actual+")")
	}

	if !p.Manifest.Capabilities.SelfCheck {
		res.Checks = append(res.Checks, "– no self-check")
		res.Summary = "ok (no self-check)"
		return res
	}

	scOut, scErr := runSelfCheckProbe(ctx, p.BinaryPath)
	res.Output = strings.TrimRight(string(scOut), "\n")
	if scErr != nil {
		res.Healthy = false
		res.Checks = append(res.Checks, fmt.Sprintf("✗ self-check failed: %v", scErr))
		res.Summary = "self-check failed"
		return res
	}
	res.Checks = append(res.Checks, "✓ self-check passed")
	res.Summary = "healthy"
	return res
}

// WriteDoctorReport prints the detailed per-plugin diagnostic report.
func WriteDoctorReport(w io.Writer, p *pluginhost.Plugin, res doctorResult) {
	status := "healthy"
	if !res.Healthy {
		status = "UNHEALTHY"
	}
	fmt.Fprintf(w, "%s %s: %s\n", p.Name(), p.Version(), status)
	fmt.Fprintf(w, "  binary: %s\n", p.BinaryPath)
	for _, c := range res.Checks {
		fmt.Fprintf(w, "  %s\n", c)
	}
	if res.Output != "" {
		fmt.Fprintln(w, "  self-check output:")
		for line := range strings.SplitSeq(res.Output, "\n") {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
}

// runSelfCheckProbe runs the plugin's standard `<binary> doctor` self-check,
// capturing stdout+stderr. A non-nil error means the self-check exited non-zero
// (or the binary could not be run).
func runSelfCheckProbe(ctx context.Context, binPath string) ([]byte, error) {
	return exec.CommandContext(ctx, binPath, "doctor").CombinedOutput()
}

// DescriptionWidth returns the width available for the wrapped description
// column: the terminal width minus prefixWidth. It returns 0 when w is not a
// terminal (piped/redirected output) or the width can't be determined, or when
// the remaining column would be too narrow to wrap usefully — callers treat 0 as
// "don't wrap, print the description in full on one line".
func DescriptionWidth(w io.Writer, prefixWidth int) int {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok || !isatty.IsTerminal(f.Fd()) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	avail := cols - prefixWidth
	if avail < 24 { // too narrow to wrap into; print full lines instead
		return 0
	}
	return avail
}

// WrapText word-wraps s into lines no wider than width cells, breaking only at
// spaces (words longer than width are kept whole). A width of 0 yields the text
// unbroken. It always returns at least one line. Wrapping is delegated to
// muesli/reflow, which is wide-rune- and ANSI-aware.
func WrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	return strings.Split(wordwrap.String(s, width), "\n")
}

// ParsePluginRef splits "name@^1.0" → ("name", "^1.0").
func ParsePluginRef(ref string) (name, constraint string) {
	if before, after, ok := strings.Cut(ref, "@"); ok {
		return before, after
	}
	return ref, ""
}

func KapiVersion() string {
	return version.Version
}

func RunVersionProbe(ctx context.Context, binPath string) ([]byte, error) {
	return exec.CommandContext(ctx, binPath, "version").CombinedOutput()
}
