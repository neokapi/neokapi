/**
 * InlineCodePipeline — documents the end-to-end flow from filter schema
 * configuration to inline code (span) rendering in the translation editor.
 *
 * The pipeline:
 *   1. Filter schema defines `codeFinderRules` (regex patterns)
 *   2. Reader matches patterns → produces coded text with Unicode markers
 *   3. Editor displays inline codes as tag chips in source and target
 *   4. Writer reconstructs original markup from coded text + spans
 *
 * This story renders each stage side-by-side so the relationship between
 * schema-defined rules and the resulting editor UI is clear.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import { InlinePreview } from "../../components/editor/InlinePreview";
import { TagChipComponent } from "../../components/editor/TagChipComponent";
import { TagPalette } from "../../components/editor/TagPalette";
import { TagValidationBar } from "../../components/editor/TagValidationBar";
import { FilterConfigEditor } from "../../components/filter";
import type { FilterSchema, FilterParamsValue } from "../../components/filter";
import type { SpanInfo } from "../../types/api";
import { buildPairs, validateTags } from "../../components/editor/tagSemantics";

// ---------------------------------------------------------------------------
// Unicode markers (match Go model constants in core/model/fragment.go)
// ---------------------------------------------------------------------------

const O = "\uE001"; // opening span marker
const C = "\uE002"; // closing span marker
const P = "\uE003"; // placeholder span marker

// ---------------------------------------------------------------------------
// Demonstration data: a JSON string containing HTML inline codes
// ---------------------------------------------------------------------------

/** Spans extracted by the code finder rules */
const demoSpans: SpanInfo[] = [
  { span_type: "opening", type: "b", id: "1", data: "<b>" },
  { span_type: "closing", type: "b", id: "1", data: "</b>" },
  { span_type: "opening", type: "a", id: "2", data: '<a href="/help">' },
  { span_type: "closing", type: "a", id: "2", data: "</a>" },
  { span_type: "placeholder", type: "br", id: "3", data: "<br/>" },
];

/** Source coded text after code finder processing */
const demoCodedText = `Click ${O}here${C} or visit the ${O}help center${C} for details.${P}Thank you!`;

/** French translation with same inline code structure */
const frenchCodedText = `Cliquez ${O}ici${C} ou visitez le ${O}centre d'aide${C} pour plus de d\u00e9tails.${P}Merci\u00a0!`;

/** Target with a missing tag — demonstrates validation */
const brokenTargetSpans: SpanInfo[] = [
  { span_type: "opening", type: "b", id: "1", data: "<b>" },
  // Missing </b> closing tag
  { span_type: "opening", type: "a", id: "2", data: '<a href="/help">' },
  { span_type: "closing", type: "a", id: "2", data: "</a>" },
  { span_type: "placeholder", type: "br", id: "3", data: "<br/>" },
];

// ---------------------------------------------------------------------------
// Filter schema used in the demo
// ---------------------------------------------------------------------------

const demoFilterSchema: FilterSchema = {
  $id: "okf_json",
  $version: "1.47.0",
  title: "JSON Filter",
  description: "Inline code finder rules that produced the spans shown below.",
  type: "object",
  "x-filter": {
    id: "okf_json",
    class: "net.sf.okapi.filters.json.JSONFilter",
    extensions: [".json"],
    mimeTypes: ["application/json"],
  },
  "x-groups": [
    {
      id: "inlineCodes",
      label: "Inline Code Detection",
      description:
        "These regex patterns matched the HTML tags in the JSON value and produced the inline codes shown in the editor below.",
      fields: ["useCodeFinder", "codeFinderRules"],
    },
  ],
  properties: {
    useCodeFinder: {
      type: "boolean",
      description: "Enable inline code detection in JSON values.",
      default: true,
    },
    codeFinderRules: {
      type: "object",
      description: "Regex patterns for inline code detection.",
      "x-widget": "codeFinderRules",
      "x-okapiFormat": "inlineCodeFinder",
    },
  },
};

// ---------------------------------------------------------------------------
// Story meta
// ---------------------------------------------------------------------------

/** Wrapper with section header styling */
function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 24 }}>
      <h3
        style={{
          fontSize: 11,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.08em",
          color: "var(--sb-color, #888)",
          marginBottom: 8,
          borderBottom: "1px solid var(--sb-border, #333)",
          paddingBottom: 4,
        }}
      >
        {title}
      </h3>
      {children}
    </div>
  );
}

const meta: Meta = {
  title: "Editor/InlineCodePipeline",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component: [
          "Demonstrates the end-to-end inline code (online code) pipeline:",
          "",
          "1. **Schema** — filter schemas define `codeFinderRules` with regex patterns",
          "2. **Coded Text** — the reader replaces matched patterns with Unicode markers (U+E001\u2013U+E003) and stores the original markup as `SpanInfo` metadata",
          "3. **Editor UI** — `SourceCellDisplay` renders inline codes as semantic tag chips; `TagPalette` lets translators insert them; `TagValidationBar` validates tag parity",
          "4. **Preview** — `InlinePreview` reconstructs safe HTML from coded text + spans for a live formatted preview",
        ].join("\n"),
      },
    },
  },
};

export default meta;
type Story = StoryObj;

/**
 * Full pipeline demonstration showing the flow from filter schema
 * configuration through coded text to editor rendering.
 */
export const FullPipeline: Story = {
  render: () => {
    const pairs = buildPairs(demoSpans);
    const validation = validateTags(demoSpans, demoSpans);

    const [filterValue, setFilterValue] = useState<FilterParamsValue>({
      useCodeFinder: true,
      codeFinderRules: {
        rules: [
          { pattern: "</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>" },
        ],
        sample: 'Click <b>here</b> or visit the <a href="/help">help center</a>',
      },
    });

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="1. Filter Schema — Code Finder Rules">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
            The filter schema defines regex patterns via the <code>codeFinderRules</code> widget.
            These patterns determine which text fragments become inline codes.
          </p>
          <FilterConfigEditor
            schema={demoFilterSchema}
            value={filterValue}
            onChange={setFilterValue}
          />
        </Section>

        <Section title="2. Extracted Spans">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
            The reader matched 5 inline codes: 2 paired tags ({`<b>`}, {`<a>`}) and 1 placeholder ({`<br/>`}).
            Each becomes a <code>SpanInfo</code> with type, id, and original markup data.
          </p>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, alignItems: "center" }}>
            {demoSpans.map((span, i) => {
              const pair = pairs.get(i);
              return (
                <TagChipComponent
                  key={i}
                  spanInfo={span}
                  index={i + 1}
                  pairIndex={pair?.pairIndex}
                />
              );
            })}
          </div>
        </Section>

        <Section title="3. Source Display — Coded Text + Tag Chips">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
            <code>SourceCellDisplay</code> renders the coded text with inline codes shown as
            colored tag chips. Hover a tag to highlight its matching pair.
          </p>
          <SourceCellDisplay codedText={demoCodedText} spans={demoSpans} />
        </Section>

        <Section title="4. Tag Palette — Available Tags for Insertion">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
            Translators use the tag palette to insert inline codes into the target.
            Tags come from the source spans extracted by the code finder rules.
          </p>
          <TagPalette sourceSpans={demoSpans} onInsert={fn()} />
        </Section>

        <Section title="5. Validation">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
            The validation bar checks that the target contains the same inline codes as the source.
          </p>
          <TagValidationBar validation={validation} />
        </Section>

        <Section title="6. Live Preview — Reconstructed HTML">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
            <code>InlinePreview</code> converts coded text + spans back to safe HTML,
            showing the formatted result as the translator would expect it.
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <div>
              <span style={{ fontSize: 10, color: "#666", fontWeight: 600 }}>SOURCE</span>
              <InlinePreview codedText={demoCodedText} spans={demoSpans} />
            </div>
            <div>
              <span style={{ fontSize: 10, color: "#666", fontWeight: 600 }}>FR-FR TARGET</span>
              <InlinePreview codedText={frenchCodedText} spans={demoSpans} />
            </div>
          </div>
        </Section>
      </div>
    );
  },
};

/**
 * Shows what happens when a target translation is missing an inline code.
 * The validation bar reports the error so the translator can fix it.
 */
export const ValidationError: Story = {
  render: () => {
    const brokenCodedText = `Cliquez ${O}ici ou visitez le ${O}centre d'aide${C}.${P}Merci\u00a0!`;
    const validation = validateTags(demoSpans, brokenTargetSpans);

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="Source (all tags present)">
          <SourceCellDisplay codedText={demoCodedText} spans={demoSpans} />
        </Section>

        <Section title="Target (missing closing &lt;b&gt; tag)">
          <SourceCellDisplay codedText={brokenCodedText} spans={brokenTargetSpans} />
        </Section>

        <Section title="Validation Result">
          <TagValidationBar validation={validation} />
        </Section>
      </div>
    );
  },
};

/**
 * Demonstrates different inline code categories and how each appears
 * as a span in the coded text model.
 */
export const SpanCategories: Story = {
  render: () => {
    const categories: Array<{
      label: string;
      description: string;
      codedText: string;
      spans: SpanInfo[];
    }> = [
      {
        label: "Paired formatting tags",
        description: "Opening + closing tags (bold, italic, underline). Both must appear in the target.",
        codedText: `${O}Bold text${C} and ${O}italic text${C}`,
        spans: [
          { span_type: "opening", type: "b", id: "1", data: "<b>" },
          { span_type: "closing", type: "b", id: "1", data: "</b>" },
          { span_type: "opening", type: "i", id: "2", data: "<i>" },
          { span_type: "closing", type: "i", id: "2", data: "</i>" },
        ],
      },
      {
        label: "Self-closing placeholders",
        description: "Standalone placeholders (line break, image). Position may change but they cannot be split.",
        codedText: `First line${P}Second line${P}`,
        spans: [
          { span_type: "placeholder", type: "br", id: "3", data: "<br/>" },
          { span_type: "placeholder", type: "img", id: "4", data: '<img src="icon.png"/>' },
        ],
      },
      {
        label: "Link with attributes",
        description: "The data field preserves the full original markup including attributes.",
        codedText: `Visit ${O}our documentation${C} for details`,
        spans: [
          { span_type: "opening", type: "a", id: "5", data: '<a href="https://docs.example.com" class="external">' },
          { span_type: "closing", type: "a", id: "5", data: "</a>" },
        ],
      },
      {
        label: "Code / monospace",
        description: "Inline code spans typically wrapping command names or variable references.",
        codedText: `Run ${O}kapi init${C} then edit ${O}config.yaml${C}`,
        spans: [
          { span_type: "opening", type: "code", id: "6", data: "<code>" },
          { span_type: "closing", type: "code", id: "6", data: "</code>" },
          { span_type: "opening", type: "code", id: "7", data: "<code>" },
          { span_type: "closing", type: "code", id: "7", data: "</code>" },
        ],
      },
      {
        label: "Mixed content",
        description: "Real-world segments often contain multiple tag types interleaved with text.",
        codedText: `${O}Important:${C} Use ${O}kapi flow run${C} to start.${P}See ${O}docs${C} for more.`,
        spans: [
          { span_type: "opening", type: "b", id: "8", data: "<b>" },
          { span_type: "closing", type: "b", id: "8", data: "</b>" },
          { span_type: "opening", type: "code", id: "9", data: "<code>" },
          { span_type: "closing", type: "code", id: "9", data: "</code>" },
          { span_type: "placeholder", type: "br", id: "10", data: "<br/>" },
          { span_type: "opening", type: "a", id: "11", data: '<a href="/docs">' },
          { span_type: "closing", type: "a", id: "11", data: "</a>" },
        ],
      },
    ];

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        {categories.map((cat) => (
          <Section key={cat.label} title={cat.label}>
            <p style={{ fontSize: 12, color: "#999", marginBottom: 8 }}>{cat.description}</p>
            <div style={{ marginBottom: 8 }}>
              <SourceCellDisplay codedText={cat.codedText} spans={cat.spans} />
            </div>
            <InlinePreview codedText={cat.codedText} spans={cat.spans} />
          </Section>
        ))}
      </div>
    );
  },
};
