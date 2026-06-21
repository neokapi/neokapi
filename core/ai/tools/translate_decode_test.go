package tools

import "testing"

// TestDecodeFirstJSON covers the padding patterns local models (e.g. Gemma)
// produce around their structured output — the cause of the
// "invalid character 'n' after top-level value" batch-translate failure.
func TestDecodeFirstJSON(t *testing.T) {
	const want = "hi"
	ok := []struct {
		name string
		in   string
	}{
		{"clean", `{"translations":[{"index":1,"text":"hi"}]}`},
		{"trailing text", `{"translations":[{"index":1,"text":"hi"}]}` + "\nnull"},
		{"trailing prose", `{"translations":[{"index":1,"text":"hi"}]} Note: done.`},
		{"code fence", "```json\n{\"translations\":[{\"index\":1,\"text\":\"hi\"}]}\n```"},
		{"bare fence", "```\n{\"translations\":[{\"index\":1,\"text\":\"hi\"}]}\n```"},
		{"leading prose", "Here is the translation:\n{\"translations\":[{\"index\":1,\"text\":\"hi\"}]}"},
		{"whitespace", "  \n\t" + `{"translations":[{"index":1,"text":"hi"}]}` + "\n"},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			var r batchResult
			if err := decodeFirstJSON(tc.in, &r); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(r.Translations) != 1 || r.Translations[0].Text != want {
				t.Fatalf("got %+v", r.Translations)
			}
		})
	}

	var r batchResult
	if err := decodeFirstJSON("the model refused and produced no JSON", &r); err == nil {
		t.Fatal("expected an error when there is no JSON value")
	}
}
