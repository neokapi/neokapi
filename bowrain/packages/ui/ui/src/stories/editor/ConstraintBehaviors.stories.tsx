/**
 * ConstraintBehaviors — demonstrates the vocabulary-driven editing
 * constraint system: deletable, cloneable, and reorderable flags.
 *
 * These constraints protect content integrity by controlling what
 * translators can and cannot do with inline tags.
 */
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagChipComponent } from "../../components/editor/TagChipComponent";
import { TagPalette } from "../../components/editor/TagPalette";
import { TagValidationBar } from "../../components/editor/TagValidationBar";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import type { SpanInfo } from "../../types/api";
import type { TagValidationResult } from "../../components/editor/tagSemantics";
import { boldOpen, boldClose, lineBreak } from "../fixtures";

// ---------------------------------------------------------------------------
// Unicode markers
// ---------------------------------------------------------------------------

const O = "\uE001";
const C = "\uE002";
const P = "\uE003";

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Editor/ConstraintBehaviors",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component: [
          "Demonstrates the three editing constraints defined by vocabularies.",
          "",
          "Each inline code type declares constraints that control translator behavior:",
          "",
          "| Constraint | What it controls | Example |",
          "|---|---|---|",
          "| **Deletable** | Can the tag be removed from the translation? | Line breaks and variables are non-deletable |",
          "| **Cloneable** | Can the tag be duplicated in the translation? | Variables cannot be duplicated |",
          "| **Reorderable** | Can the tag be moved relative to others? | ICU functions must stay in position |",
          "",
          "Constraints are defined in the vocabulary but can be overridden per-span.",
        ].join("\n"),
      },
    },
  },
};

export default meta;
type Story = StoryObj;

// ---------------------------------------------------------------------------
// Constraint-specific spans
// ---------------------------------------------------------------------------

// Fully flexible tags
const flexBoldOpen: SpanInfo = { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" };
const flexBoldClose: SpanInfo = { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" };

// Non-deletable (required in translation)
const requiredBreak: SpanInfo = { span_type: "placeholder", type: "struct:break", id: "2", data: "<br/>", deletable: false, cloneable: false, can_reorder: false };
const requiredVariable: SpanInfo = { span_type: "placeholder", type: "code:variable", id: "3", data: "{userName}", display_text: "{userName}", deletable: false, cloneable: false, can_reorder: true };
const requiredPlaceholder: SpanInfo = { span_type: "placeholder", type: "code:placeholder", id: "4", data: "{0}", display_text: "{0}", deletable: false, cloneable: false, can_reorder: true };

// Non-reorderable (fixed position)
const fixedFunction: SpanInfo = { span_type: "opening", type: "code:function", id: "5", data: "{count, plural,", display_text: "plural(", deletable: false, cloneable: false, can_reorder: false };
const fixedFunctionClose: SpanInfo = { span_type: "closing", type: "code:function", id: "5", data: "}", display_text: ")", deletable: false, cloneable: false, can_reorder: false };

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Overview of all three constraint levels. */
export const ConstraintOverview: Story = {
  render: () => (
    <div style={{ maxWidth: 640, padding: 16 }}>
      <h3 style={titleStyle}>Constraint Levels</h3>
      <p style={descStyle}>
        Vocabulary types define three orthogonal constraints. Tags display
        visual indicators based on their constraint level.
      </p>

      {/* Fully flexible */}
      <div style={sectionStyle}>
        <h4 style={sectionTitleStyle}>Fully Flexible</h4>
        <p style={sectionDescStyle}>
          Formatting tags like bold and italic can be freely deleted, duplicated,
          and reordered. Translators have full control.
        </p>
        <div style={chipRowStyle}>
          <TagChipComponent spanInfo={flexBoldOpen} showConstraints />
          <TagChipComponent spanInfo={flexBoldClose} showConstraints />
          <span style={hintStyle}>No constraint indicators — full freedom</span>
        </div>
      </div>

      {/* Required (non-deletable) */}
      <div style={sectionStyle}>
        <h4 style={sectionTitleStyle}>Required Tags (non-deletable)</h4>
        <p style={sectionDescStyle}>
          Line breaks and variables must appear in the translation.
          The editor prevents accidental deletion and shows a dashed border.
        </p>
        <div style={chipRowStyle}>
          <TagChipComponent spanInfo={requiredBreak} showConstraints locked />
          <TagChipComponent spanInfo={requiredVariable} showConstraints locked />
          <TagChipComponent spanInfo={requiredPlaceholder} showConstraints locked />
          <span style={hintStyle}>Dashed border + * indicator</span>
        </div>
      </div>

      {/* Fixed position (non-reorderable) */}
      <div style={sectionStyle}>
        <h4 style={sectionTitleStyle}>Fixed Position (non-reorderable)</h4>
        <p style={sectionDescStyle}>
          ICU plural/select functions and structural elements must maintain
          their position relative to other tags.
        </p>
        <div style={chipRowStyle}>
          <TagChipComponent spanInfo={fixedFunction} showConstraints locked />
          <TagChipComponent spanInfo={fixedFunctionClose} showConstraints locked />
          <span style={hintStyle}>Non-deletable + non-cloneable + fixed position</span>
        </div>
      </div>
    </div>
  ),
};

/** Tag palette showing how constrained tags behave when already used. */
export const PaletteConstraintBehavior: Story = {
  render: () => {
    const sourceSpans: SpanInfo[] = [
      flexBoldOpen, flexBoldClose,
      requiredVariable,
      requiredPlaceholder,
      requiredBreak,
    ];

    // Simulate: bold and variable already used in target
    const usedSpans: SpanInfo[] = [
      flexBoldOpen, flexBoldClose,
      requiredVariable,
    ];

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <h3 style={titleStyle}>Palette Constraint Behavior</h3>
        <p style={descStyle}>
          When tags are already used in the target, the palette dims them.
          Non-cloneable tags are blocked (cannot be inserted again),
          while cloneable tags like bold remain insertable (just dimmed).
        </p>
        <div style={{ marginBottom: 16 }}>
          <div style={sectionLabel}>Source tags:</div>
          <TagPalette sourceSpans={sourceSpans} onInsert={() => {}} showCategoryGroups />
        </div>
        <div>
          <div style={sectionLabel}>After using bold + variable in target:</div>
          <TagPalette sourceSpans={sourceSpans} usedSpans={usedSpans} onInsert={() => {}} showCategoryGroups />
          <p style={{ ...sectionDescStyle, marginTop: 8 }}>
            Bold tags are dimmed but still insertable (cloneable).
            The variable tag is blocked — hovering shows "cannot be duplicated".
          </p>
        </div>
      </div>
    );
  },
};

/** Validation bar showing constraint violations. */
export const ConstraintViolations: Story = {
  render: () => {
    const validResult: TagValidationResult = {
      valid: true,
      errors: [],
      warnings: [],
    };

    const missingRequired: TagValidationResult = {
      valid: false,
      errors: [
        { type: "deleted_non_deletable", message: 'Missing 1 non-deletable placeholder "Variable" tag' },
        { type: "deleted_non_deletable", message: 'Missing 1 non-deletable placeholder "Line Break" tag' },
      ],
      warnings: [],
    };

    const duplicatedNonCloneable: TagValidationResult = {
      valid: false,
      errors: [
        { type: "cloned_non_cloneable", message: 'Duplicated 1 non-cloneable placeholder "Variable" tag' },
      ],
      warnings: [],
    };

    const mixedIssues: TagValidationResult = {
      valid: false,
      errors: [
        { type: "deleted_non_deletable", message: 'Missing 1 non-deletable placeholder "Placeholder" tag' },
        { type: "unpaired", message: 'Closing "Bold" without matching opening tag' },
      ],
      warnings: [
        { type: "extra_tag", message: 'Extra 1 opening "Italic" tag' },
      ],
    };

    return (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <h3 style={titleStyle}>Constraint Violation Messages</h3>
        <p style={descStyle}>
          The validation bar shows real-time feedback when constraints are violated.
          Non-deletable tags generate errors when missing. Non-cloneable tags
          generate errors when duplicated.
        </p>

        <div style={sectionStyle}>
          <div style={sectionLabel}>All tags present (valid):</div>
          <TagValidationBar validation={validResult} />
          <p style={hintStyle}>No validation bar shown — clean state.</p>
        </div>

        <div style={sectionStyle}>
          <div style={sectionLabel}>Required tags missing:</div>
          <TagValidationBar validation={missingRequired} />
        </div>

        <div style={sectionStyle}>
          <div style={sectionLabel}>Non-cloneable tag duplicated:</div>
          <TagValidationBar validation={duplicatedNonCloneable} />
        </div>

        <div style={sectionStyle}>
          <div style={sectionLabel}>Multiple issues:</div>
          <TagValidationBar validation={mixedIssues} />
        </div>
      </div>
    );
  },
};

/** Full editing scenario showing how constraints protect content. */
export const RealWorldScenario: Story = {
  render: () => {
    const sourceText = `Hello ${P}, you have ${P} new ${O}messages${C} in your ${O}inbox${C}.${P}Check them now.`;
    const sourceSpans: SpanInfo[] = [
      { span_type: "placeholder", type: "code:variable", id: "1", data: "{userName}", display_text: "{userName}", deletable: false, cloneable: false, can_reorder: true },
      { span_type: "placeholder", type: "code:placeholder", id: "2", data: "{count}", display_text: "{count}", deletable: false, cloneable: false, can_reorder: true },
      { span_type: "opening", type: "fmt:bold", id: "3", data: "<b>" },
      { span_type: "closing", type: "fmt:bold", id: "3", data: "</b>" },
      { span_type: "opening", type: "link:hyperlink", id: "4", data: '<a href="/inbox">' },
      { span_type: "closing", type: "link:hyperlink", id: "4", data: "</a>" },
      { span_type: "placeholder", type: "struct:break", id: "5", data: "<br/>", deletable: false, cloneable: false, can_reorder: false },
    ];

    return (
      <div style={{ maxWidth: 700, padding: 16 }}>
        <h3 style={titleStyle}>Real-World: Notification Message</h3>
        <p style={descStyle}>
          A typical notification string mixing variables, formatting, links,
          and a line break. The legend shows how each type is constrained.
        </p>

        <div style={{ marginBottom: 16 }}>
          <div style={sectionLabel}>Formatted view</div>
          <div style={{ fontSize: 14, lineHeight: 1.8 }}>
            <FormattedSourceDisplay codedText={sourceText} spans={sourceSpans} />
          </div>
        </div>

        <div style={{ marginBottom: 16 }}>
          <div style={sectionLabel}>Code view</div>
          <div style={{ fontSize: 14, lineHeight: 1.8 }}>
            <SourceCellDisplay codedText={sourceText} spans={sourceSpans} />
          </div>
        </div>

        <div style={{ marginBottom: 16 }}>
          <div style={sectionLabel}>Tag palette</div>
          <TagPalette sourceSpans={sourceSpans} onInsert={() => {}} showCategoryGroups />
        </div>

        <div>
          <InlineCodeLegend spans={sourceSpans} onClose={() => {}} />
        </div>
      </div>
    );
  },
};

// ---------------------------------------------------------------------------
// Shared styles
// ---------------------------------------------------------------------------

const titleStyle: React.CSSProperties = { fontSize: 14, fontWeight: 600, marginBottom: 4 };
const descStyle: React.CSSProperties = { fontSize: 12, color: "#888", marginBottom: 16, lineHeight: 1.5 };
const sectionStyle: React.CSSProperties = { marginBottom: 20 };
const sectionTitleStyle: React.CSSProperties = { fontSize: 12, fontWeight: 600, marginBottom: 4 };
const sectionDescStyle: React.CSSProperties = { fontSize: 11, color: "#888", marginBottom: 8, lineHeight: 1.4 };
const sectionLabel: React.CSSProperties = { fontSize: 10, fontWeight: 600, color: "#888", textTransform: "uppercase", letterSpacing: "0.05em", marginBottom: 4 };
const chipRowStyle: React.CSSProperties = { display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" };
const hintStyle: React.CSSProperties = { fontSize: 10, color: "#999", fontStyle: "italic", marginLeft: 4 };
