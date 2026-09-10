package sectionedit

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const wordNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const docxXMLLimit = 32 << 20

// Offsets refer to the original XML bytes, never to a reserialization of its tree.
type docxNode struct {
	name                xml.Name
	attrs               []xml.Attr
	children            []*docxNode
	text                string
	start, end, closing int
}

func (n *docxNode) child(name string) *docxNode {
	if n == nil {
		return nil
	}
	for _, child := range n.children {
		if child.name.Space == wordNamespace && child.name.Local == name {
			return child
		}
	}
	return nil
}

func (n *docxNode) attr(name string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.attrs {
		if attr.Name.Space == wordNamespace && attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func parseDOCXXML(data []byte) (*docxNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	root := &docxNode{children: []*docxNode{}}
	stack := []*docxNode{root}
	for {
		start := int(decoder.InputOffset())
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("DOCX XML: %w", err)
		}
		parent := stack[len(stack)-1]
		switch token := token.(type) {
		case xml.StartElement:
			if len(stack) > 256 {
				return nil, errors.New("DOCX XML nesting exceeds limit")
			}
			node := &docxNode{name: token.Name, attrs: token.Attr, start: start, children: []*docxNode{}}
			parent.children = append(parent.children, node)
			stack = append(stack, node)
		case xml.EndElement:
			parent.end, parent.closing = int(decoder.InputOffset()), start
			stack = stack[:len(stack)-1]
		case xml.CharData:
			parent.text += string(token)
		case xml.Directive:
			return nil, errors.New("DOCX XML directives are unsupported")
		}
	}
	if len(root.children) != 1 {
		return nil, errors.New("DOCX XML must have one root element")
	}
	return root.children[0], nil
}

type docxDocument struct {
	raw       []byte
	body      *docxNode
	styles    map[string]*docxNode
	numbering *docxNode
	sections  []docxSection
}

type docxSection struct {
	heading                         *docxNode
	level, first, limit, start, end int
}

func readDOCX(data []byte) (*docxDocument, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("DOCX package: %w", err)
	}
	parts := map[string][]byte{}
	seen := map[string]bool{}
	for _, file := range archive.File {
		if seen[file.Name] {
			return nil, fmt.Errorf("duplicate DOCX entry %q", file.Name)
		}
		seen[file.Name] = true
		switch file.Name {
		case "word/document.xml", "word/styles.xml", "word/numbering.xml", "word/_rels/document.xml.rels":
			content, err := readDOCXPart(file)
			if err != nil {
				return nil, err
			}
			parts[file.Name] = content
		}
	}
	root, err := parseDOCXXML(parts["word/document.xml"])
	if err != nil {
		return nil, err
	}
	body := root.child("body")
	if root.name.Space != wordNamespace || root.name.Local != "document" || body == nil {
		return nil, errors.New("DOCX requires a WordprocessingML document body")
	}
	doc := &docxDocument{
		raw: parts["word/document.xml"], body: body,
		styles: map[string]*docxNode{}, sections: []docxSection{},
	}
	if data, ok := parts["word/styles.xml"]; ok {
		styles, err := parseDOCXXML(data)
		if err != nil {
			return nil, err
		}
		for _, style := range styles.children {
			if style.attr("type") == "paragraph" || style.attr("type") == "" {
				doc.styles[style.attr("styleId")] = style
			}
		}
	}
	numberingBound, err := docxNumberingBound(parts["word/_rels/document.xml.rels"])
	if err != nil {
		return nil, err
	}
	if data, ok := parts["word/numbering.xml"]; ok && numberingBound {
		doc.numbering, err = parseDOCXXML(data)
		if err != nil {
			return nil, err
		}
	}
	doc.findSections()
	return doc, nil
}

func readDOCXPart(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > docxXMLLimit {
		return nil, errors.New("DOCX XML part exceeds size limit")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, docxXMLLimit+1))
	if len(data) > docxXMLLimit {
		return nil, errors.New("DOCX XML part exceeds size limit")
	}
	return data, err
}

func outlineLevel(props *docxNode) (int, bool) {
	outline := props.child("outlineLvl")
	if outline == nil {
		return 0, false
	}
	level, err := strconv.Atoi(outline.attr("val"))
	if err != nil || level < 0 || level > 8 {
		return 0, true
	}
	return level + 1, true
}

func namedHeading(name string) int {
	name = strings.ToLower(strings.ReplaceAll(name, " ", ""))
	if len(name) == 8 && strings.HasPrefix(name, "heading") && name[7] >= '1' && name[7] <= '9' {
		return int(name[7] - '0')
	}
	return 0
}

func (doc *docxDocument) headingLevel(paragraph *docxNode) int {
	if paragraph.name.Space != wordNamespace || paragraph.name.Local != "p" {
		return 0
	}
	props := paragraph.child("pPr")
	if level, found := outlineLevel(props); found {
		return level
	}
	id := props.child("pStyle").attr("val")
	visited := map[string]bool{}
	fallback := namedHeading(id)
	for id != "" && !visited[id] {
		visited[id] = true
		style := doc.styles[id]
		if style == nil {
			break
		}
		if level, found := outlineLevel(style.child("pPr")); found {
			return level
		}
		if fallback == 0 {
			fallback = namedHeading(style.child("name").attr("val"))
		}
		id = style.child("basedOn").attr("val")
		if fallback == 0 {
			fallback = namedHeading(id)
		}
	}
	return fallback
}

func (doc *docxDocument) findSections() {
	for i, node := range doc.body.children {
		level := doc.headingLevel(node)
		if level == 0 {
			continue
		}
		section := docxSection{heading: node, level: level, first: i + 1,
			limit: len(doc.body.children), start: node.end, end: doc.body.closing}
		for j := i + 1; j < len(doc.body.children); j++ {
			next := doc.body.children[j]
			nextLevel := doc.headingLevel(next)
			boundary := next.name.Local == "sectPr" || (nextLevel > 0 && nextLevel <= level)
			if boundary {
				section.limit, section.end = j, next.start
				break
			}
		}
		doc.sections = append(doc.sections, section)
	}
}

func inspectDOCX(data []byte) ([]Section, error) {
	doc, err := readDOCX(data)
	if err != nil {
		return nil, err
	}
	sections := []Section{}
	for _, section := range doc.sections {
		blocks := []string{}
		for _, node := range doc.body.children[section.first:section.limit] {
			blocks = append(blocks, doc.projectParagraph(node))
		}
		sections = append(sections, Section{Title: docxText(section.heading), Level: section.level,
			Content:      strings.TrimSpace(strings.Join(blocks, "\n\n")),
			headingStart: section.heading.start, bodyStart: section.start, bodyEnd: section.end,
			sourcePart: "word/document.xml"})
	}
	return sections, nil
}

func planDOCX(data []byte, index int, fragment string) ([]Patch, error) {
	doc, err := readDOCX(data)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(doc.sections) {
		return nil, errors.New("DOCX section index out of range")
	}
	section := doc.sections[index]
	if err := doc.safeRange(section); err != nil {
		return nil, err
	}
	writer := docxMarkdown{doc: doc, source: []byte(fragment)}
	root := goldmark.New().Parser().Parse(text.NewReader(writer.source))
	if err := writer.blocks(root, -1); err != nil {
		return nil, err
	}
	return []Patch{{Entry: "word/document.xml", Start: section.start, End: section.end,
		Before: string(doc.raw[section.start:section.end]), Replacement: writer.output.String()}}, nil
}

// Any range marker overlapping the splice is rejected, including markers whose
// start and end both lie outside it. Keeping only one end creates dangling IDs.
func (doc *docxDocument) safeRange(section docxSection) error {
	active := map[string]int{}
	fields := []int{}
	var visit func(*docxNode) error
	visit = func(node *docxNode) error {
		name := node.name.Local
		if node.name.Space == wordNamespace {
			if name == "fldChar" {
				switch node.attr("fldCharType") {
				case "begin":
					fields = append(fields, node.start)
				case "end":
					if len(fields) > 0 {
						start := fields[len(fields)-1]
						fields = fields[:len(fields)-1]
						if start < section.end && node.end > section.start {
							return errors.New("DOCX section overlaps a complex field")
						}
					}
				}
			}
			if stem, found := strings.CutSuffix(name, "Start"); found {
				active[stem+":"+node.attr("id")] = node.start
			}
			if stem, found := strings.CutSuffix(name, "End"); found {
				key := stem + ":" + node.attr("id")
				start, found := active[key]
				if found && start < section.end && node.end > section.start {
					return fmt.Errorf("DOCX section overlaps unsupported %s range", name)
				}
				delete(active, key)
			}
		}
		for _, child := range node.children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(doc.body); err != nil {
		return err
	}
	for _, start := range fields {
		if start < section.end {
			return errors.New("DOCX section follows an unclosed complex field")
		}
	}
	for _, start := range active {
		if start < section.end {
			return errors.New("DOCX section follows an unclosed range marker")
		}
	}
	for _, node := range doc.body.children[section.first:section.limit] {
		if err := safeDOCXParagraph(node); err != nil {
			return err
		}
	}
	return nil
}

func safeDOCXParagraph(node *docxNode) error {
	if node.name.Space != wordNamespace || node.name.Local != "p" {
		return fmt.Errorf("DOCX section contains unsupported block %q", node.name.Local)
	}
	var visit func(*docxNode, bool) error
	visit = func(node *docxNode, properties bool) error {
		name := node.name.Local
		revision := name == "ins" || name == "del" || name == "moveFrom" || name == "moveTo"
		unsafeProperty := name == "sectPr" || strings.HasSuffix(name, "Change") || strings.Contains(name, "Range")
		unsafeProperty = unsafeProperty || revision
		if unsafeProperty {
			return fmt.Errorf("DOCX section contains unsupported %q", name)
		}
		if node.name.Space != wordNamespace {
			return fmt.Errorf("DOCX section contains foreign XML %q", name)
		}
		if !properties {
			switch name {
			case "p", "r", "t", "br", "cr", "tab":
			case "pPr", "rPr":
				properties = true
			default:
				return fmt.Errorf("DOCX section contains unsupported %q", name)
			}
		}
		for _, child := range node.children {
			if err := visit(child, properties); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(node, false)
}

func docxText(node *docxNode) string {
	if node == nil {
		return ""
	}
	if node.name.Space == wordNamespace {
		switch node.name.Local {
		case "t":
			return node.text
		case "tab":
			return "\t"
		case "br", "cr":
			return "\n"
		}
	}
	var out strings.Builder
	for _, child := range node.children {
		out.WriteString(docxText(child))
	}
	return out.String()
}

func docxEnabled(node *docxNode) bool {
	if node == nil {
		return false
	}
	value := node.attr("val")
	return value != "0" && value != "false" && value != "off"
}

func (doc *docxDocument) projectParagraph(node *docxNode) string {
	if err := safeDOCXParagraph(node); err != nil {
		return "[Unsupported DOCX structure: " + node.name.Local + "] " + docxText(node)
	}
	var out strings.Builder
	allCode, hasRuns := true, false
	for _, run := range node.children {
		if run.name.Local == "r" {
			hasRuns = true
			allCode = allCode && docxMono(run)
		}
	}
	if hasRuns && allCode && doc.headingLevel(node) == 0 && node.child("pPr").child("numPr") == nil {
		value := docxText(node)
		fence := docxFence(value, 3)
		return fence + "\n" + value + "\n" + fence
	}
	for _, run := range node.children {
		if run.name.Local != "r" {
			continue
		}
		value := docxMarkdownText(docxText(run))
		if docxMono(run) {
			raw := docxText(run)
			fence := docxFence(raw, 1)
			value = fence + " " + raw + " " + fence
		}
		props := run.child("rPr")
		if docxEnabled(props.child("b")) {
			value = "**" + value + "**"
		}
		if docxEnabled(props.child("i")) {
			value = "*" + value + "*"
		}
		out.WriteString(value)
	}
	value := out.String()
	if level := doc.headingLevel(node); level > 0 {
		return strings.Repeat("#", level) + " " + value
	}
	num := node.child("pPr").child("numPr")
	if num != nil {
		depth, _ := strconv.Atoi(num.child("ilvl").attr("val"))
		if depth < 0 || depth > 8 {
			depth = 0
		}
		prefix := "1. "
		if doc.numberFormat(num.child("numId").attr("val"), depth) == "bullet" {
			prefix = "- "
		}
		return strings.Repeat("  ", depth) + prefix + value
	}
	return value
}

func (doc *docxDocument) numberFormat(id string, depth int) string {
	if doc.numbering == nil {
		return ""
	}
	abstractID := ""
	for _, num := range doc.numbering.children {
		if num.name.Local == "num" && num.attr("numId") == id {
			if num.child("lvlOverride") != nil {
				return ""
			}
			abstractID = num.child("abstractNumId").attr("val")
		}
	}
	if abstractID == "" {
		return ""
	}
	for _, abstract := range doc.numbering.children {
		if abstract.name.Local != "abstractNum" || abstract.attr("abstractNumId") != abstractID {
			continue
		}
		for _, level := range abstract.children {
			if level.name.Local == "lvl" && level.attr("ilvl") == strconv.Itoa(depth) {
				return level.child("numFmt").attr("val")
			}
		}
	}
	return ""
}

func (doc *docxDocument) bulletID(depth int) (string, error) {
	if doc.numbering != nil {
		for _, num := range doc.numbering.children {
			id := num.attr("numId")
			if id != "" && id != "0" && doc.numberFormat(id, depth) == "bullet" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("DOCX replacement requires existing bullet numbering at depth %d", depth)
}

type docxMarkdown struct {
	doc    *docxDocument
	source []byte
	output strings.Builder
}

func docxEscape(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func (writer *docxMarkdown) blocks(parent ast.Node, depth int) error {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		switch node := node.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			if err := writer.paragraph(node, ""); err != nil {
				return err
			}
		case *ast.Heading:
			props := `<w:pStyle w:val="Heading` + strconv.Itoa(node.Level) + `"/><w:outlineLvl w:val="` + strconv.Itoa(node.Level-1) + `"/>`
			if err := writer.paragraph(node, props); err != nil {
				return err
			}
		case *ast.List:
			if node.IsOrdered() {
				return errors.New("DOCX ordered-list replacement requires numbering management")
			}
			if err := writer.list(node, depth+1); err != nil {
				return err
			}
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			var code strings.Builder
			for i := range node.Lines().Len() {
				line := node.Lines().At(i)
				code.Write(line.Value(writer.source))
			}
			writer.openParagraph("")
			writer.run(strings.TrimSuffix(code.String(), "\n"), `<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/>`)
			writer.output.WriteString("</w:p>")
		default:
			return fmt.Errorf("unsupported DOCX replacement block %s", node.Kind())
		}
	}
	return nil
}

func (writer *docxMarkdown) list(list *ast.List, depth int) error {
	id, err := writer.doc.bulletID(depth)
	if err != nil {
		return err
	}
	props := `<w:numPr><w:ilvl w:val="` + strconv.Itoa(depth) + `"/><w:numId w:val="` + docxEscape(id) + `"/></w:numPr>`
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		first := true
		for block := item.FirstChild(); block != nil; block = block.NextSibling() {
			switch block := block.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				paragraphProps := ""
				if first {
					paragraphProps = props
				}
				if err := writer.paragraph(block, paragraphProps); err != nil {
					return err
				}
				first = false
			case *ast.List:
				if block.IsOrdered() {
					return errors.New("DOCX ordered-list replacement requires numbering management")
				}
				if err := writer.list(block, depth+1); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported DOCX list block %s", block.Kind())
			}
		}
	}
	return nil
}

func (writer *docxMarkdown) openParagraph(props string) {
	writer.output.WriteString(`<w:p xmlns:w="` + wordNamespace + `">`)
	if props != "" {
		writer.output.WriteString("<w:pPr>" + props + "</w:pPr>")
	}
}

func (writer *docxMarkdown) paragraph(node ast.Node, props string) error {
	writer.openParagraph(props)
	if err := writer.inlines(node, ""); err != nil {
		return err
	}
	writer.output.WriteString("</w:p>")
	return nil
}

func (writer *docxMarkdown) inlines(parent ast.Node, props string) error {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch child := child.(type) {
		case *ast.Text:
			value := child.Segment.Value(writer.source)
			if !strings.Contains(props, "rFonts") {
				value = util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(value)))
			}
			writer.run(string(value), props)
			if child.HardLineBreak() {
				writer.output.WriteString("<w:r><w:br/></w:r>")
			}
			if child.SoftLineBreak() {
				writer.run(" ", props)
			}
		case *ast.String:
			writer.run(string(child.Value), props)
		case *ast.Emphasis:
			mark := "<w:i/>"
			if child.Level == 2 {
				mark = "<w:b/>"
			}
			if err := writer.inlines(child, props+mark); err != nil {
				return err
			}
		case *ast.CodeSpan:
			if err := writer.inlines(child, props+`<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/>`); err != nil {
				return err
			}
		case *ast.Link:
			return errors.New("DOCX linked replacement requires relationship management")
		default:
			return fmt.Errorf("unsupported DOCX replacement inline %s", child.Kind())
		}
	}
	return nil
}

func (writer *docxMarkdown) run(value, props string) {
	writer.output.WriteString("<w:r>")
	if props != "" {
		writer.output.WriteString("<w:rPr>" + props + "</w:rPr>")
	}
	for i, line := range strings.Split(value, "\n") {
		if i > 0 {
			writer.output.WriteString("<w:br/>")
		}
		writer.output.WriteString(`<w:t xml:space="preserve">` + docxEscape(line) + `</w:t>`)
	}
	writer.output.WriteString("</w:r>")
}

func docxMono(run *docxNode) bool {
	font := run.child("rPr").child("rFonts").attr("ascii")
	return font == "Courier New" || font == "Consolas"
}

func docxFence(value string, minimum int) string {
	longest, current := 0, 0
	for _, char := range value {
		if char == '`' {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	return strings.Repeat("`", max(minimum, longest+1))
}

func docxMarkdownText(value string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>").Replace(value)
}

// Numbering is reusable only when it belongs to the document's relationship
// graph. An orphan numbering.xml member does not define document list IDs.
func docxNumberingBound(data []byte) (bool, error) {
	if len(data) == 0 {
		return false, nil
	}
	root, err := parseDOCXXML(data)
	if err != nil {
		return false, err
	}
	for _, rel := range root.children {
		attrs := map[string]string{}
		for _, attr := range rel.attrs {
			attrs[attr.Name.Local] = attr.Value
		}
		target := attrs["Target"]
		localTarget := target == "numbering.xml" || target == "/word/numbering.xml"
		if attrs["Type"] == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" &&
			localTarget && attrs["TargetMode"] != "External" {
			return true, nil
		}
	}
	return false, nil
}
