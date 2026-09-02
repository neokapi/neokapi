package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// "Is there content here?" is a shape question, and three host paths answered it
// through the code-dropped flattening, so a block whose only run is an inline
// code read as absent. Each got a different wrong answer out of the same wrong
// question: `kapi up` re-planned and re-translated it every pass, the loop checks
// never checked it, and `kapi inspect` dropped it from its output entirely.
//
// The fixtures below are real .kbf.json bundles, so the placeholders are genuine Ph
// runs and the blocks travel the same reader the CLI uses.

// kbfDoc renders a .kbf.json document whose blocks are given as (id, source runs,
// optional target runs) triples.
func kbfDoc(t *testing.T, path, locale string, blocks []kbfBlock) {
	t.Helper()
	type wireBlock struct {
		ID           string                 `json:"id"`
		Translatable bool                   `json:"translatable"`
		Type         string                 `json:"type"`
		Source       []model.Run            `json:"source"`
		Targets      map[string][]model.Run `json:"targets,omitempty"`
	}
	out := map[string]any{
		"schemaVersion": "1.0",
		"kind":          "kapi-bundle",
		"generator":     map[string]string{"id": "test", "version": "1.0"},
		"project":       map[string]string{"id": "presence", "sourceLocale": "en"},
		"documents": []map[string]any{{
			"id": "app", "documentType": "jsx", "path": "app.tsx",
			"blocks": func() []wireBlock {
				ws := make([]wireBlock, 0, len(blocks))
				for _, b := range blocks {
					wb := wireBlock{ID: b.id, Translatable: true, Type: "jsx:element", Source: b.source}
					if b.target != nil {
						wb.Targets = map[string][]model.Run{locale: b.target}
					}
					ws = append(ws, wb)
				}
				return ws
			}(),
		}},
	}
	data, err := json.MarshalIndent(out, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

type kbfBlock struct {
	id             string
	source, target []model.Run
}

func tx(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func ph(id, equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{
		ID: id, Type: "jsx:element", Data: "{" + equiv + "}", Equiv: equiv,
	}}
}

// presenceFixture writes a source and a fully-translated target .kbf.json holding one
// ordinary block and one placeholder-only block, and returns the project root.
func presenceFixture(t *testing.T) (root string, proj *project.KapiProject) {
	t.Helper()
	root = t.TempDir()
	blocks := []kbfBlock{
		{id: "greeting", source: []model.Run{tx("Hello")}},
		{id: "price", source: []model.Run{ph("1", "p.price")}},
	}
	kbfDoc(t, filepath.Join(root, "src", "en.kbf.json"), "", blocks)

	translated := []kbfBlock{
		{id: "greeting", source: []model.Run{tx("Hello")}, target: []model.Run{tx("Hei")}},
		// The placeholder-only unit IS translated: there is nothing to translate
		// but the placeholder, and it is carried across intact.
		{id: "price", source: []model.Run{ph("1", "p.price")}, target: []model.Run{ph("1", "p.price")}},
	}
	kbfDoc(t, filepath.Join(root, "src", "nb.kbf.json"), "nb", translated)

	proj = &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{Name: "c", Path: "src/en.kbf.json", Target: "src/{lang}.kbf.json"},
		},
	}
	return root, proj
}

func presenceApp() *App {
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	return a
}

// TestUpPlan_PlaceholderOnlyTargetIsAlreadyProduced: `kapi up --plan` counted a
// placeholder-only target as missing, so every convergence pass re-planned and
// re-translated a unit that was already done — a bill for AI work on content that
// needed none, on every run, forever.
func TestUpPlan_PlaceholderOnlyTargetIsAlreadyProduced(t *testing.T) {
	root, proj := presenceFixture(t)
	a := presenceApp()

	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.Len(t, units, 1)

	plan, err := a.computeUpPlan(context.Background(), upPlanBasis{}, proj, units)
	require.NoError(t, err)
	assert.Zero(t, plan.Totals.MissingTarget,
		"both units are produced; the placeholder-only one is not work to re-do")
	assert.Empty(t, plan.Scopes)
}

// TestLoopChecks_PlaceholderOnlyTargetIsCheckable: the guardrails skipped a
// placeholder-only target, so it reached the ship gate having never been checked.
// The symptom is silence — the unit ships, and no finding was ever reported about
// it. Here the placeholder-only target drops its source's placeholder, which the
// rule-based checkset reports as a missing code; before the fix nothing looked.
func TestLoopChecks_PlaceholderOnlyTargetIsCheckable(t *testing.T) {
	root := t.TempDir()
	source := []model.Run{ph("1", "p.price"), ph("2", "p.unit")}
	kbfDoc(t, filepath.Join(root, "src", "en.kbf.json"), "", []kbfBlock{
		{id: "price", source: source},
	})
	kbfDoc(t, filepath.Join(root, "src", "nb.kbf.json"), "nb", []kbfBlock{
		// The target kept one of the source's two placeholders and dropped the
		// other — the #1456 loss, in a unit that is nothing but placeholders.
		{id: "price", source: source, target: []model.Run{ph("1", "p.price")}},
	})
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{Name: "c", Path: "src/en.kbf.json", Target: "src/{lang}.kbf.json"},
		},
	}

	a := presenceApp()
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)

	cmd := NewEnvCommand(context.Background(), "check")
	excl, err := a.computeLoopCheckExclusions(context.Background(), cmd, proj, root, units)
	require.NoError(t, err)
	assert.Equal(t, 1, excl.totalFailing(),
		"a produced placeholder-only target must reach the guardrails; its dropped code is a failure")
}

// TestInspect_PlaceholderOnlyBlockIsEmitted: `kapi inspect` dropped a
// placeholder-only block from its output. Inspect is the tool you reach for to
// see what the reader produced, so a silently absent record is the worst answer
// it can give — the block is there, and inspect says it is not.
func TestInspect_PlaceholderOnlyBlockIsEmitted(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "app.kbf.json")
	kbfDoc(t, src, "", []kbfBlock{
		{id: "greeting", source: []model.Run{tx("Hello")}},
		{id: "price", source: []model.Run{ph("1", "p.price")}},
	})

	a := presenceApp()
	var out bytes.Buffer
	cmd := NewEnvCommand(context.Background(), "inspect")
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, a.RunInspect(context.Background(), cmd, []string{src}, "json", nil))

	var recs []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &recs))
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		if k, ok := r["id"].(string); ok {
			ids = append(ids, k)
		}
	}
	assert.Len(t, recs, 2, "both blocks must be reported: %s", out.String())
	assert.Contains(t, ids, "price", "the placeholder-only block must not vanish from inspect output")
}
