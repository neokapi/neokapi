import { createRef } from "react";
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Run } from "@neokapi/kapi-format";

import {
  UnifiedTargetEditor,
  type UnifiedSaveResult,
  type UnifiedTargetEditorHandle,
} from "../components/UnifiedTargetEditor";
import type { BlockInfo } from "../types/api";

const count: Run = { ph: { id: "0", type: "jsx:var", data: "{count}", equiv: "count" } };
const runs = (list: Run[]) => list as NonNullable<BlockInfo["source_runs"]>;

function makeBlock(overrides: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    source: "You have {count} messages",
    source_runs: runs([{ text: "You have " }, count, { text: " messages" }]),
    has_inline_codes: true,
    targets: {},
    translatable: true,
    properties: {},
    ...overrides,
  };
}

/** A block whose translation carries the placeholder as a run. */
const translated = () =>
  makeBlock({
    targets: { de: "Sie haben {count} Nachrichten" },
    targets_runs: { de: runs([{ text: "Sie haben " }, count, { text: " Nachrichten" }]) },
  });

describe("UnifiedTargetEditor — initial mode", () => {
  it("opens in flat mode for a plain target", () => {
    render(
      <UnifiedTargetEditor block={translated()} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />,
    );
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
      source_runs: undefined,
      has_inline_codes: false,
    });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.queryByTestId("upgrade-to-plural")).not.toBeInTheDocument();
  });
});

describe("UnifiedTargetEditor — flat save", () => {
  it("calls onSave with the translation's runs, its codes kept", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(
      <UnifiedTargetEditor block={translated()} locale="de" onSave={onSave} onCancel={vi.fn()} />,
    );
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.runs).toEqual([{ text: "Sie haben " }, count, { text: " Nachrichten" }]);
  });

  it("seeds flat state from plain targets[locale] when the block serves no runs", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const block = makeBlock({ targets: { de: "Plain text" } });
    render(<UnifiedTargetEditor block={block} locale="de" onSave={onSave} onCancel={vi.fn()} />);
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.runs).toEqual([{ text: "Plain text" }]);
  });
});

describe("UnifiedTargetEditor — mode toggle", () => {
  it("upgrades flat → plural and seeds 'other' from the flat state", async () => {
    const user = userEvent.setup();
    render(
      <UnifiedTargetEditor block={translated()} locale="de" onSave={vi.fn()} onCancel={vi.fn()} />,
    );

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
    expect(arg.runs).toEqual([{ text: "Sie haben {count} Nachrichten" }]);
  });
});

describe("UnifiedTargetEditor — plural save", () => {
  it("saves a plural edited from ICU text as one plural run with its forms", async () => {
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
    // What a save sends is the plural run itself; a form that was ICU text
    // stays text, so nothing the person did not type is invented.
    expect(arg.runs).toEqual([
      {
        plural: {
          pivot: "count",
          forms: {
            one: [{ text: "Sie haben 1 Nachricht" }],
            other: [{ text: "Sie haben {count} Nachrichten" }],
          },
        },
      },
    ]);
  });

  // A message that is one plural (ARB, ICU) arrives as runs. Its translation
  // opens on its forms, and an untranslated one opens in plural mode on the
  // source's pivot with the source's variable offered as a chip.
  const n: Run = { ph: { id: "n", type: "code:variable", data: "#", equiv: "#" } };
  const pluralMessage = (translated: boolean): BlockInfo =>
    makeBlock({
      source: " messages",
      source_runs: runs([
        {
          plural: {
            pivot: "count",
            forms: { one: [n, { text: " message" }], other: [n, { text: " messages" }] },
          },
        },
      ]),
      targets: translated ? { de: { text: " Nachrichten", status: "translated" } } : {},
      targets_runs: translated
        ? {
            de: runs([
              {
                plural: {
                  pivot: "count",
                  forms: { one: [n, { text: " Nachricht" }], other: [n, { text: " Nachrichten" }] },
                },
              },
            ]),
          }
        : {},
    });

  it("opens a plural translation on its forms and saves it whole, each form's codes kept", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(
      <UnifiedTargetEditor
        block={pluralMessage(true)}
        locale="de"
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByTestId("unified-target-editor").getAttribute("data-mode")).toBe("plural");
    expect(screen.getByTestId("mode-header-plural").textContent).toContain("count");
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.runs).toEqual([
      {
        plural: {
          pivot: "count",
          forms: {
            one: [n, { text: " Nachricht" }],
            other: [n, { text: " Nachrichten" }],
          },
        },
      },
    ]);
  });

  it("opens an untranslated plural message in plural mode on the source's pivot", () => {
    render(
      <UnifiedTargetEditor
        block={pluralMessage(false)}
        locale="de"
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByTestId("unified-target-editor").getAttribute("data-mode")).toBe("plural");
    expect(screen.getByTestId("mode-header-plural").textContent).toContain("count");
    expect(screen.getByTestId("form-tab-other").getAttribute("aria-selected")).toBe("true");
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

describe("UnifiedTargetEditor — imperative insertText handle", () => {
  const plain = () =>
    makeBlock({
      source: "Hello",
      source_runs: undefined,
      has_inline_codes: false,
      targets: { de: "Bonjour" },
    });

  it("exposes the handle via handleRef while mounted", () => {
    const ref = createRef<UnifiedTargetEditorHandle>();
    const { unmount } = render(
      <UnifiedTargetEditor
        block={plain()}
        locale="de"
        onSave={vi.fn()}
        onCancel={vi.fn()}
        handleRef={ref}
      />,
    );
    expect(ref.current).not.toBeNull();
    unmount();
    // React nulls the ref on unmount — callers use this as the
    // "is an editor open" probe for the insert-at-cursor path.
    expect(ref.current).toBeNull();
  });

  it("inserts text at the Lexical selection without persisting; the text lands on save", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const ref = createRef<UnifiedTargetEditorHandle>();
    render(
      <UnifiedTargetEditor
        block={plain()}
        locale="de"
        onSave={onSave}
        onCancel={vi.fn()}
        handleRef={ref}
      />,
    );

    act(() => ref.current!.insertText(" monde"));

    // Insertion joins the in-progress edit — nothing is saved yet.
    expect(onSave).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.getByTestId("unified-target-editor").textContent).toContain("monde"),
    );

    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const arg = onSave.mock.calls[0][0] as UnifiedSaveResult;
    expect(arg.runs).toEqual([{ text: "Bonjour monde" }]);
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

describe("UnifiedTargetEditor — writing direction", () => {
  // The contenteditable is what a translator actually types into: pass the
  // target locale through, or the cursor and every keystroke behave as if
  // they were typing English regardless of the target language.
  it("sets the contenteditable's CSS direction for an RTL target locale", () => {
    render(
      <UnifiedTargetEditor
        block={makeBlock()}
        locale="ar-EG"
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const editable = document.querySelector<HTMLElement>('[contenteditable="true"]');
    expect(editable?.style.direction).toBe("rtl");
    expect(editable?.getAttribute("lang")).toBe("ar-EG");
  });
});
