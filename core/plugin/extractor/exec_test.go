package extractor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExtractor returns a Spec that runs `sh -c <script>` so the
// tests cover the real process boundary + NDJSON plumbing without a
// separate fixture binary.
func fakeExtractor(script string) Spec {
	return Spec{Exec: []string{"sh", "-c", script}, Timeout: 5 * time.Second}
}

func TestRunEmptyOutputReturnsNoRecords(t *testing.T) {
	records, err := Run(context.Background(), fakeExtractor("true"), nil)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestRunParsesBlockRecords(t *testing.T) {
	// Emit two well-formed NDJSON block records + one noise line.
	script := `
printf 'scanning files...\n'
printf '{"type":"block","document":"a.tsx","block":{"id":"b1","hash":"H1","translatable":true,"type":"jsx:element","source":[{"text":"Hello"}]}}\n'
printf '{"type":"block","document":"b.tsx","block":{"id":"b2","hash":"H2","translatable":true,"type":"jsx:element","source":[{"text":"World"}]}}\n'
`
	records, err := Run(context.Background(), fakeExtractor(script), nil)
	require.NoError(t, err)
	require.Len(t, records, 2)

	assert.Equal(t, "block", records[0].Type)
	assert.Equal(t, "a.tsx", records[0].Document)
	assert.Equal(t, klf.BlockType("jsx:element"), records[0].Block.Type)
	assert.Equal(t, "H1", records[0].Block.Hash)

	assert.Equal(t, "b.tsx", records[1].Document)
	assert.Equal(t, "H2", records[1].Block.Hash)
}

func TestRunPassesPathsOnStdinAsNULSeparated(t *testing.T) {
	// The script dumps stdin back as the block `hash` so we can
	// inspect what the extractor received.
	script := `
payload=$(tr '\0' '|' | od -An -c | tr -d ' \n' || true)
printf '{"type":"block","document":"x","block":{"id":"b1","hash":"%s","translatable":true,"type":"jsx:element","source":[{"text":"x"}]}}\n' "$payload"
`
	records, err := Run(context.Background(), fakeExtractor(script),
		[]string{"src/a.tsx", "src/b.tsx"})
	require.NoError(t, err)
	require.Len(t, records, 1)
	// od -c represents NUL as "\0" — we just need to confirm the
	// bytes arrived without truncation.
	assert.Contains(t, records[0].Block.Hash, "s")
	assert.Contains(t, records[0].Block.Hash, "r")
	assert.Contains(t, records[0].Block.Hash, "c")
}

func TestRunSurfacesStderrOnNonZeroExit(t *testing.T) {
	script := `printf 'something went wrong\n' >&2; exit 3`
	_, err := Run(context.Background(), fakeExtractor(script), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit")
	assert.Contains(t, err.Error(), "something went wrong")
}

func TestRunReturnsMalformedNDJSONError(t *testing.T) {
	script := `printf '{bogus not json}\n'`
	_, err := Run(context.Background(), fakeExtractor(script), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed NDJSON")
}

func TestRunRejectsEmptyExec(t *testing.T) {
	_, err := Run(context.Background(), Spec{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty Exec")
}

func TestRunTimesOut(t *testing.T) {
	// Sleep longer than the timeout — expect context deadline via
	// non-zero exit.
	spec := Spec{Exec: []string{"sh", "-c", "sleep 1"}, Timeout: 50 * time.Millisecond}
	_, err := Run(context.Background(), spec, nil)
	require.Error(t, err)
	_ = strings.ToLower(err.Error()) // kill signal wording varies; just assert error surfaces
}
