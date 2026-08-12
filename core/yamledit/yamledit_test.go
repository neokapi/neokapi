package yamledit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/yamledit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// yamlUnmarshal loads a document the way the governance loaders do: non-strict,
// so a key the type does not model is dropped rather than refused.
func yamlUnmarshal(data []byte, into any) error { return yaml.Unmarshal(data, into) }

// profile is a governance-file shape: nested mappings, a list of rules, and
// enough fields to reorder.
type profile struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Tone        tone     `yaml:"tone,omitempty"`
	Vocabulary  vocab    `yaml:"vocabulary,omitempty"`
	Examples    []string `yaml:"examples,omitempty"`
}

type tone struct {
	Personality []string `yaml:"personality,omitempty"`
	Formality   string   `yaml:"formality,omitempty"`
}

type vocab struct {
	Forbidden []rule `yaml:"forbidden_terms,omitempty"`
}

type rule struct {
	Term        string `yaml:"term"`
	Replacement string `yaml:"replacement,omitempty"`
}

const commented = `# The voice the harbour docs are written in.
# Authored by hand: every line is a decision someone made.
name: North Sea
description: |
  Plain, exact writing
  for harbour operations.
tone:
  # Restrained, because a berthing instruction is read under pressure.
  personality: [clear, restrained]
  formality: neutral
vocabulary:
  # Marketing register: never in operational prose.
  forbidden_terms:
    - term: seamless
      replacement: uninterrupted
`

func loaded(t *testing.T, doc string) *profile {
	t.Helper()
	var p profile
	require.NoError(t, yamlUnmarshal([]byte(doc), &p))
	return &p
}

func TestMarshal_UnchangedValueIsByteIdentical(t *testing.T) {
	t.Parallel()
	out, err := yamledit.Marshal([]byte(commented), loaded(t, commented))
	require.NoError(t, err)
	assert.Equal(t, commented, string(out),
		"a value that says the same thing as the document must re-render as the same bytes")
}

func TestMarshal_KeepsCommentsAcrossARealChange(t *testing.T) {
	t.Parallel()
	p := loaded(t, commented)
	p.Vocabulary.Forbidden = append(p.Vocabulary.Forbidden, rule{Term: "mooring", Replacement: "berth"})

	out, err := yamledit.Marshal([]byte(commented), p)
	require.NoError(t, err)
	got := string(out)

	for _, comment := range []string{
		"# The voice the harbour docs are written in.",
		"# Authored by hand: every line is a decision someone made.",
		"# Restrained, because a berthing instruction is read under pressure.",
		"# Marketing register: never in operational prose.",
	} {
		assert.Contains(t, got, comment, "an applied rule must not delete the file's explanation of itself")
	}
	assert.Contains(t, got, "term: mooring", "and the change must land")
	assert.Contains(t, got, "description: |", "a block scalar stays a block scalar")
	assert.Contains(t, got, "personality: [clear, restrained]", "a flow sequence stays inline")
}

func TestMarshal_PreservesAuthoredKeyOrder(t *testing.T) {
	t.Parallel()
	// The document opens with vocabulary and closes with name — the reverse of
	// the struct's field order.
	authored := `vocabulary:
  forbidden_terms:
    - term: seamless
description: Plain writing.
name: North Sea
`
	p := loaded(t, authored)
	p.Description = "Plain, exact writing."

	out, err := yamledit.Marshal([]byte(authored), p)
	require.NoError(t, err)
	assert.Equal(t, `vocabulary:
  forbidden_terms:
    - term: seamless
description: Plain, exact writing.
name: North Sea
`, string(out))
}

func TestMarshal_AddsAndRemovesKeys(t *testing.T) {
	t.Parallel()
	authored := `name: North Sea
description: Plain writing.
`
	p := loaded(t, authored)
	p.Description = ""
	p.Examples = []string{"Berths are allocated on arrival."}

	out, err := yamledit.Marshal([]byte(authored), p)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "description:", "a key the value dropped is removed")
	assert.Contains(t, string(out), "examples:", "and a key it gained is appended")
}

func TestMarshal_NoDocumentIsAPlainMarshal(t *testing.T) {
	t.Parallel()
	out, err := yamledit.Marshal(nil, &profile{Name: "North Sea"})
	require.NoError(t, err)
	assert.Equal(t, "name: North Sea\n", string(out))
}

func TestMarshal_UnparsableDocumentIsReplaced(t *testing.T) {
	t.Parallel()
	out, err := yamledit.Marshal([]byte("name: [unclosed\n"), &profile{Name: "North Sea"})
	require.NoError(t, err)
	assert.Equal(t, "name: North Sea\n", string(out),
		"a document that cannot be parsed carries nothing to preserve")
}

func TestMarshal_KeepsTheDocumentsIndentation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		authored string
	}{
		{name: "two spaces", authored: "name: North Sea\ntone:\n  formality: neutral\n"},
		{name: "four spaces", authored: "name: North Sea\ntone:\n    formality: neutral\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := yamledit.Marshal([]byte(tt.authored), loaded(t, tt.authored))
			require.NoError(t, err)
			assert.Equal(t, tt.authored, string(out))
		})
	}
}

func TestWriteFile_NoOpLeavesTheFileAlone(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "voice.yaml")
	require.NoError(t, os.WriteFile(path, []byte(commented), 0o644))
	before, err := os.Stat(path)
	require.NoError(t, err)

	changed, err := yamledit.WriteFile(path, loaded(t, commented), 0o644)
	require.NoError(t, err)
	assert.False(t, changed, "an unchanged serialization must not be written")

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime(), "the file was not touched at all")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, commented, string(got))
}

func TestWriteFile_WritesARealChange(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "voice.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(commented), 0o644))

	p := loaded(t, commented)
	p.Vocabulary.Forbidden[0].Replacement = "continuous"
	changed, err := yamledit.WriteFile(path, p, 0o644)
	require.NoError(t, err)
	assert.True(t, changed)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "replacement: continuous")
	assert.Contains(t, string(got), "# Marketing register: never in operational prose.")
}

func TestWriteFile_CreatesAMissingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "new", "voice.yaml")
	changed, err := yamledit.WriteFile(path, &profile{Name: "North Sea"}, 0o644)
	require.NoError(t, err)
	assert.True(t, changed)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "name: North Sea\n", string(got))
	assert.NoFileExists(t, path+".tmp", "the atomic write leaves no temp file behind")
}
