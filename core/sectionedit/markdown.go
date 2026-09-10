package sectionedit

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type markdownSection struct {
	section Section
	start   int
	body    int
	end     int
}

func inspectMarkdown(data []byte) ([]Section, error) {
	spans, err := markdownSections(data)
	if err != nil {
		return nil, err
	}
	sections := make([]Section, 0, len(spans))
	for _, span := range spans {
		sections = append(sections, span.section)
	}
	return sections, nil
}

func planMarkdown(data []byte, index int, fragment string) ([]Patch, error) {
	spans, err := markdownSections(data)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(spans) {
		return nil, errors.New("markdown section index is out of range")
	}
	span := spans[index]
	replacement := strings.Trim(fragment, "\r\n")
	if replacement != "" {
		replacement = "\n" + replacement + "\n\n"
	}
	// Preserve the document's line ending convention within the new body too.
	if bytes.Contains(data, []byte("\r\n")) {
		replacement = strings.ReplaceAll(strings.ReplaceAll(replacement, "\r\n", "\n"), "\n", "\r\n")
	}
	// A final heading without a newline needs a terminator before its new body.
	if replacement != "" && span.body > 0 && data[span.body-1] != '\n' {
		replacement = "\n" + replacement
	}
	return []Patch{{Start: span.body, End: span.end, Before: string(data[span.body:span.end]), Replacement: replacement}}, nil
}

func markdownSections(data []byte) ([]markdownSection, error) {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("markdown must be valid UTF-8 without NUL bytes")
	}
	document := goldmark.DefaultParser().Parse(text.NewReader(data))
	spans := []markdownSection{}
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok {
			continue
		}
		start := lineStart(data, heading.Pos())
		body := lineEnd(data, start)
		if heading.Lines().Len() > 0 {
			last := heading.Lines().At(heading.Lines().Len() - 1)
			body = lineEnd(data, max(last.Start, last.Stop-1))
		}
		// Setext's underline is not part of the AST heading text segments.
		if !markdownATXLine(data[start:lineEnd(data, start)]) {
			if body >= len(data) {
				return nil, fmt.Errorf("missing Setext underline at byte %d", start)
			}
			body = lineEnd(data, body)
		}
		title := markdownHeadingTitle(heading, data)
		spans = append(spans, markdownSection{section: Section{Title: title, Level: heading.Level, headingStart: start, bodyStart: body}, start: start, body: body, end: len(data)})
	}
	for index := range spans {
		for next := index + 1; next < len(spans); next++ {
			if spans[next].section.Level <= spans[index].section.Level {
				spans[index].end = spans[next].start
				break
			}
		}
		spans[index].section.bodyEnd = spans[index].end
		spans[index].section.Content = string(data[spans[index].body:spans[index].end])
	}
	return spans, nil
}

func lineStart(data []byte, offset int) int {
	if offset <= 0 {
		return 0
	}
	return bytes.LastIndexByte(data[:min(offset, len(data))], '\n') + 1
}

func lineEnd(data []byte, offset int) int {
	if end := bytes.IndexByte(data[offset:], '\n'); end >= 0 {
		return offset + end + 1
	}
	return len(data)
}

func markdownATXLine(line []byte) bool {
	line = bytes.TrimLeft(line, " \t")
	count := 0
	for count < len(line) && line[count] == '#' {
		count++
	}
	if count == 0 || count > 6 {
		return false
	}
	return count == len(line) || strings.ContainsAny(string(line[count:count+1]), " \t\r\n")
}

func markdownHeadingTitle(heading *ast.Heading, data []byte) string {
	var title strings.Builder
	_ = ast.Walk(heading, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch item := node.(type) {
		case *ast.Text:
			title.Write(item.Value(data))
			if item.SoftLineBreak() || item.HardLineBreak() {
				title.WriteByte(' ')
			}
		case *ast.String:
			title.Write(item.Value)
		case *ast.AutoLink:
			title.Write(item.Label(data))
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(title.String())
}
