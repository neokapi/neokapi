package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatPresetRegistry(t *testing.T) {
	t.Run("register and get", func(t *testing.T) {
		r := NewPresetRegistry()
		p := &FormatPreset{
			Name:   "wellFormed",
			Format: "okf_html",
			Config: map[string]any{"inline": true},
		}
		r.RegisterFormatPreset("okf_html", "wellFormed", p)

		got := r.GetFormatPreset("okf_html", "wellFormed")
		require.NotNil(t, got)
		assert.Equal(t, "wellFormed", got.Name)
		assert.Equal(t, map[string]any{"inline": true}, got.Config)
	})

	t.Run("get returns nil for missing preset", func(t *testing.T) {
		r := NewPresetRegistry()
		assert.Nil(t, r.GetFormatPreset("okf_html", "nonexistent"))
	})

	t.Run("get returns nil for missing format", func(t *testing.T) {
		r := NewPresetRegistry()
		r.RegisterFormatPreset("okf_html", "wellFormed", &FormatPreset{Name: "wellFormed"})
		assert.Nil(t, r.GetFormatPreset("okf_json", "wellFormed"))
	})

	t.Run("list returns sorted presets", func(t *testing.T) {
		r := NewPresetRegistry()
		r.RegisterFormatPreset("okf_html", "charlie", &FormatPreset{Name: "charlie"})
		r.RegisterFormatPreset("okf_html", "alpha", &FormatPreset{Name: "alpha"})
		r.RegisterFormatPreset("okf_html", "bravo", &FormatPreset{Name: "bravo"})

		list := r.ListFormatPresets("okf_html")
		require.Len(t, list, 3)
		assert.Equal(t, "alpha", list[0].Name)
		assert.Equal(t, "bravo", list[1].Name)
		assert.Equal(t, "charlie", list[2].Name)
	})

	t.Run("list returns nil for unknown format", func(t *testing.T) {
		r := NewPresetRegistry()
		assert.Nil(t, r.ListFormatPresets("okf_unknown"))
	})

	t.Run("overwrite existing preset", func(t *testing.T) {
		r := NewPresetRegistry()
		r.RegisterFormatPreset("okf_html", "default", &FormatPreset{
			Name:   "default",
			Config: map[string]any{"wrap": 80},
		})
		r.RegisterFormatPreset("okf_html", "default", &FormatPreset{
			Name:   "default",
			Config: map[string]any{"wrap": 120},
		})

		got := r.GetFormatPreset("okf_html", "default")
		require.NotNil(t, got)
		assert.Equal(t, map[string]any{"wrap": 120}, got.Config)

		// List should still have exactly one entry
		list := r.ListFormatPresets("okf_html")
		assert.Len(t, list, 1)
	})
}

func TestFrameworkPresetRegistry(t *testing.T) {
	t.Run("register and get", func(t *testing.T) {
		r := NewPresetRegistry()
		p := &FrameworkPreset{
			Name:        "nextjs",
			Description: "Next.js i18n setup",
			Mappings: []MappingTemplate{
				{Local: "locales/*.json", Format: "okf_json"},
			},
			Source: sourceBuiltIn,
		}
		r.RegisterFrameworkPreset("nextjs", p)

		got := r.GetFrameworkPreset("nextjs")
		require.NotNil(t, got)
		assert.Equal(t, "nextjs", got.Name)
		assert.Len(t, got.Mappings, 1)
	})

	t.Run("get returns nil for missing", func(t *testing.T) {
		r := NewPresetRegistry()
		assert.Nil(t, r.GetFrameworkPreset("nonexistent"))
	})

	t.Run("list returns sorted presets", func(t *testing.T) {
		r := NewPresetRegistry()
		r.RegisterFrameworkPreset("rails", &FrameworkPreset{Name: "rails"})
		r.RegisterFrameworkPreset("angular", &FrameworkPreset{Name: "angular"})
		r.RegisterFrameworkPreset("nextjs", &FrameworkPreset{Name: "nextjs"})

		list := r.ListFrameworkPresets()
		require.Len(t, list, 3)
		assert.Equal(t, "angular", list[0].Name)
		assert.Equal(t, "nextjs", list[1].Name)
		assert.Equal(t, "rails", list[2].Name)
	})

	t.Run("list returns nil when empty", func(t *testing.T) {
		r := NewPresetRegistry()
		assert.Nil(t, r.ListFrameworkPresets())
	})

	t.Run("overwrite existing preset", func(t *testing.T) {
		r := NewPresetRegistry()
		r.RegisterFrameworkPreset("nextjs", &FrameworkPreset{
			Name:        "nextjs",
			Description: "old",
		})
		r.RegisterFrameworkPreset("nextjs", &FrameworkPreset{
			Name:        "nextjs",
			Description: "new",
		})

		got := r.GetFrameworkPreset("nextjs")
		require.NotNil(t, got)
		assert.Equal(t, "new", got.Description)

		list := r.ListFrameworkPresets()
		assert.Len(t, list, 1)
	})
}
