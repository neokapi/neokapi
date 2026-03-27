/**
 * FormatAwareEditing — demonstrates how the vocabulary system provides
 * a consistent editing experience across different file formats.
 *
 * HTML, Markdown, and XLIFF all use different native syntax for the same
 * concepts (bold, links, etc.), but the editor presents them identically
 * because they map to the same vocabulary types.
 */
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import { TagPalette } from "../../components/editor/TagPalette";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
import { FormatVocabularyBadge } from "../../components/editor/FormatVocabularyBadge";
import type { SpanInfo } from "../../types/api";

// ---------------------------------------------------------------------------
// Unicode markers
// ---------------------------------------------------------------------------

const O = "\uE001";
const C = "\uE002";
const P = "\uE003";

// ---------------------------------------------------------------------------
// Format-specific data: same content in three formats
// ---------------------------------------------------------------------------

const sentence = `Click ${O}here${C} to ${O}learn more${C}`;

const htmlSpans: SpanInfo[] = [
  { span_type: "opening", type: "fmt:bold", id: "1", data: '<b class="cta">' },
  { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" },
  {
    span_type: "opening",
    type: "link:hyperlink",
    id: "2",
    data: '<a href="https://docs.example.com">',
  },
  { span_type: "closing", type: "link:hyperlink", id: "2", data: "</a>" },
];

const markdownSpans: SpanInfo[] = [
  { span_type: "opening", type: "fmt:bold", id: "1", data: "**" },
  { span_type: "closing", type: "fmt:bold", id: "1", data: "**" },
  { span_type: "opening", type: "link:hyperlink", id: "2", data: "[" },
  { span_type: "closing", type: "link:hyperlink", id: "2", data: "](https://docs.example.com)" },
];

const xliffSpans: SpanInfo[] = [
  {
    span_type: "opening",
    type: "fmt:bold",
    id: "1",
    data: '<pc id="1" dataRefStart="d1">',
    sub_type: "xlf:b",
  },
  { span_type: "closing", type: "fmt:bold", id: "1", data: "</pc>", sub_type: "xlf:b" },
  {
    span_type: "opening",
    type: "link:hyperlink",
    id: "2",
    data: '<pc id="2" dataRefStart="d2">',
    sub_type: "xlf:a",
  },
  { span_type: "closing", type: "link:hyperlink", id: "2", data: "</pc>", sub_type: "xlf:a" },
];

// ---------------------------------------------------------------------------
// Richer format-specific examples
// ---------------------------------------------------------------------------

const richSentence = `${O}Important:${C} Use ${O}kapi init${C} to get started.${P}See the ${O}docs${C} for more.`;

const htmlRichSpans: SpanInfo[] = [
  { span_type: "opening", type: "fmt:bold", id: "1", data: "<strong>" },
  { span_type: "closing", type: "fmt:bold", id: "1", data: "</strong>" },
  { span_type: "opening", type: "fmt:code", id: "2", data: "<code>" },
  { span_type: "closing", type: "fmt:code", id: "2", data: "</code>" },
  { span_type: "placeholder", type: "struct:break", id: "3", data: "<br/>" },
  { span_type: "opening", type: "link:hyperlink", id: "4", data: '<a href="/docs">' },
  { span_type: "closing", type: "link:hyperlink", id: "4", data: "</a>" },
];

const mdRichSpans: SpanInfo[] = [
  { span_type: "opening", type: "fmt:bold", id: "1", data: "**" },
  { span_type: "closing", type: "fmt:bold", id: "1", data: "**" },
  { span_type: "opening", type: "fmt:code", id: "2", data: "`" },
  { span_type: "closing", type: "fmt:code", id: "2", data: "`" },
  { span_type: "placeholder", type: "struct:break", id: "3", data: "  \n" },
  { span_type: "opening", type: "link:hyperlink", id: "4", data: "[" },
  { span_type: "closing", type: "link:hyperlink", id: "4", data: "](/docs)" },
];

// ---------------------------------------------------------------------------
// Code token example (ICU / i18n message format)
// ---------------------------------------------------------------------------

const i18nSentence = `Hello ${P}, you have ${P} new ${O}messages${C}.`;

const i18nSpans: SpanInfo[] = [
  {
    span_type: "placeholder",
    type: "code:variable",
    id: "1",
    data: "{userName}",
    display_text: "{userName}",
    deletable: false,
    cloneable: false,
    can_reorder: true,
  },
  {
    span_type: "placeholder",
    type: "code:placeholder",
    id: "2",
    data: "{count}",
    display_text: "{count}",
    deletable: false,
    cloneable: false,
    can_reorder: true,
  },
  { span_type: "opening", type: "fmt:bold", id: "3", data: "<b>" },
  { span_type: "closing", type: "fmt:bold", id: "3", data: "</b>" },
];

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Editor/Formatting/FormatAwareEditing",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component: [
          "Demonstrates format-aware editing powered by vocabularies.",
          "",
          "The same text appears identically in the editor regardless of whether it",
          "came from an HTML file, a Markdown document, or an XLIFF exchange file.",
          "This is because all three formats map to the same vocabulary types:",
          "",
          '- HTML `<b>` = Markdown `**` = XLIFF `<pc dataRefStart="d1">` → **fmt:bold**',
          '- HTML `<a>` = Markdown `[]()` = XLIFF `<pc dataRefStart="d2">` → **link:hyperlink**',
          "",
          "Translators see the same visual experience regardless of the source format.",
        ].join("\n"),
      },
    },
  },
};

export default meta;
type Story = StoryObj;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function FormatCard({
  format,
  description,
  nativeExample,
  codedText,
  spans,
}: {
  format: string;
  description: string;
  nativeExample: string;
  codedText: string;
  spans: SpanInfo[];
}) {
  return (
    <div style={cardStyle}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 8,
        }}
      >
        <div>
          <div style={{ fontSize: 13, fontWeight: 600 }}>{format}</div>
          <div style={{ fontSize: 10, color: "#888" }}>{description}</div>
        </div>
        <FormatVocabularyBadge spans={spans} />
      </div>
      <div
        style={{
          fontSize: 10,
          fontFamily: "monospace",
          color: "#888",
          marginBottom: 8,
          padding: "4px 8px",
          backgroundColor: "rgba(128,128,128,0.08)",
          borderRadius: 4,
          whiteSpace: "pre-wrap",
        }}
      >
        {nativeExample}
      </div>
      <div style={{ marginBottom: 6 }}>
        <div style={sectionLabel}>Formatted view (what translators see)</div>
        <div style={{ fontSize: 14, lineHeight: 1.6 }}>
          <FormattedSourceDisplay codedText={codedText} spans={spans} />
        </div>
      </div>
      <div>
        <div style={sectionLabel}>Tag chip view (advanced)</div>
        <div style={{ fontSize: 14, lineHeight: 1.6 }}>
          <SourceCellDisplay codedText={codedText} spans={spans} />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Same sentence from three different formats renders identically. */
export const CrossFormatConsistency: Story = {
  render: () => (
    <div style={{ maxWidth: 700, padding: 16 }}>
      <h3 style={titleStyle}>Cross-Format Consistency</h3>
      <p style={descStyle}>
        The same sentence extracted from HTML, Markdown, and XLIFF files. Despite completely
        different native syntax, the translator sees identical visual output because all three map
        to the same vocabulary types.
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <FormatCard
          format="HTML"
          description=".html file"
          nativeExample={
            'Click <b class="cta">here</b> to <a href="https://docs.example.com">learn more</a>'
          }
          codedText={sentence}
          spans={htmlSpans}
        />
        <FormatCard
          format="Markdown"
          description=".md file"
          nativeExample="Click **here** to [learn more](https://docs.example.com)"
          codedText={sentence}
          spans={markdownSpans}
        />
        <FormatCard
          format="XLIFF 2.0"
          description=".xlf file"
          nativeExample={
            'Click <pc id="1" dataRefStart="d1">here</pc> to <pc id="2" dataRefStart="d2">learn more</pc>'
          }
          codedText={sentence}
          spans={xliffSpans}
        />
      </div>
    </div>
  ),
};

/** Rich content with multiple tag types showing consistent treatment. */
export const RichContentAcrossFormats: Story = {
  render: () => (
    <div style={{ maxWidth: 700, padding: 16 }}>
      <h3 style={titleStyle}>Rich Content: HTML vs Markdown</h3>
      <p style={descStyle}>
        A more complex segment with bold, code, line break, and link. Both formats produce identical
        editor output.
      </p>
      <div style={{ display: "flex", gap: 16 }}>
        <div style={{ flex: 1 }}>
          <FormatCard
            format="HTML"
            description="<strong>, <code>, <br/>, <a>"
            nativeExample={
              '<strong>Important:</strong> Use <code>kapi init</code> to get started.<br/>See the <a href="/docs">docs</a>.'
            }
            codedText={richSentence}
            spans={htmlRichSpans}
          />
        </div>
        <div style={{ flex: 1 }}>
          <FormatCard
            format="Markdown"
            description="**, `, newline, []()"
            nativeExample={
              "**Important:** Use `kapi init` to get started.  \nSee the [docs](/docs)."
            }
            codedText={richSentence}
            spans={mdRichSpans}
          />
        </div>
      </div>
    </div>
  ),
};

/** Code tokens from i18n message formats (ICU, printf, etc.). */
export const CodeTokensAndVariables: Story = {
  render: () => (
    <div style={{ maxWidth: 700, padding: 16 }}>
      <h3 style={titleStyle}>Variables and Placeholders</h3>
      <p style={descStyle}>
        i18n message formats embed variables and placeholders that must be preserved during
        translation. The vocabulary system marks these as non-deletable and non-cloneable,
        protecting them from accidental changes.
      </p>
      <FormatCard
        format="JSON i18n"
        description="ICU message format variables"
        nativeExample={'{ "greeting": "Hello {userName}, you have {count} new <b>messages</b>." }'}
        codedText={i18nSentence}
        spans={i18nSpans}
      />
      <div style={{ marginTop: 16 }}>
        <div style={sectionLabel}>Tag palette (with constraint indicators)</div>
        <TagPalette sourceSpans={i18nSpans} onInsert={() => {}} showCategoryGroups />
      </div>
      <div style={{ marginTop: 16 }}>
        <div style={sectionLabel}>Inline code legend</div>
        <InlineCodeLegend spans={i18nSpans} onClose={() => {}} />
      </div>
    </div>
  ),
};

/** Tag palette with category separators for mixed-category spans. */
export const TagPaletteWithCategories: Story = {
  render: () => {
    const mixedSpans: SpanInfo[] = [
      ...htmlRichSpans,
      {
        span_type: "placeholder",
        type: "code:variable",
        id: "5",
        data: "{version}",
        display_text: "{version}",
        deletable: false,
        cloneable: false,
      },
    ];

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <h3 style={titleStyle}>Tag Palette with Category Groups</h3>
        <p style={descStyle}>
          When a segment contains tags from multiple vocabulary categories, the palette shows
          category separators for clarity.
        </p>
        <TagPalette sourceSpans={mixedSpans} onInsert={() => {}} showCategoryGroups />
      </div>
    );
  },
};

// ---------------------------------------------------------------------------
// Shared styles
// ---------------------------------------------------------------------------

const titleStyle: React.CSSProperties = {
  fontSize: 14,
  fontWeight: 600,
  marginBottom: 4,
};

const descStyle: React.CSSProperties = {
  fontSize: 12,
  color: "#888",
  marginBottom: 16,
  lineHeight: 1.5,
};

const sectionLabel: React.CSSProperties = {
  fontSize: 10,
  fontWeight: 600,
  color: "#888",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: 4,
};

const cardStyle: React.CSSProperties = {
  padding: 16,
  borderRadius: 8,
  border: "1px solid rgba(128,128,128,0.2)",
  backgroundColor: "rgba(128,128,128,0.03)",
};
