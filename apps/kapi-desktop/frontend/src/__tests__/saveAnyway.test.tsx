import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, renderHook, screen, waitFor } from "./testUtils";

import { EditionEditPanel } from "../components/edit/EditionEditPanel";
import { GatePrompt } from "../components/edit/GatePrompt";
import { useChangeSender } from "../components/edit/useChangeSender";
import { type EditionContent, editionContent, gateFindings } from "../lib/changes";
import { MemoryChanges } from "../stories/memoryChanges";

// The rich editor is a Lexical surface jsdom cannot type into; a field that
// reports its text as the editor's edits stands in for it.
vi.mock("../components/edit/EditionEditor", () => ({
  EditionEditor: ({
    content,
    onChange,
  }: {
    content: { text: string };
    onChange: (edits: { text: string }[]) => void;
  }) => (
    <input
      aria-label="wording"
      defaultValue={content.text}
      onChange={(e) => onChange(e.target.value === content.text ? [] : [{ text: e.target.value }])}
    />
  ),
}));

const GUIDE = { doc: "docs/guide.md", block: "intro", text: "We use the widget every day." };

function gatedClient() {
  return new MemoryChanges([GUIDE], {}, [{ term: "utilize", replacement: "use" }]);
}

async function utilizeOps(client: MemoryChanges) {
  const page = await client.read({ doc: GUIDE.doc, blocks: [GUIDE.block] });
  const b = page.blocks[0];
  return [
    {
      op: "set_content" as const,
      at: b.ref,
      if_match: b.rev,
      text: "We utilize the widget every day.",
    },
  ];
}

describe("save anyway", () => {
  // A save a rule refuses is held with what the check found; saving it anyway
  // sends the same change with gate report, and it lands.
  it("holds a gate_failed refusal and lands it with gate report", async () => {
    const client = gatedClient();
    const onApplied = vi.fn();
    const { result } = renderHook(() => useChangeSender(client, { onApplied }));
    const ops = await utilizeOps(client);

    await act(async () => {
      await result.current.send(ops, "Say utilize");
    });
    expect(result.current.gated).not.toBeNull();
    expect(result.current.gated?.findings.map((f) => f.rule)).toEqual(["terms.vocabulary"]);
    expect(result.current.stale).toBeNull();
    expect(onApplied).not.toHaveBeenCalled();
    expect(client.sets.at(-1)?.gate).toBeUndefined();

    await act(async () => {
      await result.current.override();
    });
    expect(result.current.gated).toBeNull();
    expect(result.current.error).toBeNull();
    expect(onApplied).toHaveBeenCalledTimes(1);
    const sent = client.sets.at(-1);
    expect(sent?.gate).toBe("report");
    expect(sent?.note).toBe("Say utilize");
    expect(sent?.ops).toEqual(ops);
    const page = await client.read({ doc: GUIDE.doc, blocks: [GUIDE.block] });
    expect(page.blocks[0].text).toBe("We utilize the widget every day.");
  });

  // Save anyway sends the change whose findings the person was shown. Wording
  // changed after the refusal puts the prompt away, and its save is checked
  // again before it can be saved anyway.
  it("saves anyway only the wording the person was shown the findings of", async () => {
    const client = gatedClient();
    const page = await client.read({ doc: GUIDE.doc, blocks: [GUIDE.block] });
    const content = editionContent(page.blocks[0]) as EditionContent;
    function Panel() {
      const sender = useChangeSender(client);
      return <EditionEditPanel sender={sender} content={content} note="Reword" />;
    }
    render(<Panel />);
    const field = screen.getByLabelText("wording");

    fireEvent.change(field, { target: { value: "We utilize the widget every day." } });
    fireEvent.click(screen.getByText("Save"));
    await screen.findByText("Save anyway");
    expect(client.sets).toHaveLength(1);

    fireEvent.change(field, { target: { value: "We utilize the gadget every day." } });
    expect(screen.queryByText("Save anyway")).toBeNull();
    expect(client.sets).toHaveLength(1);

    fireEvent.click(screen.getByText("Save"));
    await screen.findByText("Save anyway");
    expect(client.sets).toHaveLength(2);
    expect(client.sets[1].gate).toBeUndefined();

    fireEvent.click(screen.getByText("Save anyway"));
    await waitFor(() => expect(client.sets).toHaveLength(3));
    expect(client.sets[2].gate).toBe("report");
    expect(client.sets[2].ops).toEqual(client.sets[1].ops);
    const after = await client.read({ doc: GUIDE.doc, blocks: [GUIDE.block] });
    expect(after.blocks[0].text).toBe("We utilize the gadget every day.");
  });

  // Any other refusal offers no override.
  it("offers no override for a refusal a rule did not make", async () => {
    const client = gatedClient();
    const { result } = renderHook(() => useChangeSender(client));
    const ops = await utilizeOps(client);
    client.touch(GUIDE.doc, GUIDE.block, "Moved on.");
    await act(async () => {
      await result.current.send([{ ...ops[0], text: "We use it." }]);
    });
    expect(result.current.gated).toBeNull();
    expect(result.current.stale).not.toBeNull();
    await act(async () => {
      expect(await result.current.override()).toBeNull();
    });
    expect(client.sets.every((s) => s.gate === undefined)).toBe(true);
  });

  it("reads the failing findings of a gate_failed refusal only", () => {
    const finding = { rule: "terms.vocabulary", message: "use use", fails: true };
    const advisory = { rule: "terms.vocabulary", message: "say sign in", fails: false };
    expect(
      gateFindings({
        schema: "kapi.change-result/v1",
        status: "refused",
        record: null,
        docs: [
          {
            doc: "docs/guide.md",
            written: false,
            after: null,
            findings: [finding, advisory, finding],
          },
        ],
        ops: [
          {
            i: 0,
            op: "set_content",
            status: "refused",
            error: { code: "gate_failed", message: "fails" },
          },
        ],
      }),
    ).toEqual([finding]);
    expect(
      gateFindings({
        schema: "kapi.change-result/v1",
        status: "refused",
        record: null,
        docs: [],
        ops: [
          {
            i: 0,
            op: "set_content",
            status: "refused",
            error: { code: "stale", message: "moved" },
          },
        ],
      }),
    ).toBeNull();
  });

  it("draws the findings and both choices", () => {
    const onOverride = vi.fn();
    const onDismiss = vi.fn();
    render(
      <GatePrompt
        findings={[
          { rule: "terms.vocabulary", message: "Use use instead of utilize", fails: true },
        ]}
        onOverride={onOverride}
        onDismiss={onDismiss}
      />,
    );
    expect(screen.getByText("Use use instead of utilize")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Save anyway"));
    expect(onOverride).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText("Keep editing"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
