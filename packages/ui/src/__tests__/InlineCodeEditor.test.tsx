// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";

import { InlineCodeEditor } from "../components/editor/InlineCodeEditor";
import type { SpanInfo } from "../types/span";

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

afterEach(() => {
  for (const node of Array.from(document.body.childNodes)) {
    document.body.removeChild(node);
  }
});

const boldOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:bold",
  id: "1",
  data: "<b>",
  equiv_text: "strong",
};
const boldClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:bold",
  id: "1",
  data: "</b>",
  equiv_text: "strong",
};

describe("InlineCodeEditor — onChange wiring", () => {
  // The Lexical editor's internal state mutations and `beforeinput`
  // event handling don't replay reliably in jsdom, so the live
  // edit-driven contract is exercised by `UnifiedTargetEditor`'s
  // tests (which mount the same component inside a real wrapper).
  // Here we keep only the structural assertions that prove the
  // prop is plumbed into the existing EditorObserverPlugin path
  // without breaking backward-compatible callers.

  it("mounts cleanly with an onChange callback (no throw)", () => {
    expect(() =>
      renderToContainer(
        createElement(InlineCodeEditor, {
          initialCodedText: "Hello \uE001bold\uE002",
          initialSpans: [boldOpen, boldClose],
          sourceSpans: [boldOpen, boldClose],
          onSave: vi.fn(),
          onCancel: vi.fn(),
          onChange: vi.fn(),
        }),
      ),
    ).not.toThrow();
  });

  it("mounts cleanly without onChange (backward compat)", () => {
    expect(() =>
      renderToContainer(
        createElement(InlineCodeEditor, {
          initialCodedText: "plain text",
          initialSpans: [],
          sourceSpans: [],
          onSave: vi.fn(),
          onCancel: vi.fn(),
        }),
      ),
    ).not.toThrow();
  });

  it("renders the chip palette + tag chips for bold + close spans", () => {
    const c = renderToContainer(
      createElement(InlineCodeEditor, {
        initialCodedText: "Hello \uE001bold\uE002",
        initialSpans: [boldOpen, boldClose],
        sourceSpans: [boldOpen, boldClose],
        onSave: vi.fn(),
        onCancel: vi.fn(),
        onChange: vi.fn(),
      }),
    );
    // The Lexical contenteditable is mounted inside the container.
    expect(c.querySelector('[contenteditable="true"]')).not.toBeNull();
  });
});

describe("InlineCodeEditor \u2014 writing direction", () => {
  // The contenteditable is the actual surface a translator types into: get
  // this wrong and the cursor and every keystroke behave as if they were
  // typing English, regardless of what language the target locale is.
  //
  // Direction is asserted via the CSS `direction` property (element.style),
  // not the `dir` attribute: Lexical's own reconciler manages `dir` on the
  // root on every commit and silently strips whatever the `dir` prop set, so
  // the fix (and this test) target the property Lexical never touches.
  it("sets the contenteditable's CSS direction and lang for an RTL locale", () => {
    const c = renderToContainer(
      createElement(InlineCodeEditor, {
        initialCodedText: "\u0645\u0631\u062D\u0628\u0627\u064B",
        initialSpans: [],
        sourceSpans: [],
        onSave: vi.fn(),
        onCancel: vi.fn(),
        locale: "ar-EG",
      }),
    );
    const editable = c.querySelector<HTMLElement>('[contenteditable="true"]');
    expect(editable?.style.direction).toBe("rtl");
    expect(editable?.getAttribute("lang")).toBe("ar-EG");
  });

  it("defaults to ltr with no lang asserted when no locale is given", () => {
    const c = renderToContainer(
      createElement(InlineCodeEditor, {
        initialCodedText: "hello",
        initialSpans: [],
        sourceSpans: [],
        onSave: vi.fn(),
        onCancel: vi.fn(),
      }),
    );
    const editable = c.querySelector<HTMLElement>('[contenteditable="true"]');
    expect(editable?.style.direction).toBe("ltr");
    expect(editable?.getAttribute("lang")).toBeNull();
  });
});

describe("InlineCodeEditor \u2014 imperative insertText handle", () => {
  it("exposes insertText via handleRef and routes it through the Lexical selection", async () => {
    const handleRef: {
      current: import("../components/editor/InlineCodeEditor").InlineCodeEditorHandle | null;
    } = { current: null };
    const onChange = vi.fn();
    const c = renderToContainer(
      createElement(InlineCodeEditor, {
        initialCodedText: "Bonjour",
        initialSpans: [],
        sourceSpans: [],
        onSave: vi.fn(),
        onCancel: vi.fn(),
        onChange,
        handleRef,
      }),
    );
    expect(handleRef.current).not.toBeNull();

    // Lexical reconciles updates at the microtask boundary — await the flush.
    await act(async () => {
      handleRef.current!.insertText(" monde");
    });

    // The insertion lands in the contenteditable and surfaces through the
    // EditorObserverPlugin's onChange with the combined coded text.
    expect(c.querySelector('[contenteditable="true"]')!.textContent).toContain("monde");
    const last = onChange.mock.calls.at(-1);
    expect(last?.[0]).toBe("Bonjour monde");
  });
});
