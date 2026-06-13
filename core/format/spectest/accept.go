package spectest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// accept.go implements accept-mode (format-spec-cases.md §8): `-u`-style
// regeneration of expected.blocks / expected.roundtrip fixtures from the live
// engine run, with tree-sitter's guard rail — accept-mode NEVER rewrites
// class: invalid cases or any expected.error view, so a regressed reader that
// now "accepts" malformed input cannot silently clobber an error expectation.

// AcceptMode reports whether accept-mode is enabled. Enable it by setting the
// KAPI_SPEC_UPDATE environment variable to "1" or "true" before running the
// spec tests, e.g.
//
//	KAPI_SPEC_UPDATE=1 go test ./core/formats/html/ -run TestSpec
//
// git diff is then the review surface for the regenerated fixtures.
func AcceptMode() bool {
	switch os.Getenv("KAPI_SPEC_UPDATE") {
	case "1", "true", "yes":
		return true
	}
	return false
}

// isInlineBlocks reports whether an expected.blocks value is an inline JSONL
// string (recognised by containing a `{`) rather than a sibling file path.
func isInlineBlocks(blocks string) bool {
	return strings.Contains(blocks, "{")
}

// RefuseAcceptForCase returns a non-nil error naming why accept-mode must not
// rewrite a case: it is class: invalid, or it carries an expected.error view.
// This is the guard rail; callers surface the message and skip the rewrite.
func RefuseAcceptForCase(ex spec.Example) error {
	if ex.CaseClass() == spec.ClassInvalid {
		return fmt.Errorf("accept-mode (-u) refuses to rewrite class: invalid case %q (error expectations are hand-maintained)", ex.CaseID())
	}
	if ex.Expected != nil && ex.Expected.Error != nil {
		return fmt.Errorf("accept-mode (-u) refuses to rewrite the expected.error view of case %q", ex.CaseID())
	}
	return nil
}

// UpdateBlocksFixture regenerates a case's file-backed expected.blocks fixture
// from a live part stream and returns the path written. It refuses (per
// RefuseAcceptForCase) on invalid/error-class cases, refuses inline fixtures
// (only file-backed fixtures are rewritable), and errors when the case has no
// expected.blocks view.
func UpdateBlocksFixture(s *spec.Spec, ex spec.Example, parts []*model.Part) (string, error) {
	if err := RefuseAcceptForCase(ex); err != nil {
		return "", err
	}
	if ex.Expected == nil || ex.Expected.Blocks == "" {
		return "", fmt.Errorf("case %q has no expected.blocks to update", ex.CaseID())
	}
	if isInlineBlocks(ex.Expected.Blocks) {
		return "", fmt.Errorf("case %q expected.blocks is inline; accept-mode rewrites file-backed fixtures only", ex.CaseID())
	}
	dump, err := spec.DumpBlockEvents(parts)
	if err != nil {
		return "", err
	}
	return writeFixture(s, ex.Expected.Blocks, dump)
}

// UpdateRoundtripFixture regenerates a normalized roundtrip output fixture
// (expected.roundtrip.output_file) from the live writer output. It refuses on
// invalid/error-class cases and errors when no output_file is declared.
func UpdateRoundtripFixture(s *spec.Spec, ex spec.Example, output []byte) (string, error) {
	if err := RefuseAcceptForCase(ex); err != nil {
		return "", err
	}
	if ex.Expected == nil || ex.Expected.Roundtrip == nil || ex.Expected.Roundtrip.OutputFile == "" {
		return "", fmt.Errorf("case %q has no expected.roundtrip.output_file to update", ex.CaseID())
	}
	return writeFixture(s, ex.Expected.Roundtrip.OutputFile, output)
}

func writeFixture(s *spec.Spec, rel string, data []byte) (string, error) {
	path, err := spec.ResolveFilePath(s, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
