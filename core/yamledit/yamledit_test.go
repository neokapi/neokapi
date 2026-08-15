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

// profile is a governance-file shape: nested mappings, a list of rules, a key
// that accepts two spellings, and enough fields to reorder.
type profile struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Extends     *inherit `yaml:"extends,omitempty"`
	Tone        tone     `yaml:"tone,omitempty"`
	Vocabulary  vocab    `yaml:"vocabulary,omitempty"`
	Examples    []string `yaml:"examples,omitempty"`
}

// inherit is a key whose type accepts a bare name or a mapping that says more,
// and marshals back to whichever form its data implies. Every governance key of
// this shape — a voice binding, a channel, a gate — is one a re-emitted document
// can rewrite without anybody deciding to.
type inherit struct {
	Profile string `yaml:"profile,omitempty"`
	Pack    string `yaml:"pack,omitempty"`
}

func (i *inherit) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		i.Profile = node.Value
		return nil
	}
	type alias inherit
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*i = inherit(decoded)
	return nil
}

func (i inherit) MarshalYAML() (any, error) {
	if i.Pack == "" {
		return i.Profile, nil
	}
	type alias inherit
	return alias(i), nil
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
extends:
  profile: house

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
	assert.Contains(t, got, "extends:\n  profile: house\n",
		"a key the value did not change keeps the form it was authored in")
	assert.Contains(t, got, "\n\ntone:\n", "the blank line above a section is authored punctuation")
	assert.Contains(t, got, "\n\nvocabulary:\n")
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

// TestMarshal_KeepsAuthoredBlankLines drives every case through a real change,
// because a write that changes nothing returns the document untouched and would
// prove nothing: the spacing has to survive the emitter, not avoid it.
func TestMarshal_KeepsAuthoredBlankLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		authored string
		change   func(*profile)
		want     string
	}{
		{
			name:     "between top-level keys",
			authored: "name: North Sea\n\ntone:\n  formality: neutral\n",
			want:     "name: Northsea\n\ntone:\n  formality: neutral\n",
		},
		{
			name:     "several in a row",
			authored: "name: North Sea\n\n\n\ntone:\n  formality: neutral\n",
			want:     "name: Northsea\n\n\n\ntone:\n  formality: neutral\n",
		},
		{
			name:     "above a comment",
			authored: "name: North Sea\n\n# Read under pressure.\ntone:\n  formality: neutral\n",
			want:     "name: Northsea\n\n# Read under pressure.\ntone:\n  formality: neutral\n",
		},
		{
			name:     "below a comment",
			authored: "name: North Sea\n# Read under pressure.\n\ntone:\n  formality: neutral\n",
			want:     "name: Northsea\n# Read under pressure.\n\ntone:\n  formality: neutral\n",
		},
		{
			name:     "between two comment blocks",
			authored: "name: North Sea\n\n# The register.\n\n# Read under pressure.\ntone:\n  formality: neutral\n",
			want:     "name: Northsea\n\n# The register.\n\n# Read under pressure.\ntone:\n  formality: neutral\n",
		},
		{
			name:     "leading",
			authored: "\n\nname: North Sea\n",
			want:     "\n\nname: Northsea\n",
		},
		{
			name:     "trailing",
			authored: "name: North Sea\n\n\n",
			want:     "name: Northsea\n\n\n",
		},
		{
			name:     "inside a nested mapping",
			authored: "name: North Sea\ntone:\n  personality: [clear]\n\n  formality: neutral\n",
			want:     "name: Northsea\ntone:\n  personality: [clear]\n\n  formality: neutral\n",
		},
		{
			name:     "above a nested block",
			authored: "name: North Sea\ntone:\n\n  formality: neutral\n",
			want:     "name: Northsea\ntone:\n\n  formality: neutral\n",
		},
		{
			name:     "between sequence items",
			authored: "name: North Sea\nexamples:\n  - Berths are allocated on arrival.\n\n  - Movements are logged.\n",
			want:     "name: Northsea\nexamples:\n  - Berths are allocated on arrival.\n\n  - Movements are logged.\n",
		},
		{
			name:     "an appended item leaves the spacing above it alone",
			authored: "name: North Sea\nexamples:\n  - Berths are allocated on arrival.\n\n  - Movements are logged.\n",
			change: func(p *profile) {
				p.Examples = append(p.Examples, "Pilots board outside the breakwater.")
			},
			want: "name: North Sea\nexamples:\n  - Berths are allocated on arrival.\n\n  - Movements are logged.\n  - Pilots board outside the breakwater.\n",
		},
		{
			name:     "a removed key takes its own spacing and nothing else",
			authored: "name: North Sea\n\ndescription: Plain writing.\n\ntone:\n  formality: neutral\n",
			change:   func(p *profile) { p.Description = "" },
			want:     "name: North Sea\n\ntone:\n  formality: neutral\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := loaded(t, tt.authored)
			if tt.change != nil {
				tt.change(p)
			} else {
				p.Name = "Northsea"
			}
			out, err := yamledit.Marshal([]byte(tt.authored), p)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
		})
	}
}

// TestMarshal_SpacingNeverChangesWhatTheDocumentSays covers the documents where
// a blank line is not spacing at all: inside a block scalar it is content, and a
// line that opens with a hash is only a comment outside one.
func TestMarshal_SpacingNeverChangesWhatTheDocumentSays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		authored string
		want     string
	}{
		{
			name:     "a blank line inside a block scalar",
			authored: "name: North Sea\ndescription: |\n  A berth is allocated\n\n  on arrival.\n\nexamples:\n  - Movements are logged.\n",
			want:     "name: Northsea\ndescription: |\n  A berth is allocated\n\n  on arrival.\n\nexamples:\n  - Movements are logged.\n",
		},
		{
			name:     "trailing newlines a keep indicator makes content",
			authored: "name: North Sea\ndescription: |+\n  A berth is allocated.\n\n\nexamples:\n  - Movements are logged.\n",
			want:     "name: Northsea\ndescription: |+\n  A berth is allocated.\n\n\nexamples:\n  - Movements are logged.\n",
		},
		{
			name:     "a hash inside a block scalar is not a comment",
			authored: "name: North Sea\ndescription: |\n  # Berthing\n\nexamples:\n  - Movements are logged.\n",
			want:     "name: Northsea\ndescription: |\n  # Berthing\n\nexamples:\n  - Movements are logged.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := loaded(t, tt.authored)
			p.Name = "Northsea"
			out, err := yamledit.Marshal([]byte(tt.authored), p)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))

			var reloaded profile
			require.NoError(t, yamlUnmarshal(out, &reloaded))
			assert.Equal(t, p.Description, reloaded.Description, "the write moved a line the scalar owns")
			assert.Equal(t, p.Examples, reloaded.Examples)
		})
	}
}

// TestMarshal_KeepsTheAuthoredFormOfAnUnchangedKey pins the rule for a key whose
// type accepts more than one spelling: the document decides, until the value
// says something the document does not.
func TestMarshal_KeepsTheAuthoredFormOfAnUnchangedKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		authored string
		change   func(*profile)
		want     string
	}{
		{
			name:     "a mapping stays a mapping",
			authored: "name: North Sea\nextends:\n  profile: house\n",
			want:     "name: Northsea\nextends:\n  profile: house\n",
		},
		{
			name:     "a scalar stays a scalar",
			authored: "name: North Sea\nextends: house\n",
			want:     "name: Northsea\nextends: house\n",
		},
		{
			name:     "a flow mapping stays one",
			authored: "name: North Sea\nextends: {profile: house}\n",
			want:     "name: Northsea\nextends: {profile: house}\n",
		},
		{
			name:     "the comment above it survives too",
			authored: "name: North Sea\n# The profile this one starts from.\nextends:\n  profile: house\n",
			want:     "name: Northsea\n# The profile this one starts from.\nextends:\n  profile: house\n",
		},
		{
			name:     "a changed value takes the form its data implies",
			authored: "name: North Sea\nextends:\n  profile: house\n",
			change:   func(p *profile) { p.Extends.Profile = "harbour" },
			want:     "name: North Sea\nextends: harbour\n",
		},
		{
			name:     "a value that needs the long form gets it",
			authored: "name: North Sea\nextends: house\n",
			change:   func(p *profile) { p.Extends = &inherit{Pack: "operations"} },
			want:     "name: North Sea\nextends:\n  pack: operations\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := loaded(t, tt.authored)
			if tt.change != nil {
				tt.change(p)
			} else {
				p.Name = "Northsea"
			}
			out, err := yamledit.Marshal([]byte(tt.authored), p)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
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
