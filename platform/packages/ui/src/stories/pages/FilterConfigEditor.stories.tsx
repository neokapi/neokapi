import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FilterConfigEditor } from "../../components/filter";
import type { FilterSchema, FilterParamsValue } from "../../components/filter";

// ---------------------------------------------------------------------------
// HTML Filter — basic extraction settings (no inline code config)
// ---------------------------------------------------------------------------

const htmlFilterSchema: FilterSchema = {
  $id: "okf_html",
  $version: "1.0",
  title: "HTML Filter",
  description: "Configures how HTML files are parsed for translatable content.",
  type: "object",
  "x-filter": {
    id: "okf_html",
    class: "net.sf.okapi.filters.html.HtmlFilter",
    extensions: [".html", ".htm"],
    mimeTypes: ["text/html"],
  },
  "x-groups": [
    {
      id: "extraction",
      label: "Extraction",
      description: "Control which elements are extracted for translation.",
      fields: ["extractMetaTitle", "extractComments", "extractAltText"],
    },
    {
      id: "advanced",
      label: "Advanced",
      description: "Advanced parsing options.",
      collapsed: true,
      fields: ["preserveWhitespace", "maxSegmentLength"],
    },
  ],
  properties: {
    extractMetaTitle: {
      type: "boolean",
      description: "Extract the <title> meta tag for translation.",
      default: true,
    },
    extractComments: {
      type: "boolean",
      description: "Extract HTML comments as translatable content.",
      default: false,
    },
    extractAltText: {
      type: "boolean",
      description: "Extract alt attributes from image tags.",
      default: true,
    },
    preserveWhitespace: {
      type: "boolean",
      description: "Preserve original whitespace in extracted text.",
      default: false,
    },
    maxSegmentLength: {
      type: "integer",
      description: "Maximum segment length (0 = no limit).",
      default: 0,
      "x-placeholder": "0",
    },
  },
};

// ---------------------------------------------------------------------------
// JSON Filter — with codeFinderRules for inline code detection
// ---------------------------------------------------------------------------

const jsonFilterSchema: FilterSchema = {
  $id: "okf_json",
  $version: "1.47.0",
  title: "JSON Filter",
  description:
    "Configuration for the JSON filter. Use the inline code finder to detect markup patterns (e.g. HTML tags) within JSON string values.",
  type: "object",
  "x-filter": {
    id: "okf_json",
    class: "net.sf.okapi.filters.json.JSONFilter",
    extensions: [".json", ".jsonc"],
    mimeTypes: ["application/json"],
  },
  "x-groups": [
    {
      id: "extraction",
      label: "Extraction",
      description: "Controls which JSON values are extracted for translation.",
      fields: ["extractAllPairs", "extractionRules"],
    },
    {
      id: "inlineCodes",
      label: "Inline Code Detection",
      description:
        "Regex rules that identify inline markup within extracted blocks. Matched patterns become coded-text spans (opening, closing, or placeholder) that translators can reorder but not edit.",
      fields: ["useCodeFinder", "codeFinderRules"],
    },
  ],
  properties: {
    extractAllPairs: {
      type: "boolean",
      description: "Extract all key-value pairs as translatable content.",
      default: true,
    },
    extractionRules: {
      type: "string",
      description: "Extraction key patterns (regex). Leave empty to extract everything.",
      default: "",
      "x-widget": "regexBuilder",
      "x-placeholder": "e.g. ^(title|description|label)$",
    },
    useCodeFinder: {
      type: "boolean",
      description:
        "Enable inline code detection. When on, patterns in the code finder rules are matched and converted to inline spans.",
      default: true,
    },
    codeFinderRules: {
      type: "object",
      description:
        "Regex patterns that identify inline markup within translatable blocks. Each rule defines a pattern; matched text becomes an inline code (span) in the editor.",
      "x-widget": "codeFinderRules",
      "x-okapiFormat": "inlineCodeFinder",
      "x-presets": {
        htmlTags: {
          rules: [{ pattern: "</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>" }, { pattern: "&[a-zA-Z]+;" }],
          sample: "Click <b>here</b> for &mdash; details",
        },
        markdownInline: {
          rules: [
            { pattern: "\\*\\*[^*]+\\*\\*" },
            { pattern: "\\*[^*]+\\*" },
            { pattern: "`[^`]+`" },
            { pattern: "\\[[^\\]]+\\]\\([^)]+\\)" },
          ],
          sample: "Use **bold** or *italic* and `code` or [links](url)",
        },
        icuPlaceholders: {
          rules: [
            { pattern: "\\{[a-zA-Z_][a-zA-Z0-9_]*\\}" },
            { pattern: "\\{[a-zA-Z_][a-zA-Z0-9_]*, *(plural|select|selectordinal)[^}]*\\}" },
          ],
          sample: "Hello {name}, you have {count, plural, one {# item} other {# items}}",
        },
      },
    },
  },
};

// ---------------------------------------------------------------------------
// Plain Text Filter — with code finder for embedded markup
// ---------------------------------------------------------------------------

const plainTextFilterSchema: FilterSchema = {
  $id: "okf_plaintext",
  $version: "1.47.0",
  title: "Plain Text Filter",
  description:
    "Configuration for plain-text files. The inline code finder detects markup or placeholder patterns embedded in text content.",
  type: "object",
  "x-filter": {
    id: "okf_plaintext",
    class: "net.sf.okapi.filters.plaintext.PlainTextFilter",
    extensions: [".txt", ".text"],
    mimeTypes: ["text/plain"],
  },
  "x-groups": [
    {
      id: "general",
      label: "General",
      fields: ["lineBreakAsSegment", "trimLeading", "trimTrailing"],
    },
    {
      id: "inlineCodes",
      label: "Inline Code Detection",
      description:
        "Define regex patterns that identify inline codes (non-translatable tokens) within plain text. Detected patterns are protected as spans during translation.",
      fields: ["useCodeFinder", "codeFinderRules"],
    },
    {
      id: "encoding",
      label: "Encoding",
      collapsed: true,
      fields: ["inputEncoding", "outputEncoding"],
    },
  ],
  properties: {
    lineBreakAsSegment: {
      type: "boolean",
      description: "Treat each line break as a segment boundary.",
      default: true,
    },
    trimLeading: {
      type: "boolean",
      description: "Remove leading whitespace from each segment.",
      default: false,
    },
    trimTrailing: {
      type: "boolean",
      description: "Remove trailing whitespace from each segment.",
      default: true,
    },
    useCodeFinder: {
      type: "boolean",
      description: "Enable inline code pattern detection.",
      default: false,
    },
    codeFinderRules: {
      type: "object",
      description:
        "Regex rules for inline code detection. Matched patterns become non-translatable spans.",
      "x-widget": "codeFinderRules",
      "x-okapiFormat": "inlineCodeFinder",
      "x-presets": {
        variablePlaceholders: {
          rules: [
            { pattern: "\\$\\{[^}]+\\}" },
            { pattern: "%[sdfu%]" },
            { pattern: "%[0-9]*\\$[sdfu]" },
          ],
          sample: "Hello ${user.name}, you scored %d out of %1$d",
        },
        xmlTags: {
          rules: [{ pattern: "</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>" }],
          sample: "Click <link>here</link> to continue",
        },
      },
    },
    inputEncoding: {
      type: "string",
      description: "Character encoding of the input file.",
      default: "UTF-8",
      "x-placeholder": "UTF-8",
    },
    outputEncoding: {
      type: "string",
      description: "Character encoding of the output file.",
      default: "UTF-8",
      "x-placeholder": "UTF-8",
    },
  },
};

// ---------------------------------------------------------------------------
// XML Filter — comprehensive schema with multiple parameter groups
// ---------------------------------------------------------------------------

const xmlFilterSchema: FilterSchema = {
  $id: "okf_xml",
  $version: "1.47.0",
  title: "XML Filter",
  description:
    "Configuration for XML files. Supports inline code detection for mixed-content elements.",
  type: "object",
  "x-filter": {
    id: "okf_xml",
    class: "net.sf.okapi.filters.xml.XMLFilter",
    extensions: [".xml", ".resx", ".svg"],
    mimeTypes: ["application/xml", "text/xml"],
  },
  "x-groups": [
    {
      id: "general",
      label: "General",
      fields: ["preserveWhitespace", "extractNotes"],
    },
    {
      id: "inlineCodes",
      label: "Inline Code Detection",
      description:
        "Configure which XML elements and patterns are treated as inline codes rather than structural elements.",
      fields: ["useCodeFinder", "codeFinderRules"],
    },
    {
      id: "processing",
      label: "Processing",
      collapsed: true,
      fields: ["escapeGT", "collapseWhitespace"],
    },
  ],
  properties: {
    preserveWhitespace: {
      type: "boolean",
      description: "Preserve significant whitespace in text nodes.",
      default: false,
    },
    extractNotes: {
      type: "boolean",
      description: "Extract XML comments adjacent to translatable elements as notes.",
      default: true,
    },
    useCodeFinder: {
      type: "boolean",
      description: "Enable inline code detection within text content.",
      default: true,
    },
    codeFinderRules: {
      type: "object",
      description: "Patterns for detecting inline codes in mixed-content XML text nodes.",
      "x-widget": "codeFinderRules",
      "x-okapiFormat": "inlineCodeFinder",
      "x-presets": {
        commonInlineElements: {
          rules: [
            { pattern: "<(b|i|u|em|strong|code|a|span)[^>]*>" },
            { pattern: "</(b|i|u|em|strong|code|a|span)>" },
            { pattern: "<(br|hr|img)[^>]*/>" },
          ],
          sample: 'Click <b>here</b> to <a href="#">learn more</a>',
        },
        resnameVars: {
          rules: [{ pattern: "\\{[0-9]+\\}" }, { pattern: "\\%[0-9]*[sdfu]" }],
          sample: "Welcome {0}, you have {1} messages",
        },
      },
    },
    escapeGT: {
      type: "boolean",
      description: "Escape > as &gt; in output.",
      default: false,
    },
    collapseWhitespace: {
      type: "boolean",
      description: "Collapse runs of whitespace into single spaces.",
      default: true,
    },
  },
};

const meta: Meta<typeof FilterConfigEditor> = {
  title: "Pages/FilterConfigEditor",
  component: FilterConfigEditor,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Schema-driven filter configuration editor. Each filter publishes a JSON Schema with `x-groups` for parameter grouping and `x-widget` hints for specialized UI controls. The `codeFinderRules` widget is the key control for configuring inline code (online code) detection — regex patterns that identify markup tokens within translatable text.",
      },
    },
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 520, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FilterConfigEditor>;

/** Basic HTML filter with boolean and integer fields, no inline code configuration. */
export const HTMLFilter: Story = {
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractMetaTitle: true,
      extractComments: false,
      extractAltText: true,
      preserveWhitespace: false,
      maxSegmentLength: 0,
    });
    return <FilterConfigEditor schema={htmlFilterSchema} value={value} onChange={setValue} />;
  },
};

/**
 * JSON filter with **inline code detection** enabled.
 *
 * Demonstrates the `codeFinderRules` widget with regex patterns for detecting
 * HTML tags within JSON string values. Use the **Presets** dropdown to load
 * preconfigured rule sets for HTML tags, Markdown inline syntax, or ICU placeholders.
 */
export const JSONFilterWithCodeFinder: Story = {
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractAllPairs: true,
      extractionRules: "",
      useCodeFinder: true,
      codeFinderRules: {
        rules: [{ pattern: "</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>" }, { pattern: "&[a-zA-Z]+;" }],
        sample: "Click <b>here</b> for &mdash; details",
      },
    });
    return <FilterConfigEditor schema={jsonFilterSchema} value={value} onChange={setValue} />;
  },
};

/**
 * Plain text filter with code finder **disabled** by default.
 *
 * Toggle `useCodeFinder` on and add regex rules to detect embedded variables
 * and formatting tokens in plain text files. Shows the `variablePlaceholders`
 * and `xmlTags` presets.
 */
export const PlainTextFilter: Story = {
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      lineBreakAsSegment: true,
      trimLeading: false,
      trimTrailing: true,
      useCodeFinder: false,
      codeFinderRules: { rules: [], sample: "" },
      inputEncoding: "UTF-8",
      outputEncoding: "UTF-8",
    });
    return <FilterConfigEditor schema={plainTextFilterSchema} value={value} onChange={setValue} />;
  },
};

/**
 * XML filter with inline code detection for mixed-content elements.
 *
 * Demonstrates the schema layout for XML-based formats where inline tags
 * (e.g. `<b>`, `<a>`, `<br/>`) are treated as inline codes (spans) rather
 * than structural boundaries. Presets cover common inline HTML elements
 * and numbered placeholders.
 */
export const XMLFilterWithInlineCodes: Story = {
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      preserveWhitespace: false,
      extractNotes: true,
      useCodeFinder: true,
      codeFinderRules: {
        rules: [
          { pattern: "<(b|i|u|em|strong|code|a|span)[^>]*>" },
          { pattern: "</(b|i|u|em|strong|code|a|span)>" },
          { pattern: "<(br|hr|img)[^>]*/>" },
        ],
        sample: 'Click <b>here</b> to <a href="#">learn more</a>',
      },
      escapeGT: false,
      collapseWhitespace: true,
    });
    return <FilterConfigEditor schema={xmlFilterSchema} value={value} onChange={setValue} />;
  },
};

/**
 * Empty inline code rules — starting point for adding custom patterns.
 *
 * Shows the code finder enabled but with no rules defined yet.
 * Click **+ Add Rule** to begin defining patterns.
 */
export const EmptyCodeFinderRules: Story = {
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractAllPairs: true,
      extractionRules: "",
      useCodeFinder: true,
      codeFinderRules: { rules: [], sample: "" },
    });
    return <FilterConfigEditor schema={jsonFilterSchema} value={value} onChange={setValue} />;
  },
};
