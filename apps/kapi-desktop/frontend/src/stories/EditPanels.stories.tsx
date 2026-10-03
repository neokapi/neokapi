import type { Meta, StoryObj } from "@storybook/react-vite";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { EditionHistory } from "@neokapi/contract-types";

import { ChangesCard } from "../components/edit/ChangesCard";
import { EditionEditPanel } from "../components/edit/EditionEditPanel";
import { StalePrompt } from "../components/edit/StalePrompt";
import { useChangeSender } from "../components/edit/useChangeSender";
import { type EditionContent, editionContent } from "../lib/changes";
import { MemoryChanges, type MemoryBlock } from "./memoryChanges";

/**
 * The pieces every Kapi Desktop edit is made of: an edition in the editor, the
 * save that sends it to the change service with the revision it read, the
 * prompt a moved revision brings up, and the recorded changes of the edition.
 */
const meta: Meta = {
  title: "Edit/Edition panels",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const FORMATTED: MemoryBlock = {
  doc: "docs/guide.html",
  block: "p",
  text: 'Read the <x id="1"/>shop guide<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.',
  codes: {
    "1": { kind: "paired", type: "link:hyperlink", attrs: { href: "https://old.example/guide" } },
    "2": { kind: "paired", type: "fmt:bold" },
  },
};

const PLURAL: MemoryBlock = {
  doc: "src/cart.kbf.json",
  block: "cart-count",
  text: '<x id="1/"/> items in your cart',
  codes: { "1/": { kind: "placeholder", type: "jsx:var", equiv: "count", disp: "count" } },
  structures: [
    {
      path: [0],
      kind: "plural",
      pivot: "count",
      branches: {
        zero: "Your cart is empty",
        one: "1 item in your cart",
        other: '<x id="1/"/> items in your cart',
      },
    },
  ],
};

/**
 * One block in the editor, saved through an in-memory change service. With
 * `moved`, another writer saves the block right after the panel first reads it.
 */
function Panel({ block, moved }: { block: MemoryBlock; moved?: string }) {
  const client = useMemo(() => new MemoryChanges([block]), [block]);
  const [content, setContent] = useState<EditionContent | null>(null);
  const [reads, setReads] = useState(0);
  const load = useCallback(async () => {
    const page = await client.read({ doc: block.doc, blocks: [block.block] });
    setContent(page.blocks[0] ? editionContent(page.blocks[0]) : null);
    setReads((n) => n + 1);
  }, [client, block]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (moved && reads === 1) client.touch(block.doc, block.block, moved);
  }, [moved, reads, client, block]);
  const sender = useChangeSender(client, { onApplied: load, onReload: load });
  return (
    <div className="max-w-xl">
      <EditionEditPanel sender={sender} content={content} locale="en" data-slot="story-edit" />
    </div>
  );
}

/** Formatted text: each inline code is a chip, and the text is typed around them. */
export const FormattedEdit: Story = {
  name: "Formatted text",
  render: () => <Panel block={FORMATTED} />,
};

/** A plural, edited a form at a time; each changed form is saved by its path. */
export const PluralEdit: Story = {
  name: "Plural forms",
  render: () => <Panel block={PLURAL} />,
};

/** Type an edit and save: the block moved after it was read, so the prompt asks first. */
export const StaleOnSave: Story = {
  name: "Changed since it was opened",
  render: () => (
    <Panel
      block={FORMATTED}
      moved='Read the <x id="1"/>handbook<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.'
    />
  ),
};

/** The prompt itself, as a stale refusal draws it. */
export const StalePromptAlone: Story = {
  name: "Stale prompt",
  render: () => (
    <div className="max-w-xl">
      <StalePrompt
        current={{
          rev: "r:0d71f30c75e4a087",
          text: 'Read the <x id="1"/>store guide<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.',
        }}
        codes={FORMATTED.codes}
        locale="en"
        onReapply={() => {}}
        onDiscard={() => {}}
      />
    </div>
  ),
};

const HISTORY: EditionHistory = {
  ref: { doc: "locales/en.json", block: "greeting", edition: "nb" },
  rev: "r:3333333333333333",
  entries: [
    {
      record: "op-9",
      before: "r:2222222222222222",
      after: "r:3333333333333333",
      actor: { kind: "person", name: "Ingrid" },
      origin: "desktop",
      at: "2026-10-02T14:12:00Z",
    },
    {
      record: "op-7",
      before: "r:1111111111111111",
      after: "r:2222222222222222",
      actor: { kind: "agent", name: "claude", session: "s_01J9Q4" },
      origin: "mcp",
      at: "2026-10-01T16:40:00Z",
    },
    {
      record: "op-3",
      before: "absent",
      after: "r:1111111111111111",
      basis: "r:0000000000000000",
      actor: { kind: "tool", name: "translate" },
      origin: "flow:up",
      at: "2026-09-30T09:30:00Z",
    },
    {
      record: "op-2",
      before: "r:1111111111111111",
      after: "r:1111111111111112",
      actor: null,
      origin: "observed",
      at: "2026-09-29T08:00:00Z",
    },
  ],
};

/** An edition's recorded changes, most recent first, the one in force marked. */
export const HistoryList: Story = {
  name: "Recorded changes",
  render: () => (
    <div className="max-w-xl">
      <ChangesCard history={HISTORY} defaultOpen />
    </div>
  ),
};

/** An edition nothing has changed through kapi. */
export const HistoryEmpty: Story = {
  name: "Recorded changes (none)",
  render: () => (
    <div className="max-w-xl">
      <ChangesCard history={{ ...HISTORY, entries: [] }} defaultOpen />
    </div>
  ),
};
