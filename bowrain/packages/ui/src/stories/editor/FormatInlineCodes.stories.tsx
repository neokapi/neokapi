/**
 * FormatInlineCodes — demonstrates how inline codes from different source
 * formats are rendered in the formatted (default) and code (opt-in) views.
 *
 * Shows that HTML codes and Markdown codes produce identical visual
 * output when they map to the same semantic categories.
 */
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Run } from "@neokapi/kapi-format";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import { FormatVocabularyBadge } from "../../components/editor/FormatVocabularyBadge";
import { codeSpans } from "../../components/editor/editorText";
import { InlineCodeLegend } from "@neokapi/ui-primitives";
import {
  simpleBoldRuns,
  linkAndItalicRuns,
  codeInlineRuns,
  lineBreakRuns,
  richRuns,
  mdFormattingRuns,
  mdRichRuns,
  underlineOpen,
  underlineClose,
  strikeOpen,
  strikeClose,
  supOpen,
  supClose,
  imgTag,
  pair,
  runsOf,
} from "../fixtures";

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

function Comparison({ label, runs }: { label: string; runs: Run[] }) {
  return (
    <div style={{ marginBottom: 16 }}>
      <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>{label}</div>
      <div style={{ fontSize: 14, lineHeight: 1.6 }}>
        <FormattedSourceDisplay runs={runs} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Editor/Formatting/FormatInlineCodes",
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
          "HTML and Markdown codes produce identical visual output because both resolve to the same semantic categories (bold, italic, link, code, etc.).",
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
      <Section title="HTML Codes — Formatted View">
        <Comparison label="Bold" runs={simpleBoldRuns} />
        <Comparison label="Link + Italic" runs={linkAndItalicRuns} />
        <Comparison label="Inline Code" runs={codeInlineRuns} />
        <Comparison label="Line Break" runs={lineBreakRuns} />
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
      <Section title="Markdown Codes — Formatted View">
        <Comparison label="Bold + Italic (** and *)" runs={mdFormattingRuns} />
        <Comparison label="Code + Link (` and []())" runs={mdRichRuns} />
      </Section>
    </div>
  ),
};

/**
 * Side-by-side comparison showing that HTML and Markdown codes produce
 * identical visual output when rendered through FormattedSourceDisplay.
 */
export const SemanticEquivalence: Story = {
  render: () => {
    // Same sentence, different underlying markup
    const sentence = (bold: [Run, Run], italic: [Run, Run]) =>
      runsOf("Click ", bold[0], "here", bold[1], " to ", italic[0], "learn more", italic[1]);

    const htmlRuns = sentence(
      pair("1", "b", "<b>", "</b>", "b"),
      pair("2", "i", "<i>", "</i>", "i"),
    );
    const mdRuns = sentence(pair("1", "bold", "**", "**", "b"), pair("2", "italic", "*", "*", "i"));

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="Semantic Equivalence — HTML vs Markdown">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 16 }}>
            Same sentence with HTML tags vs Markdown delimiters. Both produce identical visual
            rendering because they map to the same semantic categories (bold, italic).
          </p>
          <div style={{ display: "flex", gap: 32 }}>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
                HTML ({`<b>, <i>`})
              </div>
              <div style={{ fontSize: 14, lineHeight: 1.6 }}>
                <FormattedSourceDisplay runs={htmlRuns} />
              </div>
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
                Markdown (**, *)
              </div>
              <div style={{ fontSize: 14, lineHeight: 1.6 }}>
                <FormattedSourceDisplay runs={mdRuns} />
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
          The same content shown with tag chip rendering. This is the advanced view that translators
          can toggle to when they need to see the raw inline code structure.
        </p>
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
            HTML Bold + Link
          </div>
          <div style={{ fontSize: 14, lineHeight: 1.6 }}>
            <SourceCellDisplay runs={richRuns} />
          </div>
        </div>
        <div>
          <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
            Markdown Code + Link
          </div>
          <div style={{ fontSize: 14, lineHeight: 1.6 }}>
            <SourceCellDisplay runs={mdRichRuns} />
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
    const wrap = (prefix: string, codes: [Run, Run], inner: string, suffix = "") =>
      runsOf(prefix, codes[0], inner, codes[1], suffix);
    const categories: Array<{ label: string; runs: Run[] }> = [
      { label: "Bold", runs: wrap("This is ", pair("1", "bold", "**", "**", "b"), "bold text") },
      {
        label: "Italic",
        runs: wrap("This is ", pair("2", "italic", "*", "*", "i"), "italic text"),
      },
      {
        label: "Underline",
        runs: wrap("This is ", [underlineOpen, underlineClose], "underlined text"),
      },
      { label: "Strikethrough", runs: wrap("This is ", [strikeOpen, strikeClose], "deleted text") },
      {
        label: "Link",
        runs: wrap(
          "Visit ",
          pair("5", "link", "[", "](https://example.com)", "a"),
          "our website",
          " for more",
        ),
      },
      {
        label: "Code",
        runs: wrap("Run ", pair("6", "code", "`", "`", "code"), "kapi init", " to start"),
      },
      { label: "Superscript", runs: wrap("E=mc", [supOpen, supClose], "2") },
      {
        label: "Line Break (placeholder)",
        runs: runsOf(
          "First line",
          { ph: { id: "9", type: "br", data: "<br/>", equiv: "br" } },
          "Second line",
        ),
      },
      { label: "Image (placeholder)", runs: runsOf("See the logo ", imgTag, " below") },
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
                <FormattedSourceDisplay runs={cat.runs} />
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
    const [boldOpen, boldClose] = pair("1", "fmt:bold", "<b>", "</b>", "b");
    const [linkOpen, linkClose] = pair(
      "2",
      "link:hyperlink",
      '<a href="https://example.com">',
      "</a>",
      "a",
    );
    const [codeOpen, codeClose] = pair("3", "fmt:code", "<code>", "</code>", "code");
    const version: Run = {
      ph: {
        id: "4",
        type: "code:variable",
        data: "{version}",
        equiv: "version",
        disp: "{version}",
      },
    };
    const lineBreak: Run = { ph: { id: "5", type: "struct:break", data: "<br/>", equiv: "br" } };

    const allRuns = runsOf(
      boldOpen,
      "Download",
      boldClose,
      " the latest ",
      linkOpen,
      "release",
      linkClose,
      " (",
      codeOpen,
      "v",
      codeClose,
      version,
      ").",
      lineBreak,
      "Visit the ",
      linkOpen,
      "docs",
      linkClose,
      " for details.",
    );

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Section title="Vocabulary-Driven Experience">
          <p style={{ fontSize: 12, color: "#999", marginBottom: 16 }}>
            The complete translator workflow: vocabulary badge summarizes tag categories at a
            glance, the formatted view shows text naturally, and the legend explains each tag type
            with its constraints.
          </p>

          {/* Badge */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Vocabulary Badge
            </div>
            <FormatVocabularyBadge runs={allRuns} />
          </div>

          {/* Formatted view */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Formatted View
            </div>
            <div style={{ fontSize: 14, lineHeight: 1.6 }}>
              <FormattedSourceDisplay runs={allRuns} />
            </div>
          </div>

          {/* Code view */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Code View
            </div>
            <div style={{ fontSize: 14, lineHeight: 1.6 }}>
              <SourceCellDisplay runs={allRuns} />
            </div>
          </div>

          {/* Legend */}
          <div>
            <div style={{ fontSize: 10, color: "#888", fontWeight: 600, marginBottom: 4 }}>
              Inline Code Legend
            </div>
            <InlineCodeLegend spans={codeSpans(allRuns)} onClose={() => {}} />
          </div>
        </Section>
      </div>
    );
  },
};
