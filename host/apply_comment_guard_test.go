package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment/golang"
	fmtpkg "github.com/neokapi/neokapi/core/format"
)

// commentSHA256 locates the comment id in src and returns the hex SHA-256 of
// its bytes and the lines it spans, computed here and not by the code under
// test.
func commentSHA256(t *testing.T, src, id string) (string, fmtpkg.LineRange) {
	t.Helper()
	located, err := golang.Provider{}.Locate("parse.go", []byte(src))
	require.NoError(t, err)
	for i, b := range located.Blocks() {
		if b.ID == id {
			c := located.Comments[i]
			sum := sha256.Sum256([]byte(src[c.Start:c.End]))
			return hex.EncodeToString(sum[:]), c.Lines
		}
	}
	t.Fatalf("no comment %s", id)
	return "", fmtpkg.LineRange{}
}

// findingFingerprints reads the comment_sha256 of each finding from a report's
// JSON, keyed by the finding's block.
func findingFingerprints(t *testing.T, report check.Report) map[string]string {
	t.Helper()
	body, err := json.Marshal(report)
	require.NoError(t, err)
	var doc struct {
		Findings []struct {
			Location map[string]any `json:"location"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))
	out := map[string]string{}
	for _, f := range doc.Findings {
		block, _ := f.Location["block"].(string)
		sum, _ := f.Location["comment_sha256"].(string)
		out[block] = sum
	}
	return out
}

func guardedEntry(file, id, text string, guard map[string]any) map[string]any {
	e := map[string]any{"kind": "comment", "file": file, "id": id, "text": text}
	maps.Copy(e, guard)
	return e
}
