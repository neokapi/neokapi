package formats

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// constructsDoc is the part of core/formats/constructs.yaml this test reads: the
// declared family classes and the formats each names as an example.
type constructsDoc struct {
	Families map[string]struct {
		Examples []string `yaml:"examples"`
	} `yaml:"families"`
}

// vocabularyDoc is the part of a per-format vocabulary.yaml this test reads.
type vocabularyDoc struct {
	Family string `yaml:"family"`
}

func loadConstructs(t *testing.T) constructsDoc {
	t.Helper()
	data, err := os.ReadFile("constructs.yaml")
	require.NoError(t, err)
	var doc constructsDoc
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Families)
	return doc
}

// The eight family constants are exactly the classes constructs.yaml declares.
// The taxonomy is change-controlled, so the two lists move together or not at
// all.
func TestFormatFamiliesMatchConstructsYAML(t *testing.T) {
	doc := loadConstructs(t)

	declared := make([]string, 0, len(doc.Families))
	for name := range doc.Families {
		declared = append(declared, name)
	}
	inGo := make([]string, 0, len(registry.FormatFamilies))
	for _, f := range registry.FormatFamilies {
		inGo = append(inGo, string(f))
	}
	assert.ElementsMatch(t, declared, inGo)
}

// Every registered format states what shape it carries. A format added without
// an entry in builtInFamilies fails here rather than falling silently onto the
// document preview.
func TestEveryRegisteredFormatHasAFamily(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	infos := reg.FormatInfos()
	require.NotEmpty(t, infos)

	for _, info := range infos {
		t.Run(string(info.Name), func(t *testing.T) {
			assert.NotEmpty(t, info.Family,
				"format %q declares no family; add it to builtInFamilies in core/formats/families.go", info.Name)
			assert.True(t, registry.ValidFormatFamily(info.Family),
				"format %q declares family %q, which is not one of the eight classes in constructs.yaml",
				info.Name, info.Family)
		})
	}
}

// builtInFamilies carries no entry for a format that is not registered — a
// stale row survives a format rename otherwise, and nothing reads it again.
func TestFamilyTableNamesOnlyRegisteredFormats(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for id := range builtInFamilies {
		assert.NotNil(t, reg.FormatInfo(id),
			"builtInFamilies names %q, which no longer registers", id)
	}
}

// A format that names a family in its own vocabulary.yaml gets the same answer
// from the registry. The yaml is what the maturity rubric scores against, so
// the two disagreeing means a format is measured as one shape and rendered as
// another.
func TestFamilyAgreesWithVocabularyYAML(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	paths, err := filepath.Glob(filepath.Join("*", "vocabulary.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	// The vocabulary file sits under the format's PACKAGE directory, which is
	// not always its registered id: core/formats/jsx registers as "kbf".
	packageFormat := map[string]registry.FormatID{"jsx": "kbf"}

	checked := 0
	for _, path := range paths {
		pkg := filepath.Base(filepath.Dir(path))
		id, ok := packageFormat[pkg]
		if !ok {
			id = registry.FormatID(pkg)
		}
		info := reg.FormatInfo(id)
		if info == nil {
			continue // a vocabulary for a format this build does not register
		}

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var doc vocabularyDoc
		require.NoError(t, yaml.Unmarshal(data, &doc))
		if doc.Family == "" {
			continue
		}

		checked++
		assert.Equal(t, doc.Family, string(info.Family),
			"%s declares family %q; the registry says %q", path, doc.Family, info.Family)
	}
	assert.Positive(t, checked, "no vocabulary.yaml declared a family; the check ran over nothing")
}

// The formats constructs.yaml names as examples of a family are registered
// under that family. The yaml lists are prose, so this catches the drift in the
// direction the prose is authored.
func TestConstructsExamplesAgreeWithTheRegistry(t *testing.T) {
	doc := loadConstructs(t)
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	// core/formats/jsx registers under "kbf"; constructs.yaml names it by its
	// package, the way the vocabulary matrices do.
	exampleFormat := map[string]registry.FormatID{"jsx": "kbf"}

	for family, entry := range doc.Families {
		for _, example := range entry.Examples {
			id, ok := exampleFormat[example]
			if !ok {
				id = registry.FormatID(example)
			}
			info := reg.FormatInfo(id)
			if info == nil {
				continue // an example this build does not register
			}
			assert.Equal(t, family, string(info.Family),
				"constructs.yaml lists %q under %q; the registry says %q", example, family, info.Family)
		}
	}
}
