package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pointerProject writes a recipe (and any other files) into a fresh root.
func pointerProject(t *testing.T, recipe string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	for rel, body := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

const (
	packRecipe = "version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    pack: professional-b2b\n"
	fileRecipe = "version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    profile_file: brand/voice.yaml\n"
	bareRecipe = "version: v1\nname: my-app\ndefaults:\n  source_language: en\n"
)

func TestWriteVoicePointer(t *testing.T) {
	houseVoice := "name: House Voice\ntone:\n  formality: neutral\n"

	tests := []struct {
		name string
		// recipe and files set the project up; the test then writes once.
		recipe string
		files  map[string]string
		// wantFile is the assistant file, relative to root; empty when
		// nothing is written.
		wantFile   string
		wantAction VoicePointerAction
		wantVoice  string
		// wantIn / wantNotIn are checked against the file after the write.
		wantIn    []string
		wantNotIn []string
	}{
		{
			name:       "a fresh project gets AGENTS.md",
			recipe:     packRecipe,
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerCreated,
			wantVoice:  "Professional B2B",
			wantIn:     []string{"# my-app\n\n" + coreprofile.VoicePointerStartLine, "voice, Professional B2B, is held by kapi", "`kapi voice guide`"},
		},
		{
			name:       "an existing CLAUDE.md takes the section",
			recipe:     packRecipe,
			files:      map[string]string{"CLAUDE.md": "# Rules\n\nBe brief.\n"},
			wantFile:   "CLAUDE.md",
			wantAction: VoicePointerUpdated,
			wantVoice:  "Professional B2B",
			wantIn:     []string{"# Rules\n\nBe brief.\n\n" + coreprofile.VoicePointerStartLine},
		},
		{
			name:       "AGENTS.md is preferred when both exist",
			recipe:     packRecipe,
			files:      map[string]string{"CLAUDE.md": "@AGENTS.md\n", "AGENTS.md": "# Agents\n"},
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerUpdated,
			wantVoice:  "Professional B2B",
			wantIn:     []string{"# Agents\n\n" + coreprofile.VoicePointerStartLine},
		},
		{
			name:       "a profile file binding names the profile",
			recipe:     fileRecipe,
			files:      map[string]string{"brand/voice.yaml": houseVoice},
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerCreated,
			wantVoice:  "House Voice",
			wantIn:     []string{"voice, House Voice, is held by kapi"},
		},
		{
			name:       "a bound file nobody has written yet is pointed at unnamed",
			recipe:     fileRecipe,
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerCreated,
			wantIn:     []string{"This project's voice is held by kapi"},
			wantNotIn:  []string{"voice, ,"},
		},
		{
			name:       "a convention file counts as a voice",
			recipe:     bareRecipe,
			files:      map[string]string{".kapi/voice.yaml": houseVoice},
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerCreated,
			wantVoice:  "House Voice",
		},
		{
			name: "declared profiles make the pointer per file",
			recipe: fileRecipe + "profiles:\n  acme:\n    channels: [docs]\n    voice: brand/acme.yaml\n" +
				"collections:\n  - name: acme-docs\n    channel: acme/docs\n    content:\n      - path: \"docs/**/*.md\"\n",
			files:      map[string]string{"brand/voice.yaml": houseVoice, "brand/acme.yaml": "name: Acme\n"},
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerCreated,
			wantVoice:  "House Voice",
			wantIn:     []string{"`kapi voice guide <path>`", "Some collections carry a voice of their own"},
		},
		{
			name:       "a project without a voice writes nothing",
			recipe:     bareRecipe,
			wantAction: VoicePointerNone,
		},
		{
			name:       "a project without a voice leaves an existing file alone",
			recipe:     bareRecipe,
			files:      map[string]string{"CLAUDE.md": "# Rules\n"},
			wantAction: VoicePointerNone,
			wantNotIn:  []string{coreprofile.VoicePointerStart},
		},
		{
			name:       "a stale section is removed when the voice is unbound",
			recipe:     bareRecipe,
			files:      map[string]string{"AGENTS.md": "# my-app\n\n" + coreprofile.RenderVoicePointer(coreprofile.VoicePointer{Name: "Gone"})},
			wantFile:   "AGENTS.md",
			wantAction: VoicePointerRemoved,
			wantNotIn:  []string{coreprofile.VoicePointerStart, "Gone"},
			wantIn:     []string{"# my-app\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := pointerProject(t, tt.recipe, tt.files)
			a := &App{}

			res, err := a.WriteVoicePointer(context.Background(), root)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAction, res.Action)
			assert.Equal(t, tt.wantVoice, res.Voice)
			assert.Empty(t, res.Warning)

			if tt.wantFile == "" {
				assert.Empty(t, res.File)
				_, serr := os.Stat(filepath.Join(root, "AGENTS.md"))
				if _, seeded := tt.files["AGENTS.md"]; !seeded {
					assert.True(t, os.IsNotExist(serr), "nothing creates an assistant file for a project with no voice")
				}
			} else {
				assert.Equal(t, filepath.Join(root, tt.wantFile), res.File)
			}

			// What ended up on disk.
			checkPath := tt.wantFile
			if checkPath == "" {
				for name := range tt.files {
					checkPath = name
				}
			}
			if checkPath == "" {
				return
			}
			body, rerr := os.ReadFile(filepath.Join(root, checkPath))
			require.NoError(t, rerr)
			for _, w := range tt.wantIn {
				assert.Contains(t, string(body), w)
			}
			for _, w := range tt.wantNotIn {
				assert.NotContains(t, string(body), w)
			}
		})
	}
}

// A second run is a no-op, and a changed voice replaces the section in place
// without touching what the author wrote around it.
func TestWriteVoicePointer_IdempotentAndUpdatedInPlace(t *testing.T) {
	root := pointerProject(t, packRecipe, map[string]string{
		"CLAUDE.md": "# Rules\n\nBe brief.\n",
	})
	a := &App{}
	ctx := context.Background()

	first, err := a.WriteVoicePointer(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, VoicePointerUpdated, first.Action)
	afterFirst, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)

	second, err := a.WriteVoicePointer(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, VoicePointerUnchanged, second.Action)
	afterSecond, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, string(afterFirst), string(afterSecond), "a second run writes the same bytes")
	assert.Equal(t, 1, strings.Count(string(afterSecond), coreprofile.VoicePointerStart), "one section, never two")

	// The author adds a section after the pointer, then the voice changes.
	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"),
		append(afterSecond, []byte("\n## Testing\n\nRun make test.\n")...), 0o644))
	recipe := "version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    pack: technical-docs\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))

	third, err := a.WriteVoicePointer(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, VoicePointerUpdated, third.Action)
	assert.Equal(t, "Technical Documentation", third.Voice)
	body, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)
	got := string(body)
	assert.Contains(t, got, "# Rules\n\nBe brief.\n\n"+coreprofile.VoicePointerStartLine)
	assert.Contains(t, got, "voice, Technical Documentation, is held by kapi")
	assert.NotContains(t, got, "Professional B2B")
	assert.Contains(t, got, coreprofile.VoicePointerEnd+"\n\n## Testing\n\nRun make test.\n")
	assert.Equal(t, 1, strings.Count(got, coreprofile.VoicePointerStart))
}

// A binding that exists but does not load is still pointed at, and the reason
// travels on the result rather than failing the write.
func TestWriteVoicePointer_ReportsAnUnloadableBinding(t *testing.T) {
	root := pointerProject(t, fileRecipe, map[string]string{
		"brand/voice.yaml": "name: [not a string\n",
	})
	a := &App{}
	res, err := a.WriteVoicePointer(context.Background(), root)
	require.NoError(t, err)
	assert.Equal(t, VoicePointerCreated, res.Action)
	assert.Empty(t, res.Voice)
	assert.Contains(t, res.Warning, "brand/voice.yaml")
}

// A file whose section has no end marker is left alone and named.
func TestWriteVoicePointer_RefusesAnUnterminatedSection(t *testing.T) {
	root := pointerProject(t, packRecipe, map[string]string{
		"AGENTS.md": "# Agents\n\n" + coreprofile.VoicePointerStartLine + "\nbody\n",
	})
	a := &App{}
	_, err := a.WriteVoicePointer(context.Background(), root)
	require.ErrorIs(t, err, coreprofile.ErrVoicePointerUnterminated)
	assert.Contains(t, err.Error(), "AGENTS.md")
}

func TestAssistantFile(t *testing.T) {
	root := t.TempDir()
	path, exists := AssistantFile(root)
	assert.Equal(t, filepath.Join(root, "AGENTS.md"), path)
	assert.False(t, exists)

	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("x"), 0o644))
	path, exists = AssistantFile(root)
	assert.Equal(t, filepath.Join(root, "CLAUDE.md"), path)
	assert.True(t, exists)

	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("x"), 0o644))
	path, exists = AssistantFile(root)
	assert.Equal(t, filepath.Join(root, "AGENTS.md"), path)
	assert.True(t, exists)
}
