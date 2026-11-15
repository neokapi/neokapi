package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchResultDecodesTranslations(t *testing.T) {
	want := batchTranslations{{ID: "1", Text: "Hei"}, {ID: "2", Text: "Verden"}}
	tests := []struct {
		name    string
		input   string
		want    batchTranslations
		wantErr bool
	}{
		{name: "array", input: `{"translations":[{"id":"1","text":"Hei"},{"id":"2","text":"Verden"}]}`, want: want},
		{name: "empty array", input: `{"translations":[]}`, want: batchTranslations{}},
		{name: "string-wrapped array", input: `{"translations":"[{\"id\":\"1\",\"text\":\"Hei\"},{\"id\":\"2\",\"text\":\"Verden\"}]"}`, want: want},
		{name: "string-wrapped object", input: `{"translations":"{\"translations\":[{\"id\":\"1\",\"text\":\"Hei\"},{\"id\":\"2\",\"text\":\"Verden\"}]}"}`, want: want},
		{name: "non-JSON string", input: `{"translations":"Hei Verden"}`, wantErr: true},
		{name: "string with malformed array", input: `{"translations":"[{\"id\":\"1\""}`, wantErr: true},
		{name: "string-wrapped object without translations", input: `{"translations":"{\"id\":\"1\"}"}`, wantErr: true},
		{name: "number", input: `{"translations":42}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got batchResult
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Translations)
		})
	}
}
