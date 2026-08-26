package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The format declares the ceiling and the recipe narrows it. Every case here is
// a direction the narrowing must not go: it must not widen the set, must not
// fail on a type the format cannot carry, and must not silently empty the set
// when the recipe says nothing.
func TestEffectiveInlineAnnotations(t *testing.T) {
	t.Parallel()
	declared := []string{"term", "voice"}

	tests := []struct {
		name  string
		write []string
		want  []string
	}{
		{
			name:  "no recipe opinion leaves the declaration standing",
			write: nil,
			want:  []string{"term", "voice"},
		},
		{
			name:  "a recipe narrows to what it named",
			write: []string{"term"},
			want:  []string{"term"},
		},
		{
			name: "a recipe cannot widen past what the format carries",
			// entity is not in the format's declaration, so asking for it asks
			// for nothing: a recipe describes many formats at once.
			write: []string{"term", "entity"},
			want:  []string{"term"},
		},
		{
			name:  "naming only what the format cannot carry draws nothing",
			write: []string{"entity"},
			want:  []string{},
		},
		{
			name:  "the format's declared order is kept, not the recipe's",
			write: []string{"voice", "term"},
			want:  []string{"term", "voice"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := AnnotationDefaults{Write: tc.write}.EffectiveInlineAnnotations(declared)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A format that carries nothing carries nothing, whatever the recipe asks.
func TestEffectiveInlineAnnotationsUndeclaredFormat(t *testing.T) {
	t.Parallel()
	assert.Empty(t, AnnotationDefaults{Write: []string{"term"}}.EffectiveInlineAnnotations(nil))
}

func TestAnnotationDefaultsRoundTripYAML(t *testing.T) {
	t.Parallel()
	var d Defaults
	err := yaml.Unmarshal([]byte("annotations:\n  write: [term]\n"), &d)
	require.NoError(t, err)
	assert.Equal(t, []string{"term"}, d.Annotations.Write)

	// A project with no opinion writes no key.
	out, err := yaml.Marshal(Defaults{})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "annotations")
}
