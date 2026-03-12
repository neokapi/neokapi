package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// unitCounts maps size tiers to translatable unit counts.
// Actual file sizes vary by format — a "large" JSON is ~300KB while
// a "large" DOCX is ~300KB compressed but with different content density.
var unitCounts = map[string]int{
	"small":  20,
	"medium": 500,
	"large":  5000,
}

// Sample sentences for generating realistic localization content.
var sampleSentences = []string{
	"Welcome to our application.",
	"Please enter your username and password to continue.",
	"Your settings have been saved successfully.",
	"An error occurred while processing your request.",
	"Click the button below to get started.",
	"This feature is currently in beta testing.",
	"Contact our support team for further assistance.",
	"Your subscription will expire in 30 days.",
	"Please review the terms and conditions carefully.",
	"The file has been uploaded successfully.",
	"You have 5 unread notifications.",
	"Select your preferred language from the dropdown.",
	"Are you sure you want to delete this item?",
	"Your password must be at least 8 characters long.",
	"The connection timed out. Please try again.",
	"Data synchronization is now complete.",
	"This action cannot be undone.",
	"Enable two-factor authentication for added security.",
	"Your account has been verified successfully.",
	"The maximum file size is 25 megabytes.",
}

// generateTestData creates test files for all format/size combinations.
func generateTestData(dir string, formats, sizes []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for _, format := range formats {
		for _, size := range sizes {
			units := unitCounts[size]
			if units == 0 {
				return fmt.Errorf("unknown size tier %q (expected small, medium, large)", size)
			}

			subdir := filepath.Join(dir, format, size)
			if err := os.MkdirAll(subdir, 0o755); err != nil {
				return err
			}

			path, err := generateForFormat(format, units, subdir)
			if err != nil {
				return fmt.Errorf("generate %s/%s: %w", format, size, err)
			}

			info, _ := os.Stat(path)
			fmt.Printf("  Generated %s (%d bytes, %d units)\n", path, info.Size(), units)
		}
	}
	return nil
}

func generateForFormat(format string, units int, outDir string) (string, error) {
	switch format {
	case "json":
		return writeFile(outDir, "input.json", generateJSON(units)), nil
	case "html":
		return writeFile(outDir, "input.html", generateHTML(units)), nil
	case "xml":
		return writeFile(outDir, "input.xml", generateXML(units)), nil
	case "xliff":
		return writeFile(outDir, "input.xlf", generateXLIFF(units)), nil
	case "properties":
		return writeFile(outDir, "input.properties", generateProperties(units)), nil
	case "po":
		return writeFile(outDir, "input.po", generatePO(units)), nil
	case "yaml":
		return writeFile(outDir, "input.yml", generateYAML(units)), nil
	case "plaintext":
		return writeFile(outDir, "input.txt", generatePlaintext(units)), nil
	case "docx":
		return generateDOCX(outDir, units)
	case "pptx":
		return generatePPTX(outDir, units)
	case "xlsx":
		return generateXLSX(outDir, units)
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func writeFile(dir, name, content string) string {
	path := filepath.Join(dir, name)
	os.WriteFile(path, []byte(content), 0o644)
	return path
}

func sentence(i int) string {
	return sampleSentences[i%len(sampleSentences)]
}

func generateJSON(units int) string {
	var b strings.Builder
	b.WriteString("{\n")
	for i := 0; i < units; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, "  \"key_%04d\": \"%s\"", i, sentence(i))
	}
	b.WriteString("\n}\n")
	return b.String()
}

func generateHTML(units int) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head><title>Benchmark Document</title></head>\n<body>\n")
	for i := 0; i < units; i++ {
		if i%5 == 0 {
			fmt.Fprintf(&b, "<h2>Section %d</h2>\n", i/5+1)
		}
		fmt.Fprintf(&b, "<p>%s</p>\n", sentence(i))
	}
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func generateXML(units int) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<resources>\n")
	for i := 0; i < units; i++ {
		fmt.Fprintf(&b, "  <string name=\"key_%04d\">%s</string>\n", i, sentence(i))
	}
	b.WriteString("</resources>\n")
	return b.String()
}

func generateXLIFF(units int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="qps" datatype="plaintext" original="benchmark">
    <body>
`)
	for i := 0; i < units; i++ {
		fmt.Fprintf(&b, "      <trans-unit id=\"tu_%04d\">\n        <source>%s</source>\n      </trans-unit>\n", i, sentence(i))
	}
	b.WriteString("    </body>\n  </file>\n</xliff>\n")
	return b.String()
}

func generateProperties(units int) string {
	var b strings.Builder
	b.WriteString("# Benchmark properties file\n")
	for i := 0; i < units; i++ {
		fmt.Fprintf(&b, "key_%04d=%s\n", i, sentence(i))
	}
	return b.String()
}

func generatePO(units int) string {
	var b strings.Builder
	b.WriteString(`# Benchmark PO file
msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Language: en\n"

`)
	for i := 0; i < units; i++ {
		fmt.Fprintf(&b, "#: benchmark:%d\nmsgid \"%s\"\nmsgstr \"\"\n\n", i, sentence(i))
	}
	return b.String()
}

func generateYAML(units int) string {
	var b strings.Builder
	b.WriteString("# Benchmark YAML file\n")
	for i := 0; i < units; i++ {
		fmt.Fprintf(&b, "key_%04d: \"%s\"\n", i, sentence(i))
	}
	return b.String()
}

func generatePlaintext(units int) string {
	var b strings.Builder
	for i := 0; i < units; i++ {
		b.WriteString(sentence(i))
		b.WriteString("\n")
	}
	return b.String()
}

// --- Collection generation ---

// collectionSpec defines the file mix for each collection size tier.
type collectionFileSpec struct {
	Format string
	Units  int
	Name   string // base filename without extension
}

var collectionSpecs = map[string][]collectionFileSpec{
	"small": {
		{Format: "json", Units: 15, Name: "settings"},
		{Format: "html", Units: 20, Name: "help-page"},
		{Format: "properties", Units: 10, Name: "messages"},
		{Format: "yaml", Units: 12, Name: "config"},
		{Format: "docx", Units: 8, Name: "readme"},
	},
	"medium": {
		{Format: "json", Units: 80, Name: "app-strings"},
		{Format: "json", Units: 60, Name: "settings"},
		{Format: "html", Units: 100, Name: "user-guide"},
		{Format: "xml", Units: 50, Name: "resources"},
		{Format: "properties", Units: 40, Name: "messages"},
		{Format: "properties", Units: 30, Name: "labels"},
		{Format: "yaml", Units: 50, Name: "ui-strings"},
		{Format: "po", Units: 60, Name: "catalog"},
		{Format: "docx", Units: 30, Name: "release-notes"},
		{Format: "xlsx", Units: 25, Name: "glossary"},
	},
	"large": {
		{Format: "json", Units: 500, Name: "app-strings"},
		{Format: "json", Units: 300, Name: "admin-panel"},
		{Format: "json", Units: 200, Name: "mobile-strings"},
		{Format: "html", Units: 400, Name: "documentation"},
		{Format: "html", Units: 200, Name: "marketing-pages"},
		{Format: "xml", Units: 300, Name: "android-resources"},
		{Format: "xliff", Units: 500, Name: "translations"},
		{Format: "properties", Units: 200, Name: "server-messages"},
		{Format: "properties", Units: 150, Name: "email-templates"},
		{Format: "yaml", Units: 300, Name: "ui-strings"},
		{Format: "po", Units: 400, Name: "main-catalog"},
		{Format: "docx", Units: 200, Name: "user-manual"},
		{Format: "docx", Units: 100, Name: "release-notes"},
		{Format: "pptx", Units: 150, Name: "product-deck"},
		{Format: "xlsx", Units: 100, Name: "glossary"},
	},
}

// generateCollections creates collection test directories.
func generateCollections(dir string, sizes []string) error {
	for _, size := range sizes {
		specs, ok := collectionSpecs[size]
		if !ok {
			return fmt.Errorf("unknown collection size %q", size)
		}

		collDir := filepath.Join(dir, "collection", size)
		if err := os.MkdirAll(collDir, 0o755); err != nil {
			return err
		}

		totalUnits := 0
		for _, spec := range specs {
			ext := formatExtension(spec.Format)
			path, err := generateForFormat(spec.Format, spec.Units, collDir)
			if err != nil {
				return fmt.Errorf("generate collection %s/%s: %w", size, spec.Name, err)
			}

			// Rename to the specified name.
			newPath := filepath.Join(collDir, spec.Name+ext)
			if path != newPath {
				os.Rename(path, newPath)
			}

			totalUnits += spec.Units
		}

		info, _ := dirSize(collDir)
		fmt.Printf("  Generated collection/%s (%d files, %d units, %d bytes)\n",
			size, len(specs), totalUnits, info)
	}
	return nil
}

func formatExtension(format string) string {
	switch format {
	case "json":
		return ".json"
	case "html":
		return ".html"
	case "xml":
		return ".xml"
	case "xliff":
		return ".xlf"
	case "properties":
		return ".properties"
	case "po":
		return ".po"
	case "yaml":
		return ".yml"
	case "plaintext":
		return ".txt"
	case "docx":
		return ".docx"
	case "pptx":
		return ".pptx"
	case "xlsx":
		return ".xlsx"
	default:
		return ".txt"
	}
}

func dirSize(path string) (int64, error) {
	var total int64
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			info, err := e.Info()
			if err == nil {
				total += info.Size()
			}
		}
	}
	return total, nil
}
