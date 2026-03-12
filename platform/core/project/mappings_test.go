package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRemotePath(t *testing.T) {
	tmpDir := t.TempDir()
	bowrainDir := filepath.Join(tmpDir, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := &Config{
		Defaults: Defaults{
			SourceLanguage: "en",
		},
		Content: []ContentEntry{
			{
				Path:   "src/locales/**/*.json",
				Format: "json",
				Base:   "src/locales/",
			},
			{
				Path:   "content/*.md",
				Format: "markdown",
			},
			{
				Path:   "data/*.yaml",
				Format: "yaml",
			},
		},
	}

	require.NoError(t, SaveConfig(bowrainDir, cfg))

	project, err := FindProject(tmpDir)
	require.NoError(t, err)

	t.Run("resolve with base prefix stripping", func(t *testing.T) {
		localFile := filepath.Join(tmpDir, "src/locales/en/messages.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(localFile), 0755))
		require.NoError(t, os.WriteFile(localFile, []byte("{}"), 0644))

		remote, format, err := project.ResolveRemotePath("src/locales/en/messages.json")
		require.NoError(t, err)
		assert.Equal(t, "en/messages.json", remote)
		assert.Equal(t, "json", format)
	})

	t.Run("resolve without base (full path)", func(t *testing.T) {
		localFile := filepath.Join(tmpDir, "content/faq.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(localFile), 0755))
		require.NoError(t, os.WriteFile(localFile, []byte("# FAQ"), 0644))

		remote, format, err := project.ResolveRemotePath("content/faq.md")
		require.NoError(t, err)
		assert.Equal(t, "content/faq.md", remote)
		assert.Equal(t, "markdown", format)
	})

	t.Run("resolve yaml entry", func(t *testing.T) {
		localFile := filepath.Join(tmpDir, "data/settings.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(localFile), 0755))
		require.NoError(t, os.WriteFile(localFile, []byte("key: value"), 0644))

		remote, format, err := project.ResolveRemotePath("data/settings.yaml")
		require.NoError(t, err)
		assert.Equal(t, "data/settings.yaml", remote)
		assert.Equal(t, "yaml", format)
	})

	t.Run("no matching content entry", func(t *testing.T) {
		_, _, err := project.ResolveRemotePath("unmapped/file.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no content entry found")
	})
}

func TestExpandTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		path     string
		expected string
	}{
		{
			name:     "path template",
			template: "app/{path}",
			path:     "src/locales/en/messages.json",
			expected: "app/src/locales/en/messages",
		},
		{
			name:     "filename template",
			template: "files/{filename}",
			path:     "content/doc.md",
			expected: "files/doc.md",
		},
		{
			name:     "basename template",
			template: "configs/{basename}",
			path:     "data/settings.yaml",
			expected: "configs/settings",
		},
		{
			name:     "multiple templates",
			template: "{path}/{filename}",
			path:     "src/file.json",
			expected: "src/file/file.json",
		},
		{
			name:     "no template",
			template: "static/path",
			path:     "any/file.txt",
			expected: "static/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
