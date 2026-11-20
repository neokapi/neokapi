package host

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/require"
)

type outputObservation struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func observeOutput(t *testing.T, id, status, detail string) {
	t.Helper()
	data, err := json.Marshal(outputObservation{ID: id, Status: status, Detail: detail})
	require.NoError(t, err)
	t.Logf("KAPI_OUTPUT %s", data)
}

// TestOutputMaturity enumerates the built-in kconv targets. The audit consumes
// each completed subtest's observation from go test -json. A projection can
// return success even when the target reader recovers none of the source text.
func TestOutputMaturity(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	properties := writeToolboxFile(t, dir, "strings.properties", "subject=Project update\nbody=The draft is ready.\n")
	apple := writeToolboxFile(t, dir, "strings.strings", "\"subject\" = \"Project update\";\n\"body\" = \"The draft is ready.\";\n")
	csv := writeToolboxFile(t, dir, "source.csv", "subject,body\nProject update,The draft is ready.\n")
	tsv := writeToolboxFile(t, dir, "source.tsv", "subject\tbody\nProject update\tThe draft is ready.\n")
	srt := writeToolboxFile(t, dir, "source.srt", "1\n00:00:01,000 --> 00:00:02,000\nProject update\n\n2\n00:00:03,000 --> 00:00:04,000\nThe draft is ready.\n")
	vtt := writeToolboxFile(t, dir, "source.vtt", "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nProject update\n\n00:00:03.000 --> 00:00:04.000\nThe draft is ready.\n")

	check := func(t *testing.T, target registry.FormatID, info registry.FormatInfo) {
		t.Helper()
		source := properties
		capability := "project-text"
		switch target {
		case "properties":
			source = apple
		case "csv":
			source, capability = tsv, "project-cells"
		case "tsv":
			source, capability = csv, "project-cells"
		case "srt":
			source, capability = vtt, "project-cues"
		case "vtt":
			source, capability = srt, "project-cues"
		}
		out := filepath.Join(t.TempDir(), "result")
		if !info.HasWriter {
			_, err := app.ResolveTargetFormat(string(target), "")
			require.Error(t, err)
			observeOutput(t, capability, "unsupported", "The registry provides no writer for this target.")
			return
		}
		resolved, err := app.ResolveTargetFormat(string(target), "")
		require.NoError(t, err)
		require.Equal(t, target, resolved)

		err = app.convertDocument(t.Context(), source, resolved, "", out)
		switch {
		case info.Interchange:
			require.ErrorContains(t, err, "kapi extract")
			observeOutput(t, capability, "unsupported", "kconv directs interchange output to kapi extract.")
		case !info.Generative:
			require.ErrorContains(t, err, "cannot generate")
			observeOutput(t, capability, "unsupported", "The writer requires an original document of this format.")
		case target == "audio" || target == "video":
			require.ErrorContains(t, err, "no media part")
			observeOutput(t, capability, "unsupported", "The writer requires a media part for output.")
		case err != nil:
			t.Log(err)
			observeOutput(t, capability, "failed", "kconv returned an error while projecting the fixture.")
		default:
			checkProjectedText(t, info.Name, capability, source, out)
		}
	}
	for _, info := range app.FormatReg.FormatInfos() {
		t.Run(string(info.Name), func(t *testing.T) { check(t, info.Name, info) })
	}
	// These aliases select the catalog writers. The canonical IDs select
	// structural record writers in kconv.
	for _, target := range []registry.FormatID{"json-catalog", "yaml-catalog"} {
		t.Run(string(target), func(t *testing.T) {
			canonical, ok := app.FormatReg.AliasTarget(target)
			require.True(t, ok)
			check(t, target, *app.FormatReg.FormatInfo(canonical))
		})
	}
}

func checkProjectedText(t *testing.T, formatID registry.FormatID, capability, source, out string) {
	t.Helper()
	reader := newToolboxApp(t)
	reader.FormatFlag = string(formatID)
	var texts []string
	var timings []int64
	_, err := reader.StreamBlocks(t.Context(), out, func(_ int, b *model.Block) error {
		texts = append(texts, b.SourceText())
		if timing, ok := b.Timing(); ok {
			timings = append(timings, timing.StartMS, timing.EndMS)
		}
		return nil
	})
	if err != nil {
		t.Log(err)
		observeOutput(t, capability, "failed", "The target reader returned an error on the generated file.")
		return
	}
	for _, want := range []string{"Project update", "The draft is ready."} {
		if !slices.Contains(texts, want) {
			observeOutput(t, capability, "failed", fmt.Sprintf("The target reader did not recover %q as a separate block from %s.", want, filepath.Base(source)))
			return
		}
	}
	if capability == "project-cues" && !slices.Equal(timings, []int64{1000, 2000, 3000, 4000}) {
		observeOutput(t, capability, "failed", "The target reader did not recover the source cue times.")
		return
	}
	observeOutput(t, capability, "verified", fmt.Sprintf("The target reader recovered both text values from %s.", filepath.Base(source)))
}
