package sectionedit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"golang.org/x/net/html"
)

type htmlElement struct {
	tag      string
	text     string
	attrs    []html.Attribute
	start    int
	body     int
	close    int
	end      int
	parent   *htmlElement
	children []*htmlElement
}

type htmlSection struct {
	section Section
	body    int
	end     int
}

func inspectHTML(data []byte) ([]Section, error) {
	spans, err := htmlSections(data)
	if err != nil {
		return nil, err
	}
	sections := make([]Section, 0, len(spans))
	for _, span := range spans {
		sections = append(sections, span.section)
	}
	return sections, nil
}

func planHTML(data []byte, index int, fragment string) ([]Patch, error) {
	spans, err := htmlSections(data)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(spans) {
		return nil, errors.New("HTML section index is out of range")
	}
	var rendered bytes.Buffer
	// Goldmark's default renderer omits raw HTML. The shared API validates the
	// Markdown fragment before this adapter is called.
	if err := goldmark.Convert([]byte(fragment), &rendered); err != nil {
		return nil, err
	}
	span := spans[index]
	replacement := rendered.String()
	if replacement != "" {
		replacement = "\n" + replacement
	}
	return []Patch{{Start: span.body, End: span.end, Before: string(data[span.body:span.end]), Replacement: replacement}}, nil
}

func htmlSections(data []byte) ([]htmlSection, error) {
	root, err := parseHTMLSpans(data)
	if err != nil {
		return nil, err
	}
	headings := []*htmlElement{}
	var visit func(*htmlElement)
	visit = func(node *htmlElement) {
		if htmlHeadingLevel(node.tag) > 0 {
			headings = append(headings, node)
		}
		for _, child := range node.children {
			visit(child)
		}
	}
	visit(root)
	sections := make([]htmlSection, 0, len(headings))
	for _, heading := range headings {
		if !htmlSectionContainer(heading.parent.tag) {
			return nil, fmt.Errorf("unsupported heading container <%s>", heading.parent.tag)
		}
		level := htmlHeadingLevel(heading.tag)
		body := heading.end
		end := heading.parent.close
		children := []*htmlElement{}
		for _, sibling := range heading.parent.children {
			if sibling.start < body {
				continue
			}
			if next := htmlHeadingLevel(sibling.tag); next > 0 && next <= level {
				end = sibling.start
				break
			}
			children = append(children, sibling)
		}
		title, err := htmlHeadingText(heading.children)
		if err != nil {
			return nil, fmt.Errorf("heading projection: %w", err)
		}
		content, err := htmlBlocks(children)
		if err != nil {
			return nil, fmt.Errorf("section %q: %w", title, err)
		}
		sections = append(sections, htmlSection{section: Section{Title: strings.TrimSpace(title), Level: level, Path: htmlHeadingPath(heading), Content: content, headingStart: heading.start, bodyStart: body, bodyEnd: end}, body: body, end: end})
	}
	return sections, nil
}

// Strict token matching avoids HTML parser repairs changing section ownership.
// Optional closing tags and foreign content are deliberately unsupported.
func parseHTMLSpans(data []byte) (*htmlElement, error) {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("HTML must be valid UTF-8 without NUL bytes")
	}
	root := &htmlElement{tag: "", close: len(data), end: len(data), attrs: []html.Attribute{}, children: []*htmlElement{}}
	stack := []*htmlElement{root}
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	offset := 0
	for {
		kind := tokenizer.Next()
		start := offset
		offset += len(tokenizer.Raw())
		if kind == html.ErrorToken {
			if err := tokenizer.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			break
		}
		token := tokenizer.Token()
		parent := stack[len(stack)-1]
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "svg" || token.Data == "math" {
				return nil, errors.New("foreign HTML content is unsupported")
			}
			if kind == html.SelfClosingTagToken && !htmlVoid(token.Data) {
				return nil, fmt.Errorf("self-closing non-void <%s> is unsupported", token.Data)
			}
			node := &htmlElement{tag: token.Data, attrs: token.Attr, start: start, body: offset, close: offset, end: offset, parent: parent, children: []*htmlElement{}}
			parent.children = append(parent.children, node)
			if !htmlVoid(token.Data) {
				stack = append(stack, node)
			}
		case html.EndTagToken:
			if len(stack) == 1 || parent.tag != token.Data {
				return nil, fmt.Errorf("unmatched HTML closing tag </%s>", token.Data)
			}
			parent.close = start
			parent.end = offset
			stack = stack[:len(stack)-1]
		case html.TextToken:
			parent.children = append(parent.children, &htmlElement{tag: "#text", text: token.Data, start: start, end: offset, parent: parent, attrs: []html.Attribute{}, children: []*htmlElement{}})
		case html.CommentToken, html.DoctypeToken:
			parent.children = append(parent.children, &htmlElement{tag: "#opaque", start: start, end: offset, parent: parent, attrs: []html.Attribute{}, children: []*htmlElement{}})
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("unclosed HTML tag <%s>", stack[len(stack)-1].tag)
	}
	return root, nil
}

func htmlHeadingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 0
}

func htmlSectionContainer(tag string) bool {
	switch tag {
	case "", "body", "main", "article", "section", "div", "header", "footer", "nav", "aside":
		return true
	}
	return false
}

func htmlVoid(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func htmlBlocks(nodes []*htmlElement) (string, error) {
	var out strings.Builder
	for _, node := range nodes {
		var block string
		var err error
		switch {
		case node.tag == "#text":
			if strings.TrimSpace(node.text) != "" {
				return "", errors.New("unwrapped HTML body text is unsupported")
			}
			continue
		case node.tag == "p":
			block, err = htmlInline(node.children)
		case htmlHeadingLevel(node.tag) > 0:
			block, err = htmlInline(node.children)
			block = strings.Repeat("#", htmlHeadingLevel(node.tag)) + " " + block
		case node.tag == "ul" || node.tag == "ol":
			block, err = htmlList(node)
		case node.tag == "pre":
			block, err = htmlCodeBlock(node)
		default:
			return "", fmt.Errorf("unsupported HTML body element <%s>", node.tag)
		}
		if err != nil {
			return "", err
		}
		out.WriteString(block)
		out.WriteString("\n\n")
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func htmlInline(nodes []*htmlElement) (string, error) {
	var out strings.Builder
	for _, node := range nodes {
		switch node.tag {
		case "#text":
			out.WriteString(escapeMarkdownText(collapseHTMLSpace(node.text)))
		case "em", "i", "strong", "b", "a", "code", "br":
			value, err := htmlInlineElement(node)
			if err != nil {
				return "", err
			}
			out.WriteString(value)
		default:
			return "", fmt.Errorf("unsupported HTML inline element <%s>", node.tag)
		}

	}
	return out.String(), nil
}

func htmlInlineElement(node *htmlElement) (string, error) {
	if node.tag == "br" {
		return "  \n", nil
	}
	if node.tag == "code" {
		value, err := htmlPlainText(node.children)
		if err != nil {
			return "", err
		}
		fence := codeFence(value, "`")
		return fence + " " + value + " " + fence, nil
	}
	content, err := htmlInline(node.children)
	if err != nil {
		return "", err
	}
	switch node.tag {
	case "em", "i":
		return "*" + content + "*", nil
	case "strong", "b":
		return "**" + content + "**", nil
	case "a":
		href := htmlAttribute(node, "href")
		if href == "" || strings.ContainsAny(href, "\n\r<>") {
			return "", errors.New("HTML link needs a supported href")
		}
		title := ""
		if value := htmlAttribute(node, "title"); value != "" {
			title = " \"" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"", "\\\"") + "\""
		}
		return "[" + content + "](<" + href + ">" + title + ")", nil
	}
	return "", errors.New("unsupported inline projection")
}

func htmlPlainText(nodes []*htmlElement) (string, error) {
	var out strings.Builder
	for _, node := range nodes {
		if node.tag != "#text" {
			return "", fmt.Errorf("unexpected element <%s> in code", node.tag)
		}
		out.WriteString(node.text)
	}
	return out.String(), nil
}

func htmlCodeBlock(node *htmlElement) (string, error) {
	children := node.children
	language := ""
	if len(children) == 1 && children[0].tag == "code" {
		language = strings.TrimPrefix(htmlAttribute(children[0], "class"), "language-")
		if strings.ContainsAny(language, " \t\r\n`") {
			return "", errors.New("unsupported code language annotation")
		}
		children = children[0].children
	}
	value, err := htmlPlainText(children)
	if err != nil {
		return "", err
	}
	fence := codeFence(value, "```")
	return fence + language + "\n" + strings.TrimSuffix(value, "\n") + "\n" + fence, nil
}

func codeFence(value, minimum string) string {
	fence := minimum
	for strings.Contains(value, fence) {
		fence += "`"
	}
	return fence
}

func htmlList(node *htmlElement) (string, error) {
	lines := []string{}
	number := 1
	if start := htmlAttribute(node, "start"); start != "" {
		parsed, err := strconv.Atoi(start)
		if err != nil || parsed < 0 {
			return "", errors.New("unsupported ordered list start")
		}
		number = parsed
	}
	for _, item := range node.children {
		if item.tag == "#text" && strings.TrimSpace(item.text) == "" {
			continue
		}
		if item.tag != "li" {
			return "", errors.New("HTML list must contain explicit list items")
		}
		prefix := "- "
		if node.tag == "ol" {
			prefix = strconv.Itoa(number) + ". "
			number++
		}
		value, err := htmlListItem(item)
		if err != nil {
			return "", err
		}
		lines = append(lines, prefix+strings.ReplaceAll(value, "\n", "\n"+strings.Repeat(" ", len(prefix))))
	}
	return strings.Join(lines, "\n"), nil
}

func htmlListItem(item *htmlElement) (string, error) {
	parts := []string{}
	inline := []*htmlElement{}
	flush := func() error {
		if len(inline) == 0 {
			return nil
		}
		value, err := htmlInline(inline)
		if err != nil {
			return err
		}
		parts = append(parts, strings.TrimSpace(value))
		inline = []*htmlElement{}
		return nil
	}
	for _, child := range item.children {
		if child.tag == "p" || child.tag == "ul" || child.tag == "ol" || child.tag == "pre" {
			if err := flush(); err != nil {
				return "", err
			}
			value, err := htmlBlocks([]*htmlElement{child})
			if err != nil {
				return "", err
			}
			parts = append(parts, value)
		} else {
			inline = append(inline, child)
		}
	}
	if err := flush(); err != nil {
		return "", err
	}
	return strings.Join(parts, "\n\n"), nil
}

func htmlAttribute(node *htmlElement, name string) string {
	for _, attr := range node.attrs {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func escapeMarkdownText(value string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[",
		"]", "\\]", "-", "\\-", "+", "\\+", ".", "\\.", "=", "\\=", ")", "\\)", "(", "\\(", "`", "\\`", "<", "\\<", ">", "\\>", "#", "\\#", "!", "\\!").Replace(value)
}

func collapseHTMLSpace(value string) string {
	if value == "" {
		return ""
	}
	result := strings.Join(strings.Fields(value), " ")
	if result == "" {
		return " "
	}
	if strings.ContainsAny(value[:1], " \t\r\n") {
		result = " " + result
	}
	if strings.ContainsAny(value[len(value)-1:], " \t\r\n") {
		result += " "
	}
	return result
}

func htmlHeadingPath(heading *htmlElement) []string {
	path := []string{}
	levels := []int{}
	for _, sibling := range heading.parent.children {
		level := htmlHeadingLevel(sibling.tag)
		if level == 0 {
			continue
		}
		title, _ := htmlHeadingText(sibling.children)
		for len(levels) > 0 && levels[len(levels)-1] >= level {
			levels = levels[:len(levels)-1]
			path = path[:len(path)-1]
		}
		levels = append(levels, level)
		path = append(path, strings.TrimSpace(title))
		if sibling == heading {
			return path
		}
	}
	return path
}

func htmlHeadingText(nodes []*htmlElement) (string, error) {
	var out strings.Builder
	for _, node := range nodes {
		if node.tag == "#text" {
			out.WriteString(collapseHTMLSpace(node.text))
			continue
		}
		switch node.tag {
		case "em", "i", "strong", "b", "a", "code":
			value, err := htmlHeadingText(node.children)
			if err != nil {
				return "", err
			}
			out.WriteString(value)
		case "br":
			out.WriteByte(' ')
		default:
			return "", fmt.Errorf("unsupported HTML heading element <%s>", node.tag)
		}
	}
	return out.String(), nil
}
