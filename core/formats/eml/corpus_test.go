package eml_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".eml"))
	require.NoError(t, err)
	return raw
}

func roundTrip(t *testing.T, raw []byte, mutate func(*model.Block), locale model.LocaleID) ([]byte, error) {
	t.Helper()
	r, w := eml.NewReader(), eml.NewWriter()
	store, err := format.NewWiredSkeleton(r, w)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	parts, err := spec.ReadParts(r, raw)
	require.NoError(t, err)
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && mutate != nil {
			mutate(b)
		}
	}
	w.SetLocale(locale)
	return spec.WriteParts(w, parts, nil)
}

func TestCorpus(t *testing.T) {
	tests := map[string][]string{
		"plain":        {"Project update", "The draft is ready."},
		"multipart":    {"Résumé notes", "café and tea"},
		"base64":       {"Encoded body", "First line.\nCafé."},
		"signed":       {"Signed message"},
		"python-email": {"Grüße aus Österreich", "Ankündigung für März.\n"},
	}
	paths, err := filepath.Glob("testdata/*.eml")
	require.NoError(t, err)
	require.Len(t, paths, len(tests))
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			raw := fixture(t, name)
			var got []string
			out, err := roundTrip(t, raw, func(b *model.Block) {
				got = append(got, b.SourceText())
				role := model.RoleParagraph
				if b.Name == "subject" {
					role = model.RoleHeading
				}
				require.Equal(t, role, b.SemanticRole())
			}, "")
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.True(t, bytes.Equal(raw, out), "EML wire bytes changed")
		})
	}
}
