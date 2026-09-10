package sectionedit

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func docxTestArchive(t *testing.T, document string) []byte {
	t.Helper()
	parts := map[string]string{
		"_rels/.rels":                  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rDoc" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"[Content_Types].xml":          `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/></Types>`,
		"word/document.xml":            document,
		"word/styles.xml":              `<w:styles xmlns:w="` + wordNamespace + `"><w:style w:type="paragraph" w:styleId="SectionBase"><w:pPr><w:outlineLvl w:val="1"/></w:pPr></w:style><w:style w:type="paragraph" w:styleId="PreviewSection"><w:basedOn w:val="SectionBase"/></w:style><w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/></w:style></w:styles>`,
		"word/numbering.xml":           `<w:numbering xmlns:w="` + wordNamespace + `"><w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/></w:lvl><w:lvl w:ilvl="1"><w:numFmt w:val="bullet"/><w:lvlText w:val="◦"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rNum" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/><Relationship Id="rStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"custom/preserve.bin":          "\x00\xffpreserve\x00",
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range parts {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func docxTestDocument(body string) string {
	return `<?xml version="1.0"?><w:document xmlns:w="` + wordNamespace + `"><w:body>` + body +
		`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr></w:body></w:document>`
}

func docxTestHeading(title, style string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r><w:t>` + title + `</w:t></w:r></w:p>`
}

func TestDOCXHeadingFreeFixture(t *testing.T) {
	data, err := os.ReadFile("../formats/openxml/testdata/simple.docx")
	if err != nil {
		t.Fatal(err)
	}
	sections, err := inspectDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	if sections == nil || len(sections) != 0 {
		t.Fatalf("heading-free fixture sections: %+v", sections)
	}
	if _, err := planDOCX(data, 0, "new body"); err == nil {
		t.Fatal("heading-free edit accepted")
	}
}

func TestDOCXBodyPatchAndNativeBlocks(t *testing.T) {
	prefix := `<w:p><w:r><w:t>Preamble</w:t></w:r></w:p>` + docxTestHeading("Share a preview", "PreviewSection")
	oldBody := `  <w:p><w:r><w:t>Old body</w:t></w:r></w:p>` + docxTestHeading("Old nested", "Heading3") +
		`<w:p><w:r><w:t>Nested body</w:t></w:r></w:p>  `
	suffix := docxTestHeading("Unrelated section", "PreviewSection") + `<w:p><w:r><w:t>Keep me</w:t></w:r></w:p>`
	data := docxTestArchive(t, docxTestDocument(prefix+oldBody+suffix))
	before := bytes.Clone(data)
	sections, err := inspectDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 3 || sections[0].Level != 2 || sections[0].Title != "Share a preview" {
		t.Fatalf("custom inherited heading not recognized: %+v", sections)
	}
	if !strings.Contains(sections[0].Content, "### Old nested") {
		t.Fatal(sections[0].Content)
	}
	fragment := "A **bold** and *gentle* paragraph with `code`.\n\n### Check the preview\n\n- First\n  - Nested\n- Second\n\n```bash\nnpm run build\nnpm run preview\n```\n\nLiteral \\* and &amp; stay text."
	patches, err := planDOCX(data, 0, fragment)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, before) {
		t.Fatal("planning mutated input archive")
	}
	if len(patches) != 1 {
		t.Fatalf("patches: %+v", patches)
	}
	patch := patches[0]
	if patch.Entry != "word/document.xml" || patch.Before != oldBody {
		t.Fatalf("wrong splice: %+v", patch)
	}
	doc, err := readDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	changed := string(doc.raw[:patch.Start]) + patch.Replacement + string(doc.raw[patch.End:])
	if !strings.HasPrefix(changed, string(doc.raw[:patch.Start])) || !strings.HasSuffix(changed, string(doc.raw[patch.End:])) {
		t.Fatal("unrelated XML changed")
	}
	if _, err := parseDOCXXML([]byte(changed)); err != nil {
		t.Fatal(err)
	}
	for _, native := range []string{"<w:b/>", "<w:i/>", `<w:ilvl w:val="1"/>`, `<w:numId w:val="1"/>`,
		`<w:pStyle w:val="Heading3"/>`, `Courier New`, `<w:br/>`, `Literal *`, `&amp; stay text.`} {
		if !strings.Contains(patch.Replacement, native) {
			t.Errorf("native replacement missing %q: %s", native, patch.Replacement)
		}
	}
	updated := docxTestApply(t, data, patch)
	newSections, err := inspectDOCX(updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(newSections) != 3 || newSections[1].Title != "Check the preview" || newSections[2].Content != "Keep me" {
		t.Fatalf("unexpected replacement sections: %+v", newSections)
	}
	assertDOCXOtherPartsIdentical(t, data, updated)
}

// The production format writer owns application. This local harness checks
// that the adapter's single immutable patch is sufficient without ZIP changes.
func docxTestApply(t *testing.T, data []byte, patch Patch) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, part := range archive.File {
		if part.Name != patch.Entry {
			if err := writer.Copy(part); err != nil {
				t.Fatal(err)
			}
			continue
		}
		raw, err := readDOCXPart(part)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw[patch.Start:patch.End]) != patch.Before {
			t.Fatal("patch Before mismatch")
		}
		updated := append(bytes.Clone(raw[:patch.Start]), []byte(patch.Replacement)...)
		updated = append(updated, raw[patch.End:]...)
		member, err := writer.CreateHeader(&part.FileHeader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(updated); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func assertDOCXOtherPartsIdentical(t *testing.T, before, after []byte) {
	t.Helper()
	compressed := func(data []byte) map[string][]byte {
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		parts := map[string][]byte{}
		for _, part := range archive.File {
			if part.Name == "word/document.xml" {
				continue
			}
			reader, err := part.OpenRaw()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			parts[part.Name] = raw
		}
		return parts
	}
	old, updated := compressed(before), compressed(after)
	if len(old) != len(updated) {
		t.Fatal("unrelated ZIP member count changed")
	}
	for name, raw := range old {
		if !bytes.Equal(raw, updated[name]) {
			t.Errorf("unrelated compressed member changed: %s", name)
		}
	}
}

func TestDOCXUnsafeSelectedStructures(t *testing.T) {
	cases := map[string]string{
		"section break":          `<w:p><w:pPr><w:sectPr/></w:pPr><w:r><w:t>Text</w:t></w:r></w:p>`,
		"tracked insertion":      `<w:p><w:ins w:id="1"><w:r><w:t>Text</w:t></w:r></w:ins></w:p>`,
		"property change":        `<w:p><w:pPr><w:pPrChange/></w:pPr></w:p>`,
		"deleted paragraph mark": `<w:p><w:pPr><w:rPr><w:del/></w:rPr></w:pPr><w:r><w:t>Text</w:t></w:r></w:p>`,
		"bookmark":               `<w:p><w:bookmarkStart w:id="1"/><w:r><w:t>Text</w:t></w:r><w:bookmarkEnd w:id="1"/></w:p>`,
		"comment":                `<w:p><w:r><w:commentReference w:id="1"/></w:r></w:p>`,
		"field":                  `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r></w:p>`,
		"image":                  `<w:p><w:r><w:drawing/></w:r></w:p>`,
		"table":                  `<w:tbl><w:tr><w:tc><w:p/></w:tc></w:tr></w:tbl>`,
		"content control":        `<w:sdt><w:sdtContent><w:p/></w:sdtContent></w:sdt>`,
		"footnote":               `<w:p><w:r><w:footnoteReference w:id="1"/></w:r></w:p>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			data := docxTestArchive(t, docxTestDocument(docxTestHeading("Target", "Heading1")+body))
			if _, err := planDOCX(data, 0, "replacement"); err == nil {
				t.Fatal("unsafe section accepted")
			}
		})
	}
}

func TestDOCXCrossBoundaryMarkersAndUnrelatedStructures(t *testing.T) {
	start := `<w:p><w:bookmarkStart w:id="4" w:name="wide"/><w:r><w:t>Before</w:t></w:r></w:p>`
	target := docxTestHeading("Target", "Heading1") + `<w:p><w:r><w:t>Replace</w:t></w:r></w:p>`
	end := docxTestHeading("After", "Heading1") + `<w:p><w:bookmarkEnd w:id="4"/></w:p>`
	data := docxTestArchive(t, docxTestDocument(start+target+end))
	if _, err := planDOCX(data, 0, "replacement"); err == nil {
		t.Fatal("cross-boundary bookmark accepted")
	}
	fieldStart := `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r></w:p>`
	fieldEnd := docxTestHeading("After", "Heading1") + `<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`
	data = docxTestArchive(t, docxTestDocument(fieldStart+target+fieldEnd))
	if _, err := planDOCX(data, 0, "replacement"); err == nil {
		t.Fatal("cross-boundary complex field accepted")
	}
	data = docxTestArchive(t, docxTestDocument(`<w:tbl/>`+target+docxTestHeading("After", "Heading1")+`<w:tbl/>`))
	if _, err := planDOCX(data, 0, "replacement"); err != nil {
		t.Fatalf("unrelated unsupported structures block edit: %v", err)
	}
}

func TestDOCXReplacementLimitations(t *testing.T) {
	data := docxTestArchive(t, docxTestDocument(docxTestHeading("Target", "Heading1")))
	for _, fragment := range []string{"[link](https://example.com)", "1. ordered", "> quote", "![image](image.png)", "<div>HTML</div>"} {
		if _, err := planDOCX(data, 0, fragment); err == nil {
			t.Errorf("unsupported replacement accepted: %s", fragment)
		}
	}
	doc, err := readDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	doc.numbering = nil
	if _, err := doc.bulletID(0); err == nil {
		t.Fatal("missing numbering accepted")
	}
	if _, err := doc.bulletID(9); err == nil {
		t.Fatal("unsupported list depth accepted")
	}
}

func TestDOCXUnboundNumbering(t *testing.T) {
	for _, data := range []string{"", `<Relationships/>`,
		`<Relationships><Relationship Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml" TargetMode="External"/></Relationships>`} {
		bound, err := docxNumberingBound([]byte(data))
		if err != nil || bound {
			t.Fatalf("unbound numbering accepted: bound=%v err=%v", bound, err)
		}
	}
}

func TestDOCXExplicitOutlineAndNamespacePrefix(t *testing.T) {
	body := `<w:p><w:pPr><w:outlineLvl w:val="2"/></w:pPr><w:r><w:t>Direct outline</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Body</w:t></w:r></w:p>`
	document := strings.ReplaceAll(docxTestDocument(body), "w:", "x:")
	document = strings.ReplaceAll(document, "xmlns:w", "xmlns:x")
	data := docxTestArchive(t, document)
	sections, err := inspectDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || sections[0].Level != 3 {
		t.Fatalf("outline: %+v", sections)
	}
	patches, err := planDOCX(data, 0, "new text")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inspectDOCX(docxTestApply(t, data, patches[0])); err != nil {
		t.Fatal(err)
	}
}
