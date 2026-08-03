package messageformat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/reconcile"
)

// A MessageFormat file carries no keys — it is one pattern per line — so unlike
// PO or properties there is no natural key to name a block by, and the line
// ordinal stays as the scope. What these tests pin down is what the scope is
// there for: within one file no two blocks may share a name, and the pieces of
// one message are named by the structure of the message rather than by the
// order they happened to be emitted in.

func identityNames(t *testing.T, input string) []string {
	t.Helper()
	blocks := readBlocks(t, input)
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	return names
}

// Two messages that pick on the same argument produce the same branch paths.
// Before the line scope they produced the same NAMES too, so a decision
// recorded about one branch landed on the other's.
func TestIdentity_SameBranchPathOnTwoLinesDiffers(t *testing.T) {
	names := identityNames(t, "{count, plural, one {# file} other {# files}}\n{count, plural, one {# folder} other {# folders}}\n")
	require.Len(t, names, 4)
	assert.Equal(t, []string{
		"line.1/count.one", "line.1/count.other",
		"line.2/count.one", "line.2/count.other",
	}, names)
	assert.Len(t, unique(names), 4)
}

// Identical text in two different branches of one message is still two blocks
// and must keep two names.
func TestIdentity_SameTextDifferentBranches(t *testing.T) {
	names := identityNames(t, "{count, plural, one {items} other {items}}\n")
	require.Len(t, names, 2)
	assert.Len(t, unique(names), 2)
}

func TestIdentity_StableAcrossTwoReads(t *testing.T) {
	const src = "Hello world\n{count, plural, one {# item} other {# items}}\n"
	assert.Equal(t, identityNames(t, src), identityNames(t, src))
}

// A name must never derive from the block's own text: reconcile grades the name
// as CONTEXT and the text as CONTENT, so a name that moves with the text moves
// both at once and an edited block grades New, losing its history. A branch
// name here is the argument name plus the plural/select keyword — the message's
// structure, never its words — so editing a branch keeps the block.
func TestIdentity_EditedBranchTextKeepsIdentity(t *testing.T) {
	const scope = "messages.mf"
	var prior []reconcile.Unit
	for _, b := range readBlocks(t, "{count, plural, one {# file} other {# files}}\n") {
		u := reconcile.Identify(scope, b)
		u.Key = b.Name
		prior = append(prior, u)
	}

	grades := make(map[string]reconcile.Kind)
	for _, res := range reconcile.Blocks(scope, readBlocks(t, "{count, plural, one {# document} other {# files}}\n"), prior) {
		grades[res.Block.Name] = res.Kind
	}

	assert.Equal(t, reconcile.Edited, grades["line.1/count.one"],
		"the branch path held still while the words changed")
	assert.Equal(t, reconcile.Unchanged, grades["line.1/count.other"])
}

// The gap, recorded rather than papered over: a MessageFormat file has no
// author-controlled key, so a block's name is scoped by its line and deleting
// an earlier line renames every message below it. This is the same positional
// gap the prose formats carry (core/formats/identity_conformance_test.go);
// closing it needs a key the format does not have. Naming a block after its own
// text instead would be worse: content and context would then move together, so
// an edited message would read as a brand-new one and lose its history.
func TestIdentity_LineScopeIsStillPositional(t *testing.T) {
	before := identityNames(t, "Alpha\nBravo\nCharlie\n")
	after := identityNames(t, "Alpha\nCharlie\n")

	require.Equal(t, []string{"line.1", "line.2", "line.3"}, before)
	assert.Equal(t, []string{"line.1", "line.2"}, after,
		"known gap: Charlie is renamed by a deletion above it")
}

func unique(names []string) map[string]bool {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		seen[n] = true
	}
	return seen
}
