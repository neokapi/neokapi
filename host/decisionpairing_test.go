package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// A decision pairs the revision of the translation it blesses with the
// revision of the source it blessed it for. A revision counts inline codes,
// so an edit that moves a link and leaves every word where it was retires the
// decision, which a pairing of text hashes cannot see. A decision recorded
// before revisions is read by its hashes, as it always was.

// pairingProject is a markdown project whose source and French translation
// each hold one paragraph with a link to href.
func pairingProject(t *testing.T, srcHref, frHref string) (*App, string) {
	t.Helper()
	a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.md", Target: "i18n/{lang}/*.md"}, map[string]string{
		"docs/guide.md":    "Read the [guide](" + srcHref + ") first.\n",
		"i18n/fr/guide.md": "Lisez d'abord le [guide](" + frHref + ").\n",
	}, func(p *project.KapiProject) { p.Defaults.TargetLanguages = []model.LocaleID{"fr"} })
	t.Cleanup(a.Shutdown)
	return a, recipe
}

// pairingKey is the key of the one paragraph block in the pairing project.
func pairingKey(t *testing.T, a *App, recipe string) string {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, Origin: "test"})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "docs/guide.md"})
	require.NoError(t, err)
	for _, b := range page.Blocks {
		if strings.Contains(b.Text, "Read the") {
			return b.Ref.Block
		}
	}
	require.FailNow(t, "no paragraph in docs/guide.md")
	return ""
}

// frCoverage is the French coverage of the project: how many units read
// stale, and the percentage at least established.
func frCoverage(t *testing.T, a *App, recipe string) (stale, established int) {
	t.Helper()
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	root := filepath.Dir(recipe)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	tally, err := a.ProjectCoverageTally(context.Background(), proj, root, units, nil)
	require.NoError(t, err)
	for _, lc := range tally.Rollup(gate.RuleSet{}) {
		if lc.Locale == "fr" {
			stale += lc.Stale
			established = lc.Pct[string(model.TargetStatusEstablished)]
		}
	}
	return stale, established
}

func writePairingFile(t *testing.T, recipe, rel, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), filepath.FromSlash(rel)), []byte(body), 0o644))
}

// An approval made through the change service records both revisions beside
// both hashes, and coverage reads it by the revisions: a link that moves under
// it in the source retires it though no word moved, and the source returning
// to the link it approved restores it.
func TestDecisionPairing_AnApprovalIsReadByItsRevisions(t *testing.T) {
	a, recipe := pairingProject(t, "https://a.example/guide", "https://a.example/guide")
	ctx := context.Background()
	key := pairingKey(t, a, recipe)

	changed, err := decideUnit(ctx, a, recipe, ReviewUnitRef{File: "i18n/fr/guide.md", Key: key, Locale: "fr"}, ReviewDecisionApproved, "")
	require.NoError(t, err)
	require.True(t, changed)

	root := filepath.Dir(recipe)
	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	u, ok := st.Get(ctx, state.Key{Scope: a.DocumentScope(ctx, root, filepath.Join(root, "docs", "guide.md")), Unit: key, Variant: model.Variant("fr")})
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(u.Basis, "r:"), "the basis is a revision: %q", u.Basis)
	assert.True(t, strings.HasPrefix(u.Revision, "r:"), "the translation is named by revision: %q", u.Revision)
	assert.NotEmpty(t, u.ContentHash, "the hashes ride beside the revisions")
	assert.NotEmpty(t, u.TargetHash)

	stale, established := frCoverage(t, a, recipe)
	assert.Zero(t, stale)
	assert.Equal(t, 100, established, "the approval stands")

	writePairingFile(t, recipe, "docs/guide.md", "Read the [guide](https://b.example/guide) first.\n")
	stale, established = frCoverage(t, a, recipe)
	assert.Equal(t, 1, stale, "the link moved under the approval")
	assert.Zero(t, established)

	writePairingFile(t, recipe, "docs/guide.md", "Read the [guide](https://a.example/guide) first.\n")
	stale, established = frCoverage(t, a, recipe)
	assert.Zero(t, stale, "the source is back at the basis the approval names")
	assert.Equal(t, 100, established)

	writePairingFile(t, recipe, "i18n/fr/guide.md", "Lisez d'abord le [guide](https://c.example/guide).\n")
	_, established = frCoverage(t, a, recipe)
	assert.Zero(t, established, "a link moved in the translation retires the approval too")
}

// A decision recorded before revisions carries the two hashes alone. It keeps
// answering by them: a link change moves no hash, so it stands, and a change
// of wording retires it.
func TestDecisionPairing_ADecisionRecordedBeforeRevisionsIsReadByItsHashes(t *testing.T) {
	a, recipe := pairingProject(t, "https://a.example/guide", "https://a.example/guide")
	ctx := context.Background()
	key := pairingKey(t, a, recipe)
	root := filepath.Dir(recipe)
	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)

	blocks := pairingBlocks(t, a, recipe)
	require.Len(t, blocks, 1)
	b := blocks[0]
	legacy := state.UnitState{
		Scope: a.DocumentScope(ctx, root, filepath.Join(root, "docs", "guide.md")), Unit: key, Variant: model.Variant("fr"),
		Status:      model.TargetStatusEstablished,
		TargetHash:  state.TargetHash(b.TargetText("fr")),
		ContentHash: state.SourceHash(b.SourceText()),
		Decision:    state.Decision{ReviewState: "approved", At: "2026-09-01T00:00:00Z"},
		Updated:     "2026-09-01T00:00:00Z",
	}
	require.NoError(t, st.Put(ctx, legacy))

	stale, established := frCoverage(t, a, recipe)
	assert.Zero(t, stale)
	assert.Equal(t, 100, established, "the decision answers by its hashes")

	writePairingFile(t, recipe, "docs/guide.md", "Read the [guide](https://b.example/guide) first.\n")
	stale, established = frCoverage(t, a, recipe)
	assert.Zero(t, stale, "a link change moves no hash")
	assert.Equal(t, 100, established)

	writePairingFile(t, recipe, "docs/guide.md", "Read the whole [guide](https://b.example/guide) first.\n")
	stale, _ = frCoverage(t, a, recipe)
	assert.Equal(t, 1, stale, "a change of wording retires it")
}

// pairingBlocks is the pairing project's French unit as coverage reads it:
// the source with the translation overlaid.
func pairingBlocks(t *testing.T, a *App, recipe string) []*model.Block {
	t.Helper()
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, filepath.Dir(recipe), "fr")
	require.NoError(t, err)
	require.Len(t, units, 1)
	blocks, missing, err := a.bilingualBlocks(context.Background(), units[0])
	require.NoError(t, err)
	require.False(t, missing)
	var out []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

// A source approval binds to the source's revision, so a link moved in the
// source withdraws it, as a change of wording does.
func TestDecisionPairing_ASourceApprovalIsReadByItsRevision(t *testing.T) {
	a, recipe := pairingProject(t, "https://a.example/guide", "https://a.example/guide")
	ctx := context.Background()
	key := pairingKey(t, a, recipe)
	root := filepath.Dir(recipe)

	changed, err := approveSource(ctx, a, recipe, SourceUnitRef{File: "docs/guide.md", Key: key})
	require.NoError(t, err)
	require.True(t, changed)

	approvals, err := a.loadSourceApprovals(ctx, root, "en")
	require.NoError(t, err)
	scope := a.DocumentScope(ctx, root, filepath.Join(root, "docs", "guide.md"))
	src := func() *model.Block {
		blocks, err := a.readBlocksAs(ctx, filepath.Join(root, "docs", "guide.md"), "", nil, "en")
		require.NoError(t, err)
		for _, b := range blocks {
			if blockKey(b) == key {
				return b
			}
		}
		require.FailNow(t, "no paragraph")
		return nil
	}
	assert.True(t, approvals.approves(scope, key, src()))

	writePairingFile(t, recipe, "docs/guide.md", "Read the [guide](https://b.example/guide) first.\n")
	assert.False(t, approvals.approves(scope, key, src()), "the link moved under the approval")
}

// A translation a flow wrote is graded by the basis its write recorded, by
// revision: a link moved in the source reads it stale, so the loop drafts it
// again.
func TestDecisionPairing_TheLoopsBasisIsReadByRevision(t *testing.T) {
	a, recipe := pairingProject(t, "https://a.example/guide", "https://a.example/guide")
	recordFlowWrite(t, a, recipe, "docs/guide.md", "fr", model.Origin{Kind: model.OriginAI})

	stale, _ := frCoverage(t, a, recipe)
	require.Zero(t, stale, "the flow made the translation from the source in front of it")

	writePairingFile(t, recipe, "docs/guide.md", "Read the [guide](https://b.example/guide) first.\n")
	stale, _ = frCoverage(t, a, recipe)
	assert.Equal(t, 1, stale, "the link moved under the loop's translation")
}

// A reader that declares the language of the file it reads keeps it where the
// change service reads the file, and a project read files the source under the
// project's language. A decision taken through the one is graded through the
// other by the same content: the reader's language rides on the block a
// project read makes (model.PropReadSourceLocale), and the decision stands.
func TestDecisionPairing_ADecisionStandsWhereTheReaderDeclaresAnotherLanguage(t *testing.T) {
	a, recipe := changeProject(t, project.ContentItem{Path: "l10n/app_en.arb", Target: "l10n/app_{lang}.arb"}, map[string]string{
		"l10n/app_en.arb": `{"@@locale": "en", "greeting": "Read the guide"}` + "\n",
		"l10n/app_fr.arb": `{"@@locale": "fr", "greeting": "Lisez le guide"}` + "\n",
	}, func(p *project.KapiProject) {
		p.Defaults.SourceLanguage = "en-US"
		p.Defaults.TargetLanguages = []model.LocaleID{"fr"}
	})
	t.Cleanup(a.Shutdown)
	ctx := context.Background()

	changed, err := decideUnit(ctx, a, recipe, ReviewUnitRef{File: "l10n/app_fr.arb", Key: "greeting", Locale: "fr"}, ReviewDecisionApproved, "")
	require.NoError(t, err)
	require.True(t, changed)

	root := filepath.Dir(recipe)
	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	u, ok := st.Get(ctx, state.Key{Scope: a.DocumentScope(ctx, root, filepath.Join(root, "l10n", "app_en.arb")), Unit: "greeting", Variant: model.Variant("fr")})
	require.True(t, ok)
	require.NotEmpty(t, u.Basis)

	stale, established := frCoverage(t, a, recipe)
	assert.Zero(t, stale, "the decision was made on the source the project holds")
	assert.Equal(t, 100, established)

	writePairingFile(t, recipe, "l10n/app_en.arb", `{"@@locale": "en", "greeting": "Read the whole guide"}`+"\n")
	stale, _ = frCoverage(t, a, recipe)
	assert.Equal(t, 1, stale, "and a source edit still retires it")
}

// A project read keeps the language a reader declared for a block it files
// under the project's, and leaves a block whose reader declared none, or the
// project's own, as it was.
func TestFileUnderSource(t *testing.T) {
	declared := model.NewBlock("a", "Hello")
	declared.SourceLocale = "en"
	none := model.NewBlock("b", "Hello")
	same := model.NewBlock("c", "Hello")
	same.SourceLocale = "en_US"
	fileUnderSource([]*model.Block{declared, none, same}, "en-US")

	for _, b := range []*model.Block{declared, none, same} {
		assert.Equal(t, model.LocaleID("en-US"), b.SourceLocale)
	}
	assert.Equal(t, "en", declared.Properties[model.PropReadSourceLocale])
	assert.NotContains(t, none.Properties, model.PropReadSourceLocale)
	assert.NotContains(t, same.Properties, model.PropReadSourceLocale)
}
