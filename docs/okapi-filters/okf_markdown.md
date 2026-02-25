# okf_markdown - Markdown Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_markdown` |
| Java Class | `net.sf.okapi.filters.markdown.MarkdownFilter` |
| MIME Types | `text/markdown` |
| Extensions | `.md, .markdown` |
| Okapi Module | `markdown` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/markdown/src/test/java/`

#### MarkdownFilterTest.java (82 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testCloseWithoutInput` | Closing filter without providing input | P3 |
| 2 | `testEventsFromEmptyInput` | Empty input produces correct event sequence | P1 |
| 3 | `testAutoLink` | Auto-detected links in markdown | P2 |
| 4 | `testBlockQuoteEvents` | Block quote event extraction | P1 |
| 5 | `testBulletList` | Bullet list text unit extraction | P1 |
| 6 | `testCode` | Inline code spans | P1 |
| 7 | `testCodeWithHtmlEntity` | Code spans containing HTML entities | P2 |
| 8 | `testHtmlAttributeExpression` | HTML attributes within markdown | P2 |
| 9 | `testEmphasis` | Emphasis (italic) inline formatting | P1 |
| 10 | `testCodeAndEmphasis` | Combined code and emphasis | P1 |
| 11 | `testEmphasisAcrossLines` | Emphasis spanning multiple lines | P2 |
| 12 | `testFencedCodeBlock` | Fenced code blocks (``` delimiters) | P1 |
| 13 | `testFencedCodeBlockWithHtmlEntity` | Fenced code blocks with HTML entities | P2 |
| 14 | `testNestedBulletWithFencedCodeBlock` | Nested bullets containing fenced code blocks | P2 |
| 15 | `testNestedBulletWithFencedCodeBlockCRLF` | Nested bullets with fenced code blocks and CRLF endings | P2 |
| 16 | `testHeadingPrefix` | ATX-style headings (# prefix) | P1 |
| 17 | `testGenerateHeaderAnchors` | Header anchor ID generation | P2 |
| 18 | `testHeadingPrefixWithoutSpace` | Headings without space after # | P2 |
| 19 | `testHeadingUnderline` | Setext-style headings (underline) | P1 |
| 20 | `testHtmlTable` | HTML tables within markdown | P1 |
| 21 | `testHtmlBlockWithMarkdown` | HTML blocks containing markdown | P1 |
| 22 | `testMixedHtmlInlineAndMarkdown` | Mixed HTML inline elements and markdown | P2 |
| 23 | `testEscapedHtmlBlockWithMarkdown` | Escaped HTML blocks with markdown content | P2 |
| 24 | `testHtmlInline` | Inline HTML elements | P1 |
| 25 | `testHtmlInlineWithAttributes` | Inline HTML with attributes | P2 |
| 26 | `testHtmlBreakElement` | HTML `<br>` element handling | P2 |
| 27 | `testHtmlCommentAtColumn1` | HTML comment at start of line | P2 |
| 28 | `testHtmlCommentAtColumn5` | HTML comment indented | P2 |
| 29 | `testImage` | Markdown image syntax | P1 |
| 30 | `testExtractImageTitleAndAltText` | Image title and alt text extraction | P1 |
| 31 | `testExtractImageTitleButNotAltText` | Image title extraction without alt text | P2 |
| 32 | `testImageWithTranslatableUrl` | Image with translatable URL | P2 |
| 33 | `testImageRef` | Reference-style image | P1 |
| 34 | `testImgTagWithAlt` | HTML `<img>` tag with alt attribute | P2 |
| 35 | `testImageRefWithTranslatableUrl` | Reference image with translatable URL | P2 |
| 36 | `testIndentedImageRef` | Indented reference-style image | P2 |
| 37 | `testIndentedInlineImage` | Indented inline image | P2 |
| 38 | `testIndentedCodeBlock` | Indented code block (4 spaces) | P1 |
| 39 | `testExcludeIndentedCodeBlock` | Excluding indented code blocks from extraction | P2 |
| 40 | `testTabIndentedCodeBlock` | Tab-indented code block | P2 |
| 41 | `testUnescapeBackslashes` | Backslash unescaping in markdown | P1 |
| 42 | `testLink` | Inline link syntax | P1 |
| 43 | `testLinkSubflow` | Link with subflow text | P1 |
| 44 | `testLinkSubflowWithDNT` | Link subflow with do-not-translate | P2 |
| 45 | `testLinkWithTitle` | Link with title attribute | P1 |
| 46 | `testLinkWithTranslatableUrlByPattern` | Link with translatable URL matched by pattern | P2 |
| 47 | `testIndentedLink` | Indented link | P2 |
| 48 | `testHtmlEntities` | HTML entity handling in markdown | P1 |
| 49 | `testLinkRef` | Reference-style link | P1 |
| 50 | `testReferenceDefinition` | Link reference definition | P1 |
| 51 | `testEmphasisAndStrong` | Combined emphasis and strong (bold) | P1 |
| 52 | `testHtmlEmphasisAndStrong` | HTML `<em>` and `<strong>` tags | P2 |
| 53 | `testStrikethroughSubscript` | Strikethrough and subscript syntax | P2 |
| 54 | `testThematicBreak` | Thematic break (horizontal rule) | P2 |
| 55 | `testTable1TextUnits` | Markdown table text unit extraction (pipe tables) | P1 |
| 56 | `testTable2TextUnits` | Second table format text units | P1 |
| 57 | `testDontTranslateFencedCodeBlocks` | Fenced code blocks not extracted by default | P1 |
| 58 | `testTranslateFencedCodeBlocks` | Fenced code blocks extracted when configured | P2 |
| 59 | `testDontTranslateMetadataHeader` | YAML front matter not extracted by default | P1 |
| 60 | `testTranslateMetadataHeader` | YAML front matter extracted when configured | P2 |
| 61 | `testInlineHtmlTag` | Inline HTML tags as inline codes | P1 |
| 62 | `testNestedInlineHtmlTag` | Nested inline HTML tags | P2 |
| 63 | `testATagWithTitleAttr` | `<a>` tag with title attribute extraction | P2 |
| 64 | `testATagWithTitletWithinDiv` | `<a>` with title inside `<div>` | P2 |
| 65 | `testMathTag` | Math element handling | P2 |
| 66 | `testMathElementWithCommentBehavior` | Math elements treated as comments | P2 |
| 67 | `testMathBlockOnSingleLine` | Single-line math blocks | P2 |
| 68 | `testMathBlocksInListItems` | Math blocks within list items | P2 |
| 69 | `testUnderlinedTextWithinAsterisks` | Underlined text with asterisks | P2 |
| 70 | `testHtmlSubfilterConfig` | Custom HTML subfilter configuration | P2 |
| 71 | `testEmphasisAtParaStart` | Emphasis at paragraph start | P2 |
| 72 | `testCodeFinder` | Code finder pattern detection in markdown | P2 |
| 73 | `testNeighboringMarks` | Adjacent formatting marks | P2 |
| 74 | `testNonTranslatableBlockQuotes` | Block quotes marked as non-translatable | P2 |
| 75 | `testComplexFrontmatterYaml` | Complex YAML front matter parsing | P1 |
| 76 | `testComplexFrontmatterYamlHtml` | Complex YAML front matter with HTML | P2 |
| 77 | `testHardLineBreak` | Hard line breaks (trailing spaces or backslash) | P1 |
| 78 | `testCRLF` | CRLF line ending handling | P2 |
| 79 | `testRunQuotedFencedCodeBlock` | Quoted fenced code blocks | P2 |
| 80 | `testNativeCodeTypes` | Native code type detection | P2 |
| 81 | `testLinkRefAsPairedCode` | Link reference as paired inline code | P2 |
| 82 | `testHtmlAndYamlTransUnitIndexing` | TU indexing with mixed HTML and YAML | P2 |

#### MarkdownWriterTest.java (52 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `writeDocumentParts` | Document part writing | P1 |
| 2 | `writeTextUnitsAndDocumentPartsText` | Text unit and document part output | P1 |
| 3 | `writeTextUnitsAndDocumentPartsWithEscapes` | Escape handling in writer output | P1 |
| 4 | `writeTextUnitsAndDocumentPartsHtml` | HTML content writing | P1 |
| 5 | `writeTextUnitsAndDocumentPartsList` | List writing output | P1 |
| 6 | `writeTextUnitsAndDocumentPartsHardLineBreak` | Hard line break in writer | P1 |
| 7 | `testCommonMarkRoundTrip` | CommonMark roundtrip fidelity | P1 |
| 8 | `testCommonMarkChangedOutput` | CommonMark changed translation output | P1 |
| 9 | `testListsRoundTrip` | List roundtrip preservation | P1 |
| 10 | `testBQInListItemRoundTrip` | Block quote in list item roundtrip | P2 |
| 11 | `testBQInListItemRoundTrip2` | Block quote in list item roundtrip variant | P2 |
| 12 | `testListChangedOutput` | List changed translation output | P1 |
| 13 | `testNestedListWithBlankLines` | Nested list with blank line preservation | P2 |
| 14 | `testTable1RoundTrip` | Table 1 roundtrip | P1 |
| 15 | `testTable1ChangedOutput` | Table 1 changed output | P1 |
| 16 | `testTable2RoundTrip` | Table 2 roundtrip | P1 |
| 17 | `testTable2ChangedOutput` | Table 2 changed output | P1 |
| 18 | `testMinimalMathRoundTrip` | Minimal math block roundtrip | P2 |
| 19 | `testComplexMathRoundTrip` | Complex math block roundtrip | P2 |
| 20 | `testImgWithAltRoundTrip` | Image with alt text roundtrip | P2 |
| 21 | `testHtmlListRoundTrip` | HTML list roundtrip | P2 |
| 22 | `testHtmlListChangedOutput` | HTML list changed output | P2 |
| 23 | `testHtmlTable1RoundTrip` | HTML table roundtrip | P2 |
| 24 | `testQuotedPara` | Quoted paragraph roundtrip | P2 |
| 25 | `testQuotedList` | Quoted list roundtrip | P2 |
| 26 | `testUlInTable` | Unordered list in table | P2 |
| 27 | `testTbodyTdInTable` | tbody/td elements in table | P2 |
| 28 | `testHtmlBlockWithEmptyLines` | HTML block with empty lines | P2 |
| 29 | `testHeadingsAfterList` | Headings after list items | P2 |
| 30 | `testReferencedLinkAndImage` | Referenced link and image output | P1 |
| 31 | `testLinkAndImage` | Inline link and image output | P1 |
| 32 | `testDeadLinkRef` | Dead link reference handling | P2 |
| 33 | `testTooManyTUs` | Too many text units error handling | P2 |
| 34 | `testQuotesAfterHtmlInTableCell` | Quotes after HTML in table cell | P2 |
| 35 | `testCdata` | CDATA section handling | P2 |
| 36 | `testCdataCRLF` | CDATA with CRLF line endings | P2 |
| 37 | `testImageWoAlt` | Image without alt attribute | P2 |
| 38 | `testComplexFrontMatterIgnoredByDefault` | Complex front matter ignored by default | P1 |
| 39 | `testComplexFrontMatterIncludedDefaultFilterUnix` | Front matter included on Unix | P2 |
| 40 | `testComplexFrontMatterIncludedDefaultFilterWindows` | Front matter included on Windows | P2 |
| 41 | `newLinesInsertionInListsWithHtmlTagsClarified` | Newline insertion in lists with HTML tags | P2 |
| 42 | `testCustomConfigurationFromString` | Custom configuration loading from string | P2 |
| 43 | `testRoundTripWithHeadersCustomConfiguration` | Roundtrip with custom header configuration | P2 |
| 44 | `testHardLineBreak` | Hard line break writer roundtrip | P1 |
| 45 | `testHardLineBreakVarious` | Various hard line break scenarios | P2 |
| 46 | `testHardLineBreakWithCRLF` | Hard line break with CRLF | P2 |
| 47 | `testHardLineBreakBetweenInlineMarkupPair` | Hard line break between inline markup | P2 |
| 48 | `testIndentedCodeBlockWithCRLF` | Indented code block with CRLF | P2 |
| 49 | `testQuotedCodeBlocks` | Quoted code block output | P2 |
| 50 | `roundTripsCodesAndCodeBlocks` | Code and code block roundtrip | P1 |
| 51 | `testRoundtripWithEscaping` | Roundtrip with escape sequences | P1 |
| 52 | `roundTripsEmphasis` | Emphasis roundtrip preservation | P1 |

#### MarkdownSkeletonWriterTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testProcessTextUnit` | Skeleton writer text unit processing | P2 |
| 2 | `testAppendLinePrefix` | Line prefix appending in skeleton | P2 |

#### parser/MarkdownParserTest.java (73 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testAutoLink` | Parser auto-link detection | P2 |
| 2 | `testBlockQuote1` | Block quote parsing variant 1 | P2 |
| 3 | `testBlockQuote2` | Block quote parsing variant 2 | P2 |
| 4 | `testBulletList1` | Bullet list parsing variant 1 | P2 |
| 5 | `testBulletList2` | Bullet list parsing variant 2 | P2 |
| 6 | `testBulletListWithWhitespace` | Bullet list with whitespace | P2 |
| 7 | `testCode1` | Code span parsing variant 1 | P2 |
| 8 | `testCode2` | Code span parsing variant 2 | P2 |
| 9 | `testEmphasis1` | Emphasis parsing variant 1 | P2 |
| 10 | `testEmphasis2` | Emphasis parsing variant 2 | P2 |
| 11 | `testFencedCodeBlock` | Fenced code block tokenization | P2 |
| 12 | `testFencedCodeBlockWithInfo` | Fenced code block with info string | P2 |
| 13 | `testFencedCodeBlockWithInfoWithSpace` | Fenced code block info string with space | P2 |
| 14 | `testHeading1A` | ATX heading level 1 variant A | P2 |
| 15 | `testHeading1AX` | ATX heading level 1 variant AX | P2 |
| 16 | `testHeading1B` | ATX heading level 1 variant B | P2 |
| 17 | `testHeading2A` | ATX heading level 2 variant A | P2 |
| 18 | `testHeading2B` | ATX heading level 2 variant B | P2 |
| 19 | `testHtmlBlock1` | HTML block parsing variant 1 | P2 |
| 20 | `testHtmlBlock2` | HTML block parsing variant 2 | P2 |
| 21 | `testHtmlBlockWithMarkdown` | HTML block containing markdown | P2 |
| 22 | `testHtmlCommentBlock` | HTML comment block parsing | P2 |
| 23 | `testHtmlEntity` | HTML entity parsing | P2 |
| 24 | `testHtmlInline` | Inline HTML parsing | P2 |
| 25 | `testImage` | Image syntax parsing | P2 |
| 26 | `testImageRef` | Reference image parsing | P2 |
| 27 | `testImageRefBug` | Image reference bug fix | P2 |
| 28 | `testIndentedCodeBlock` | Indented code block parsing | P2 |
| 29 | `testHardLineBreak` | Hard line break parsing | P2 |
| 30 | `testLink1`-`testLink6` | Link parsing variants (6 tests) | P2 |
| 31 | `testLinkRef` | Reference link parsing | P2 |
| 32 | `testParagraph` | Paragraph parsing | P2 |
| 33 | `testOrderedList` | Ordered list parsing | P2 |
| 34 | `testOrderedListWithNestedIndents` | Ordered list with nested indentation | P2 |
| 35 | `testReferenceDefinition1` | Reference definition parsing | P2 |
| 36 | `testMdxExport` | MDX export statement parsing | P2 |
| 37 | `testReferenceDefinition1plus` | Reference definition variant | P2 |
| 38 | `testReferenceDefinition2` | Reference definition variant 2 | P2 |
| 39 | `testTitleAdmonitions` | Admonition with title parsing | P2 |
| 40 | `testNoTitleAdmonitions` | Admonition without title | P2 |
| 41 | `testNoHeaderAdmonitions` | Admonition without header | P2 |
| 42 | `testAdmonitionsWithNewline` | Admonition with newline | P2 |
| 43 | `testAdmonitionWithIndent` | Indented admonition | P2 |
| 44 | `testAdmonitionWhitespaceBeforeHeading` | Admonition whitespace before heading | P2 |
| 45 | `testAdmonitionWithNestedBulletList` | Admonition with nested bullet list | P2 |
| 46 | `testDocusaurusAdmonition` | Docusaurus-style admonition | P2 |
| 47 | `testDocusaurusAdmonitionWithTitle` | Docusaurus admonition with title | P2 |
| 48 | `testDocusaurusAdmonitionWithHtmlBlock` | Docusaurus admonition with HTML block | P2 |
| 49 | `testNestedCollapsibleAdmonitions` | Nested collapsible admonitions | P2 |
| 50 | `testNestedFencedCodeBlock` | Nested fenced code blocks | P2 |
| 51 | `testIndentedTextInBulletList` | Indented text within bullet list | P2 |
| 52 | `testSoftLineBreak` | Soft line break parsing | P2 |
| 53 | `testStrongEmphasis1` | Strong emphasis parsing variant 1 | P2 |
| 54 | `testStrongEmphasis2` | Strong emphasis parsing variant 2 | P2 |
| 55 | `testThematicBreak` | Thematic break parsing | P2 |
| 56 | `testCommonMarkTokens` | CommonMark token stream | P2 |
| 57 | `testTable1Tokens` | Table token stream | P2 |
| 58 | `testLinkWithText` | Link with text parsing | P2 |
| 59 | `testMathlm` | Math element parsing | P2 |
| 60 | `testMathlmSingleLine` | Single-line math element | P2 |
| 61 | `testMathlmInListItem` | Math element in list item | P2 |
| 62 | `testIndentedText` | Indented text parsing | P2 |
| 63 | `testIndentedHtml` | Indented HTML parsing | P2 |
| 64 | `testIndentedAutoLink` | Indented auto-link parsing | P2 |
| 65 | `testIndentedInlineLink` | Indented inline link parsing | P2 |
| 66 | `testIndentedHtmlBlock` | Indented HTML block parsing | P2 |
| 67 | `testIndentedOrderedListAfterDocusaurusAdmonition` | Indented ordered list after admonition | P2 |
| 68 | `testHtmlBlockMisparsedDocusaurusAdmonition` | HTML block misparsed as admonition | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripMarkdownIT` | `integration-tests/okapi/src/test/java/.../RoundTripMarkdownIT.java` | 3 |

**Test files used**: 47 files + 19 suite files in `integration-tests/okapi/src/test/resources/markdown/`

**Known failing files**: `test-html-block-newline.md`, `html_list_original.md`, `html_table_changed.md`, `admonitions.md`, `html_list_changed.md`, `html-table-w-empty-lines.md`, `html_table1_original.md` (non-Linux newline handling issues)

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `MarkdownXliffCompareIT` | `integration-tests/okapi/src/test/java/.../MarkdownXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/markdown/src/test/resources/net/sf/okapi/filters/markdown/`

| File | Used By | Purpose |
|------|---------|---------|
| `1099.md` | various | Issue 1099 repro |
| `block-quote-in-list-item.md` / `block-quote-in-list-item2.md` | `testBQInListItemRoundTrip` | Block quotes in list items |
| `bullet-para.md` | various | Bullet paragraphs |
| `code_and_codeblock_tests.md` | `roundTripsCodesAndCodeBlocks` | Code and code block combinations |
| `commonmark_original.md` / `commonmark_changed.md` | `testCommonMarkRoundTrip`, `testCommonMarkChangedOutput` | CommonMark roundtrip test pair |
| `complex_frontmatter.md` / `complex_frontmatter_crlf.md` | `testComplexFrontmatterYaml`, `testComplexFrontMatterIncluded*` | YAML front matter |
| `custom-configs/` | `testCustomConfigurationFromString`, `testRoundTripWithHeadersCustomConfiguration` | Custom filter configurations (3 .fprm files) |
| `dead-ref-link.md` / `dead-ref-link-uppercased.md` | `testDeadLinkRef` | Dead reference links |
| `direct-links.md` / `direct-links-uppercased.md` | `testLinkAndImage` | Direct inline links |
| `DirectShape.md` | various | Direct shape test |
| `emphasis.md` | `roundTripsEmphasis` | Emphasis roundtrip |
| `empty-line-test.md` | various | Empty line handling |
| `escaping_tests.md` | `testRoundtripWithEscaping` | Escape sequence roundtrip |
| `hard-line-break*.md` | `testHardLineBreak*` | Hard line break variants (3 files) |
| `heading-after-list.md` | `testHeadingsAfterList` | Heading after list |
| `html_list_original.md` / `html_list_changed.md` | `testHtmlListRoundTrip`, `testHtmlListChangedOutput` | HTML list roundtrip pair |
| `html_table1_original.md` / `html_table_changed.md` | `testHtmlTable1RoundTrip` | HTML table roundtrip pair |
| `html_yaml_transunit_ids.md` | `testHtmlAndYamlTransUnitIndexing` | TU indexing test |
| `html-cdata-sample*.md` | `testCdata`, `testCdataCRLF` | CDATA handling (3 variants) |
| `html-table-w-empty-lines.md` | `testHtmlBlockWithEmptyLines` | HTML table with empty lines |
| `image-wo-alt.md` | `testImageWoAlt` | Image without alt |
| `img_w_alt_attr_original.md` | `testImgWithAltRoundTrip` | Image with alt roundtrip |
| `indented-code-block-simple_crlf.md` | `testIndentedCodeBlockWithCRLF` | Indented code block CRLF |
| `lists_original.md` / `lists_changed.md` | `testListsRoundTrip`, `testListChangedOutput` | List roundtrip pair |
| `metadata_header.md` | `testDontTranslateMetadataHeader`, `testTranslateMetadataHeader` | YAML front matter |
| `min_math_original.md` | `testMinimalMathRoundTrip` | Minimal math block |
| `multiple-segments.md` | various | Multiple segments |
| `nested_list_with_blank_lines.md` | `testNestedListWithBlankLines` | Nested list blank lines |
| `nested-bullet-and-fenced-codeblock*.md` | `testNestedBulletWithFencedCodeBlock*` | Nested bullets + code blocks |
| `okf_markdown*.fprm` / `okf_yaml*.fprm` | config tests | Filter parameter files (4 files) |
| `quoted-code-blocks.md` | `testQuotedCodeBlocks` | Quoted code blocks |
| `quoted-list.md` / `quoted-para.md` | `testQuotedList`, `testQuotedPara` | Quoted content |
| `quotes-after-html-in-table.md` | `testQuotesAfterHtmlInTableCell` | Quotes after HTML in table |
| `ref-links.md` / `ref-links-uppercased.md` | `testReferencedLinkAndImage` | Reference links |
| `regressing_test_single_page.md` | regression test | Regression test |
| `sample_html_combo.md` | various | HTML + markdown combo |
| `space-test.md` | various | Space handling |
| `table1_original.md` / `table1_changed.md` | `testTable1RoundTrip`, `testTable1ChangedOutput` | Table 1 roundtrip pair |
| `table2_original.md` / `table2_changed.md` | `testTable2RoundTrip`, `testTable2ChangedOutput` | Table 2 roundtrip pair |
| `ul-in-table.md` | `testUlInTable` | Unordered list in table |

Parser test resources:

| File | Used By | Purpose |
|------|---------|---------|
| `parser/commonmark.md` | `testCommonMarkTokens` | CommonMark token test |
| `parser/table1.md` | `testTable1Tokens` | Table token test |

65 total unit test resource files (including .md, .fprm, .yml, and config files).

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/markdown/`

47 files in the main directory plus 19 files in `suite/` subdirectory (CommonMark spec-derived test files):

Main directory: `admonitions.md`, `code_and_codeblock_tests.md`, `commonmark_changed.md`, `commonmark_original.md`, `dead-ref-link-uppercased.md`, `dead-ref-link.md`, `deployconfigure-reality.md`, `direct-links-uppercased.md`, `direct-links.md`, `DirectShape.md`, `empty-line-test.md`, `example1.md`-`example5.md`, `heading-after-list.md`, `html_list_changed.md`, `html_list_original.md`, `html_table_changed.md`, `html_table1_original.md`, `html-cdata-sample-uppercased.md`, `html-cdata-sample.md`, `html-table-w-empty-lines.md`, `image-wo-alt.md`, `img_w_alt_attr_original.md`, `link-titles.md`, `lists_changed.md`, `lists_original.md`, `metadata_header.md`, `min_math_original.md`, `multiple-segments.md`, `quoted-list.md`, `quoted-para.md`, `quotes-after-html-in-table.md`, `ref-links-uppercased.md`, `ref-links.md`, `regressing_test_single_page.md`, `sample_html_combo.md`, `space-test.md`, `table1_changed.md`, `table1_original.md`, `table2_changed.md`, `table2_original.md`, `test-html-block-newline.md`, `ul-in-table.md`

Suite directory: `Amps and angle encoding.md`, `Auto links.md`, `Backslash escapes.md`, `Blockquotes with code blocks.md`, `Hard-wrapped paragraphs with list-like lines.md`, `Horizontal rules.md`, `Inline HTML (Advanced).md`, `Inline HTML (Simple).md`, `Inline HTML comments.md`, `Links, inline style.md`, `Links, reference style.md`, `Literal quotes in titles.md`, `Markdown Documentation - Basics.md`, `Markdown Documentation - Syntax.md`, `Nested blockquotes.md`, `Ordered and unordered lists.md`, `Strong and em together.md`, `Tabs.md`, `Tidyness.md`

## Test Data Collection

```bash
# Unit test resources
cp -r okapi/filters/markdown/src/test/resources/net/sf/okapi/filters/markdown/*.md okapi-testdata/okf_markdown/
cp -r okapi/filters/markdown/src/test/resources/net/sf/okapi/filters/markdown/*.fprm okapi-testdata/okf_markdown/
cp -r okapi/filters/markdown/src/test/resources/net/sf/okapi/filters/markdown/custom-configs/ okapi-testdata/okf_markdown/custom-configs/
cp -r okapi/filters/markdown/src/test/resources/net/sf/okapi/filters/markdown/parser/ okapi-testdata/okf_markdown/parser/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/markdown/* okapi-testdata/okf_markdown/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_markdown`

Build tag: `//go:build integration`

#### markdown_test.go - Extraction Tests

```go
func TestExtract_BasicParagraph(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "single paragraph",
            input: "Hello World",
            wantTexts: []string{"Hello World"},
            javaRef: "MarkdownFilterTest#testEventsFromEmptyInput",
        },
        // ... additional test cases
    }
}

func TestExtract_Headings(t *testing.T) {
    // Maps to MarkdownFilterTest: testHeadingPrefix, testHeadingUnderline
}

func TestExtract_InlineFormatting(t *testing.T) {
    // Maps to MarkdownFilterTest: testEmphasis, testCodeAndEmphasis, testEmphasisAndStrong
}

func TestExtract_Links(t *testing.T) {
    // Maps to MarkdownFilterTest: testLink, testLinkRef, testReferenceDefinition
}

func TestExtract_Images(t *testing.T) {
    // Maps to MarkdownFilterTest: testImage, testImageRef, testExtractImageTitleAndAltText
}

func TestExtract_CodeBlocks(t *testing.T) {
    // Maps to MarkdownFilterTest: testFencedCodeBlock, testIndentedCodeBlock, testDontTranslateFencedCodeBlocks
}

func TestExtract_Tables(t *testing.T) {
    // Maps to MarkdownFilterTest: testTable1TextUnits, testTable2TextUnits
}

func TestExtract_HtmlBlocks(t *testing.T) {
    // Maps to MarkdownFilterTest: testHtmlTable, testHtmlBlockWithMarkdown, testHtmlInline
}

func TestExtract_FrontMatter(t *testing.T) {
    // Maps to MarkdownFilterTest: testDontTranslateMetadataHeader, testTranslateMetadataHeader, testComplexFrontmatterYaml
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_markdown/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_markdown/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] XLIFF compare structure matches
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - CommonMark-based parser with extensions (tables, strikethrough, admonitions, math)
  - YAML front matter (between `---` delimiters) optionally extracted via subfilter
  - Fenced code blocks not extracted by default (configurable via `translateCodeBlocks`)
  - HTML blocks within markdown processed by HTML subfilter
  - Inline HTML tags become inline codes in text units
  - Images: alt text, title, and optionally URL are translatable
  - Links: text is translatable, title optionally, URL by pattern match (`translateUrls`)
  - Reference-style links/images: definition text extracted, reference used as inline code
  - Hard line breaks: trailing `\` or two trailing spaces preserved as skeleton
  - Pipe tables: each cell is a separate text unit
  - Admonitions: Docusaurus-style (`:::`) and classic `!!!` style supported
  - Math blocks (`$$...$$`) treated as non-translatable by default
  - CRLF line endings cause known issues with some HTML block roundtrips
  - Custom configurations via `.fprm` files (YAML-based) for HTML/YAML subfilters
  - Code finder can detect patterns within markdown text units

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/markdown/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `MarkdownFilterTest.java` | `okapi/filters/markdown/src/test/java/.../` | 82 |
| `MarkdownWriterTest.java` | `okapi/filters/markdown/src/test/java/.../` | 52 |
| `MarkdownSkeletonWriterTest.java` | `okapi/filters/markdown/src/test/java/.../` | 2 |
| `MarkdownParserTest.java` | `okapi/filters/markdown/src/test/java/.../parser/` | 73 |
