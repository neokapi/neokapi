package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One locale, one answer.
//
// `kapi up` closes with a per-locale table, `kapi status` prints a per-scope
// grid, and `kapi status --ship` emits the manifest a language picker reads.
// Three surfaces, and a user has no way to tell which to believe when they
// disagree — so they are held here to the same numbers and the same verdict,
// back to back over one tree, with no command between them (#2024).
//
// The fixture is samples/compass driven through the journey its README
// documents, because that is the input the disagreement was found on and the
// one a reader follows.

// compassCopy copies the committed sample into a throwaway directory and
// returns the recipe path and root, under the dogfood isolation contract
// (CLAUDE.md): every root this run could otherwise inherit is pinned to a
// throwaway dir and project discovery is off.
func compassCopy(t *testing.T) (recipe, root string) {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "samples", "compass"))
	require.NoError(t, err)

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	root = filepath.Join(real, "compass")
	require.NoError(t, os.CopyFS(root, os.DirFS(src)))

	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	return filepath.Join(root, "kapi.yaml"), root
}

// reviewQueue is the shape `kapi status --review --json` emits, narrowed to the
// fields the journey's change-set is built from.
type reviewQueue struct {
	Pending []struct {
		Locale string `json:"locale"`
		File   string `json:"file"`
		Key    string `json:"key"`
		Target string `json:"target"`
	} `json:"pending"`
}

// approveLocale performs one of the journey's review round-trips: read the
// review queue, turn the units this reviewer accepts into a change-set, apply
// it, and commit. `accept` is the reviewer's judgement — the README's Dutch
// step declines what the offline stub drafted, the Norwegian step takes
// everything.
func approveLocale(t *testing.T, a *App, recipe, root, locale string, accept func(target string) bool) int {
	t.Helper()

	statusOut, err := runCLI(t, NewStatusCmd(a), "--project", recipe, "--review", "--json")
	require.NoError(t, err, statusOut)
	var queue reviewQueue
	require.NoError(t, json.Unmarshal([]byte(statusOut), &queue), "review queue must be JSON: %s", statusOut)

	var lines []string
	for _, item := range queue.Pending {
		if item.Locale != locale || !accept(item.Target) {
			continue
		}
		line, merr := json.Marshal(map[string]string{
			"kind": "review", "op": "add", "file": item.File,
			"id": item.Key, "locale": item.Locale, "status": "reviewed",
		})
		require.NoError(t, merr)
		lines = append(lines, string(line))
	}
	require.NotEmpty(t, lines, "the journey's %s review has units to accept", locale)

	changeset := filepath.Join(root, locale+"-review.jsonl")
	require.NoError(t, os.WriteFile(changeset, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	applyOut, err := runCLI(t, NewApplyCmd(a), changeset, "--project", recipe)
	require.NoError(t, err, applyOut)
	assert.NotContains(t, applyOut, "error", applyOut)

	commitOut, err := runCLI(t, NewCommitCmd(a), "--project", recipe)
	require.NoError(t, err, commitOut)
	return len(lines)
}

// surfaces is what the three commands say about one locale, read back to back.
type surfaces struct {
	upTranslated        int
	upShippable         bool
	upVerified          bool
	upFailingChecks     int
	statusTranslated    int
	statusShippable     bool
	statusVerified      bool
	statusFailingChecks int
	shipShippable       bool
	shipVerified        bool
	statusText          string
}

// readSurfaces runs `kapi up`, `kapi status` and `kapi status --ship` over the
// same tree, in that order, with nothing in between, and reads every locale's
// answer out of each. `up` runs exactly once: it is the verb that changes the
// tree, so a second call would be measuring a different project.
func readSurfaces(t *testing.T, a *App, recipe, root string) map[string]surfaces {
	t.Helper()
	out := map[string]surfaces{}

	upJSON, err := runCLI(t, NewUpCmd(a), "--project", recipe, "--json")
	require.NoError(t, err, upJSON)
	// --json is the run's NDJSON event stream; the closing `result` event is the
	// structured form of the table the text output prints.
	var up struct {
		Locales []struct {
			Locale        string         `json:"locale"`
			Shippable     bool           `json:"shippable"`
			Verified      bool           `json:"verified"`
			Pct           map[string]int `json:"pct"`
			FailingChecks int            `json:"failingChecks"`
		} `json:"locales"`
	}
	result := ""
	for line := range strings.SplitSeq(strings.TrimSpace(upJSON), "\n") {
		if strings.Contains(line, `"type":"result"`) {
			result = line
		}
	}
	require.NotEmpty(t, result, "up --json must close with a result event: %s", upJSON)
	require.NoError(t, json.Unmarshal([]byte(result), &up), "up result must be JSON: %s", result)
	require.NotEmpty(t, up.Locales, "up must report the project's locales: %s", result)
	for _, lc := range up.Locales {
		out[lc.Locale] = surfaces{
			upTranslated: lc.Pct["translated"], upShippable: lc.Shippable,
			upVerified: lc.Verified, upFailingChecks: lc.FailingChecks,
			// A locale is no stronger than its weakest collection scope, which is
			// the fold BuildShipManifest applies; the grid's rows are per scope.
			statusTranslated: 100, statusShippable: true, statusVerified: true,
		}
	}

	statusJSON, err := runCLI(t, NewStatusCmd(a), "--project", recipe, "--json")
	require.NoError(t, err, statusJSON)
	var status StatusOutput
	require.NoError(t, json.Unmarshal([]byte(statusJSON), &status), "status must emit JSON: %s", statusJSON)
	seen := map[string]bool{}
	for _, lc := range status.Locales {
		s, ok := out[lc.Locale]
		require.True(t, ok, "status reports %s, which up did not: %s", lc.Locale, statusJSON)
		seen[lc.Locale] = true
		if lc.Pct["translated"] < s.statusTranslated {
			s.statusTranslated = lc.Pct["translated"]
		}
		s.statusShippable = s.statusShippable && lc.Shippable
		s.statusVerified = s.statusVerified && lc.Verified
		s.statusFailingChecks += lc.FailingChecks
		out[lc.Locale] = s
	}
	for locale := range out {
		require.True(t, seen[locale], "status must report %s: %s", locale, statusJSON)
	}

	shipPath := filepath.Join(root, "ship-read.json")
	shipOut, err := runCLI(t, NewStatusCmd(a), "--project", recipe, "--ship", "--emit", shipPath)
	require.NoError(t, err, shipOut)
	shipBytes, rerr := os.ReadFile(shipPath)
	require.NoError(t, rerr)
	var ship map[string]struct {
		Shippable bool `json:"shippable"`
		Verified  bool `json:"verified"`
	}
	require.NoError(t, json.Unmarshal(shipBytes, &ship), "ship manifest must be JSON: %s", shipBytes)

	statusText, err := runCLI(t, NewStatusCmd(a), "--project", recipe)
	require.NoError(t, err, statusText)

	for locale, s := range out {
		entry, ok := ship[locale]
		require.True(t, ok, "ship manifest must carry %s: %s", locale, shipBytes)
		s.shipShippable, s.shipVerified = entry.Shippable, entry.Verified
		s.statusText = statusText
		out[locale] = s
	}
	return out
}

// assertAgree is the property the three surfaces are held to.
func assertAgree(t *testing.T, s surfaces, locale string) {
	t.Helper()
	assert.Equal(t, s.statusTranslated, s.upTranslated,
		"%s: up and status must publish one `translated` figure\nstatus:\n%s", locale, s.statusText)
	assert.Equal(t, s.shipShippable, s.upShippable,
		"%s: up's state column and the ship manifest must be the same predicate\nstatus:\n%s",
		locale, s.statusText)
	assert.Equal(t, s.shipShippable, s.statusShippable,
		"%s: status's ship column and the ship manifest must be the same predicate\nstatus:\n%s",
		locale, s.statusText)
	assert.Equal(t, s.shipVerified, s.upVerified, "%s: one answer about verified", locale)
	assert.Equal(t, s.shipVerified, s.statusVerified, "%s: one answer about verified", locale)
	assert.Equal(t, s.statusFailingChecks, s.upFailingChecks,
		"%s: one count of the units failing the project's checks\nstatus:\n%s", locale, s.statusText)
}

// breakPlaceholder drops one placeholder from a locale's committed catalog: a
// defect on a unit that is translated and reviewed, which is the shape the
// three surfaces disagreed on.
func breakPlaceholder(t *testing.T, root, locale string) {
	t.Helper()
	path := filepath.Join(root, "site", "locales", locale+".json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	// A targeted text edit, not a re-encode: every other unit's bytes stay
	// exactly as the loop wrote them, so the one unit under test is the only
	// thing that changed.
	require.Contains(t, string(raw), " {until}", "sample shape: %s carries the {until} placeholder", path)
	edited := strings.Replace(string(raw), " {until}", "", 1)
	require.NoError(t, os.WriteFile(path, []byte(edited), 0o644))
}

// TestCompass_ThreeSurfacesGiveOneAnswer drives the journey samples/compass
// documents and holds `kapi up`, `kapi status` and `kapi status --ship` to one
// answer about EVERY locale — first with the content clean, then with a real
// finding on a unit every one of whose 38 strings carries a committed review
// decision.
//
// The second state is the one that used to publish three answers: up called the
// locale `parked (needs human)` at `translated 95%`, status called it
// `translated 100% / ready`, and the manifest the deployed page reads offered it
// as governed.
//
// Every locale, not just the one carrying the defect: a run that grades one tree
// and leaves another behind mis-reports whichever locale it declined to deliver,
// which is a different locale from the one a test author is looking at.
func TestCompass_ThreeSurfacesGiveOneAnswer(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := compassCopy(t)

	// Converge. Approved wording is recycled first; the remainder is drafted by
	// the offline `demo` provider, which brackets its output in ⟦…⟧.
	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	// The journey's two review round-trips: Dutch accepts what a person wrote
	// and declines the stub's drafts; Norwegian is finished outright.
	approveLocale(t, a, recipe, root, "nl", func(target string) bool {
		return !strings.HasPrefix(target, "⟦")
	})
	approveLocale(t, a, recipe, root, "nb", func(string) bool { return true })

	clean := readSurfaces(t, a, recipe, root)
	for locale, s := range clean {
		assertAgree(t, s, locale)
	}
	nb := clean["nb"]
	assert.True(t, nb.shipShippable, "the journey ends with nb offered:\n%s", nb.statusText)
	assert.True(t, nb.shipVerified, "and with its AI marker off:\n%s", nb.statusText)
	assert.Zero(t, nb.upFailingChecks, "nothing fails the guardrails yet:\n%s", nb.statusText)

	// A defect on content every one of whose 38 units carries a committed review
	// decision. Whatever the loop then does with it — this pass re-drafts the
	// unit, which withdraws its approval — the surfaces must describe the
	// outcome identically.
	breakPlaceholder(t, root, "nb")

	broken := readSurfaces(t, a, recipe, root)
	for locale, s := range broken {
		assertAgree(t, s, locale)
	}
	nb = broken["nb"]
	assert.False(t, nb.shipShippable,
		"the defect withholds the locale — and the manifest a deployed picker reads says so, "+
			"rather than offering what the run declined to publish:\n%s", nb.statusText)
	assert.False(t, nb.upShippable,
		"which is the same predicate up's state column reports:\n%s", nb.statusText)
	assert.Positive(t, nb.upFailingChecks,
		"and up names the finding, on the tree it leaves behind:\n%s", nb.statusText)
	// The reported figure, on the input it was reported on: every unit here has
	// a committed target, so `translated` is 100% on every surface. It read 95%
	// on `up` alone, because that surface — and only that surface — subtracted
	// the units carrying a finding from a lifecycle percentage.
	assert.Equal(t, 100, nb.upTranslated,
		"a unit carrying a finding is still translated:\n%s", nb.statusText)
	assert.Equal(t, 100, nb.statusTranslated, nb.statusText)
}
