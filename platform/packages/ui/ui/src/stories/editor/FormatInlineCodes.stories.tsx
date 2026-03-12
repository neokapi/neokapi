/**
 * FormatInlineCodes — demonstrates how inline codes are rendered as
 * formatted text (the default translator experience) versus tag chips
 * (the opt-in code view for advanced users).
 *
 * Shows that HTML spans and Markdown spans produce identical visual
 * output when rendered through FormattedSourceDisplay, because both
 * resolve to the same semantic categories.
 */
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import { FormatVocabularyBadge } from "../../components/editor/FormatVocabularyBadge";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
import type { SpanInfo } from "../../types/api";
import {
  simpleBoldCodedText, simpleBoldSpans,
  linkAndItalicCodedText, linkAndItalicSpans,
  codeInlineCodedText, codeInlineSpans,
  lineBreakCodedText, lineBreakSpans,
  richCodedText, richSpans,
  mdFormattingCodedText, mdFormattingSpans,
  mdRichCodedText, mdRichSpans,
  underlineOpen, underlineClose,
  strikeOpen, strikeClose,
  supOpen, supClose,
  imgTag,
} from "../fixtures";

// ---------------------------------------------------------------------------
// Unicode markers
// ---------------------------------------------------------------------------

const O = "\uE001";
const C = "\uE002";
const P = "\uE003";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

function Comparison({
  label,
  codedText,
  spans,
}: {
  label: string;
  codedText: string;
  spans: SpanInfo[];
}) {
  return (
    <div style={{ marginBottom: 16 }}>
      <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
        {label}
      </div>
      <div style={{ fontSize: 14, lineHeight: 1.6 }}>
        <FormattedSourceDisplay codedText={codedText} spans={spans} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Editor/FormatInlineCodes",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component: [
          "Shows how inline codes are rendered in the default formatted view.",
          "",
          "- **Formatted view** (default): text appears with actual formatting — bold is bold, links are underlined, code is monospace — with faint background tints marking span boundaries.",
          "- **Code view** (opt-in): text shows abstract tag chips like `[B>] text [/B]` for advanced users who need to see the raw structure.",
          "",
          "HTML and Markdown spans produce identical visual output because both resolve to the same semantic categories (bold, italic, link, code, etc.).",
        ].join("\n"),
      },
    },
  },
};

export default meta;
type Story = StoryObj;

/**
 * HTML inline codes rendered as formatted text with background tints.
 */
export const HTMLFormat: Story = {
  render: () => (
    <div style={{ maxWidth: 640, padding: 16 }}>
      <Section title="HTML Spans — Formatted View">
        <Comparison label="Bold" codedText={simpleBoldCodedText} spans={simpleBoldSpans} />
        <Comparison label="Link + Italic" codedText={linkAndItalicCodedText} spans={linkAndItalicSpans} />
        <Comparison label="Inline Code" codedText={codeInlineCodedText} spans={codeInlineSpans} />
        <Comparison label="Line Break" codedText={lineBreakCodedText} spans={lineBreakSpans} />
      </Section>
    </div>
  ),
};

/**
 * Markdown inline codes rendered as formatted text — same visual output
 * as HTML despite different underlying syntax.
 */
export const MarkdownFormat: Story = {
  render: () => (
    <div style={{ maxWidth: 640, padding: 16 }}>
      <Section title="Markdown Spans — Formatted View">
        <Comparison
          label="Bold + Italic (** and *)"
          codedText={mdFormattingCodedText}
          spans={mdFormattingSpans}
        />
        <Comparison
          label="Code + Link (` and []())"
          codedText={mdRichCodedText}
          spans={mdRichSpans}
        />
      </Section>
    </div>
  ),
};

/**
 * Side-by-side comparison showing that HTML and Markdown spans produce
 * identical visual output when rendered through FormattedSourceDisplay.
 */
export const SemanticEquivalence: Story = {
  render: () => {
    // Same sentence, different underlying markup
    const sentence = `Click ${O}here${C} to ${O}learn more${C}`;

    const htmlSpans: SpanInfo[] = [
      { span_type: "opening", type: "b", id: "1", data: "<b>" },
      { span_type: "closing", type: "b", id: "1", data: "</b>" },
      { span_type: "opening", type: "i", id: "2", data: "<i>" },
      { span_type: "closing", type: "i", id: "2", data: "</i>" },
    ];

    const mdSpans: SpanInfo[] = [
      { span_type: "opening", type: "bold", id: "1", data: "**" },
      { span_type: "closing", type: "bold", id: "1", data: "**" },
      { span_type: "opening", type: "italic", id: "2", data: "*" },
      { span_type: "closing", type: "italic", id: "2", data: "*" },
    ];

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="Semantic Equivalence — HTML vs Markdown">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 16 }}>
            Same sentence with HTML tags vs Markdown delimiters. Both produce
            identical visual rendering because they map to the same semantic
            categories (bold, italic).
          </p>
          <div style={{ display: "flex", gap: 32 }}>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
                HTML ({`<b>, <i>`})
              </div>
              <div style={{ fontSize: 14, lineHeight: 1.6 }}>
                <FormattedSourceDisplay codedText={sentence} spans={htmlSpans} />
              </div>
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
                Markdown (**, *)
              </div>
              <div style={{ fontSize: 14, lineHeight: 1.6 }}>
                <FormattedSourceDisplay codedText={sentence} spans={mdSpans} />
              </div>
            </div>
          </div>
        </Section>
      </div>
    );
  },
};

/**
 * The same segments shown with tag chips — the opt-in code view
 * for advanced users who need to see raw inline code structure.
 */
export const CodeView: Story = {
  render: () => (
    <div style={{ maxWidth: 640, padding: 16 }}>
      <Section title="Code View — Tag Chips (Opt-in)">
        <p style={{ fontSize: 12, color: "#999", marginBottom: 12 }}>
          The same content shown with tag chip rendering. This is the advanced
          view that translators can toggle to when they need to see the raw
          inline code structure.
        </p>
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
            HTML Bold + Link
          </div>
          <div style={{ fontSize: 14, lineHeight: 1.6 }}>
            <SourceCellDisplay codedText={richCodedText} spans={richSpans} />
          </div>
        </div>
        <div>
          <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
            Markdown Code + Link
          </div>
          <div style={{ fontSize: 14, lineHeight: 1.6 }}>
            <SourceCellDisplay codedText={mdRichCodedText} spans={mdRichSpans} />
          </div>
        </div>
      </Section>
    </div>
  ),
};

/**
 * One example per semantic category showing the formatted rendering.
 */
export const AllCategories: Story = {
  render: () => {
    const categories: Array<{
      label: string;
      codedText: string;
      spans: SpanInfo[];
    }> = [
      {
        label: "Bold",
        codedText: `This is ${O}bold text${C}`,
        spans: [
          { span_type: "opening", type: "bold", id: "1", data: "**" },
          { span_type: "closing", type: "bold", id: "1", data: "**" },
        ],
      },
      {
        label: "Italic",
        codedText: `This is ${O}italic text${C}`,
        spans: [
          { span_type: "opening", type: "italic", id: "2", data: "*" },
          { span_type: "closing", type: "italic", id: "2", data: "*" },
        ],
      },
      {
        label: "Underline",
        codedText: `This is ${O}underlined text${C}`,
        spans: [underlineOpen, underlineClose],
      },
      {
        label: "Strikethrough",
        codedText: `This is ${O}deleted text${C}`,
        spans: [strikeOpen, strikeClose],
      },
      {
        label: "Link",
        codedText: `Visit ${O}our website${C} for more`,
        spans: [
          { span_type: "opening", type: "link", id: "5", data: "[" },
          { span_type: "closing", type: "link", id: "5", data: "](https://example.com)" },
        ],
      },
      {
        label: "Code",
        codedText: `Run ${O}kapi init${C} to start`,
        spans: [
          { span_type: "opening", type: "code", id: "6", data: "`" },
          { span_type: "closing", type: "code", id: "6", data: "`" },
        ],
      },
      {
        label: "Superscript",
        codedText: `E=mc${O}2${C}`,
        spans: [supOpen, supClose],
      },
      {
        label: "Line Break (placeholder)",
        codedText: `First line${P}Second line`,
        spans: [{ span_type: "placeholder" as const, type: "br", id: "9", data: "<br/>" }],
      },
      {
        label: "Image (placeholder)",
        codedText: `See the logo ${P} below`,
        spans: [imgTag],
      },
    ];

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="All Semantic Categories">
          {categories.map((cat) => (
            <div key={cat.label} style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
                {cat.label}
              </div>
              <div style={{ fontSize: 14, lineHeight: 1.6 }}>
                <FormattedSourceDisplay codedText={cat.codedText} spans={cat.spans} />
              </div>
            </div>
          ))}
        </Section>
      </div>
    );
  },
};

/**
 * Full vocabulary-driven experience showing badge, legend, and formatted
 * rendering working together — the complete translator workflow.
 */
export const VocabularyDrivenExperience: Story = {
  render: () => {
    const allSpans: SpanInfo[] = [
      { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" },
      { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" },
      { span_type: "opening", type: "link:hyperlink", id: "2", data: '<a href="https://example.com">' },
      { span_type: "closing", type: "link:hyperlink", id: "2", data: "</a>" },
      { span_type: "opening", type: "fmt:code", id: "3", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "3", data: "</code>" },
      { span_type: "placeholder", type: "code:variable", id: "4", data: "{version}", display_text: "{version}", deletable: false, cloneable: false, can_reorder: true },
      { span_type: "placeholder", type: "struct:break", id: "5", data: "<br/>" },
    ];

    const codedText = `${O}Download${C} the latest ${O}release${C} (${O}v${C}${P}).${P}Visit the ${O}docs${C} for details.`;

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="Vocabulary-Driven Experience">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 16 }}>
            The complete translator workflow: vocabulary badge summarizes tag
            categories at a glance, the formatted view shows text naturally,
            and the legend explains each tag type with its constraints.
          </p>

          {/* Badge */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Vocabulary Badge
            </div>
            <FormatVocabularyBadge spans={allSpans} />
          </div>

          {/* Formatted view */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Formatted View
            </div>
            <div style={{ fontSize: 14, lineHeight: 1.6 }}>
              <FormattedSourceDisplay codedText={codedText} spans={allSpans} />
            </div>
          </div>

          {/* Code view */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Code View
            </div>
            <div style={{ fontSize: 14, lineHeight: 1.6 }}>
              <SourceCellDisplay codedText={codedText} spans={allSpans} />
            </div>
          </div>

          {/* Legend */}
          <div>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Inline Code Legend
            </div>
            <InlineCodeLegend spans={allSpans} onClose={() => {}} />
          </div>
        </Section>
      </div>
    );
  },
};
