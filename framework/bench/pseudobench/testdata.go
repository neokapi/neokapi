package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// TestFile describes a test file to use in benchmarks.
type TestFile struct {
	Name       string // filename
	Format     string // format name (e.g., "xliff", "openxml", "html")
	Category   string // "large", "medium", "small", "tiny"
	SourcePath string // relative path within okapi-testdata root
}

// testFiles lists the 21 files that both kapi-native and kapi-bridge handle.
var testFiles = []TestFile{
	// Large — 3 files
	{"large.xlsx", "openxml", "large", "okapi/filters/openxml/src/test/resources/large.xlsx"},
	{"Conference_Talk.pptx", "openxml", "large", "integration-tests/okapi/src/test/resources/openxml/pptx/Conference_Talk.pptx"},
	{"big.docx", "openxml", "large", "integration-tests/okapi/src/test/resources/openxml/docx/big.docx"},

	// Medium — 9 files
	{"ugly_big.htm", "html", "medium", "okapi/filters/html/src/test/resources/ugly_big.htm"},
	{"958-4.pptx", "openxml", "medium", "okapi/filters/openxml/src/test/resources/958-4.pptx"},
	{"content_category_test.docx", "openxml", "medium", "okapi/filters/openxml/src/test/resources/content_category_test.docx"},
	{"delTextAmp.docx", "openxml", "medium", "okapi/filters/openxml/src/test/resources/delTextAmp.docx"},
	{"SF-12-Test01.xlf", "xliff", "medium", "okapi/filters/xliff/src/test/resources/SF-12-Test01.xlf"},
	{"Test_nautilus.af.po", "po", "medium", "okapi/filters/po/src/test/resources/Test_nautilus.af.po"},
	{"en (2).yml", "yaml", "medium", "integration-tests/okapi/src/test/resources/yaml/en (2).yml"},
	{"Endpara.pptx", "openxml", "medium", "okapi/filters/openxml/src/test/resources/Endpara.pptx"},
	{"992.docx", "openxml", "medium", "okapi/filters/openxml/src/test/resources/992.docx"},

	// Small — 7 files
	{"burlington_ufo_center.html", "html", "small", "okapi/filters/html/src/test/resources/burlington_ufo_center.html"},
	{"sanitizer.html", "html", "small", "okapi/filters/html/src/test/resources/sanitizer.html"},
	{"W3CHTMHLTest1.html", "html", "small", "okapi/filters/html/src/test/resources/W3CHTMHLTest1.html"},
	{"Josh Test News Email.json", "json", "small", "integration-tests/okapi/src/test/resources/json/Josh Test News Email.json"},
	{"en (3).yml", "yaml", "small", "integration-tests/okapi/src/test/resources/yaml/en (3).yml"},
	{"openoffice_input.xml", "xml", "small", "okapi/filters/its/src/test/resources/openoffice_input.xml"},
	{"Demo V1.srt", "srt", "small", "integration-tests/okapi/src/test/resources/srt/Demo V1.srt"},

	// Tiny — 2 files
	{"Test01.properties", "properties", "tiny", "okapi/filters/properties/src/test/resources/Test01.properties"},
	{"AllCasesTest.po", "po", "tiny", "okapi/filters/po/src/test/resources/AllCasesTest.po"},
}

// copyTestData copies all test files from the okapi-testdata repository into destDir.
func copyTestData(okapiTestdataRoot, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating dest dir: %w", err)
	}

	for _, tf := range testFiles {
		src := filepath.Join(okapiTestdataRoot, tf.SourcePath)
		dst := filepath.Join(destDir, tf.Name)

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copying %s: %w", tf.Name, err)
		}

		fmt.Printf("  Copied %s (%s, %s)\n", tf.Name, tf.Format, tf.Category)
	}

	return nil
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

// totalInputSize returns the total size in bytes of all files in testdataDir.
func totalInputSize(testdataDir string) int64 {
	var total int64
	for _, tf := range testFiles {
		info, err := os.Stat(filepath.Join(testdataDir, tf.Name))
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}
