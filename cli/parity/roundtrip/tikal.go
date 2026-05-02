//go:build parity

package roundtrip

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// ExtraExtractArgs is appended to the `tikal -x` invocation
	// (e.g. "-fc okf_html@my-config" to pin a filter configuration).
	// Optional.
	ExtraExtractArgs []string
}

// Name returns "tikal".
func (e *TikalEngine) Name() string { return "tikal" }

// Available looks for tikal on PATH, in $OKAPI_HOME, or at the
// $OKAPI_TIKAL override. Returns a descriptive error when missing
// — the harness records this as Skipped.
func (e *TikalEngine) Available() error {
	if _, err := tikalPath(); err != nil {
		return err
	}
	return nil
}

// RoundTrip extracts via tikal, fills XLIFF targets, merges, and
// returns the merged output bytes.
func (e *TikalEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	path, err := tikalPath()
	if err != nil {
		t.Skipf("TikalEngine: %v", err)
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, in.Filename)
	if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
		t.Fatalf("TikalEngine: write input: %v", err)
	}

	src := spec.SrcLocale()
	tgt := spec.TgtLocale()

	extractArgs := []string{"-x", inputPath, "-sl", src, "-tl", tgt}
	extractArgs = append(extractArgs, e.ExtraExtractArgs...)
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
	if out, err := tikalRun(path, []string{"-m", xliffPath, "-od", outDir}, 60*time.Second); err != nil {
		t.Fatalf("TikalEngine: tikal -m failed: %v\n%s", err, out)
	}

	mergedPath := filepath.Join(outDir, in.Filename)
	mergedData, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("TikalEngine: read merged %s: %v", mergedPath, err)
	}
	return mergedData
}

// transUnit accumulates one <trans-unit>'s tokens during XLIFF
// rewriting; flushTransUnit replays them with a fresh <target>
// inserted after </source>.
type transUnit struct {
	translate string
	sourceTxt string
	buf       []xml.Token
	inSource  bool
	inTarget  bool
}

// fillXLIFFTargets walks the XLIFF document, finds every
// <trans-unit>'s <source>, and writes its wrapped form into
// <target>. Untranslatable units (translate="no") are left alone.
//
// We re-emit the XLIFF rather than doing string substitution so we
// don't accidentally rewrite text inside unrelated nodes (alt-trans,
// notes, etc.). Tikal's XLIFF is well-formed standard XLIFF 1.2 —
// encoding/xml handles it without surprises.
func fillXLIFFTargets(in []byte, spec PseudoSpec) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(in))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)

	var tu *transUnit
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "trans-unit":
				tu = &transUnit{translate: attr(t, "translate")}
				tu.buf = append(tu.buf, xml.CopyToken(t))
				continue
			case "source":
				if tu != nil {
					tu.inSource = true
					tu.buf = append(tu.buf, xml.CopyToken(t))
					continue
				}
			case "target":
				if tu != nil {
					tu.inTarget = true
					continue
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "trans-unit":
				if tu != nil {
					tu.buf = append(tu.buf, xml.CopyToken(t))
					if err := flushTransUnit(enc, tu, spec); err != nil {
						return nil, err
					}
					tu = nil
					continue
				}
			case "source":
				if tu != nil {
					tu.inSource = false
					tu.buf = append(tu.buf, xml.CopyToken(t))
					continue
				}
			case "target":
				if tu != nil {
					tu.inTarget = false
					continue
				}
			}
		case xml.CharData:
			if tu != nil {
				if tu.inSource {
					tu.sourceTxt += string(t)
				}
				if tu.inTarget {
					continue
				}
				tu.buf = append(tu.buf, xml.CopyToken(t))
				continue
			}
		}
		if tu != nil {
			tu.buf = append(tu.buf, xml.CopyToken(tok))
			continue
		}
		if err := enc.EncodeToken(tok); err != nil {
			return nil, err
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// flushTransUnit writes a buffered <trans-unit> back out, injecting
// a <target> populated with spec.Wrap(source) immediately after
// </source>. Skips injection when translate="no" or source is empty.
func flushTransUnit(enc *xml.Encoder, tu *transUnit, spec PseudoSpec) error {
	wrote := false
	for _, tok := range tu.buf {
		if err := enc.EncodeToken(tok); err != nil {
			return err
		}
		if end, ok := tok.(xml.EndElement); ok && end.Name.Local == "source" && !wrote {
			if tu.translate != "no" && tu.sourceTxt != "" {
				targetStart := xml.StartElement{Name: xml.Name{Local: "target"}}
				if err := enc.EncodeToken(targetStart); err != nil {
					return err
				}
				if err := enc.EncodeToken(xml.CharData(spec.Wrap(tu.sourceTxt))); err != nil {
					return err
				}
				if err := enc.EncodeToken(xml.EndElement{Name: targetStart.Name}); err != nil {
					return err
				}
			}
			wrote = true
		}
	}
	return nil
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
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
