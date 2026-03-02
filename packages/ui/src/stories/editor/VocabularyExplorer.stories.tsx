/**
 * VocabularyExplorer — interactive browsing of all vocabulary categories,
 * span types, visual representations, and editing constraints.
 *
 * Demonstrates how vocabulary-driven semantic types provide a consistent,
 * format-independent editing experience across HTML, Markdown, XLIFF, etc.
 */
import type { Meta, StoryObj } from "@storybook/react-vite";
import { VocabularyExplorer } from "../../components/editor/VocabularyExplorer";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
import { FormatVocabularyBadge } from "../../components/editor/FormatVocabularyBadge";
import { TagChipComponent } from "../../components/editor/TagChipComponent";
import type { SpanInfo } from "../../types/api";
import {
  boldOpen, boldClose, italicOpen, italicClose, linkOpen, linkClose,
  codeOpen, codeClose, lineBreak, imgTag, underlineOpen, underlineClose,
  strikeOpen, strikeClose, supOpen, supClose,
  richSpans,
} from "../fixtures";

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Editor/VocabularyExplorer",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component: [
          "Interactive explorer for the vocabulary system that drives inline code rendering.",
          "",
          "Vocabularies define how each inline code type (bold, links, variables, etc.) looks,",
          "behaves, and is constrained in the translation editor. The same vocabulary drives",
          "consistent behavior across HTML, Markdown, XLIFF, and all other formats.",
          "",
          "**Key features:**",
          "- Category-grouped type browser with chip previews",
          "- Constraint indicators (required, no duplicates, fixed position)",
          "- Format-agnostic — HTML `<b>` and Markdown `**` both map to `fmt:bold`",
        ].join("\n"),
      },
    },
  },
};

export default meta;
type Story = StoryObj;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Browse all vocabulary categories and their span types. */
export const AllVocabularies: Story = {
  render: () => (
    <div style={{ maxWidth: 480, padding: 16 }}>
      <h3 style={titleStyle}>Vocabulary Explorer</h3>
      <p style={descStyle}>
        Click a category to expand it and see all span types, their chip
        representations, and editing constraints.
      </p>
      <VocabularyExplorer />
    </div>
  ),
};

/** Filter mode: only types present in the current document are highlighted. */
export const FilteredByActiveTypes: Story = {
  render: () => (
    <div style={{ maxWidth: 480, padding: 16 }}>
      <h3 style={titleStyle}>Filtered by Document Types</h3>
      <p style={descStyle}>
        When a document only uses certain types, inactive categories are dimmed.
        This document uses bold, italic, and hyperlinks.
      </p>
      <VocabularyExplorer activeTypes={["fmt:bold", "fmt:italic", "link:hyperlink"]} />
    </div>
  ),
};

/** Compact mode for tighter spaces like sidebars. */
export const CompactMode: Story = {
  render: () => (
    <div style={{ maxWidth: 320, padding: 16 }}>
      <h3 style={titleStyle}>Compact Mode</h3>
      <VocabularyExplorer compact />
    </div>
  ),
};

/** Inline code legend showing tags in the current segment. */
export const InlineCodeLegendPanel: Story = {
  render: () => {
    const spans: SpanInfo[] = [
      boldOpen, boldClose,
      linkOpen, linkClose,
      codeOpen, codeClose,
      lineBreak,
    ];

    return (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <h3 style={titleStyle}>Inline Code Legend</h3>
        <p style={descStyle}>
          Shows all inline tag types in the current segment, grouped by category,
          with constraint indicators.
        </p>
        <InlineCodeLegend spans={spans} onClose={() => {}} />
      </div>
    );
  },
};

/** Legend for a segment with code tokens (variables, placeholders). */
export const CodeTokenLegend: Story = {
  render: () => {
    const spans: SpanInfo[] = [
      { span_type: "placeholder", type: "code:variable", id: "1", data: "{userName}", deletable: false, cloneable: false, can_reorder: true },
      { span_type: "placeholder", type: "code:placeholder", id: "2", data: "{0}", deletable: false, cloneable: false, can_reorder: true },
      { span_type: "opening", type: "code:function", id: "3", data: "{count, plural,", deletable: false, cloneable: false, can_reorder: false },
      { span_type: "closing", type: "code:function", id: "3", data: "}", deletable: false, cloneable: false, can_reorder: false },
      boldOpen, boldClose,
    ];

    return (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <h3 style={titleStyle}>Code Tokens + Formatting</h3>
        <p style={descStyle}>
          A segment mixing code tokens (variables, placeholders, ICU functions)
          with formatting tags. Note the different constraint levels.
        </p>
        <InlineCodeLegend spans={spans} onClose={() => {}} />
      </div>
    );
  },
};

/** Format vocabulary badge — compact summary of tag categories. */
export const VocabularyBadge: Story = {
  render: () => (
    <div style={{ maxWidth: 480, padding: 16 }}>
      <h3 style={titleStyle}>Vocabulary Badge</h3>
      <p style={descStyle}>
        Compact inline badge showing which vocabulary categories are active.
        Typically displayed in the editor card header.
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 12, marginTop: 16 }}>
        <div>
          <span style={labelStyle}>Simple formatting:</span>
          <FormatVocabularyBadge spans={[boldOpen, boldClose, italicOpen, italicClose]} />
        </div>
        <div>
          <span style={labelStyle}>Rich content:</span>
          <FormatVocabularyBadge spans={richSpans} />
        </div>
        <div>
          <span style={labelStyle}>Code tokens:</span>
          <FormatVocabularyBadge
            spans={[
              { span_type: "placeholder", type: "code:variable", id: "1", data: "{name}" },
              { span_type: "placeholder", type: "code:placeholder", id: "2", data: "{0}" },
              boldOpen, boldClose,
            ]}
          />
        </div>
      </div>
    </div>
  ),
};

/** All chip styles across every vocabulary type. */
export const AllChipStyles: Story = {
  render: () => {
    const allTypes: Array<{ label: string; spans: SpanInfo[] }> = [
      { label: "Bold", spans: [boldOpen, boldClose] },
      { label: "Italic", spans: [italicOpen, italicClose] },
      { label: "Underline", spans: [underlineOpen, underlineClose] },
      { label: "Strikethrough", spans: [strikeOpen, strikeClose] },
      { label: "Superscript", spans: [supOpen, supClose] },
      { label: "Hyperlink", spans: [linkOpen, linkClose] },
      { label: "Inline Code", spans: [codeOpen, codeClose] },
      { label: "Line Break", spans: [lineBreak] },
      { label: "Image", spans: [imgTag] },
      {
        label: "Variable",
        spans: [{ span_type: "placeholder", type: "code:variable", id: "1", data: "{name}" }],
      },
      {
        label: "Placeholder",
        spans: [{ span_type: "placeholder", type: "code:placeholder", id: "1", data: "{0}" }],
      },
      {
        label: "Function (ICU)",
        spans: [
          { span_type: "opening", type: "code:function", id: "1", data: "{count, plural," },
          { span_type: "closing", type: "code:function", id: "1", data: "}" },
        ],
      },
    ];

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <h3 style={titleStyle}>All Chip Styles</h3>
        <p style={descStyle}>
          Every vocabulary type rendered as tag chips, with constraint indicators.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 16 }}>
          {allTypes.map(({ label, spans }) => (
            <div key={label} style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ ...labelStyle, width: 120 }}>{label}</span>
              <div style={{ display: "flex", gap: 4 }}>
                {spans.map((s, i) => (
                  <TagChipComponent key={i} spanInfo={s} showConstraints />
                ))}
              </div>
            </div>
          ))}
        </div>
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

const labelStyle: React.CSSProperties = {
  fontSize: 12,
  color: "#888",
  marginRight: 8,
  display: "inline-block",
};
