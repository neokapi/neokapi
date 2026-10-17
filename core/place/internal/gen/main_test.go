package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandRange(t *testing.T) {
	tests := map[string][]string{
		"no03":    {"no03"},
		"ad02~4":  {"ad02", "ad03", "ad04"},
		"ag10~1":  {"ag10", "ag11"},
		"gbenf~g": {"gbenf", "gbeng"},
	}
	for in, want := range tests {
		got, err := expandRange(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"ad09~2", "ad~", "a~bcd"} {
		_, err := expandRange(bad)
		assert.Error(t, err, bad)
	}
}

func TestBuild(t *testing.T) {
	regular := map[string]bool{"frara": true, "fr01": true, "fr69m": true, "no03": true, "gbsct": true, "gbabe": true}
	parents := map[string]string{"frara": "fr", "fr01": "frara", "fr69m": "frcp", "frcp": "frara", "no03": "no", "gbsct": "gb", "gbabe": "gbsct"}
	names := map[string]string{"frara": "Auvergne-Rhône-Alpes", "fr01": "Ain", "fr69m": "Métropole de Lyon", "no03": "Oslo", "gbsct": "Scotland", "gbabe": "Aberdeen"}
	rows, err := build(regular, parents, names)
	require.NoError(t, err)
	assert.Equal(t, []row{
		{"fr-01", "fr-ara", "Ain"},
		{"fr-69m", "fr-ara", "Métropole de Lyon"},
		{"fr-ara", "fr", "Auvergne-Rhône-Alpes"},
		{"gb-abe", "gb-sct", "Aberdeen"},
		{"gb-sct", "gb", "Scotland"},
		{"no-03", "no", "Oslo"},
	}, rows, "a container CLDR does not mark regular is skipped for the nearest one it does")

	delete(names, "no03")
	_, err = build(regular, parents, names)
	assert.ErrorContains(t, err, "no03 has no English name")
}

func TestApplyAmendments(t *testing.T) {
	rows := []row{{"no-03", "no", "Oslo"}, {"no-30", "no", "Viken"}}
	got, err := applyAmendments(rows, []byte("# comment\n\nadd\tno-32\tno\tAkershus\tsource\nretire\tno-30\t-\t-\tsource\n"))
	require.NoError(t, err)
	assert.Equal(t, []row{{"no-03", "no", "Oslo"}, {"no-32", "no", "Akershus"}}, got)

	tests := map[string]string{
		"an add the release holds":            "add\tno-03\tno\tOslo\tsource\n",
		"a retire the release lacks":          "retire\tno-99\t-\t-\tsource\n",
		"a line without a source":             "add\tno-32\tno\tAkershus\t\n",
		"a line with too few columns":         "add\tno-32\tno\n",
		"an operation neither add nor retire": "rename\tno-30\tno\tViken\tsource\n",
	}
	for name, amendment := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := applyAmendments([]row{{"no-03", "no", "Oslo"}}, []byte(amendment))
			assert.Error(t, err)
		})
	}
}

// TestAmendmentsFile checks the committed amendments parse and that each one
// names a source.
func TestAmendmentsFile(t *testing.T) {
	rows := []row{{"no-03", "no", "Oslo"}, {"no-30", "no", "Viken"}, {"no-38", "no", "Vestfold og Telemark"}, {"no-54", "no", "Troms og Finnmark"}}
	data, err := os.ReadFile("amendments.tsv")
	require.NoError(t, err)
	got, err := applyAmendments(rows, data)
	require.NoError(t, err)
	var ids []string
	for _, r := range got {
		ids = append(ids, r.id)
	}
	assert.Equal(t, []string{"no-03", "no-31", "no-32", "no-33", "no-39", "no-40", "no-55", "no-56"}, ids)
}
