/**
 * FormatAwareEditing — demonstrates how the vocabulary system provides
 * a consistent editing experience across different file formats.
 *
 * HTML, Markdown, and XLIFF all use different native syntax for the same
 * concepts (bold, links, etc.), but the editor presents them identically
 * because they map to the same vocabulary types.
 */
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Run } from "@neokapi/kapi-format";
import { TagPalette, InlineCodeLegend, type SpanInfo } from "@neokapi/ui-primitives";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import { FormatVocabularyBadge } from "../../components/editor/FormatVocabularyBadge";
import { codeSpans } from "../../components/editor/editorText";
import { pair, runsOf } from "../fixtures";

// ---------------------------------------------------------------------------
// Format-specific data: same content in three formats
// ---------------------------------------------------------------------------

/** "Click <b>here</b> to <a>learn more</a>" with one format's codes. */
function sentence(bold: [Run, Run], link: [Run, Run]): Run[] {
  return runsOf("Click ", bold[0], "here", bold[1], " to ", link[0], "learn more", link[1]);
}

const htmlRuns = sentence(
  pair("1", "fmt:bold", '<b class="cta">', "</b>", "b"),
  pair("2", "link:hyperlink", '<a href="https://docs.example.com">', "</a>", "a"),
);

const markdownRuns = sentence(
  pair("1", "fmt:bold", "**", "**", "b"),
  pair("2", "link:hyperlink", "[", "](https://docs.example.com)", "a"),
);

/** An XLIFF `<pc>` pair, which names the original element in its subType. */
function xliffPair(id: string, type: string, dataRef: string, subType: string): [Run, Run] {
  return [
    {
      pcOpen: {
        id,
        type,
        subType,
        data: `<pc id="${id}" dataRefStart="${dataRef}">`,
        equiv: subType,
      },
    },
    { pcClose: { id, type, subType, data: "</pc>", equiv: subType } },
  ];
}

const xliffRuns = sentence(
  xliffPair("1", "fmt:bold", "d1", "xlf:b"),
  xliffPair("2", "link:hyperlink", "d2", "xlf:a"),
);

// ---------------------------------------------------------------------------
// Richer format-specific examples
// ---------------------------------------------------------------------------

/** "<strong>Important:</strong> Use <code>kapi init</code> to get started.<br/>See the <a>docs</a> for more." */
function richSentence(bold: [Run, Run], code: [Run, Run], lineBreak: Run, link: [Run, Run]): Run[] {
  return runsOf(
    bold[0],
    "Important:",
    bold[1],
    " Use ",
    code[0],
    "kapi init",
    code[1],
    " to get started.",
    lineBreak,
    "See the ",
    link[0],
    "docs",
    link[1],
    " for more.",
  );
}

const htmlRichRuns = richSentence(
  pair("1", "fmt:bold", "<strong>", "</strong>", "strong"),
  pair("2", "fmt:code", "<code>", "</code>", "code"),
  { ph: { id: "3", type: "struct:break", data: "<br/>", equiv: "br" } },
  pair("4", "link:hyperlink", '<a href="/docs">', "</a>", "a"),
);

const mdRichRuns = richSentence(
  pair("1", "fmt:bold", "**", "**", "b"),
  pair("2", "fmt:code", "`", "`", "code"),
  { ph: { id: "3", type: "struct:break", data: "  \n", equiv: "br" } },
  pair("4", "link:hyperlink", "[", "](/docs)", "a"),
);

// ---------------------------------------------------------------------------
// Code token example (ICU / i18n message format)
// ---------------------------------------------------------------------------

const [i18nBoldOpen, i18nBoldClose] = pair("3", "fmt:bold", "<b>", "</b>", "b");
const i18nRuns = runsOf(
  "Hello ",
  {
    ph: {
      id: "1",
      type: "code:variable",
      data: "{userName}",
      equiv: "userName",
      disp: "{userName}",
    },
  },
  ", you have ",
  { ph: { id: "2", type: "code:placeholder", data: "{count}", equiv: "count", disp: "{count}" } },
  " new ",
  i18nBoldOpen,
  "messages",
  i18nBoldClose,
  ".",
);
const i18nSpans = codeSpans(i18nRuns);

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
  runs,
}: {
  format: string;
  description: string;
  nativeExample: string;
  runs: Run[];
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
        <FormatVocabularyBadge runs={runs} />
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
          <FormattedSourceDisplay runs={runs} />
        </div>
      </div>
      <div>
        <div style={sectionLabel}>Tag chip view (advanced)</div>
        <div style={{ fontSize: 14, lineHeight: 1.6 }}>
          <SourceCellDisplay runs={runs} />
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
          runs={htmlRuns}
        />
        <FormatCard
          format="Markdown"
          description=".md file"
          nativeExample="Click **here** to [learn more](https://docs.example.com)"
          runs={markdownRuns}
        />
        <FormatCard
          format="XLIFF 2.0"
          description=".xlf file"
          nativeExample={
            'Click <pc id="1" dataRefStart="d1">here</pc> to <pc id="2" dataRefStart="d2">learn more</pc>'
          }
          runs={xliffRuns}
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
            runs={htmlRichRuns}
          />
        </div>
        <div style={{ flex: 1 }}>
          <FormatCard
            format="Markdown"
            description="**, `, newline, []()"
            nativeExample={
              "**Important:** Use `kapi init` to get started.  \nSee the [docs](/docs)."
            }
            runs={mdRichRuns}
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
        runs={i18nRuns}
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
      ...codeSpans(htmlRichRuns),
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
