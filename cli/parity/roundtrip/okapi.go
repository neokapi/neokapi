//go:build parity

package roundtrip

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/cli/parity"
)

// OkapiEngine drives the okapi-bridge launcher's `pseudo` subcommand,
// which composes upstream Okapi's
// {@code RawDocumentToFilterEventsStep → TextModificationStep
//   (TYPE_EXTREPLACE / SCRIPT_EXT_LATIN) → FilterEventsToRawDocumentStep}
// pipeline. The same Latin-extended substitution is replicated on the
// Go side by applyPseudoToBlock so all engines transform identically.
//
// This replaces the older TikalEngine (which shelled out to
// `tikal -x`/`-m` and rewrote XLIFF targets via regex). The launcher
// route gives us:
//   - real upstream pseudo-translate semantics (inline codes preserved)
//   - one filter-routed pipeline instead of an XLIFF round-trip
//   - no naive XLIFF placeholder rewriting that mangled inline `<ph>`
//
// The launcher binary is the same `kapi-okapi-bridge` already shipped
// in the parity sandbox — no extra tool needs to be installed.
type OkapiEngine struct {
	// FilterClass is the Okapi filter class or short id (e.g.
	// "okf_html"). Required.
	FilterClass string

	// ParamConfig, when non-empty, is forwarded to the pseudo
	// subcommand as `--fprm <content>`. It must use Okapi's native
	// .fprm format (e.g. "#v1\nmergeCaptions.b=false\n") and is
	// loaded via IParameters.fromString() against the filter.
	ParamConfig string
}

// Name returns "okapi" — this engine produces the upstream Okapi
// reference output via the bridge launcher.
func (e *OkapiEngine) Name() string { return "okapi" }

// Available reports nil if the parity sandbox has the okapi-bridge
// launcher installed. Configuration errors (missing FilterClass) are
// also flagged here so the harness fails the whole binary cleanly via
// TestMain rather than mid-test.
func (e *OkapiEngine) Available() error {
	if e.FilterClass == "" {
		return errors.New("FilterClass is required")
	}
	s, err := parity.LoadSandbox()
	if err != nil {
		return err
	}
	if _, err := os.Stat(s.OkapiBridgeBinary); err != nil {
		return fmt.Errorf("okapi-bridge launcher missing at %s: %w", s.OkapiBridgeBinary, err)
	}
	return nil
}

// RoundTrip writes the input to a tempdir, shells out to
// `kapi-okapi-bridge pseudo --filter <class> --input <in> --output <out>`
// and returns the merged output bytes.
func (e *OkapiEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	s, err := parity.LoadSandbox()
	if err != nil {
		t.Fatalf("OkapiEngine: %v", err)
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, in.Filename)
	if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
		t.Fatalf("OkapiEngine: write input: %v", err)
	}
	outputPath := filepath.Join(tmpDir, "merged-"+in.Filename)

	args := []string{
		"pseudo",
		"--filter", e.FilterClass,
		"--input", inputPath,
		"--output", outputPath,
		"--src-lang", spec.SrcLocale(),
		"--tgt-lang", spec.TgtLocale(),
	}
	if e.ParamConfig != "" {
		args = append(args, "--fprm", e.ParamConfig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.OkapiBridgeBinary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("OkapiEngine: launcher failed: %v\n--- args: %v\n--- output:\n%s", err, args, out)
	}

	merged, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("OkapiEngine: read merged %s: %v\n--- launcher output:\n%s", outputPath, err, out)
	}
	if len(merged) == 0 {
		t.Fatalf("OkapiEngine: launcher produced empty output\n--- launcher output:\n%s", out)
	}
	return merged
}

// Compile-time interface check.
var _ Engine = (*OkapiEngine)(nil)
