import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { PluralTargetCell, toKapiBlock } from "../components/PluralTargetCell";
import type { BlockInfo } from "../types/api";

function makeBlock(overrides: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
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
    ...overrides,
  };
}

describe("PluralTargetCell", () => {
  it("renders the flat textarea view for a plain-text target", () => {
    const block = makeBlock({ targets: { de: "Sie haben {count} Nachrichten" } });
    render(
      <PluralTargetCell
        block={block}
        locale="de"
        open={true}
        onSave={() => {}}
        onCancel={() => {}}
      />,
    );
    const editor = screen.getByTestId("plural-target-dialog");
    expect(editor.querySelector('[data-neokapi-plural-editor="flat"]')).not.toBeNull();
  });

  it("opens in the per-form view when the stored target is an ICU plural string", () => {
    const block = makeBlock({
      targets: {
        de: "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}",
      },
    });
    render(
      <PluralTargetCell
        block={block}
        locale="de"
        open={true}
        onSave={() => {}}
        onCancel={() => {}}
      />,
    );
    const editor = screen.getByTestId("plural-target-dialog");
    expect(editor.querySelector('[data-neokapi-plural-editor="plural"]')).not.toBeNull();
  });

  it("upgrades flat → plural and saves as ICU syntax", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({ targets: { de: "Sie haben {count} Nachrichten" } });

    render(
      <PluralTargetCell
        block={block}
        locale="de"
        open={true}
        onSave={onSave}
        onCancel={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: /upgrade to plural/i }));

    await waitFor(() => {
      const editor = screen.getByTestId("plural-target-dialog");
      expect(editor.querySelector('[data-neokapi-plural-editor="plural"]')).not.toBeNull();
    });

    await user.click(screen.getByTestId("plural-save"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const saved = onSave.mock.calls[0][0] as string;
    expect(saved).toContain(", plural,");
    expect(saved).toContain("{count}");
    expect(saved.startsWith("{count, plural,")).toBe(true);
  });

  it("flatten back downgrades the ICU plural to the `other` form", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({
      targets: {
        de: "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}",
      },
    });

    render(
      <PluralTargetCell
        block={block}
        locale="de"
        open={true}
        onSave={onSave}
        onCancel={() => {}}
      />,
    );

    // Button uses aria-label="Collapse plural target to flat text";
    // match on that (accessible name) rather than the visible label.
    await user.click(screen.getByRole("button", { name: /collapse plural target/i }));

    await waitFor(() => {
      const editor = screen.getByTestId("plural-target-dialog");
      expect(editor.querySelector('[data-neokapi-plural-editor="flat"]')).not.toBeNull();
    });

    await user.click(screen.getByTestId("plural-save"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const saved = onSave.mock.calls[0][0] as string;
    // Flattened target is the previous `other` form as plain text.
    expect(saved).toBe("Sie haben {count} Nachrichten");
  });

  it("calls onCancel when the dialog closes without a save", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    render(
      <PluralTargetCell
        block={makeBlock()}
        locale="de"
        open={true}
        onSave={() => {}}
        onCancel={onCancel}
      />,
    );
    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});

describe("toKapiBlock adapter", () => {
  it("synthesises a single-run source + pivot candidates from spans", () => {
    const block = makeBlock();
    const adapted = toKapiBlock(block);
    expect(adapted.source).toEqual([{ text: "You have {count} messages" }]);
    expect(adapted.placeholders.map((p) => p.name)).toEqual(["count"]);
    expect(adapted.placeholders[0].kind).toBe("variable");
  });

  it("dedupes paired-code spans (opening + closing share equiv)", () => {
    const block = makeBlock({
      source_spans: [
        { span_type: "opening", type: "pc", id: "0", data: "<b>", equiv_text: "strong" },
        { span_type: "closing", type: "pc", id: "0", data: "</b>", equiv_text: "strong" },
      ],
    });
    const adapted = toKapiBlock(block);
    expect(adapted.placeholders.map((p) => p.name)).toEqual(["strong"]);
  });

  it("ignores spans with no equiv_text", () => {
    const block = makeBlock({
      source_spans: [{ span_type: "placeholder", type: "jsx:var", id: "0", data: "{x}" }],
    });
    const adapted = toKapiBlock(block);
    expect(adapted.placeholders).toEqual([]);
  });
});
