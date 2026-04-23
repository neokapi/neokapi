import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { useState } from "react";

import { PluralTargetCell } from "../../components/PluralTargetCell";
import type { BlockInfo } from "../../types/api";

const messagesBlock: BlockInfo = {
  id: "blk-messages",
  source: "You have {count} messages",
  source_spans: [
    {
      span_type: "placeholder",
      type: "jsx:var",
      id: "0",
      data: "{count}",
      equiv_text: "count",
    },
  ],
  targets: {},
  translatable: true,
  has_spans: true,
  properties: {},
};

const meta: Meta<typeof PluralTargetCell> = {
  title: "Editor/Core/PluralTargetCell",
  component: PluralTargetCell,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Dialog-wrapped plural target editor. Adapts bowrain BlockInfo into the kapi-format Block shape the underlying `<PluralTargetEditor>` consumes, and round-trips targets through ICU syntax so the runtime's `resolveICU` picks the right form at render time. See issue #408.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof PluralTargetCell>;

function Wrapper({ block, initialTarget }: { block: BlockInfo; initialTarget: string }) {
  const [target, setTarget] = useState(initialTarget);
  const [open, setOpen] = useState(true);
  const block$ = { ...block, targets: { ...block.targets, de: target } };
  return (
    <div style={{ minHeight: 400, padding: 16 }}>
      <div style={{ marginBottom: 12, fontSize: 14 }}>
        <strong>Saved target ({`de`}):</strong>
        <pre style={{ background: "#f4f4f5", padding: 8, borderRadius: 4, marginTop: 4 }}>
          {target || "(empty)"}
        </pre>
      </div>
      <PluralTargetCell
        block={block$}
        locale="de"
        open={open}
        onSave={(next) => {
          setTarget(next);
          setOpen(false);
        }}
        onCancel={() => setOpen(false)}
      />
      {!open && (
        <button type="button" onClick={() => setOpen(true)}>
          Reopen editor
        </button>
      )}
    </div>
  );
}

export const FlatTarget: Story = {
  name: "Flat target (plain text)",
  render: () => <Wrapper block={messagesBlock} initialTarget="Sie haben {count} Nachrichten" />,
};

export const PluralTarget: Story = {
  name: "Plural target (ICU syntax)",
  render: () => (
    <Wrapper
      block={messagesBlock}
      initialTarget="{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}"
    />
  ),
};

export const EmptyTarget: Story = {
  name: "Empty target (first translation)",
  render: () => <Wrapper block={messagesBlock} initialTarget="" />,
};

export const NoPivotCandidates: Story = {
  name: "No pivot candidates (no spans)",
  render: () => (
    <Wrapper
      block={{ ...messagesBlock, source: "Welcome back!", source_spans: [] }}
      initialTarget="Willkommen zurück!"
    />
  ),
};

export const RichSourceSpans: Story = {
  name: "Rich source (paired code + variable)",
  render: () => (
    <Wrapper
      block={{
        ...messagesBlock,
        source: "Click <strong>here</strong> — {count} pending.",
        source_spans: [
          { span_type: "opening", type: "pc", id: "0", data: "<strong>", equiv_text: "strong" },
          { span_type: "closing", type: "pc", id: "0", data: "</strong>", equiv_text: "strong" },
          {
            span_type: "placeholder",
            type: "jsx:var",
            id: "1",
            data: "{count}",
            equiv_text: "count",
          },
        ],
      }}
      initialTarget="Klicken Sie hier — {count} ausstehend."
    />
  ),
};

export const ControlledOpenClose: Story = {
  name: "Controlled open/close",
  args: {
    block: messagesBlock,
    locale: "de",
    open: false,
    onSave: fn(),
    onCancel: fn(),
  },
};
