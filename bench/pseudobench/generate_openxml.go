package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- DOCX Generation ---

func generateDOCX(outDir string, units int) (string, error) {
	path := filepath.Join(outDir, "input.docx")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// [Content_Types].xml
	writeZipFile(w, "[Content_Types].xml", docxContentTypes)

	// _rels/.rels
	writeZipFile(w, "_rels/.rels", docxRels)

	// word/_rels/document.xml.rels
	writeZipFile(w, "word/_rels/document.xml.rels", docxDocRels)

	// word/document.xml
	writeZipFile(w, "word/document.xml", generateDocxBody(units))

	return path, nil
}

func generateDocxBody(units int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"
            xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
            xmlns:o="urn:schemas-microsoft-com:office:office"
            xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
            xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"
            xmlns:v="urn:schemas-microsoft-com:vml"
            xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
            xmlns:w10="urn:schemas-microsoft-com:office:word"
            xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"
            xmlns:wpg="http://schemas.microsoft.com/office/word/2010/wordprocessingGroup"
            xmlns:wpi="http://schemas.microsoft.com/office/word/2010/wordprocessingInk"
            xmlns:wne="http://schemas.microsoft.com/office/word/2006/wordml"
            xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"
            mc:Ignorable="w14 wp14">
  <w:body>
`)
	for i := 0; i < units; i++ {
		if i%20 == 0 {
			// Add a heading every 20 paragraphs.
			fmt.Fprintf(&b, `    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r><w:t>Section %d</w:t></w:r>
    </w:p>
`, i/20+1)
		}
		fmt.Fprintf(&b, `    <w:p>
      <w:r><w:t>%s</w:t></w:r>
    </w:p>
`, sentence(i))
	}
	b.WriteString(`  </w:body>
</w:document>`)
	return b.String()
}

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const docxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const docxDocRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`

// --- PPTX Generation ---

func generatePPTX(outDir string, units int) (string, error) {
	path := filepath.Join(outDir, "input.pptx")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// Distribute units across slides, ~5 text boxes per slide.
	unitsPerSlide := 5
	slideCount := (units + unitsPerSlide - 1) / unitsPerSlide
	if slideCount < 1 {
		slideCount = 1
	}

	// [Content_Types].xml
	writeZipFile(w, "[Content_Types].xml", generatePptxContentTypes(slideCount))

	// _rels/.rels
	writeZipFile(w, "_rels/.rels", pptxRels)

	// ppt/presentation.xml
	writeZipFile(w, "ppt/presentation.xml", generatePptxPresentation(slideCount))

	// ppt/_rels/presentation.xml.rels
	writeZipFile(w, "ppt/_rels/presentation.xml.rels", generatePptxPresentationRels(slideCount))

	// Slide layout & master (minimal)
	writeZipFile(w, "ppt/slideLayouts/slideLayout1.xml", pptxSlideLayout)
	writeZipFile(w, "ppt/slideLayouts/_rels/slideLayout1.xml.rels", pptxSlideLayoutRels)
	writeZipFile(w, "ppt/slideMasters/slideMaster1.xml", pptxSlideMaster)
	writeZipFile(w, "ppt/slideMasters/_rels/slideMaster1.xml.rels", pptxSlideMasterRels)

	// Generate slides
	unitIdx := 0
	for s := 1; s <= slideCount; s++ {
		count := unitsPerSlide
		if unitIdx+count > units {
			count = units - unitIdx
		}
		slideContent := generatePptxSlide(s, unitIdx, count)
		writeZipFile(w, fmt.Sprintf("ppt/slides/slide%d.xml", s), slideContent)
		writeZipFile(w, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", s), pptxSlideRels)
		unitIdx += count
	}

	return path, nil
}

func generatePptxContentTypes(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
  <Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
  <Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
`)
	for i := 1; i <= slideCount; i++ {
		fmt.Fprintf(&b, `  <Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
`, i)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func generatePptxPresentation(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
                xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
                xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:sldMasterIdLst>
    <p:sldMasterId id="2147483648" r:id="rId1"/>
  </p:sldMasterIdLst>
  <p:sldIdLst>
`)
	for i := 1; i <= slideCount; i++ {
		fmt.Fprintf(&b, `    <p:sldId id="%d" r:id="rId%d"/>
`, 255+i, 10+i)
	}
	b.WriteString(`  </p:sldIdLst>
  <p:sldSz cx="9144000" cy="6858000" type="screen4x3"/>
  <p:notesSz cx="6858000" cy="9144000"/>
</p:presentation>`)
	return b.String()
}

func generatePptxPresentationRels(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>
`)
	for i := 1; i <= slideCount; i++ {
		fmt.Fprintf(&b, `  <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>
`, 10+i, i)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func generatePptxSlide(slideNum, unitOffset, count int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
       xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
       xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:nvGrpSpPr>
        <p:cNvPr id="1" name=""/>
        <p:cNvGrpSpPr/>
        <p:nvPr/>
      </p:nvGrpSpPr>
      <p:grpSpPr/>
`)
	// Title shape
	fmt.Fprintf(&b, `      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title %d"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="title"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr/>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle/>
          <a:p><a:r><a:t>Slide %d</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
`, slideNum, slideNum)

	// Content shape with text boxes
	yPos := 1600000
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, `      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="%d" name="TextBox %d"/>
          <p:cNvSpPr txBox="1"/>
          <p:nvPr/>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="457200" y="%d"/>
            <a:ext cx="8229600" cy="400000"/>
          </a:xfrm>
        </p:spPr>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle/>
          <a:p><a:r><a:t>%s</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
`, 100+i, i+1, yPos, sentence(unitOffset+i))
		yPos += 500000
	}

	b.WriteString(`    </p:spTree>
  </p:cSld>
</p:sld>`)
	return b.String()
}

const pptxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`

const pptxSlideRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`

const pptxSlideLayout = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
             xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
             xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
             type="blank" preserve="1">
  <p:cSld name="Blank">
    <p:spTree>
      <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
      <p:grpSpPr/>
    </p:spTree>
  </p:cSld>
</p:sldLayout>`

const pptxSlideLayoutRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>
</Relationships>`

const pptxSlideMaster = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
             xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
             xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:bg>
      <p:bgPr>
        <a:solidFill><a:schemeClr val="bg1"/></a:solidFill>
        <a:effectLst/>
      </p:bgPr>
    </p:bg>
    <p:spTree>
      <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
      <p:grpSpPr/>
    </p:spTree>
  </p:cSld>
  <p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2"
            accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>
  <p:sldLayoutIdLst>
    <p:sldLayoutId id="2147483649" r:id="rId1"/>
  </p:sldLayoutIdLst>
</p:sldMaster>`

const pptxSlideMasterRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`

// --- XLSX Generation ---

func generateXLSX(outDir string, units int) (string, error) {
	path := filepath.Join(outDir, "input.xlsx")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	writeZipFile(w, "[Content_Types].xml", xlsxContentTypes)
	writeZipFile(w, "_rels/.rels", xlsxRels)
	writeZipFile(w, "xl/_rels/workbook.xml.rels", xlsxWorkbookRels)
	writeZipFile(w, "xl/workbook.xml", xlsxWorkbook)
	writeZipFile(w, "xl/styles.xml", xlsxStyles)
	writeZipFile(w, "xl/sharedStrings.xml", generateXlsxSharedStrings(units))
	writeZipFile(w, "xl/worksheets/sheet1.xml", generateXlsxSheet(units))

	return path, nil
}

func generateXlsxSharedStrings(units int) string {
	var b strings.Builder
	// Header row + unit rows → units+2 strings (key header + value header + key_N + sentence_N)
	totalStrings := 2 + units*2
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">
`, totalStrings, totalStrings)
	// Header strings
	b.WriteString("  <si><t>Key</t></si>\n")
	b.WriteString("  <si><t>Value</t></si>\n")
	// Data strings
	for i := 0; i < units; i++ {
		fmt.Fprintf(&b, "  <si><t>key_%04d</t></si>\n", i)
		fmt.Fprintf(&b, "  <si><t>%s</t></si>\n", sentence(i))
	}
	b.WriteString("</sst>")
	return b.String()
}

func generateXlsxSheet(units int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
           xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheetData>
    <row r="1">
      <c r="A1" t="s"><v>0</v></c>
      <c r="B1" t="s"><v>1</v></c>
    </row>
`)
	for i := 0; i < units; i++ {
		row := i + 2
		keyIdx := 2 + i*2
		valIdx := 3 + i*2
		fmt.Fprintf(&b, `    <row r="%d">
      <c r="A%d" t="s"><v>%d</v></c>
      <c r="B%d" t="s"><v>%d</v></c>
    </row>
`, row, row, keyIdx, row, valIdx)
	}
	b.WriteString(`  </sheetData>
</worksheet>`)
	return b.String()
}

const xlsxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

const xlsxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const xlsxWorkbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const xlsxWorkbook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
          xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Translations" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`

const xlsxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
</styleSheet>`

// writeZipFile adds a file to a zip archive.
func writeZipFile(w *zip.Writer, name, content string) {
	f, err := w.Create(name)
	if err != nil {
		return
	}
	f.Write([]byte(content))
}
