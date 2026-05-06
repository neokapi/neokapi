package main

// CorpusFormat describes how to discover the okapi-testdata fixtures
// for one format and which Okapi filter class to use when invoking the
// `kapi-okapi-bridge pseudo` reference engine. Mirrors the per-format
// scan metadata in cli/parity/roundtrip/coverage_test.go (formatScan)
// so the bench's input set stays aligned with the parity harness's
// input set.
//
// This file deliberately duplicates the format → {filterClass, sources,
// extensions, …} mapping rather than importing the parity test (which
// is under a `parity` build tag and lives in another go module). The
// per-format data is small, low-churn, and easy to keep in sync;
// drift would surface immediately as missing fixtures or "no input"
// errors from a bench run.
type CorpusFormat struct {
	FormatID    string
	FilterClass string

	// Sources lists tarball-relative directories to scan, OR explicit
	// files (when ExplicitFiles is non-empty those win and Sources is
	// ignored).
	Sources       []string
	Extensions    []string // case-insensitive (".json", ".xml", ...)
	Recurse       bool
	ExplicitFiles []string

	// OkapiParamConfig is forwarded to `kapi-okapi-bridge pseudo`'s
	// `--fprm` flag (raw .fprm content). Used by VTT/TTML to disable
	// caption-merging on round-trip.
	OkapiParamConfig string

	// IsZip flips the read mode for archive formats (idml, openxml).
	// The bench treats these the same way as plain files for the
	// pseudo invocation; the flag is preserved for completeness so
	// future categorisation / size weighting can use it.
	IsZip bool

	// Category is a coarse size bucket — assigned at discovery time
	// based on file size, not declared here. Kept on CorpusFormat for
	// formats that should always be treated as a particular tier
	// regardless of file size (none today).
}

// ParityCorpus returns the per-format scan spec for every format the
// parity harness exercises. Order is stable (matches the coverage test).
//
// To regenerate from the parity test: `grep -A 1 "formatID:"
// cli/parity/roundtrip/coverage_test.go` and update.
func ParityCorpus() []CorpusFormat {
	return []CorpusFormat{
		// ── Plain-text family ─────────────────────────────────────
		{
			FormatID:    "plaintext",
			FilterClass: "okf_plaintext",
			Sources:     []string{"integration-tests/okapi/src/test/resources/plaintext"},
			Extensions:  []string{".txt"},
		},
		{
			FormatID:      "paraplaintext",
			FilterClass:   "okf_paraplaintext",
			ExplicitFiles: []string{"integration-tests/okapi/src/test/resources/plaintext/test_paragraphs1.txt"},
		},
		{
			FormatID:    "splicedlines",
			FilterClass: "okf_splicedlines",
			ExplicitFiles: []string{
				"okapi/filters/plaintext/src/test/resources/combined_lines.txt",
				"okapi/filters/plaintext/src/test/resources/combined_lines_end.txt",
				"okapi/filters/plaintext/src/test/resources/combined_lines2.txt",
			},
		},
		{
			FormatID:    "mosestext",
			FilterClass: "okf_mosestext",
			Sources:     []string{"okapi/filters/mosestext/src/test/resources"},
			Extensions:  []string{".txt"},
		},

		// ── HTML / markup ─────────────────────────────────────────
		{
			FormatID:    "html",
			FilterClass: "okf_html",
			Sources:     []string{"integration-tests/okapi/src/test/resources/html"},
			Extensions:  []string{".html"},
		},
		{
			FormatID:    "markdown",
			FilterClass: "okf_markdown",
			Sources:     []string{"integration-tests/okapi/src/test/resources/markdown"},
			Extensions:  []string{".md"},
		},
		{
			FormatID:    "wiki",
			FilterClass: "okf_wiki",
			Sources:     []string{"integration-tests/okapi/src/test/resources/wiki"},
			Extensions:  []string{".txt"},
		},

		// ── Resource bundles & messages ───────────────────────────
		{
			FormatID:    "po",
			FilterClass: "okf_po",
			Sources:     []string{"integration-tests/okapi/src/test/resources/po"},
			Extensions:  []string{".po", ".pot"},
		},
		{
			FormatID:    "properties",
			FilterClass: "okf_properties",
			Sources:     []string{"integration-tests/okapi/src/test/resources/properties"},
			Extensions:  []string{".properties"},
		},
		{
			FormatID:    "json",
			FilterClass: "okf_json",
			Sources:     []string{"integration-tests/okapi/src/test/resources/json"},
			Extensions:  []string{".json"},
		},
		{
			FormatID:    "yaml",
			FilterClass: "okf_yaml",
			Sources:     []string{"integration-tests/okapi/src/test/resources/yaml"},
			Extensions:  []string{".yml", ".yaml"},
		},
		{
			FormatID:    "phpcontent",
			FilterClass: "okf_phpcontent",
			Sources:     []string{"integration-tests/okapi/src/test/resources/phpcontent"},
			Extensions:  []string{".php"},
		},

		// ── Tabular ────────────────────────────────────────────────
		{
			FormatID:    "csv",
			FilterClass: "okf_commaseparatedvalues",
			Sources:     []string{"integration-tests/okapi/src/test/resources/csv"},
			Extensions:  []string{".csv"},
		},
		{
			FormatID:    "tsv",
			FilterClass: "okf_tabseparatedvalues",
			Sources:     []string{"integration-tests/okapi/src/test/resources/tsv"},
			Extensions:  []string{".tsv"},
		},
		{
			FormatID:    "fixedwidth",
			FilterClass: "okf_fixedwidthcolumns",
			Sources:     []string{"integration-tests/okapi/src/test/resources/fixedwidth"},
			Extensions:  []string{".txt"},
		},

		// ── Code & docs ───────────────────────────────────────────
		{
			FormatID:    "doxygen",
			FilterClass: "okf_doxygen",
			Sources:     []string{"integration-tests/okapi/src/test/resources/doxygen"},
			Extensions:  []string{".h", ".c", ".cpp"},
		},

		// ── XML & XML-derived ─────────────────────────────────────
		{
			FormatID:    "xml",
			FilterClass: "okf_xml",
			Sources:     []string{"integration-tests/okapi/src/test/resources/xml"},
			Extensions:  []string{".xml"},
		},
		{
			FormatID:    "xliff",
			FilterClass: "okf_xliff",
			Sources:     []string{"integration-tests/okapi/src/test/resources/xliff"},
			Extensions:  []string{".xlf", ".xliff"},
		},
		{
			FormatID:    "xliff2",
			FilterClass: "okf_xliff2",
			Sources:     []string{"integration-tests/okapi/src/test/resources/xliff2"},
			Extensions:  []string{".xlf", ".xlf2"},
		},
		{
			FormatID:    "tmx",
			FilterClass: "okf_tmx",
			Sources:     []string{"integration-tests/okapi/src/test/resources/tmx"},
			Extensions:  []string{".tmx"},
		},
		{
			FormatID:    "ts",
			FilterClass: "okf_ts",
			Sources:     []string{"integration-tests/okapi/src/test/resources/ts"},
			Extensions:  []string{".ts"},
		},

		// ── Subtitle / timed-text ─────────────────────────────────
		{
			FormatID:    "vtt",
			FilterClass: "okf_vtt",
			Sources:     []string{"integration-tests/okapi/src/test/resources/vtt"},
			Extensions:  []string{".vtt"},
			OkapiParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
mergeCaptions.b=false
`,
		},
		{
			FormatID:    "ttml",
			FilterClass: "okf_ttml",
			Sources:     []string{"integration-tests/okapi/src/test/resources/ttml"},
			Extensions:  []string{".xml", ".ttml"},
			OkapiParamConfig: `#v1
mergeCaptions.b=false
`,
		},

		// ── Document formats ─────────────────────────────────────
		{
			FormatID:    "dtd",
			FilterClass: "okf_dtd",
			Sources:     []string{"integration-tests/okapi/src/test/resources/dtd"},
			Extensions:  []string{".dtd"},
		},
		{
			FormatID:    "tex",
			FilterClass: "okf_tex",
			Sources:     []string{"integration-tests/okapi/src/test/resources/tex"},
			Extensions:  []string{".tex"},
		},
		{
			FormatID:    "transtable",
			FilterClass: "okf_transtable",
			Sources:     []string{"integration-tests/okapi/src/test/resources/transtable"},
			Extensions:  []string{".txt", ".tsv"},
		},

		// ── Adobe ────────────────────────────────────────────────
		{
			FormatID:    "idml",
			FilterClass: "okf_idml",
			Sources:     []string{"integration-tests/okapi/src/test/resources/idml"},
			Extensions:  []string{".idml"},
			IsZip:       true,
		},
		{
			FormatID:    "icml",
			FilterClass: "okf_icml",
			Sources:     []string{"integration-tests/okapi/src/test/resources/icml"},
			Extensions:  []string{".icml"},
		},

		// ── Office ───────────────────────────────────────────────
		{
			FormatID:    "openxml",
			FilterClass: "okf_openxml",
			Sources:     []string{"integration-tests/okapi/src/test/resources/openxml"},
			Extensions:  []string{".docx", ".xlsx", ".pptx"},
			Recurse:     true,
			IsZip:       true,
		},

		// ── FrameMaker ────────────────────────────────────────────
		{
			FormatID:    "mif",
			FilterClass: "okf_mif",
			Sources:     []string{"integration-tests/okapi/src/test/resources/mif"},
			Extensions:  []string{".mif"},
		},
	}
}
