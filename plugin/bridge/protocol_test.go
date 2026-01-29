package bridge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandMarshal(t *testing.T) {
	cmd := Command{
		Command: "open",
		Params: OpenParams{
			FilterClass:   "net.sf.okapi.filters.openxml.OpenXMLFilter",
			URI:            "test.docx",
			SourceLocale:   "en",
			Encoding:       "UTF-8",
			ContentBase64:  "dGVzdA==",
			MimeType:       "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "open", parsed["command"])

	params := parsed["params"].(map[string]interface{})
	assert.Equal(t, "net.sf.okapi.filters.openxml.OpenXMLFilter", params["filter_class"])
	assert.Equal(t, "test.docx", params["uri"])
	assert.Equal(t, "en", params["source_locale"])
	assert.Equal(t, "dGVzdA==", params["content_base64"])
}

func TestCommandMarshalNoParams(t *testing.T) {
	cmd := Command{Command: "shutdown"}
	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "shutdown", parsed["command"])
	_, hasParams := parsed["params"]
	assert.False(t, hasParams)
}

func TestResponseUnmarshalOK(t *testing.T) {
	raw := `{"status":"ok","data":{"ready":true}}`
	var resp Response
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	assert.True(t, resp.IsOK())
	assert.Empty(t, resp.Error)

	var ready ReadyData
	require.NoError(t, json.Unmarshal(resp.Data, &ready))
	assert.True(t, ready.Ready)
}

func TestResponseUnmarshalError(t *testing.T) {
	raw := `{"status":"error","error":"filter not found"}`
	var resp Response
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	assert.False(t, resp.IsOK())
	assert.Equal(t, "filter not found", resp.Error)
}

func TestInfoDataUnmarshal(t *testing.T) {
	raw := `{"name":"openxml","display_name":"Microsoft Office","mime_types":["application/vnd.openxmlformats-officedocument.wordprocessingml.document"],"extensions":[".docx",".xlsx",".pptx"]}`
	var info InfoData
	require.NoError(t, json.Unmarshal([]byte(raw), &info))
	assert.Equal(t, "openxml", info.Name)
	assert.Equal(t, "Microsoft Office", info.DisplayName)
	assert.Contains(t, info.Extensions, ".docx")
	assert.Len(t, info.MimeTypes, 1)
}

func TestWriteDataUnmarshal(t *testing.T) {
	raw := `{"output_base64":"dGVzdCBvdXRwdXQ="}`
	var wd WriteData
	require.NoError(t, json.Unmarshal([]byte(raw), &wd))
	assert.Equal(t, "dGVzdCBvdXRwdXQ=", wd.OutputBase64)
}

func TestListFiltersDataUnmarshal(t *testing.T) {
	raw := `{"filters":[{"filter_class":"net.sf.okapi.filters.html.HtmlFilter","name":"html","display_name":"HTML","mime_types":["text/html"],"extensions":[".html",".htm"]}]}`
	var lf ListFiltersData
	require.NoError(t, json.Unmarshal([]byte(raw), &lf))
	require.Len(t, lf.Filters, 1)
	assert.Equal(t, "html", lf.Filters[0].Name)
	assert.Equal(t, "net.sf.okapi.filters.html.HtmlFilter", lf.Filters[0].FilterClass)
}

func TestOpenParamsRoundTrip(t *testing.T) {
	original := OpenParams{
		FilterClass:   "net.sf.okapi.filters.html.HtmlFilter",
		URI:            "index.html",
		SourceLocale:   "en-US",
		Encoding:       "UTF-8",
		ContentBase64:  "PGh0bWw+PC9odG1sPg==",
		MimeType:       "text/html",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded OpenParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original, decoded)
}

func TestWriteParamsRoundTrip(t *testing.T) {
	original := WriteParams{
		FilterClass:          "net.sf.okapi.filters.html.HtmlFilter",
		Parts:                []map[string]interface{}{{"part_type": 0}},
		Locale:               "fr-FR",
		Encoding:             "UTF-8",
		OriginalContentBase64: "PGh0bWw+PC9odG1sPg==",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "net.sf.okapi.filters.html.HtmlFilter", parsed["filter_class"])
	assert.Equal(t, "fr-FR", parsed["locale"])
	assert.Equal(t, "PGh0bWw+PC9odG1sPg==", parsed["original_content_base64"])
}
