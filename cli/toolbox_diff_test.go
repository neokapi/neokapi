package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- alignment heuristics -----------------------------------------------------

func TestTrailingInt(t *testing.T) {
	cases := []struct {
		in string
		n  int
		ok bool
	}{
		{"tu1", 1, true},
		{"tu12", 12, true},
		{"d3", 3, true},
		{"123", 123, true},
		{"greeting", 0, false},
		{"a", 0, false},
		{"items[0]", 0, false}, // trailing ']' is not a digit
		{"menu.open", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		n, ok := trailingInt(c.in)
		assert.Equal(t, c.ok, ok, "ok for %q", c.in)
		if c.ok {
			assert.Equal(t, c.n, n, "n for %q", c.in)
		}
	}
}

func TestPositionalAndKeyedSide(t *testing.T) {
	positional := []diffBlock{{ID: "tu1"}, {ID: "tu2"}, {ID: "tu3"}}
	assert.True(t, positionalSide(positional), "tu1..tu3 encode order")
	assert.False(t, keyedSide(positional), "positional IDs are not semantic keys")

	keys := []diffBlock{{ID: "greeting"}, {ID: "farewell"}, {ID: "menu.open"}}
	assert.False(t, positionalSide(keys))
	assert.True(t, keyedSide(keys), "semantic key paths are keyed")

	// Short single-letter keys (a JSON catalog) are keyed, not positional.
	letters := []diffBlock{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	assert.True(t, keyedSide(letters))

	// Non-increasing positional numbers are not "positional" (so a side that
	// happens to use numeric-suffixed keys out of order stays keyed).
	shuffled := []diffBlock{{ID: "x3"}, {ID: "x1"}, {ID: "x2"}}
	assert.False(t, positionalSide(shuffled))

	// Duplicate or empty IDs are never keyed.
	assert.False(t, keyedSide([]diffBlock{{ID: "a"}, {ID: "a"}}))
	assert.False(t, keyedSide([]diffBlock{{ID: ""}, {ID: "b"}}))
	assert.False(t, keyedSide(nil))
}

// --- LCS ----------------------------------------------------------------------

func TestLCSPairs(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "x", "c", "d"}
	pairs, ok := lcsPairs(a, b)
	require.True(t, ok)
	// LCS is a, c, d → pairs (0,0),(2,2),(3,3)
	assert.Equal(t, [][2]int{{0, 0}, {2, 2}, {3, 3}}, pairs)

	// Empty inputs.
	pairs, ok = lcsPairs(nil, []string{"x"})
	require.True(t, ok)
	assert.Empty(t, pairs)

	// Identical.
	pairs, ok = lcsPairs([]string{"a", "b"}, []string{"a", "b"})
	require.True(t, ok)
	assert.Equal(t, [][2]int{{0, 0}, {1, 1}}, pairs)
}

// --- diffByID -----------------------------------------------------------------

func TestDiffByID(t *testing.T) {
	a := []diffBlock{{ID: "g", Text: "Hello"}, {ID: "f", Text: "Goodbye"}, {ID: "k", Text: "Keep"}}
	b := []diffBlock{{ID: "g", Text: "Hi"}, {ID: "k", Text: "Keep"}, {ID: "w", Text: "Welcome"}}
	ops := diffByID(a, b)

	got := map[string]diffKind{}
	for _, op := range ops {
		got[op.ID] = op.Kind
	}
	assert.Equal(t, diffChange, got["g"], "g text changed")
	assert.Equal(t, diffEqual, got["k"], "k unchanged")
	assert.Equal(t, diffAdd, got["w"], "w added")
	assert.Equal(t, diffRemove, got["f"], "f removed")
}

func TestDiffByIDMoveDetection(t *testing.T) {
	a := []diffBlock{{ID: "a", Text: "One"}, {ID: "b", Text: "Two"}, {ID: "c", Text: "Three"}}
	b := []diffBlock{{ID: "c", Text: "Three"}, {ID: "a", Text: "One"}, {ID: "b", Text: "Two"}}
	ops := diffByID(a, b)

	var moved []string
	for _, op := range ops {
		if op.Kind == diffMove {
			moved = append(moved, op.ID)
		}
		assert.NotEqual(t, diffChange, op.Kind, "pure reorder is never a change (%s)", op.ID)
	}
	// Moving "c" to the front (or equivalently a,b after it) is the minimal move.
	assert.Contains(t, moved, "c")
}

// --- diffByContent ------------------------------------------------------------

func TestDiffByContentInsertion(t *testing.T) {
	a := []diffBlock{{Text: "intro"}, {Text: "body"}, {Text: "end"}}
	b := []diffBlock{{Text: "intro"}, {Text: "NEW"}, {Text: "body"}, {Text: "end"}}
	ops, capped := diffByContent(a, b)
	require.False(t, capped)

	var adds, removes, changes int
	for _, op := range ops {
		switch op.Kind {
		case diffAdd:
			adds++
			assert.Equal(t, "NEW", op.BText)
		case diffRemove:
			removes++
		case diffChange:
			changes++
		}
	}
	assert.Equal(t, 1, adds, "exactly one inserted block")
	assert.Zero(t, removes)
	assert.Zero(t, changes, "an insertion is not a cascade of changes")
}

func TestDiffByContentChangeCoalesced(t *testing.T) {
	a := []diffBlock{{Text: "keep"}, {Text: "old line"}, {Text: "tail"}}
	b := []diffBlock{{Text: "keep"}, {Text: "new line"}, {Text: "tail"}}
	ops, _ := diffByContent(a, b)

	var change *diffOp
	for i := range ops {
		if ops[i].Kind == diffChange {
			change = &ops[i]
		}
	}
	require.NotNil(t, change, "a 1:1 replacement coalesces into a change")
	assert.Equal(t, "old line", change.AText)
	assert.Equal(t, "new line", change.BText)
}

func TestDiffByContentEmptyBlocksDropped(t *testing.T) {
	a := []diffBlock{{ID: "x", Text: ""}, {Text: "real"}}
	b := []diffBlock{{Text: "real"}}
	ops, _ := diffByContent(a, b)
	for _, op := range ops {
		assert.NotEqual(t, diffRemove, op.Kind, "empty-text blocks carry no prose and are dropped")
	}
}

func TestCoalesceChanges(t *testing.T) {
	in := []diffOp{
		{Kind: diffEqual, AText: "h"},
		{Kind: diffRemove, ANum: 2, AText: "a"},
		{Kind: diffRemove, ANum: 3, AText: "b"},
		{Kind: diffAdd, BNum: 2, BText: "A"},
		{Kind: diffEqual, AText: "t"},
	}
	out := coalesceChanges(in)
	// First remove pairs with the add → change; second remove stays a remove.
	var changes, removes int
	for _, op := range out {
		switch op.Kind {
		case diffChange:
			changes++
			assert.Equal(t, "a", op.AText)
			assert.Equal(t, "A", op.BText)
		case diffRemove:
			removes++
			assert.Equal(t, "b", op.AText)
		}
	}
	assert.Equal(t, 1, changes)
	assert.Equal(t, 1, removes)
}

func TestSummarize(t *testing.T) {
	ops := []diffOp{
		{Kind: diffChange}, {Kind: diffChange}, {Kind: diffAdd},
		{Kind: diffRemove}, {Kind: diffMove}, {Kind: diffEqual},
	}
	s := summarize(ops)
	assert.Equal(t, 2, s.changed)
	assert.Equal(t, 1, s.added)
	assert.Equal(t, 1, s.removed)
	assert.Equal(t, 1, s.moved)
	assert.Equal(t, 1, s.unchanged)
	assert.Contains(t, s.line(), "2 changed")
	assert.Contains(t, s.line(), "1 moved")
}

// --- integration over the real reader pipeline --------------------------------

func TestRunDiffRevisionJSON(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"greeting":"Hello","farewell":"Goodbye","note":"Keep"}`)
	b := writeToolboxFile(t, dir, "b.json", `{"greeting":"Hi there","note":"Keep","welcome":"Welcome"}`)

	out, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a, b}, diffOptions{by: "auto"})
	})
	require.ErrorIs(t, err, ErrSilentExit, "differ → exit 1")
	assert.Contains(t, out, `"greeting" (changed)`)
	assert.Contains(t, out, "- Hello")
	assert.Contains(t, out, "+ Hi there")
	assert.Contains(t, out, `"welcome" (added)`)
	assert.Contains(t, out, `"farewell" (removed)`)
}

func TestRunDiffIdenticalIsSilentAndExitZero(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"k":"same"}`)
	b := writeToolboxFile(t, dir, "b.json", `{"k":"same"}`)

	out, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a, b}, diffOptions{by: "auto"})
	})
	require.NoError(t, err, "identical inputs exit 0")
	assert.Empty(t, out, "identical inputs print nothing")
}

func TestRunDiffBrief(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"k":"one"}`)
	b := writeToolboxFile(t, dir, "b.json", `{"k":"two"}`)

	out, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a, b}, diffOptions{by: "auto", brief: true})
	})
	require.ErrorIs(t, err, ErrSilentExit)
	assert.Contains(t, out, "differ")
	assert.NotContains(t, out, "@@", "brief mode prints no hunks")
}

func TestRunDiffJSONOutput(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"a":"1","b":"2"}`)
	b := writeToolboxFile(t, dir, "b.json", `{"a":"1","b":"two"}`)

	out, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a, b}, diffOptions{by: "auto", json: true})
	})
	require.ErrorIs(t, err, ErrSilentExit)

	var got revisionJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "revision", got.Mode)
	assert.Equal(t, "id", got.Alignment, "a JSON catalog aligns by key")
	assert.True(t, got.Differ)
	assert.Equal(t, 1, got.Summary.Changed)
	require.Len(t, got.Changes, 1)
	assert.Equal(t, "b", got.Changes[0].ID)
	assert.Equal(t, "2", got.Changes[0].Source)
	assert.Equal(t, "two", got.Changes[0].Target)
}

func TestRunDiffContentAlignmentProse(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "p1.md", "The quick fox.\n\nIt jumped.\n\nThe end.\n")
	b := writeToolboxFile(t, dir, "p2.md", "The quick fox.\n\nA new line.\n\nIt jumped.\n\nThe end.\n")

	out, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a, b}, diffOptions{by: "auto"})
	})
	require.ErrorIs(t, err, ErrSilentExit)
	// A single inserted paragraph is one "added" block, not a cascade.
	assert.Contains(t, out, "+ A new line.")
	assert.NotContains(t, out, "(changed)", "an insertion must not read as changes")
}

func TestRunDiffCoverage(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	xliff := writeToolboxFile(t, dir, "cov.xliff", `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file source-language="en" target-language="fr" datatype="plaintext" original="app">
<body>
<trans-unit id="welcome"><source>Welcome</source><target>Bienvenue</target></trans-unit>
<trans-unit id="logout"><source>Log out</source><target>Log out</target></trans-unit>
<trans-unit id="settings"><source>Settings</source><target></target></trans-unit>
</body></file></xliff>`)

	t.Run("text", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runDiff(context.Background(), []string{xliff}, diffOptions{targetLoc: "fr"})
		})
		require.ErrorIs(t, err, ErrSilentExit, "pending translation work → exit 1")
		assert.Contains(t, out, `"settings" (untranslated)`)
		assert.Contains(t, out, `"logout" (identical)`)
		assert.NotContains(t, out, "welcome", "a translated block is not reported")
	})

	t.Run("json", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runDiff(context.Background(), []string{xliff}, diffOptions{targetLoc: "fr", json: true})
		})
		require.ErrorIs(t, err, ErrSilentExit)
		var got coverageJSON
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "coverage", got.Mode)
		assert.Equal(t, 1, got.Translated)
		assert.Equal(t, 1, got.Untranslated)
		assert.Equal(t, 1, got.Identical)
	})
}

func TestRunDiffSingleFileWithoutTargetErrors(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"k":"v"}`)

	_, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a}, diffOptions{})
	})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrSilentExit, "a usage error is not a silent diff status")
	assert.Equal(t, ExitUsage, ExitCode(nil, mapToolboxErr(err)))
}

func TestRunDiffMissingFileIsTrouble(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"k":"v"}`)
	missing := filepath.Join(dir, "nope.json")

	_, err := captureStdout(t, func() error {
		return app.runDiff(context.Background(), []string{a, missing}, diffOptions{by: "auto"})
	})
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, mapToolboxErr(err)), "an unreadable input maps to exit 2")
}

// TestDiffProxyDetached verifies the hidden `kapi diff` proxy is flag-detached
// and behaves like kdiff (so -q is brief, not kapi's --quiet global).
func TestDiffProxyDetached(t *testing.T) {
	app := newToolboxApp(t)
	c := findCmd(app.NewToolboxProxies(), "diff")
	require.NotNil(t, c)
	assert.True(t, c.Hidden)
	assert.True(t, c.DisableFlagParsing)

	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"k":"one"}`)
	b := writeToolboxFile(t, dir, "b.json", `{"k":"two"}`)
	out, err := captureStdout(t, func() error {
		c.SetArgs([]string{"-q", a, b})
		return c.Execute()
	})
	require.ErrorIs(t, err, ErrSilentExit, "-q is brief (diff status), not kapi's --quiet")
	assert.Contains(t, out, "differ")
}

func TestBusyboxRootKdiff(t *testing.T) {
	app := newToolboxApp(t)
	for _, name := range []string{"kdiff", "/usr/local/bin/kdiff", "kdiff.exe"} {
		require.NotNil(t, BusyboxRoot(app, name), "prog %q should map to a toolbox root", name)
	}
}
