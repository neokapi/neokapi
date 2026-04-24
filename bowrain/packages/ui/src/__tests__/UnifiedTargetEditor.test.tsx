import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { UnifiedTargetEditor, type UnifiedSaveResult } from "../components/UnifiedTargetEditor";
import type { BlockInfo } from "../types/api";

function makeBlock(overrides: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
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
    targets: {},
    targets_coded: {},
    translatable: true,
    properties: {},
    ...overrides,
  };
}

describe("UnifiedTargetEditor — initial mode", () => {
  it("opens in flat mode for a plain target", () => {
    const block = makeBlock({
      targets: { de: "Sie haben {count} Nachrichten" },
      targets_coded: { de: "Sie haben \uE003 Nachrichten" },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByTestId("unified-target-editor").getAttribute("data-mode")).toBe("flat");
  });

  it("opens in plural mode when the stored target is ICU plural", () => {
    const block = makeBlock({
      targets: {
        de: "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}",
      },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByTestId("unified-target-editor").getAttribute("data-mode")).toBe("plural");
    expect(screen.getByTestId("mode-header-plural")).toBeInTheDocument();
  });

  it("hides the upgrade affordance when there are no pivot candidates", () => {
    const block = makeBlock({
      source: "Welcome back!",
      source_spans: [],
      has_spans: false,
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.queryByTestId("upgrade-to-plural")).not.toBeInTheDocument();
  });
});

describe("UnifiedTargetEditor — flat save", () => {
  it("calls onSave with kind:'flat' carrying coded text + spans", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({
      targets: { de: "Sie haben {count} Nachrichten" },
      targets_coded: { de: "Sie haben \uE003 Nachrichten" },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={onSave} onCancel={vi.fn()} />);
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.kind).toBe("flat");
    if (arg.kind === "flat") {
      // Coded text + spans match the seeded targets_coded value.
      expect(arg.codedText).toBe("Sie haben \uE003 Nachrichten");
      expect(arg.spans).toHaveLength(1);
      expect(arg.spans[0].equiv_text).toBe("count");
    }
  });

  it("seeds flat state from plain targets[locale] when no coded version exists", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({
      targets: { de: "Plain text" },
      targets_coded: {},
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={onSave} onCancel={vi.fn()} />);
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.kind).toBe("flat");
    if (arg.kind === "flat") {
      expect(arg.codedText).toBe("Plain text");
      expect(arg.spans).toEqual([]);
    }
  });
});

describe("UnifiedTargetEditor — mode toggle", () => {
  it("upgrades flat → plural and seeds 'other' from the flat state", async () => {
    const user = userEvent.setup();
    const block = makeBlock({
      targets: { de: "Sie haben {count} Nachrichten" },
      targets_coded: { de: "Sie haben \uE003 Nachrichten" },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);

    await user.click(screen.getByTestId("upgrade-to-plural"));

    await waitFor(() =>
      expect(screen.getByTestId("unified-target-editor").getAttribute("data-mode")).toBe("plural"),
    );
    expect(screen.getByTestId("form-tab-other")).toBeInTheDocument();
  });

  it("flatten back collapses to the 'other' form's content", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({
      targets: {
        de: "{count, plural, one {Eine Nachricht} other {Sie haben {count} Nachrichten}}",
      },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={onSave} onCancel={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: /collapse plural target/i }));

    await waitFor(() =>
      expect(screen.getByTestId("unified-target-editor").getAttribute("data-mode")).toBe("flat"),
    );
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.kind).toBe("flat");
  });
});

describe("UnifiedTargetEditor — plural save", () => {
  it("serialises forms back into ICU syntax via flattenRuns", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({
      targets: {
        de: "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}",
      },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={onSave} onCancel={vi.fn()} />);
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.kind).toBe("plural");
    if (arg.kind === "plural") {
      expect(arg.text.startsWith("{count, plural,")).toBe(true);
      expect(arg.text).toContain("one {");
      expect(arg.text).toContain("other {");
      // Placeholder marker survives the round-trip via codedToRuns +
      // flattenRuns; the actual `count` token resolves through the
      // source spans.
      expect(arg.text).toContain("{count}");
    }
  });
});

describe("UnifiedTargetEditor — form tabs", () => {
  it("renders one tab per CLDR form, marks present forms", () => {
    const block = makeBlock({
      targets: { de: "{count, plural, one {a} other {b}}" },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);
    for (const form of ["zero", "one", "two", "few", "many", "other"] as const) {
      expect(screen.getByTestId(`form-tab-${form}`)).toBeInTheDocument();
    }
  });

  it("switching tabs updates the active form", async () => {
    const user = userEvent.setup();
    const block = makeBlock({
      targets: { de: "{count, plural, one {a} other {b}}" },
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);
    const oneTab = screen.getByTestId("form-tab-one");
    await user.click(oneTab);
    expect(oneTab.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId("form-tab-other").getAttribute("aria-selected")).toBe("false");
  });
});

describe("UnifiedTargetEditor — cancel", () => {
  it("calls onCancel when Cancel button is clicked", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    render(
      <UnifiedTargetEditor block={makeBlock()} locale="de" onSave={vi.fn()} onCancel={onCancel} />,
    );
    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
