package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// ArchiveEntryInfo describes one inner file of an archive container, for the
// file-explorer tree. Format is extension-derived (cheap); empty means kapi has
// no reader for that extension (the entry is shown but not previewable).
type ArchiveEntryInfo struct {
	Name   string `json:"name"`   // slash-separated path inside the archive
	Format string `json:"format"` // detected format id, or ""
	Size   int64  `json:"size"`
}

// IsArchive reports whether a path is a container the explorer can expand into a
// tree of inner entries (ZIP/TAR/TAR.GZ).
func (a *App) IsArchive(filePath string) bool {
	return container.IsContainerPath(filePath)
}

// ListArchiveEntries lists the inner entries of an archive without decompressing
// their content (ZIP central directory / TAR header scan). The frontend renders
// these as expandable children of the archive node.
func (a *App) ListArchiveEntries(filePath string) ([]ArchiveEntryInfo, error) {
	_, headers, err := container.ListEntries(filePath)
	if err != nil {
		return nil, fmt.Errorf("list archive entries: %w", err)
	}
	out := make([]ArchiveEntryInfo, 0, len(headers))
	for _, h := range headers {
		out = append(out, ArchiveEntryInfo{
			Name:   h.Name,
			Format: a.DetectFormat(h.Name),
			Size:   h.Size,
		})
	}
	return out, nil
}

// InspectArchiveEntry parses a single archive entry and returns the same content
// tree JSON as InspectFile, so the viewer/BlockInspector can preview a file that
// lives inside a container. Only that entry is read (random access for ZIP, scan
// for TAR) — the whole archive is never loaded.
func (a *App) InspectArchiveEntry(tabID, archivePath, entry string) (string, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return "", fmt.Errorf("tab %q not found", tabID)
	}
	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)
	if sourceLang == "" {
		sourceLang = "en"
	}

	content, _, err := container.OpenEntry(archivePath, entry)
	if err != nil {
		return "", fmt.Errorf("open %s!%s: %w", archivePath, entry, err)
	}
	// Detect from the entry name (+content fallback), not the archive.
	fmtName := a.DetectFormat(entry)
	if fmtName == "" {
		if det, derr := a.formatReg.Detector().Detect(entry, bytes.NewReader(content), ""); derr == nil {
			fmtName = det
		}
	}
	if fmtName == "" {
		return "", fmt.Errorf("could not detect a format for %q", entry)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reader, err := a.formatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return "", fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()
	if cfg, ok := reader.(project.Configurable); ok {
		if cerr := pctx.ConfigureReader(cfg, fmtName); cerr != nil {
			return "", fmt.Errorf("configure reader for %q: %w", fmtName, cerr)
		}
	}

	doc := &model.RawDocument{
		URI:          archivePath + "!" + entry,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return "", fmt.Errorf("open %s!%s: %w", archivePath, entry, err)
	}
	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return "", fmt.Errorf("read %s!%s: %w", archivePath, entry, pr.Error)
		}
		if pr.Part != nil {
			parts = append(parts, pr.Part)
		}
	}

	tree := editor.BuildContentTree(parts, fmtName)
	b, err := json.Marshal(tree)
	if err != nil {
		return "", fmt.Errorf("marshal content tree: %w", err)
	}
	return string(b), nil
}
