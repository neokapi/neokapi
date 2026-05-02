//go:build parity

package roundtrip

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TikalEngine shells out to the upstream tikal CLI from the Okapi
// distribution. The flow is:
//
//  1. Write input.bytes to <tmp>/<filename>.
//  2. Run `tikal -x <filename> -sl en -tl fr` → emits
//     <filename>.xlf with empty <target> elements.
//  3. Walk the XLIFF, populate every <target> = wrap(<source>).
//  4. Run `tikal -m <filename>.xlf -od <tmp>/merged` → writes the
//     translated document at merged/<filename>.
//  5. Read and return those bytes.
//
// We intentionally do not use Okapi's own pseudo-translation step
// (tikal -psd) — that would force a step ordering and accent map
// the other engines can't replicate. Hand-populating <target>
// elements is engine-agnostic and exercises tikal's merge path
// directly.
type TikalEngine struct {
	// ExtraExtractArgs is appended to BOTH `tikal -x` and `tikal -m`
	// (e.g. "-fc okf_xliff2" to pin a filter configuration when
	// extension auto-detection misroutes). Tikal needs the same -fc
	// at merge time it had at extract time. Optional.
	ExtraExtractArgs []string

	// ParamConfig, when non-empty, is written to a temp .fprm file
	// and `-pd <tmpdir>` is appended to both extract and merge
	// invocations. This lets a fixture override default filter
	// parameters (e.g. mergeCaptions.b=false to disable VTT/TTML
	// caption merging on round-trip).
	//
	// The variant name comes from the `-fc okf_xxx@<variant>` value
	// in ExtraExtractArgs — the file written to disk is named
	// `<full-fc-value>.fprm` so tikal's `-pd` resolution finds it.
	// If ExtraExtractArgs has no `@<variant>` form, ParamConfig is
	// rejected at runtime to keep the wiring explicit.
	ParamConfig string
}

// Name returns "tikal".
func (e *TikalEngine) Name() string { return "tikal" }

// Available looks for tikal on PATH, in $OKAPI_HOME, or at the
// $OKAPI_TIKAL override. Returns a descriptive error when missing.
// The package's TestMain calls this once up front and aborts the
// whole binary if tikal is not found — tikal is the comparator,
// not an optional engine.
func (e *TikalEngine) Available() error {
	if _, err := tikalPath(); err != nil {
		return err
	}
	return nil
}

// RoundTrip extracts via tikal, fills XLIFF targets, merges, and
// returns the merged output bytes. Tikal is hard-required (checked
// at TestMain), so a missing binary here means the env changed
// mid-run — fail loudly rather than skipping.
func (e *TikalEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	path, err := tikalPath()
	if err != nil {
		t.Fatalf("TikalEngine: %v", err)
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, in.Filename)
	if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
		t.Fatalf("TikalEngine: write input: %v", err)
	}

	src := spec.SrcLocale()
	tgt := spec.TgtLocale()

	extraArgs := append([]string(nil), e.ExtraExtractArgs...)
	if e.ParamConfig != "" {
		variant := extractFCVariant(extraArgs)
		if variant == "" {
			t.Fatalf("TikalEngine: ParamConfig set but ExtraExtractArgs has no '-fc okf_xxx@<variant>' (the variant name is the .fprm filename tikal will look for)")
		}
		fprmPath := filepath.Join(tmpDir, variant+".fprm")
		if err := os.WriteFile(fprmPath, []byte(e.ParamConfig), 0o644); err != nil {
			t.Fatalf("TikalEngine: write param config: %v", err)
		}
		extraArgs = append(extraArgs, "-pd", tmpDir)
	}

	extractArgs := append([]string{"-x", inputPath, "-sl", src, "-tl", tgt}, extraArgs...)
	if out, err := tikalRun(path, extractArgs, 60*time.Second); err != nil {
		t.Fatalf("TikalEngine: tikal -x failed: %v\n%s", err, out)
	}

	xliffPath := inputPath + ".xlf"
	xliffData, err := os.ReadFile(xliffPath)
	if err != nil {
		t.Fatalf("TikalEngine: read XLIFF %s: %v", xliffPath, err)
	}

	rewritten, err := fillXLIFFTargets(xliffData, spec)
	if err != nil {
		t.Fatalf("TikalEngine: rewrite XLIFF targets: %v", err)
	}
	if err := os.WriteFile(xliffPath, rewritten, 0o644); err != nil {
		t.Fatalf("TikalEngine: write rewritten XLIFF: %v", err)
	}

	outDir := filepath.Join(tmpDir, "merged")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("TikalEngine: mkdir merged: %v", err)
	}
	mergeArgs := append([]string{"-m", xliffPath, "-od", outDir}, extraArgs...)
	if out, err := tikalRun(path, mergeArgs, 60*time.Second); err != nil {
		t.Fatalf("TikalEngine: tikal -m failed: %v\n%s", err, out)
	}

	mergedPath := filepath.Join(outDir, in.Filename)
	mergedData, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("TikalEngine: read merged %s: %v", mergedPath, err)
	}
	return mergedData
}

// xlfTransUnitRE matches one <trans-unit …>…</trans-unit> block in
// tikal's XLIFF output. The body is captured non-greedily so adjacent
// units don't collapse. Group 1: full attribute string of the open
// tag. Group 2: inner content.
var xlfTransUnitRE = regexp.MustCompile(`(?s)<trans-unit\b([^>]*)>(.*?)</trans-unit>`)

// xlfSourceRE matches the inner text of <source …>…</source>. We
// only need the text (no nested inline-codes) for this harness's
// pseudo-translation — tikal's XLIFF for the formats we exercise is
// flat text. Captures the <source> attributes (group 1) and content
// (group 2).
var xlfSourceRE = regexp.MustCompile(`(?s)<source\b([^>]*)>(.*?)</source>`)

// xlfTargetRE matches an existing <target …>…</target> element. We
// replace its inner text with the wrapped source.
var xlfTargetRE = regexp.MustCompile(`(?s)<target\b([^>]*)>(.*?)</target>`)

// xlfTranslateNoRE detects translate="no" on the trans-unit open
// attributes — those units are skipped (translation isn't expected).
var xlfTranslateNoRE = regexp.MustCompile(`\btranslate\s*=\s*"no"`)

// fillXLIFFTargets rewrites every <trans-unit>'s <target> in
// tikal's XLIFF output to carry spec.Wrap(<source-text>). Operates
// on the raw bytes via regex rather than encoding/xml because Go's
// encoder mangles namespace declarations on roundtrip (each element
// re-emits xmlns attributes, and xmlns:* prefix decls become
// _xmlns:* attribute names — both break tikal's merger).
//
// Tikal's XLIFF for the formats this harness exercises is flat
// text inside <source>/<target> with no inline-code (`<g>`, `<x>`,
// etc.). Embedded markup would need decoding; the harness doesn't
// need that today.
func fillXLIFFTargets(in []byte, spec PseudoSpec) ([]byte, error) {
	out := xlfTransUnitRE.ReplaceAllFunc(in, func(unit []byte) []byte {
		m := xlfTransUnitRE.FindSubmatch(unit)
		if m == nil {
			return unit
		}
		openAttrs, body := m[1], m[2]
		if xlfTranslateNoRE.Match(openAttrs) {
			return unit
		}
		srcMatch := xlfSourceRE.FindSubmatch(body)
		if srcMatch == nil {
			return unit
		}
		// Strip XML-escapes from the source text before wrapping —
		// the wrap markers shouldn't double-encode entities. We
		// re-encode below.
		srcDecoded := xmlUnescape(string(srcMatch[2]))
		wrapped := xmlEscape(spec.Wrap(srcDecoded))

		var newBody []byte
		if xlfTargetRE.Match(body) {
			newBody = xlfTargetRE.ReplaceAllFunc(body, func(target []byte) []byte {
				tm := xlfTargetRE.FindSubmatch(target)
				if tm == nil {
					return target
				}
				return []byte(fmt.Sprintf("<target%s>%s</target>", tm[1], wrapped))
			})
		} else {
			// Append a fresh <target> after </source>, inheriting
			// no namespace decl (the surrounding default xmlns
			// applies).
			newBody = []byte(strings.Replace(string(body), "</source>", "</source>\n<target>"+wrapped+"</target>", 1))
		}
		return []byte(fmt.Sprintf("<trans-unit%s>%s</trans-unit>", openAttrs, newBody))
	})
	return out, nil
}

// xmlEscape converts plain text into XLIFF-safe content: only the
// five XML-mandatory escapes. `«»` and other Unicode pass through
// — tikal's XLIFF is UTF-8, no entity-encoding needed.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// xmlUnescape reverses the five mandatory escapes plus the common
// numeric character references tikal might emit. Sufficient for the
// flat text inside tikal's <source> elements for the formats this
// harness exercises.
func xmlUnescape(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
	)
	return r.Replace(s)
}

// extractFCVariant returns the full `-fc` value (e.g. "okf_ttml@nomerge")
// from a tikal arg slice, or "" if no -fc with an @variant is present.
// Used to derive the .fprm filename tikal will look for under -pd.
func extractFCVariant(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-fc" {
			v := args[i+1]
			if strings.Contains(v, "@") {
				return v
			}
			return ""
		}
	}
	return ""
}

// tikalPath resolves the tikal launcher honouring $OKAPI_TIKAL,
// $OKAPI_HOME/tikal.sh, then PATH.
func tikalPath() (string, error) {
	if explicit := os.Getenv("OKAPI_TIKAL"); explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit, nil
		}
		return "", fmt.Errorf("OKAPI_TIKAL=%s: not found", explicit)
	}
	if home := os.Getenv("OKAPI_HOME"); home != "" {
		candidate := filepath.Join(home, "tikal.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if found, err := exec.LookPath("tikal"); err == nil {
		return found, nil
	}
	if found, err := exec.LookPath("tikal.sh"); err == nil {
		return found, nil
	}
	return "", errors.New("tikal not found — set $OKAPI_TIKAL or $OKAPI_HOME, or place tikal on PATH")
}

func tikalRun(name string, args []string, d time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Compile-time interface check.
var _ Engine = (*TikalEngine)(nil)
