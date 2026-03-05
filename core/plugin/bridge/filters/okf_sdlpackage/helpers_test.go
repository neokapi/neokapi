//go:build integration

package okf_sdlpackage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.sdlpackage.SdlPackageFilter"
const mimeType = "application/x-sdlpackage"

// readPackageFileWithLocales reads an SDL package file with specific source and target locales.
// The SDL package filter uses the target locale to determine which subfolder of .sdlxliff
// files to process, so the correct locale must be provided.
func readPackageFileWithLocales(t *testing.T, relPath string, srcLocale, tgtLocale model.LocaleID) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", path)

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)

	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: srcLocale,
		TargetLocale: tgtLocale,
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part from bridge")
		parts = append(parts, pr.Part)
	}

	require.NoError(t, reader.Close())
	return parts
}

// readPackageFileFromPath reads an SDL package file from an absolute path with specific locales.
func readPackageFileFromPath(t *testing.T, absPath string, srcLocale, tgtLocale model.LocaleID) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)

	content, err := os.ReadFile(absPath)
	require.NoError(t, err, "reading file %s", absPath)

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)

	doc := &model.RawDocument{
		URI:          absPath,
		SourceLocale: srcLocale,
		TargetLocale: tgtLocale,
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part from bridge")
		parts = append(parts, pr.Part)
	}

	require.NoError(t, reader.Close())
	return parts
}

// roundtripPackageFile performs a roundtrip with specific locales.
func roundtripPackageFile(t *testing.T, relPath string, srcLocale, tgtLocale model.LocaleID) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", path)

	return bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass, content, path, mimeType, nil, srcLocale, tgtLocale)
}

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

