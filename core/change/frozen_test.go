package change_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change/changeschema"
)

// The result and the read are pinned as the request is: a change to either is
// a change to what a caller reads, and shows up in review as a change to its
// golden file. go test ./core/change -run TestReplySchemaGoldens -update
// rewrites them.
func TestReplySchemaGoldens(t *testing.T) {
	for name, got := range map[string][]byte{
		"result.golden.json": changeschema.ResultSchema(),
		"read.golden.json":   changeschema.ReadSchema(),
	} {
		path := filepath.Join("testdata", name)
		if *update {
			require.NoError(t, os.WriteFile(path, append(got, '\n'), 0o644))
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err, "run go test ./core/change -run TestReplySchemaGoldens -update to create it")
		assert.Equal(t, string(bytes.TrimSpace(want)), string(got), name)
	}
}

// frozenAdvice is what a failing extend-only test says to do.
const frozenAdvice = "A frozen contract may only be extended: add an optional property, an operation, an enum value " +
	"or a $defs entry, or change a description. A breaking change needs a new schema version (kapi.change/v2) " +
	"and a new frozen file beside this one."

// kapi.change/v1 is frozen. The schema the Go types generate today must
// extend the one frozen at the freeze: every value a sender could write then
// still decodes to the same meaning. -update never writes the frozen file,
// and no target regenerates it.
func TestSchemaExtendsFrozenV1(t *testing.T) {
	assertExtendsFrozen(t, "schema.v1.frozen.json", changeschema.Schema(), changeschema.Writer)
}

// kapi.change-result/v1 and the read record are frozen from the reader's
// side: no property a caller reads is removed, retyped or made optional.
func TestResultSchemaExtendsFrozenV1(t *testing.T) {
	assertExtendsFrozen(t, "result.v1.frozen.json", changeschema.ResultSchema(), changeschema.Reader)
}

func TestReadSchemaExtendsFrozenV1(t *testing.T) {
	assertExtendsFrozen(t, "read.v1.frozen.json", changeschema.ReadSchema(), changeschema.Reader)
}

func assertExtendsFrozen(t *testing.T, name string, current []byte, side changeschema.Side) {
	t.Helper()
	frozen, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var head struct {
		Comment string `json:"$comment"`
	}
	require.NoError(t, json.Unmarshal(frozen, &head))
	assert.Contains(t, head.Comment, "frozen at", "the frozen file names the commit it was frozen at")
	problems, err := changeschema.Extends(frozen, current, side)
	require.NoError(t, err)
	assert.Empty(t, problems, "the schema no longer extends testdata/%s:\n  %s\n%s", name, strings.Join(problems, "\n  "), frozenAdvice)
}
