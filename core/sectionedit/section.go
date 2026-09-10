// Package sectionedit provides guarded heading-section edits for Markdown,
// HTML and DOCX. Reading projections help an editor author a replacement;
// adapters splice that replacement into the original source document.
package sectionedit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ErrStale means the source no longer matches the inspected document.
var ErrStale = format.ErrPatchStale

// Patch describes an immutable offset replacement consumed by the format writer.
type Patch = format.OffsetPatch

// Plan binds offset patches to the inspected source.
type Plan = format.OffsetPatchPlan

// Section identifies a heading and its body, including subordinate headings.
// Content is a Markdown reading projection, not a faithful source serialization.
// IDs are meaningful only together with the containing document's snapshot.
type Section struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Level        int              `json:"level"`
	Path         []string         `json:"path"`
	Content      string           `json:"content"`
	Range        model.BlockRange `json:"range"`
	headingStart int
	bodyStart    int
	bodyEnd      int
	sourcePart   string
}

// Document is the section reading surface. File is supplied by the host.
type Document struct {
	File          string    `json:"file,omitempty"`
	Format        string    `json:"format"`
	Snapshot      string    `json:"snapshot"`
	ContentFormat string    `json:"content_format"`
	Sections      []Section `json:"sections"`
}

// Edit replaces one section body. Text is a Markdown fragment; the selected
// heading remains in place. Snapshot and ID must come from the same inspection.
type Edit struct {
	ID       string `json:"id"`
	Snapshot string `json:"snapshot"`
	Text     string `json:"text"`
}

// Prepared contains a validated replacement and a readable before/after view.
// Preparing does not write files. The host rechecks the snapshot before writing.
type Prepared struct {
	Before   Section `json:"before"`
	After    Section `json:"after"`
	Snapshot string  `json:"snapshot"`
	Plan     Plan    `json:"plan"`
	Data     []byte  `json:"-"`
}

// Snapshot fingerprints the entire source, including a DOCX package's assets.
func Snapshot(data []byte) string {
	return format.SourceSnapshot(data)
}

// FormatForFile resolves the formats supported by the section-edit POC.
func FormatForFile(file string) (string, error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".md", ".markdown":
		return "markdown", nil
	case ".html", ".htm":
		return "html", nil
	case ".docx":
		return "docx", nil
	default:
		return "", fmt.Errorf("section editing supports .md, .markdown, .html, .htm and .docx: %q", file)
	}
}

// Inspect returns heading sections in source order, including nested sections.
func Inspect(ctx context.Context, format string, data []byte) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	var sections []Section
	var err error
	switch format {
	case "markdown":
		sections, err = inspectMarkdown(data)
	case "html":
		sections, err = inspectHTML(data)
	case "docx":
		sections, err = inspectDOCX(data)
	default:
		return Document{}, fmt.Errorf("unsupported section format %q", format)
	}
	if err != nil {
		return Document{}, err
	}
	if sections == nil {
		sections = []Section{}
	}
	if err := bindNativeRanges(ctx, format, data, sections); err != nil {
		return Document{}, err
	}
	for i := range sections {
		// Adapters can provide a container-aware path (HTML headings may live
		// in separate containers). Otherwise heading levels define ancestry.
		if sections[i].Path != nil {
			continue
		}
		sections[i].Path = []string{sections[i].Title}
		for j := i - 1; j >= 0; j-- {
			if sections[j].Level < sections[i].Level {
				sections[i].Path = append(append([]string{}, sections[j].Path...), sections[i].Title)
				break
			}
		}
	}
	return Document{
		Format: format, Snapshot: Snapshot(data), ContentFormat: "markdown", Sections: sections,
	}, nil
}

// Prepare validates an exact section target and renders its replacement in
// memory. Unsupported fragment or source structures return an error.
func Prepare(ctx context.Context, format string, data []byte, edit Edit) (Prepared, error) {
	if edit.Snapshot == "" || edit.ID == "" {
		return Prepared{}, errors.New("section edits require id and snapshot from inspect --sections")
	}
	if edit.Snapshot != Snapshot(data) {
		return Prepared{}, ErrStale
	}
	doc, err := Inspect(ctx, format, data)
	if err != nil {
		return Prepared{}, err
	}
	index := -1
	for i, section := range doc.Sections {
		if section.ID == edit.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return Prepared{}, fmt.Errorf("unknown section id %q; inspect again", edit.ID)
	}
	if err := validateFragment(edit.Text, doc.Sections[index].Level); err != nil {
		return Prepared{}, err
	}
	var patches []Patch
	switch format {
	case "markdown":
		patches, err = planMarkdown(data, index, edit.Text)
	case "html":
		patches, err = planHTML(data, index, edit.Text)
	case "docx":
		patches, err = planDOCX(data, index, edit.Text)
	}
	if err != nil {
		return Prepared{}, err
	}
	plan := Plan{Format: format, Snapshot: edit.Snapshot, Patches: patches, Range: &doc.Sections[index].Range}
	updated, err := applyPlan(data, plan)
	if err != nil {
		return Prepared{}, err
	}
	after, err := Inspect(ctx, format, updated)
	if err != nil {
		return Prepared{}, fmt.Errorf("replacement could not be inspected: %w", err)
	}
	if len(after.Sections) <= index {
		return Prepared{}, errors.New("replacement lost selected heading")
	}
	selected := after.Sections[index]
	if selected.Title != doc.Sections[index].Title || selected.Level != doc.Sections[index].Level {
		return Prepared{}, errors.New("replacement changed selected heading")
	}
	return Prepared{
		Before: doc.Sections[index], After: selected, Snapshot: after.Snapshot, Plan: plan, Data: updated,
	}, nil
}

func validateFragment(fragment string, level int) error {
	if strings.TrimSpace(fragment) == "" {
		return errors.New("section text must contain a nonempty Markdown body")
	}
	if !utf8.ValidString(fragment) {
		return errors.New("section text must be UTF-8")
	}
	for _, r := range fragment {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return errors.New("section text contains a control character")
		}
	}
	source := []byte(fragment)
	parseContext := parser.NewContext()
	mdParser := goldmark.New().Parser()
	doc := mdParser.Parse(text.NewReader(source), parser.WithContext(parseContext))
	if len(parseContext.References()) != 0 {
		return errors.New("section fragments cannot define document-wide link references; use inline links")
	}
	// An unfinished fence must not consume an untouched following section.
	// A boundary heading stays visible after every complete supported fragment.
	const boundary = "kapi-fragment-boundary"
	guarded := []byte(fragment + "\n\n# " + boundary + "\n")
	boundaryDoc := mdParser.Parse(text.NewReader(guarded))
	last, ok := boundaryDoc.LastChild().(*ast.Heading)
	if !ok || markdownHeadingTitle(last, guarded) != boundary {
		return errors.New("section fragment has an unclosed construct that crosses its boundary")
	}
	return ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case ast.KindDocument, ast.KindParagraph, ast.KindTextBlock, ast.KindText,
			ast.KindString, ast.KindCodeSpan, ast.KindEmphasis, ast.KindLink,
			ast.KindList, ast.KindListItem, ast.KindFencedCodeBlock, ast.KindCodeBlock:
		case ast.KindHeading:
			if node.(*ast.Heading).Level <= level {
				return ast.WalkStop, errors.New("fragment headings must be below the preserved section heading")
			}
		default:
			return ast.WalkStop, fmt.Errorf("section fragment does not support %s", node.Kind())
		}
		if link, ok := node.(*ast.Link); ok {
			destination := strings.ToLower(strings.TrimSpace(string(link.Destination)))
			if strings.Contains(destination, ":") && !strings.HasPrefix(destination, "https:") &&
				!strings.HasPrefix(destination, "http:") && !strings.HasPrefix(destination, "mailto:") {
				return ast.WalkStop, errors.New("section fragment link uses an unsupported URL scheme")
			}
		}
		return ast.WalkContinue, nil
	})
}

func applyPlan(data []byte, plan Plan) ([]byte, error) {
	return format.ApplyOffsetPatches(data, plan)
}
