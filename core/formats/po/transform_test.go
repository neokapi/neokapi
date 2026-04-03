package po_test

import (
	"testing"

	_ "github.com/neokapi/neokapi/core/formats/po"

	"github.com/neokapi/neokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiPOTransform_Registration(t *testing.T) {
	t.Parallel()
	assert.True(t, config.DefaultTransforms.Has(
		config.OkapiFilterConfigKind("po"), config.FormatConfigKind("po")))
}

func TestOkapiPOTransform_DropsOkapiOnlyParams(t *testing.T) {
	t.Parallel()
	from := config.OkapiFilterConfigKind("po")
	to := config.FormatConfigKind("po")
	spec := map[string]any{
		"useCodeFinder":        true,
		"codeFinderRules":      []string{`%[sdf]`},
		"bilingualMode":        true,
		"protectApproved":      true,
		"preserveUntranslated": true,
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	// Okapi-only params should be dropped.
	assert.Nil(t, result["useCodeFinder"])
	assert.Nil(t, result["codeFinderRules"])
	assert.Nil(t, result["bilingualMode"])
	assert.Nil(t, result["protectApproved"])

	// Shared params should be kept.
	assert.Equal(t, true, result["preserveUntranslated"])
}

func TestOkapiPOTransform_EmptySpec(t *testing.T) {
	t.Parallel()
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("po"), config.FormatConfigKind("po"), map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}
