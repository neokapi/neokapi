package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
)

// translateEvery writes "qps " and the source as the qps target of every
// translatable block, the same text on every run.
type translateEvery struct{ tool.BaseTool }

func (s *translateEvery) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			if err := tool.WriteAs(ctx, b, "translate-every", func(v tool.VariantView) error {
				v.SetTargetText("qps", "qps "+v.SourceText())
				return nil
			}); err != nil {
				return err
			}
		}
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// BenchmarkFlowRecord_ManyDocuments measures a run over one document of 300
// strings in a project whose store knows 200 documents. Every translation
// the run produces is one the file already holds, so the record looks up the
// most recent change to each string's translation, by the document's key, to
// tell whether the run's write is recorded already. Naming that key costs one
// lookup of the one document, whatever the project holds; resolving it from
// the project's whole document index, once per string, made the record grow
// with the project.
func BenchmarkFlowRecord_ManyDocuments(b *testing.B) {
	const documents, strs = 200, 300
	dir := b.TempDir()
	b.Setenv("KAPI_CONFIG_DIR", b.TempDir())
	b.Setenv("XDG_DATA_HOME", b.TempDir())
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	b.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	b.Setenv("KAPI_PLUGINS_DIR", b.TempDir())
	b.Setenv("KAPI_NO_PROJECT", "1")
	src := filepath.Join(dir, "src", "en")
	require.NoError(b, os.MkdirAll(src, 0o755))
	for d := range documents {
		n := 3
		if d == 0 {
			n = strs
		}
		var doc strings.Builder
		doc.WriteString("{")
		for i := range n {
			if i > 0 {
				doc.WriteString(", ")
			}
			fmt.Fprintf(&doc, "%q: %q", fmt.Sprintf("s%03d", i), fmt.Sprintf("String %d of document %d", i, d))
		}
		doc.WriteString("}\n")
		require.NoError(b, os.WriteFile(filepath.Join(src, fmt.Sprintf("doc-%03d.json", d)), []byte(doc.String()), 0o644))
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(b, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "FlowRecordBench",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"qps"},
			Flow:            "pseudo",
			TranslateAfter:  string(model.TranslateAfterNone),
		},
		Collections: []project.Collection{{Name: "app", Path: "src/en/*.json", Target: "src/{lang}/*.json"}},
		Flows:       map[string]*flow.StepsSpec{"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}}},
	}))
	b.Chdir(dir)
	a := &App{}
	b.Cleanup(a.Shutdown)
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(b, cmd.Flags().Set("project", recipe))

	// One pass extracts every document into the store and translates it.
	proj, err := project.Load(recipe)
	require.NoError(b, err)
	var out ConvergeOutput
	require.NoError(b, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{MaxPasses: 1, noChecks: true, capture: &out}))

	a.ProjectContext = project.NewProjectContext(proj, recipe)
	home, docs := a.flowDocuments(context.Background(), cmd, dir)
	require.NotNil(b, docs)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: a.FormatReg, SourceLocale: "en", Home: home, Documents: docs})
	run := func() {
		err := runner.RunFile(context.Background(), "edit", []tool.Tool{&translateEvery{ToolName: "translate-every"}},
			filepath.Join(src, "doc-000.json"), filepath.Join(dir, "src", "qps", "doc-000.json"), "qps")
		require.NoError(b, err)
	}
	run() // the first run writes and records its own wording
	for b.Loop() {
		run()
	}
}
