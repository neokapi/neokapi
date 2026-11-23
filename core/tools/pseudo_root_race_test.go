package tools_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// One pseudo-translate tool serves every file of a run, and the runner sets
// the project root for each file as it starts it (core/flow bindProjectRoot),
// so files running at once set the root and process parts concurrently. Run
// under -race, this fails when the root is a plain field.
func TestPseudoTranslateTool_FilesRunningAtOnceShareTheTool(t *testing.T) {
	t.Parallel()
	tl := tools.NewPseudoTranslateTool(&tools.PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"})

	const files = 8
	errs := make([]error, files)
	var wg sync.WaitGroup
	for i := range files {
		wg.Go(func() {
			tl.SetProjectRoot(t.TempDir())
			in := make(chan *model.Part, 1)
			out := make(chan *model.Part, 1)
			in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b", "Hello")}
			close(in)
			errs[i] = tl.Process(t.Context(), in, out)
		})
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err)
	}
}
