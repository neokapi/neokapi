import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import { UnifiedTargetEditor } from "../../components/UnifiedTargetEditor";
import type { BlockInfo } from "../../types/api";

function makeMessagesBlock(targets: Record<string, string>): BlockInfo {
  return {
    id: "blk-messages",
    source: "You have {count} messages",
    has_spans: true,
    source_spans: [
      {
        span_type: "placeholder",
        type: "jsx:var",
        id: "0",
        data: "{count}",
        equiv_text: "count",
      },
    ],
    targets,
    targets_coded: {},
    translatable: true,
    properties: {},
  };
}

function makeRichBlock(targets: Record<string, string>): BlockInfo {
  return {
    id: "blk-rich",
    source: "Click <strong>here</strong> for {count} pending.",
    has_spans: true,
    source_spans: [
      {
        span_type: "opening",
        type: "fmt:bold",
        id: "0",
        data: "<strong>",
        equiv_text: "strong",
      },
      {
        span_type: "closing",
        type: "fmt:bold",
        id: "0",
        data: "</strong>",
        equiv_text: "strong",
      },
      {
        span_type: "placeholder",
        type: "jsx:var",
        id: "1",
        data: "{count}",
        equiv_text: "count",
      },
    ],
    targets,
    targets_coded: {},
    translatable: true,
    properties: {},
  };
}

function makePlainBlock(targets: Record<string, string>): BlockInfo {
  return {
    id: "blk-plain",
    source: "Welcome back!",
    has_spans: false,
    source_spans: [],
    targets,
    targets_coded: {},
    translatable: true,
    properties: {},
  };
}

const meta: Meta<typeof UnifiedTargetEditor> = {
  title: "Editor/Core/UnifiedTargetEditor",
  component: UnifiedTargetEditor,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Single editor surface for every target — flat or plural, with inline codes or plain. " +
          "Replaces TargetCellEditor + the textarea fallback + the Plurals dialog. Lexical chips " +
          "render identically across modes; plural authoring is a mode toggle inside the editor. " +
          "See AD #408 / #409.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof UnifiedTargetEditor>;

function Wrapper({ block, locale = "de" }: { block: BlockInfo; locale?: string }) {
  const [savedFlat, setSavedFlat] = useState<{ codedText: string; spansCount: number } | null>(
    null,
  );
  const [savedPlural, setSavedPlural] = useState<string | null>(null);
  const [open, setOpen] = useState(true);

  return (
    <div style={{ minHeight: 500, padding: 16, fontFamily: "sans-serif" }}>
      <div style={{ marginBottom: 12, fontSize: 14 }}>
        <strong>Source:</strong> <code>{block.source}</code>
      </div>
      <div style={{ marginBottom: 12, fontSize: 14 }}>
        <strong>Target ({locale}):</strong> <code>{block.targets[locale] ?? "(empty)"}</code>
      </div>
      {open ? (
        <UnifiedTargetEditor
          block={block}
          locale={locale}
          onSave={(result) => {
            if (result.kind === "flat") {
              setSavedFlat({ codedText: result.codedText, spansCount: result.spans.length });
            } else {
              setSavedPlural(result.text);
            }
            setOpen(false);
          }}
          onCancel={() => setOpen(false)}
        />
      ) : (
        <button type="button" onClick={() => setOpen(true)}>
          Reopen editor
        </button>
      )}
      {savedFlat && (
        <pre style={{ background: "#f4f4f5", padding: 8, borderRadius: 4, marginTop: 12 }}>
          flat saved → codedText={JSON.stringify(savedFlat.codedText)} spans=
          {savedFlat.spansCount}
        </pre>
      )}
      {savedPlural && (
        <pre
          style={{
            background: "#f4f4f5",
            padding: 8,
            borderRadius: 4,
            marginTop: 12,
            whiteSpace: "pre-wrap",
          }}
        >
          plural saved → {savedPlural}
        </pre>
      )}
    </div>
  );
}

export const FlatPlaceholder: Story = {
  name: "Flat target with placeholder",
  render: () => <Wrapper block={makeMessagesBlock({ de: "Sie haben {count} Nachrichten" })} />,
};

export const FlatRichInlineCodes: Story = {
  name: "Flat target with paired inline codes",
  render: () => <Wrapper block={makeRichBlock({ de: "Klicken Sie hier — {count} ausstehend." })} />,
};

export const FlatPlainText: Story = {
  name: "Flat plain text (no spans)",
  render: () => <Wrapper block={makePlainBlock({ de: "Willkommen zurück!" })} />,
};

export const PluralTarget: Story = {
  name: "Plural target — opens in per-form view",
  render: () => (
    <Wrapper
      block={makeMessagesBlock({
        de: "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}",
      })}
    />
  ),
};

export const PluralWithInlineCodes: Story = {
  name: "Plural target with paired inline codes",
  render: () => (
    <Wrapper
      block={makeRichBlock({
        de: "{count, plural, one {Klicken Sie {=strong}hier{/=strong} — 1 ausstehend.} other {Klicken Sie {=strong}hier{/=strong} — {count} ausstehend.}}",
      })}
    />
  ),
};

export const EmptyFlatThenUpgrade: Story = {
  name: "Empty target — author flat, then upgrade",
  render: () => <Wrapper block={makeMessagesBlock({})} />,
};

export const NoPivotCandidates: Story = {
  name: "No pivot candidates — upgrade button hidden",
  render: () => <Wrapper block={makePlainBlock({})} />,
};
