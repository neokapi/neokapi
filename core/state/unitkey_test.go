package state_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

func block(id, name, text string) *model.Block {
	return &model.Block{
		ID:     id,
		Name:   name,
		Source: []model.Run{{Text: &model.TextRun{Text: text}}},
	}
}

// A decision is about TEXT, so its key must follow the text — not the position
// the text happened to occupy, and not the name it happened to be filed under.
//
// The key used to be the block key (Name when set, else the positional ID).
// Measured on the JSON reader before this changed:
//
//	v1: tu1=Alpha  tu2=Bravo  tu3=Charlie
//	v2: tu1=Alpha  tu2=Charlie          ← tu2 now means different content
//
// An approval recorded against tu2 therefore survived into a world where tu2
// was something else. TargetHash caught the misapplication, so nothing was
// wrongly shipped — but a block removed and re-added rejoined its approval only
// if it landed back at the same position.
func TestUnitKey_FollowsContentNotPosition(t *testing.T) {
	// The v1/v2/v3 walk, with the ids the JSON reader actually assigns.
	v1Bravo := block("tu2", "", "Bravo")     // v1: Bravo is the second unit
	v2Charlie := block("tu2", "", "Charlie") // v2: Bravo removed, Charlie inherits tu2
	v3Bravo := block("tu3", "", "Bravo")     // v3: Bravo returns, at a different position

	require.NotEqual(t, state.UnitKey(v1Bravo), state.UnitKey(v2Charlie),
		"a positional id reused by different content must not inherit its decisions")

	assert.Equal(t, state.UnitKey(v1Bravo), state.UnitKey(v3Bravo),
		"a block removed and re-added elsewhere keeps its identity, so its approval rejoins")
}

// Renaming a key without touching the words is not a new decision — nobody
// re-decided anything, so the approval should survive.
func TestUnitKey_SurvivesARename(t *testing.T) {
	before := block("tu1", "buttons.save", "Save changes")
	after := block("tu1", "actions.save", "Save changes")

	assert.Equal(t, state.UnitKey(before), state.UnitKey(after),
		"the words did not change, so the decision about them still stands")
}

// Editing the words IS a new decision. The key changes, and TargetHash would
// have caught it even if it had not — two independent guards, deliberately.
func TestUnitKey_ChangesWhenTheWordsDo(t *testing.T) {
	before := block("tu1", "buttons.save", "Save changes")
	after := block("tu1", "buttons.save", "Save all changes")

	assert.NotEqual(t, state.UnitKey(before), state.UnitKey(after))
}

// The deliberate trade: identical source text is one unit, so approving it once
// approves it everywhere. Identical text in the same variant should not carry
// two different translations; where it genuinely must, the difference belongs
// in the VariantKey (tone/channel), not in a positional accident of the file.
func TestUnitKey_IdenticalTextIsOneUnit(t *testing.T) {
	inMenu := block("tu1", "menu.save", "Save")
	onButton := block("tu9", "button.save", "Save")

	assert.Equal(t, state.UnitKey(inMenu), state.UnitKey(onButton),
		"same words, same decision — consistency is the point")
}

func TestUnitKey_NilIsEmptyNotAPanic(t *testing.T) {
	assert.Empty(t, state.UnitKey(nil), "an ingest path must not panic on a nil block")
}
