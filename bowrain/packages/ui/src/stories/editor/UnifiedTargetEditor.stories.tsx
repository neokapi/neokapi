import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { flattenRuns, isPlural } from "@neokapi/kapi-format";

import { UnifiedTargetEditor } from "../../components/UnifiedTargetEditor";
import type { BlockInfo } from "../../types/api";
import { getTargetText } from "../../components/editor/blockStatus";

function makeMessagesBlock(targets: Record<string, string>): BlockInfo {
  return {
    id: "blk-messages",
    source: "You have {count} messages",
    has_inline_codes: true,
    source_runs: [
      { text: "You have " },
      { ph: { id: "0", type: "jsx:var", data: "{count}", equiv: "count" } },
      { text: " messages" },
    ],
    targets,
    translatable: true,
    properties: {},
  };
}

function makeRichBlock(targets: Record<string, string>): BlockInfo {
  return {
    id: "blk-rich",
    source: "Click <strong>here</strong> for {count} pending.",
    has_inline_codes: true,
    source_runs: [
      { text: "Click " },
      { pcOpen: { id: "0", type: "fmt:bold", data: "<strong>", equiv: "strong" } },
      { text: "here" },
      { pcClose: { id: "0", type: "fmt:bold", data: "</strong>", equiv: "strong" } },
      { text: " for " },
      { ph: { id: "1", type: "jsx:var", data: "{count}", equiv: "count" } },
      { text: " pending." },
    ],
    targets,
    translatable: true,
    properties: {},
  };
}

function makePlainBlock(targets: Record<string, string>): BlockInfo {
  return {
    id: "blk-plain",
    source: "Welcome back!",
    targets,
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
  const [savedFlat, setSavedFlat] = useState<{ text: string; runs: number } | null>(null);
  const [savedPlural, setSavedPlural] = useState<string | null>(null);
  const [open, setOpen] = useState(true);

  return (
    <div style={{ minHeight: 500, padding: 16, fontFamily: "sans-serif" }}>
      <div style={{ marginBottom: 12, fontSize: 14 }}>
        <strong>Source:</strong> <code>{block.source}</code>
      </div>
      <div style={{ marginBottom: 12, fontSize: 14 }}>
        <strong>Target ({locale}):</strong> <code>{getTargetText(block, locale) || "(empty)"}</code>
      </div>
      {open ? (
        <UnifiedTargetEditor
          block={block}
          locale={locale}
          onSave={(result) => {
            // A plural saves as one plural run; anything else is the flat
            // translation's runs. Both are shown through flattenRuns, the
            // one string form that keeps every code and form.
            if (isPlural(result.runs)) {
              setSavedPlural(flattenRuns(result.runs));
            } else {
              setSavedFlat({ text: flattenRuns(result.runs), runs: result.runs.length });
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
          flat saved → {JSON.stringify(savedFlat.text)} ({savedFlat.runs} runs)
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

/** A message that is one plural (ARB, ICU), as the server serves it: runs, the variable in each form. */
function makePluralMessageBlock(translated: boolean): BlockInfo {
  const n = { ph: { id: "n", type: "code:variable", data: "#", equiv: "#" } };
  return {
    id: "blk-plural-message",
    source: " messages",
    source_runs: [
      {
        plural: {
          pivot: "count",
          forms: { one: [n, { text: " message" }], other: [n, { text: " messages" }] },
        },
      },
    ],
    has_inline_codes: true,
    targets: translated ? { de: { text: " Nachrichten", status: "translated" } } : {},
    targets_runs: translated
      ? {
          de: [
            {
              plural: {
                pivot: "count",
                forms: { one: [n, { text: " Nachricht" }], other: [n, { text: " Nachrichten" }] },
              },
            },
          ],
        }
      : {},
    translatable: true,
    properties: {},
  };
}

export const PluralTranslationReadBack: Story = {
  name: "Plural translation read back as runs — opens on its forms",
  render: () => <Wrapper block={makePluralMessageBlock(true)} />,
};

export const UntranslatedPluralMessage: Story = {
  name: "Untranslated plural message — opens in plural mode on the source pivot",
  render: () => <Wrapper block={makePluralMessageBlock(false)} />,
};

export const EmptyFlatThenUpgrade: Story = {
  name: "Empty target — author flat, then upgrade",
  render: () => <Wrapper block={makeMessagesBlock({})} />,
};

export const NoPivotCandidates: Story = {
  name: "No pivot candidates — upgrade button hidden",
  render: () => <Wrapper block={makePlainBlock({})} />,
};
