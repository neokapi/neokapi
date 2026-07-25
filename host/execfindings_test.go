package host

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// A check tool's result lives in a stand-off annotation no writer serializes, so
// before this an exec run of one printed nothing at all: `kapi exec qa` and
// `kapi exec term-check` were advertised in the exec help and silent.

// checkedBlockPart runs dnt-check over one bilingual block and returns the
// resulting part, so the collector is fed exactly what a real run feeds it.
func checkedBlockPart(t *testing.T, id, source, target string, terms []string) *model.Part {
	t.Helper()
	b := &model.Block{ID: id, Translatable: true,
		Source: []model.Run{{Text: &model.TextRun{Text: source}}}}
	reg := registry.NewToolRegistry()
	coretools.RegisterAll(reg)
	cfg := map[string]any{"terms": terms}
	tl, err := reg.NewToolWithConfig("dnt-check", cfg, "nb")
	require.NoError(t, err)
	bv := tl.(interface {
		Process(context.Context, <-chan *model.Part, chan<- *model.Part) error
	})

	// Give the block its target, then stream it through the tool.
	blockTargetText(b, "nb", target)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- bv.Process(context.Background(), in, out)
	}()
	got := <-out
	require.NoError(t, <-errc)
	require.NotNil(t, got)
	return got
}

func blockTargetText(b *model.Block, locale, text string) {
	b.SetTargetRuns(model.LocaleID(locale), []model.Run{{Text: &model.TextRun{Text: text}}})
}

// Which tools get a collector is derived from the IO contract, not a list: a tool
// that declares a findings port reports, everything else is untouched.
func TestProducesFindingsFollowsTheIOContract(t *testing.T) {
	reg := registry.NewToolRegistry()
	coretools.RegisterAll(reg)

	for _, name := range []string{"dnt-check", "placeholder-check", "qa", "term-check", "xml-validation"} {
		assert.True(t, ProducesFindings(reg.Schema(registry.ToolID(name))),
			"%q produces check findings, so an exec run must report them", name)
		assert.NotNil(t, NewFindingsCollectorFor(reg.Schema(registry.ToolID(name))),
			"%q must get a findings collector", name)
	}
	for _, name := range []string{"pseudo-translate", "case-transform", "search-replace", "create-target"} {
		assert.False(t, ProducesFindings(reg.Schema(registry.ToolID(name))),
			"%q writes content, not findings", name)
		assert.Nil(t, NewFindingsCollectorFor(reg.Schema(registry.ToolID(name))))
	}
	assert.Nil(t, NewFindingsCollectorFor(nil), "a tool with no schema gets no collector")
	assert.False(t, ProducesFindings(&schema.ComponentSchema{}))
}

// The collector turns the blocks' annotations into a located, ordered report.
func TestFindingsCollectorReportsWhatTheCheckFound(t *testing.T) {
	c := &findingsCollector{}
	part := checkedBlockPart(t, "save", "Save now with Acme Cloud", "Lagre nå med Toppskyen", []string{"Acme Cloud"})
	item := &flow.Item{Input: &model.RawDocument{URI: "src/app.xlf"}}
	require.NoError(t, c.Collect(context.Background(), item, []*model.Part{part}))

	res, err := c.Result()
	require.NoError(t, err)
	report, ok := res.Data.(execFindingsReport)
	require.True(t, ok)

	require.Len(t, report.Findings, 1)
	d := report.Findings[0]
	assert.Equal(t, check.SeverityCritical, d.Severity)
	assert.Equal(t, "dnt-check.do-not-translate", d.Rule, "the rule id names the tool that was run")
	assert.Equal(t, "src/app.xlf", d.Location.File)
	assert.Equal(t, "save", d.Location.Block)
	assert.Equal(t, 1, report.Summary.Critical)
	assert.Equal(t, 1, report.Target.Blocks)

	var buf bytes.Buffer
	report.FormatTable(&buf)
	out := buf.String()
	assert.Contains(t, out, "CRITICAL")
	assert.Contains(t, out, "Acme Cloud")
	assert.Contains(t, out, "1 finding(s) (1 critical")
}

// A clean run says so, rather than printing nothing — "no output" and "no
// findings" must not look the same.
func TestFindingsCollectorSaysSoWhenThereAreNone(t *testing.T) {
	c := &findingsCollector{}
	part := checkedBlockPart(t, "save", "Save now with Acme Cloud", "Lagre nå med Acme Cloud", []string{"Acme Cloud"})
	require.NoError(t, c.Collect(context.Background(), &flow.Item{
		Input: &model.RawDocument{URI: "src/app.xlf"},
	}, []*model.Part{part}))

	res, err := c.Result()
	require.NoError(t, err)
	report := res.Data.(execFindingsReport)
	assert.Empty(t, report.Findings)

	var buf bytes.Buffer
	report.FormatTable(&buf)
	assert.Contains(t, buf.String(), "No findings.")
}

// exec holds the content to no bar, so the report carries no verdict: a
// `"pass": true` beside a critical finding is the reassurance this class of
// defect hands out. The gate and the exit code belong to `kapi check`.
func TestFindingsReportCarriesNoVerdict(t *testing.T) {
	c := &findingsCollector{}
	part := checkedBlockPart(t, "save", "Save now with Acme Cloud", "Lagre nå med Toppskyen", []string{"Acme Cloud"})
	require.NoError(t, c.Collect(context.Background(), &flow.Item{
		Input: &model.RawDocument{URI: "src/app.xlf"},
	}, []*model.Part{part}))
	res, err := c.Result()
	require.NoError(t, err)

	raw, err := json.Marshal(res.Data)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.NotContains(t, decoded, "pass")
	assert.NotContains(t, decoded, "gate")
	// The fields a consumer of `kapi check --output-format json` already reads.
	assert.Contains(t, decoded, "findings")
	assert.Contains(t, decoded, "summary")
}
