//go:build parity

package roundtrip

import "testing"

// PseudoSpec is the deterministic block-text transform every engine
// applies. Defaults yield "«Hello»" from "Hello", which survives
// every text-bearing format we ship without tripping XML/JSON
// escaping rules. Override Prefix/Suffix to test alternative
// markers; the harness only needs *some* deterministic wrap to
// distinguish translated text from skeleton.
type PseudoSpec struct {
	// Prefix is prepended to each translatable block's source text
	// to form the target. Defaults to "«".
	Prefix string

	// Suffix is appended. Defaults to "»".
	Suffix string

	// SourceLocale and TargetLocale default to "en" / "fr" when
	// empty.
	SourceLocale string
	TargetLocale string
}

// Wrap applies the pseudo transform to a single source string.
func (p PseudoSpec) Wrap(s string) string {
	prefix, suffix := "«", "»"
	if p.Prefix != "" {
		prefix = p.Prefix
	}
	if p.Suffix != "" {
		suffix = p.Suffix
	}
	return prefix + s + suffix
}

// SrcLocale returns the source locale with default applied.
func (p PseudoSpec) SrcLocale() string {
	if p.SourceLocale != "" {
		return p.SourceLocale
	}
	return "en"
}

// TgtLocale returns the target locale with default applied.
func (p PseudoSpec) TgtLocale() string {
	if p.TargetLocale != "" {
		return p.TargetLocale
	}
	return "fr"
}

// Input bundles a single document fixture for the harness. Filename
// matters because tikal and the bridge daemon dispatch on extension.
type Input struct {
	// Bytes is the document contents.
	Bytes []byte
	// Filename is the document's basename (e.g. "test.html"). Its
	// extension drives both tikal's filter detection and the
	// bridge's MIME inference.
	Filename string
	// Companions are sibling files that must be present in the same
	// directory as the input for the format to parse correctly
	// (e.g. okf_xml's `<?its-rules href="X.xml"?>` references). The
	// harness writes each entry alongside the input in its tmpDir;
	// keys are basenames, values are file bytes. Discovery is the
	// caller's responsibility — typically same-directory siblings
	// sharing the input's stem prefix.
	Companions map[string][]byte
}

// Result is one engine's round-trip outcome.
type Result struct {
	// Engine identifies which implementation produced this result
	// ("native", "bridge", "tikal").
	Engine string
	// Output is the merged document bytes returned by the engine.
	Output []byte
	// Skipped is true when the engine wasn't available on this
	// runner (e.g. tikal not installed). The harness records the
	// skip and continues with the engines that did run.
	Skipped bool
	// SkipReason explains why the engine bowed out.
	SkipReason string
}

// Engine is one round-trip implementation. Each engine is responsible
// for end-to-end extract → pseudo-translate → merge of the input
// using its own toolchain. Failures should call t.Fatal — a
// participating engine's hard failure means the format is broken
// for that engine and the test should surface it.
type Engine interface {
	// Name identifies the engine for reports and logs.
	Name() string

	// Available reports whether the engine can run on this host.
	// Returning a non-nil error makes the harness mark the engine
	// Skipped with the error as SkipReason instead of running it.
	Available() error

	// RoundTrip performs extract → pseudo-translate → merge and
	// returns the merged document bytes.
	RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte
}
