package state_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approved(unit, locale, targetHash string) state.UnitState {
	return state.UnitState{
		Unit:       unit,
		Variant:    model.Variant(model.LocaleID(locale)),
		Status:     model.TargetStatusEstablished,
		TargetHash: targetHash,
		Decision:   state.Decision{ReviewState: "approved", By: "alice", At: "2026-06-29T00:00:00Z"},
		Updated:    "2026-06-29T00:00:00Z",
	}
}

// TestUnitState_StaleOnTranslationChange verifies the targetHash link: an
// approval no longer applies once the translation it blessed changes.
func TestUnitState_StaleOnTranslationChange(t *testing.T) {
	u := approved("h1", "fr-FR", "sha256:aaa")
	aaa, bbb := state.Reading{TargetHash: "sha256:aaa"}, state.Reading{TargetHash: "sha256:bbb"}
	assert.True(t, u.Established(aaa), "established for the translation it blessed")
	assert.False(t, u.Established(bbb), "a changed translation invalidates the approval")
	assert.True(t, u.Stale(bbb))
	assert.False(t, u.Stale(aaa))
}

// TestUnitState_SourceStale covers the basis: the other half of what a decision
// blesses. An approval bound only to the translation survived its source being
// rewritten, so a reviewer's blessing of "refreshed every ten minutes" stayed on
// a unit whose source now said five.
func TestUnitState_SourceStale(t *testing.T) {
	tests := []struct {
		name    string
		basis   string
		current string
		want    bool
	}{
		{"same source", "sha256:src-a", "sha256:src-a", false},
		{"source rewritten", "sha256:src-a", "sha256:src-b", true},
		{"no basis recorded", "", "sha256:src-a", false},
		{"nothing to compare against", "sha256:src-a", "", false},
		{"neither known", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := approved("h1", "nb", "sha256:tgt")
			u.ContentHash = tt.basis
			assert.Equal(t, tt.want, u.SourceStale(state.Reading{ContentHash: tt.current}))
		})
	}
}

// TestUnitState_FreshNeedsBothHalves: a decision is about a pairing, so either
// half moving retires it.
func TestUnitState_FreshNeedsBothHalves(t *testing.T) {
	u := approved("h1", "nb", "sha256:tgt")
	u.ContentHash = "sha256:src"

	assert.True(t, u.Fresh(state.Reading{TargetHash: "sha256:tgt", ContentHash: "sha256:src"}))
	assert.False(t, u.Fresh(state.Reading{TargetHash: "sha256:other", ContentHash: "sha256:src"}), "the translation moved")
	assert.False(t, u.Fresh(state.Reading{TargetHash: "sha256:tgt", ContentHash: "sha256:other"}), "the source moved")
}

// A record that carries revisions is read by them: a change to an inline code
// alone moves the revision where the text, and so the hash, stays. A record
// written before revisions is read by its hashes, as it always was.
func TestUnitState_ReadsByRevisionWhereItCarriesOne(t *testing.T) {
	link := func(href string) []model.Run {
		return []model.Run{
			model.TextR("Read the "),
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
			model.TextR("guide"),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
		}
	}
	b := model.NewRunsBlock("b1", link("https://a.example"))
	b.SourceLocale = "en"
	b.SetTargetRuns("fr", []model.Run{model.TextR("Lisez le guide")})
	was := state.ReadTarget(b, "fr", "en")
	require.NotEmpty(t, was.Basis)
	require.NotEmpty(t, was.Revision)

	decided := approved("b1", "fr", was.TargetHash)
	decided.ContentHash, decided.Basis, decided.Revision = was.ContentHash, was.Basis, was.Revision
	legacy := approved("b1", "fr", was.TargetHash)
	legacy.ContentHash = was.ContentHash

	assert.True(t, decided.Fresh(was))
	assert.True(t, legacy.Fresh(was))

	b.EditSourceRuns(link("https://b.example"))
	now := state.ReadTarget(b, "fr", "en")
	require.Equal(t, was.ContentHash, now.ContentHash, "the source text did not change")
	assert.True(t, decided.SourceStale(now), "the source's link moved under the decision")
	assert.False(t, legacy.SourceStale(now), "a record written before revisions is read by its hash")

	b.EditSourceRuns(link("https://a.example"))
	b.SetTargetRuns("fr", []model.Run{model.TextR("Lisez le "), model.PhR(model.PlaceholderRun{ID: "2", Type: "lb", Data: "<br/>"}), model.TextR("guide")})
	moved := state.ReadTarget(b, "fr", "en")
	assert.False(t, decided.SourceStale(moved), "the source is back at the basis")
	assert.True(t, decided.Stale(moved), "the translation gained a code")
	assert.False(t, decided.Established(moved))

	// A reader with no revision in hand reads every record by hash.
	byHash := state.Reading{TargetHash: moved.TargetHash, ContentHash: moved.ContentHash}
	assert.Equal(t, decided.TargetHash != moved.TargetHash, decided.Stale(byHash))
	assert.Equal(t, was.Basis, state.ReadingOf(decided).Basis, "a record's own reading is the pairing it was recorded against")
	assert.Equal(t, was.Revision, state.ReadingOf(decided).Revision)

	// A basis taken by a read that filed the source under no language is
	// the same content, and reads current.
	plain := model.NewRunsBlock("b1", link("https://a.example"))
	plain.SetTargetRuns("fr", []model.Run{model.TextR("Lisez le "), model.PhR(model.PlaceholderRun{ID: "2", Type: "lb", Data: "<br/>"}), model.TextR("guide")})
	fromPlain := approved("b1", "fr", "")
	fromPlain.Basis = state.ReadTarget(plain, "fr", "").Basis
	require.NotEqual(t, fromPlain.Basis, moved.Basis, "the keys differ")
	assert.False(t, fromPlain.SourceStale(moved), "the content is the same under another key")
	assert.True(t, decided.BasisKnown())
	assert.False(t, state.UnitState{}.BasisKnown())
}

// A pre-review is bound to the translation it judged the same way: by
// revision where it carries one.
func TestAIReview_FreshByRevision(t *testing.T) {
	r := &state.AIReview{Score: 80, TargetHash: "sha256:t", Revision: "r:1111111111111111"}
	assert.True(t, r.Fresh(state.Reading{TargetHash: "sha256:t", Revision: "r:1111111111111111"}))
	assert.False(t, r.Fresh(state.Reading{TargetHash: "sha256:t", Revision: "r:2222222222222222"}), "the codes moved")
	assert.True(t, r.Fresh(state.Reading{TargetHash: "sha256:t"}), "a reader with no revision reads the hash")
	old := &state.AIReview{Score: 80, TargetHash: "sha256:t"}
	assert.True(t, old.Fresh(state.Reading{TargetHash: "sha256:t", Revision: "r:2222222222222222"}), "a review recorded before revisions reads the hash")
	assert.False(t, old.Fresh(state.Reading{TargetHash: "sha256:u", Revision: "r:2222222222222222"}))
	var none *state.AIReview
	assert.False(t, none.Fresh(state.Reading{}))
}

// The pairing's revisions are omitted while empty, so a record written before
// them serializes, and so is addressed, exactly as it was.
func TestUnitState_RevisionsAreOmittedWhileEmpty(t *testing.T) {
	legacy := approved("h1", "nb", "sha256:tgt")
	legacy.ContentHash = "sha256:src"
	data, err := json.Marshal(legacy)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"basis"`)
	assert.NotContains(t, string(data), `"revision"`)

	withRevs := legacy
	withRevs.Basis, withRevs.Revision = "r:1111111111111111", "r:2222222222222222"
	data, err = json.Marshal(withRevs)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"revision":"r:2222222222222222"`)
	assert.Contains(t, string(data), `"basis":"r:1111111111111111"`)
	var back state.UnitState
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, withRevs, back)
}

// TestSourceHash_IsTheIdentityHash: the basis and the identity signal
// core/reconcile matches on are one number, so a decision recorded on one path
// is comparable with a source read on another. Two compositions here would be
// two answers to "is this the same wording".
func TestSourceHash_IsTheIdentityHash(t *testing.T) {
	assert.Equal(t, model.ComputeContentHash("Refreshed every ten minutes."),
		state.SourceHash("Refreshed every ten minutes."))
	assert.NotEqual(t, state.SourceHash("Refreshed every ten minutes."),
		state.SourceHash("Refreshed every five minutes."))
}

// TestUnitState_GoverningContext: the record answers what governed its answer
// from the field the writer recorded, and a record written before the field
// existed still answers from the producer's stamp on its origin.
func TestUnitState_GoverningContext(t *testing.T) {
	tests := []struct {
		name string
		unit state.UnitState
		want string
	}{
		{
			name: "the recorded fingerprint wins",
			unit: state.UnitState{GoverningFingerprint: "fp-decision", Origin: model.Origin{ContextFingerprint: "fp-produced"}},
			want: "fp-decision",
		},
		{
			name: "a record without one falls back to the producer's stamp",
			unit: state.UnitState{Origin: model.Origin{ContextFingerprint: "fp-produced"}},
			want: "fp-produced",
		},
		{
			name: "nothing recorded reads as ungoverned",
			unit: state.UnitState{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.unit.GoverningContext())
		})
	}
}

// TestUnitState_GoverningFingerprintTravelsBesideContextHash pins the two
// fields as distinct quantities on the wire: block identity and the governing
// context round-trip independently, and an empty fingerprint is omitted so a
// record written before the field existed serializes exactly as before.
func TestUnitState_GoverningFingerprintTravelsBesideContextHash(t *testing.T) {
	u := state.UnitState{Unit: "greeting", Variant: model.Variant("nb"), ContextHash: "ctx-identity", GoverningFingerprint: "fp-governing"}
	data, err := json.Marshal(u)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"contextHash":"ctx-identity"`)
	assert.Contains(t, string(data), `"governingFingerprint":"fp-governing"`)

	var back state.UnitState
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, "ctx-identity", back.ContextHash)
	assert.Equal(t, "fp-governing", back.GoverningFingerprint)

	bare, err := json.Marshal(state.UnitState{Unit: "greeting", Variant: model.Variant("nb"), ContextHash: "ctx-identity"})
	require.NoError(t, err)
	assert.NotContains(t, string(bare), "governingFingerprint")
}
