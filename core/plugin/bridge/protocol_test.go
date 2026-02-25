package bridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeFilterParams_NilMap(t *testing.T) {
	result := encodeFilterParams(nil)
	assert.Nil(t, result)
}

func TestEncodeFilterParams_EmptyMap(t *testing.T) {
	result := encodeFilterParams(map[string]any{})
	assert.Nil(t, result)
}

func TestEncodeFilterParams_StringValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"key": "value",
	})
	assert.Equal(t, "value", result["key"])
}

func TestEncodeFilterParams_BooleanValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"flag": true,
	})
	assert.Equal(t, "true", result["flag"])
}

func TestEncodeFilterParams_IntValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"count": 42,
	})
	assert.Equal(t, "42", result["count"])
}

func TestEncodeFilterParams_ComplexValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"codeFinderRules": map[string]any{
			"rules": []map[string]string{
				{"pattern": "<[^>]+>"},
			},
		},
	})
	assert.Contains(t, result["codeFinderRules"], "rules")
	assert.Contains(t, result["codeFinderRules"], "pattern")
}

func TestInfoData(t *testing.T) {
	info := InfoData{
		Name:        "openxml",
		DisplayName: "Microsoft Office",
		MimeTypes:   []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		Extensions:  []string{".docx", ".xlsx", ".pptx"},
	}
	assert.Equal(t, "openxml", info.Name)
	assert.Contains(t, info.Extensions, ".docx")
	assert.Len(t, info.MimeTypes, 1)
}

func TestListFiltersData(t *testing.T) {
	lf := ListFiltersData{
		Filters: []FilterEntry{
			{FilterClass: "net.sf.okapi.filters.html.HtmlFilter", Name: "html", DisplayName: "HTML"},
		},
	}
	assert.Len(t, lf.Filters, 1)
	assert.Equal(t, "html", lf.Filters[0].Name)
}
