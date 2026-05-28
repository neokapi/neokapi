package check

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotateWritesFindingsAnnotation(t *testing.T) {
	b := &model.Block{ID: "b1", Source: []model.Run{{Text: &model.TextRun{Text: "Leverage our synergy."}}}}
	v := tool.NewBlockView(b)

	findings := []Finding{
		{Category: "terminology", Severity: SeverityMajor, Message: "forbidden term"},
	}
	score := Annotate(v, "acme-checkset", findings)

	assert.Equal(t, 100-5, score.Overall)

	ann, ok := v.Annotations()[AnnotationKey]
	require.True(t, ok, "annotation written under %q", AnnotationKey)
	fa, ok := ann.(*FindingsAnnotation)
	require.True(t, ok)
	assert.Equal(t, "acme-checkset", fa.Source)
	assert.Equal(t, score.Overall, fa.Score)
	assert.Len(t, fa.Findings, 1)
	assert.Equal(t, AnnotationKey, fa.AnnotationType())
}
