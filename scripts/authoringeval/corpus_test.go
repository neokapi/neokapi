package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryPlantIsInItsDocument.
//
// A plant is ground truth, and a plant whose text is not in the body is a
// violation that was never written. The check would report it as a miss for
// ever, and recall would be capped below 100% by a typo rather than by the
// check.
func TestEveryPlantIsInItsDocument(t *testing.T) {
	for _, d := range corpus {
		for _, p := range d.Plants {
			// Whitespace is normalized on both sides. The documents are wrapped
			// at 78 columns, so a three-word plant can straddle a line break —
			// "has\nbeen confirmed" — and a line break is a fact about the
			// file rather than about the prose the plant is a claim over.
			assert.Contains(t, flatten(d.Body), flatten(p.Text),
				"%s plants %q, which is not in the document", d.Name, p.Text)
		}
	}
}

// flatten collapses every run of whitespace to one space.
func flatten(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestEveryPlantNamesAMechanism: the mechanism is the axis the whole report is
// grouped by, so an unset one would silently create a fourth group.
func TestEveryPlantNamesAMechanism(t *testing.T) {
	valid := map[string]bool{mechTerm: true, mechPattern: true, mechDeclared: true}
	for _, d := range corpus {
		for _, p := range d.Plants {
			assert.True(t, valid[p.Mechanism],
				"%s plants %q with mechanism %q, which is not one of the three", d.Name, p.Text, p.Mechanism)
		}
	}
}

// TestOnProfileDocumentsPlantNothing.
//
// The on-profile half is the whole false-positive measurement. A plant there
// would make the document count as both the thing to find and the thing to stay
// quiet about.
func TestOnProfileDocumentsPlantNothing(t *testing.T) {
	for _, d := range corpus {
		if d.Kind == onProfile {
			assert.Empty(t, d.Plants, "%s is on-profile and plants a violation", d.Name)
		}
	}
}

// TestTheCorpusHasBothHalves: recall needs planted violations and false
// positives need documents with none. A corpus missing either half measures
// half the question and reports a whole number.
func TestTheCorpusHasBothHalves(t *testing.T) {
	assert.NotEmpty(t, docsOfKind(onProfile), "no clean documents: false positives cannot be measured")
	assert.NotEmpty(t, docsOfKind(offProfile), "no violating documents: recall cannot be measured")
}

// TestEveryMechanismHasEvidence.
//
// The declared mechanism is the one the offline check does not implement, and
// its recall is the finding. That only holds while the corpus plants something
// for it — with no declared plant, the row would vanish and the report would
// read as though the check covers everything.
func TestEveryMechanismHasEvidence(t *testing.T) {
	by := plantsByMechanism()
	for _, m := range []string{mechTerm, mechPattern, mechDeclared} {
		assert.Positive(t, by[m], "no plant exercises the %q mechanism", m)
	}
}

// TestBothProfilesAreValid.
//
// A profile kapi rejects makes every number a measurement of the fixture. The
// contrast profile was written with `sentence_length: long`, which is not one
// of the enum's values, and the run refused to start — this test moves that
// from a runtime failure to a test failure.
func TestBothProfilesAreValid(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	bin := findKapi(root)
	if bin == "" {
		t.Skip("no kapi binary: `make build` first")
	}
	dir := t.TempDir()
	for name, body := range map[string]string{"voice.yaml": referenceProfile, "contrast.yaml": contrastProfile} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		out, err := kapiRun(context.Background(), bin, dir, "voice", "validate", path)
		assert.NoError(t, err, "%s: %s", name, out)
	}
}

// TestTheContrastProfileDisagrees.
//
// authoring-effect subtracts the contrast profile's gain from the reference
// profile's. If the two profiles agreed, the subtraction would cancel a real
// effect and report zero, so the control has to actually pull the other way.
func TestTheContrastProfileDisagrees(t *testing.T) {
	for _, field := range []string{"active_voice", "person_pov", "formality"} {
		ref := valueOf(referenceProfile, field)
		con := valueOf(contrastProfile, field)
		require.NotEmpty(t, ref, "reference profile does not set %s", field)
		require.NotEmpty(t, con, "contrast profile does not set %s", field)
		assert.NotEqual(t, ref, con, "both profiles set %s to %q, so it is not a contrast", field, ref)
	}
}

func valueOf(profile, field string) string {
	for line := range strings.SplitSeq(profile, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && k == field {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// TestTheCommittedCorpusIsCurrent.
//
// The measurements call models and cannot be reproduced in a test, but the
// corpus is a pure function of this package. So the guard is over the corpus
// half: edit a document or a plant without rerunning, and the dashboard shows
// prose the numbers were not computed from.
func TestTheCommittedCorpusIsCurrent(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(root, DefaultOut))
	if os.IsNotExist(err) {
		t.Skip("no dataset yet: run `go run ./scripts/authoringeval`")
	}
	require.NoError(t, err)

	var committed Report
	require.NoError(t, json.Unmarshal(raw, &committed))

	want, err := json.Marshal(describeCorpus())
	require.NoError(t, err)
	got, err := json.Marshal(committed.Corpus)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got),
		"the corpus changed since the dataset was written — rerun: go run ./scripts/authoringeval")
}
