package kaz

import (
	"bytes"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreExportRoundtrip(t *testing.T) {
	blocks := []ExportBlock{
		{
			ID:          "b1",
			Name:        "greeting",
			Source:      "Hello world",
			ContentHash: "abc123",
			ContextHash: "def456",
			Targets:     map[string]string{"fr": "Bonjour le monde"},
			Properties:  map[string]string{"context": "homepage"},
		},
		{
			ID:     "b2",
			Source: "Goodbye",
		},
	}

	versions := []ExportVersion{
		{
			ID:       "v1",
			Label:    "v1.0",
			BlockIDs: []string{"b1", "b2"},
		},
	}

	connectors := []ConnectorMeta{
		{
			ID:       "c1",
			Type:     "file",
			Name:     "Local Files",
			Category: "file",
			Config:   map[string]string{"path": "/data"},
		},
	}

	tmData := []byte(`[{"source":"Hello","target":"Bonjour"}]`)
	termsData := []byte(`[{"term":"API","definition":"Application Programming Interface"}]`)

	var buf bytes.Buffer
	err := ExportStore(&buf, StoreExportOptions{
		ProjectID:     "proj-1",
		ProjectName:   "Test Project",
		SourceLocale:  "en",
		TargetLocales: []string{"fr", "de"},
		Blocks:        blocks,
		Versions:      versions,
		Connectors:    connectors,
		TMData:        tmData,
		TermsData:     termsData,
		VersionLabel:  "v1.0",
	})
	require.NoError(t, err)

	pkg, err := ImportStoreFromBytes(buf.Bytes())
	require.NoError(t, err)

	// Manifest
	assert.Equal(t, "2.0", pkg.Manifest.FormatVersion)
	assert.Equal(t, "proj-1", pkg.Manifest.ProjectID)
	assert.Equal(t, "Test Project", pkg.Manifest.ProjectName)
	assert.Equal(t, "en", pkg.Manifest.SourceLocale)
	assert.Equal(t, []string{"fr", "de"}, pkg.Manifest.TargetLocales)
	assert.Equal(t, 2, pkg.Manifest.BlockCount)
	assert.Equal(t, "v1.0", pkg.Manifest.VersionLabel)

	// Blocks
	require.Len(t, pkg.Blocks, 2)
	assert.Equal(t, "b1", pkg.Blocks[0].ID)
	assert.Equal(t, "Hello world", pkg.Blocks[0].Source)
	assert.Equal(t, "abc123", pkg.Blocks[0].ContentHash)
	assert.Equal(t, "Bonjour le monde", pkg.Blocks[0].Targets["fr"])
	assert.Equal(t, "b2", pkg.Blocks[1].ID)

	// Versions
	require.Len(t, pkg.Versions, 1)
	assert.Equal(t, "v1.0", pkg.Versions[0].Label)
	assert.Equal(t, []string{"b1", "b2"}, pkg.Versions[0].BlockIDs)

	// Connectors
	require.Len(t, pkg.Connectors, 1)
	assert.Equal(t, "file", pkg.Connectors[0].Type)

	// TM and terms
	assert.Equal(t, tmData, pkg.TMData)
	assert.Equal(t, termsData, pkg.TermsData)
}

func TestStoreExportMinimal(t *testing.T) {
	var buf bytes.Buffer
	err := ExportStore(&buf, StoreExportOptions{
		ProjectID:    "proj-2",
		ProjectName:  "Minimal",
		SourceLocale: "en",
		Blocks:       []ExportBlock{{ID: "b1", Source: "Hi"}},
	})
	require.NoError(t, err)

	pkg, err := ImportStoreFromBytes(buf.Bytes())
	require.NoError(t, err)

	assert.Equal(t, "proj-2", pkg.Manifest.ProjectID)
	require.Len(t, pkg.Blocks, 1)
	assert.Nil(t, pkg.Versions)
	assert.Nil(t, pkg.Connectors)
	assert.Nil(t, pkg.TMData)
	assert.Nil(t, pkg.TermsData)
}

func TestBlockConversion(t *testing.T) {
	b := model.NewBlock("test-1", "Hello world")
	b.Name = "greeting"
	b.Type = "text"
	b.Properties = map[string]string{"note": "UI string"}
	b.SetTargetText("fr", "Bonjour le monde")
	b.Identity = &model.BlockIdentity{ContentHash: "hash1", ContextHash: "hash2"}
	b.ContentRef = &model.ContentRef{ConnectorID: "c1", ExternalID: "ext-1"}

	eb := BlockToExport(b)
	assert.Equal(t, "test-1", eb.ID)
	assert.Equal(t, "greeting", eb.Name)
	assert.Equal(t, "Hello world", eb.Source)
	assert.Equal(t, "Bonjour le monde", eb.Targets["fr"])
	assert.Equal(t, "hash1", eb.ContentHash)
	assert.Equal(t, "c1", eb.ConnectorID)

	// Round-trip
	b2 := ExportToBlock(eb)
	assert.Equal(t, "test-1", b2.ID)
	assert.Equal(t, "greeting", b2.Name)
	assert.Equal(t, "Hello world", b2.SourceText())
	assert.Equal(t, "hash1", b2.Identity.ContentHash)
	assert.Equal(t, "c1", b2.ContentRef.ConnectorID)
}

func TestImportBadFormat(t *testing.T) {
	_, err := ImportStoreFromBytes([]byte("not a zip"))
	assert.Error(t, err)
}
