// @vitest-environment jsdom
import { act, createElement, useState } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SchemaForm } from "../components/schema-form";
import type { ComponentSchema } from "../components/schema-form/types";
import type { SchemaFormHost, SchemaFormBrowseRequest } from "../components/schema-form/host";

// Non-optional shape of host.onBrowse, for typed vi.fn() mocks.
type BrowseFn = (request: SchemaFormBrowseRequest) => Promise<string | null>;

// ── Test harness ─────────────────────────────────────────────────────────
//
// The repo's UI tests drive React via react-dom/client + act (no RTL), so we
// follow that convention. A small controlled wrapper threads the form's
// values through useState and forwards each commit to a spy so assertions can
// inspect the assembled flat values object.

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

function Harness({
  schema,
  initial = {},
  host,
  onChange,
}: {
  schema: ComponentSchema;
  initial?: Record<string, unknown>;
  host?: SchemaFormHost;
  onChange: (v: Record<string, unknown>) => void;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initial);
  return createElement(SchemaForm, {
    schema,
    values,
    host,
    onChange: (v: Record<string, unknown>) => {
      setValues(v);
      onChange(v);
    },
  });
}

function mount(args: {
  schema: ComponentSchema;
  initial?: Record<string, unknown>;
  host?: SchemaFormHost;
}): { container: HTMLDivElement; onChange: ReturnType<typeof vi.fn> } {
  const onChange = vi.fn<(v: Record<string, unknown>) => void>();
  const container = renderToContainer(createElement(Harness, { ...args, onChange }));
  return { container, onChange };
}

// React installs a value-setter on input/textarea prototypes so writing
// through it fires change events for controlled components.
function setNativeValue(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  const proto =
    el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  act(() => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function typeInput(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  setNativeValue(el, value);
}

function selectOption(el: HTMLSelectElement, value: string): void {
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set;
  act(() => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function click(el: Element): void {
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function lastValue(onChange: ReturnType<typeof vi.fn>): Record<string, unknown> {
  return onChange.mock.calls[onChange.mock.calls.length - 1][0] as Record<string, unknown>;
}

function buttonByText(c: HTMLElement, text: string): HTMLButtonElement | undefined {
  return Array.from(c.querySelectorAll("button")).find((b) => b.textContent?.includes(text));
}

// ── number-list ────────────────────────────────────────────────────────────

describe("number-list widget", () => {
  const schema: ComponentSchema = {
    title: "Number List",
    type: "object",
    properties: {
      pages: { type: "string", title: "Pages", "ui:widget": "number-list" },
    },
  };

  it("commits the raw comma-separated string", () => {
    const { container, onChange } = mount({ schema });
    const input = container.querySelector("input") as HTMLInputElement;
    typeInput(input, "1, 2, 3");
    expect(lastValue(onChange).pages).toBe("1, 2, 3");
  });

  it("flags non-numeric tokens with an inline warning", () => {
    const { container } = mount({ schema, initial: { pages: "1, x, 3" } });
    const input = container.querySelector("input") as HTMLInputElement;
    typeInput(input, "1, x, 3");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(container.textContent).toContain('"x"');
  });

  it("clears the value to undefined when emptied", () => {
    const { container, onChange } = mount({ schema, initial: { pages: "1, 2" } });
    const input = container.querySelector("input") as HTMLInputElement;
    typeInput(input, "");
    expect(lastValue(onChange).pages).toBeUndefined();
  });
});

// ── file-picker / path ───────────────────────────────────────────────────

describe("file-picker / path widget", () => {
  const schema: ComponentSchema = {
    title: "File",
    type: "object",
    properties: {
      input: {
        type: "string",
        title: "Input File",
        "ui:widget": "file-picker",
        "ui:widget-options": {
          browseTitle: "Pick input",
          filters: [{ name: "Text (*.txt)", extensions: "*.txt" }],
        },
      },
    },
  };

  it("renders a plain text input with no Browse button when no host handler", () => {
    const { container } = mount({ schema });
    expect(container.querySelector("input")).not.toBeNull();
    expect(buttonByText(container, "Browse")).toBeUndefined();
  });

  it("renders filter labels from ui:widget-options", () => {
    const { container } = mount({ schema });
    expect(container.textContent).toContain("Text (*.txt)");
  });

  it("commits typed paths", () => {
    const { container, onChange } = mount({ schema });
    const input = container.querySelector("input") as HTMLInputElement;
    typeInput(input, "/tmp/in.txt");
    expect(lastValue(onChange).input).toBe("/tmp/in.txt");
  });

  it("shows a Browse button and invokes the host onBrowse with file kind + metadata", async () => {
    const onBrowse = vi.fn<BrowseFn>().mockResolvedValue("/picked/file.txt");
    const { container, onChange } = mount({ schema, host: { onBrowse } });
    const browse = buttonByText(container, "Browse");
    expect(browse).toBeDefined();
    await act(async () => {
      browse!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(onBrowse).toHaveBeenCalledTimes(1);
    const req = onBrowse.mock.calls[0][0];
    expect(req.kind).toBe("file");
    expect(req.field).toBe("input");
    expect(req.title).toBe("Pick input");
    expect(req.filters).toEqual([{ name: "Text (*.txt)", extensions: "*.txt" }]);
    expect(lastValue(onChange).input).toBe("/picked/file.txt");
  });
});

// ── folder / folder-picker ─────────────────────────────────────────────────

describe("folder / folder-picker widget", () => {
  const schema: ComponentSchema = {
    title: "Folder",
    type: "object",
    properties: {
      dir: { type: "string", title: "Output Dir", "ui:widget": "folder-picker" },
    },
  };

  it("degrades to a text input when no host handler", () => {
    const { container } = mount({ schema });
    expect(container.querySelector("input")).not.toBeNull();
    expect(buttonByText(container, "Browse")).toBeUndefined();
  });

  it("invokes the host onBrowse with directory kind", async () => {
    const onBrowse = vi.fn<BrowseFn>().mockResolvedValue("/picked/dir");
    const { container, onChange } = mount({ schema, host: { onBrowse } });
    const browse = buttonByText(container, "Browse")!;
    await act(async () => {
      browse.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(onBrowse.mock.calls[0][0].kind).toBe("directory");
    expect(lastValue(onChange).dir).toBe("/picked/dir");
  });

  it("keeps the value unchanged when the host dialog is cancelled", async () => {
    const onBrowse = vi.fn<BrowseFn>().mockResolvedValue(null);
    const { container, onChange } = mount({
      schema,
      initial: { dir: "/orig" },
      host: { onBrowse },
    });
    const browse = buttonByText(container, "Browse")!;
    await act(async () => {
      browse.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(onChange).not.toHaveBeenCalled();
  });
});

// ── credential-picker ──────────────────────────────────────────────────────

describe("credential-picker widget", () => {
  const inlineSchema: ComponentSchema = {
    title: "AI",
    type: "object",
    properties: {
      credential: {
        type: "string",
        title: "Credential",
        "ui:widget": "credential-picker",
        options: [
          { value: "", label: "Custom (manual entry)" },
          { value: "my-key", label: "My Anthropic" },
        ],
      },
    },
  };

  it("renders a select from inline schema options and commits the selected value", () => {
    const { container, onChange } = mount({ schema: inlineSchema });
    const select = container.querySelector("select") as HTMLSelectElement;
    expect(select).not.toBeNull();
    expect(Array.from(select.options).map((o) => o.value)).toEqual(["", "my-key"]);
    selectOption(select, "my-key");
    expect(lastValue(onChange).credential).toBe("my-key");
  });

  it("populates from a host-injected credential list when no inline options", () => {
    const hostSchema: ComponentSchema = {
      title: "AI",
      type: "object",
      properties: {
        credential: { type: "string", title: "Credential", "ui:widget": "credential-picker" },
      },
    };
    const host: SchemaFormHost = {
      credentials: () => [
        { value: "work", label: "Work OpenAI" },
        { value: "home", label: "Home Ollama" },
      ],
    };
    const { container, onChange } = mount({ schema: hostSchema, host });
    const select = container.querySelector("select") as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.textContent)).toEqual([
      "Work OpenAI",
      "Home Ollama",
    ]);
    selectOption(select, "home");
    expect(lastValue(onChange).credential).toBe("home");
  });

  it("degrades to a text input when neither options nor host list are available", () => {
    const bareSchema: ComponentSchema = {
      title: "AI",
      type: "object",
      properties: {
        credential: { type: "string", title: "Credential", "ui:widget": "credential-picker" },
      },
    };
    const { container, onChange } = mount({ schema: bareSchema });
    expect(container.querySelector("select")).toBeNull();
    const input = container.querySelector("input") as HTMLInputElement;
    expect(input).not.toBeNull();
    typeInput(input, "manual-cred");
    expect(lastValue(onChange).credential).toBe("manual-cred");
  });
});

// ── tags ─────────────────────────────────────────────────────────────────

describe("tags widget", () => {
  const schema: ComponentSchema = {
    title: "Tags",
    type: "object",
    properties: { tags: { type: "string", title: "Tags", "ui:widget": "tags" } },
  };

  it("renders existing tags from a comma-separated string", () => {
    const { container } = mount({ schema, initial: { tags: "a, b" } });
    expect(container.textContent).toContain("a");
    expect(container.textContent).toContain("b");
  });

  it("adds a tag and commits a joined comma-separated string", () => {
    const { container, onChange } = mount({ schema });
    const input = container.querySelector("input") as HTMLInputElement;
    typeInput(input, "html");
    act(() => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(lastValue(onChange).tags).toBe("html");
  });
});

// ── segmented ──────────────────────────────────────────────────────────────

describe("segmented widget", () => {
  const schema: ComponentSchema = {
    title: "Segmented",
    type: "object",
    properties: {
      output: {
        type: "string",
        title: "Output",
        "ui:widget": "segmented",
        enum: ["source", "target", "both"],
      },
    },
  };

  it("renders one button per enum option", () => {
    const { container } = mount({ schema });
    const buttons = Array.from(container.querySelectorAll("button"));
    expect(buttons).toHaveLength(3);
  });

  it("commits the clicked option", () => {
    const { container, onChange } = mount({ schema });
    const target = buttonByText(container, "target")!;
    click(target);
    expect(lastValue(onChange).output).toBe("target");
  });
});

// ── select ───────────────────────────────────────────────────────────────

describe("select widget", () => {
  const schema: ComponentSchema = {
    title: "Select",
    type: "object",
    properties: {
      mode: {
        type: "string",
        title: "Mode",
        "ui:widget": "select",
        options: [
          { value: "fast", label: "Fast" },
          { value: "slow", label: "Slow" },
        ],
      },
    },
  };

  it("renders a clickable option list and commits the choice", () => {
    const { container, onChange } = mount({ schema });
    const slow = buttonByText(container, "Slow")!;
    click(slow);
    expect(lastValue(onChange).mode).toBe("slow");
  });
});

// ── checklist ──────────────────────────────────────────────────────────────

describe("checklist widget", () => {
  const schema: ComponentSchema = {
    title: "Checklist",
    type: "object",
    properties: {
      checks: {
        type: "object",
        title: "Checks",
        "ui:widget": "checklist",
        "ui:widget-options": {
          entries: [
            { name: "trim", title: "Trim whitespace" },
            { name: "dedupe", title: "De-duplicate" },
          ],
        },
      },
    },
  };

  it("renders a toggle per entry", () => {
    const { container } = mount({ schema });
    expect(container.textContent).toContain("Trim whitespace");
    expect(container.textContent).toContain("De-duplicate");
  });

  it("toggles an entry and commits a keyed boolean map", () => {
    const { container, onChange } = mount({ schema });
    const toggle = container.querySelector('[role="switch"], button[role="switch"], button');
    click(toggle as Element);
    const v = lastValue(onChange).checks as Record<string, boolean>;
    expect(v.trim).toBe(true);
  });
});

// ── element-rules / attribute-rules (MapEditor) ────────────────────────────

describe("element-rules widget", () => {
  const schema: ComponentSchema = {
    title: "Element Rules",
    type: "object",
    properties: {
      rules: {
        type: "object",
        title: "Rules",
        "ui:widget": "element-rules",
        additionalProperties: {
          type: "object",
          properties: { translatable: { type: "boolean", title: "Translatable" } },
        },
      },
    },
  };

  it("renders existing map keys", () => {
    const { container } = mount({ schema, initial: { rules: { div: { translatable: true } } } });
    expect(container.textContent).toContain("div");
  });

  it("adds a new entry keyed by the typed element name", () => {
    const { container, onChange } = mount({ schema });
    const keyInput = container.querySelector("input") as HTMLInputElement;
    typeInput(keyInput, "span");
    const add = buttonByText(container, "Add")!;
    click(add);
    const v = lastValue(onChange).rules as Record<string, unknown>;
    expect(Object.keys(v)).toContain("span");
  });
});

describe("attribute-rules widget", () => {
  const schema: ComponentSchema = {
    title: "Attribute Rules",
    type: "object",
    properties: {
      attrs: {
        type: "object",
        title: "Attrs",
        "ui:widget": "attribute-rules",
        additionalProperties: { type: "string" },
      },
    },
  };

  it("uses an attribute-name key placeholder and adds entries", () => {
    const { container, onChange } = mount({ schema });
    const keyInput = container.querySelector("input") as HTMLInputElement;
    expect(keyInput.getAttribute("placeholder")).toBe("attribute name");
    typeInput(keyInput, "title");
    click(buttonByText(container, "Add")!);
    const v = lastValue(onChange).attrs as Record<string, unknown>;
    expect(Object.keys(v)).toContain("title");
  });
});

// ── regex (CodeMirror — structural) ────────────────────────────────────────

describe("regex widget", () => {
  const schema: ComponentSchema = {
    title: "Regex",
    type: "object",
    properties: { pattern: { type: "string", title: "Pattern", "ui:widget": "regex" } },
  };

  // CodeMirror's contenteditable model does not replay edits reliably in
  // jsdom, so we assert the editor mounts (mirrors InlineCodeEditor.test.tsx).
  it("mounts a CodeMirror editor for the regex pattern", () => {
    const { container } = mount({ schema, initial: { pattern: "\\d+" } });
    expect(container.querySelector(".cm-editor")).not.toBeNull();
  });
});

// ── simplifier-rules (CodeMirror — structural) ─────────────────────────────

describe("simplifier-rules widget", () => {
  const schema: ComponentSchema = {
    title: "Simplifier",
    type: "object",
    properties: { rules: { type: "string", title: "Rules", "ui:widget": "simplifier-rules" } },
  };

  it("mounts a CodeMirror editor for the simplifier rules", () => {
    const { container } = mount({ schema, initial: { rules: 'if TYPE = "b";' } });
    expect(container.querySelector(".cm-editor")).not.toBeNull();
  });
});

// ── code-finder ────────────────────────────────────────────────────────────

describe("code-finder widget", () => {
  const schema: ComponentSchema = {
    title: "Code Finder",
    type: "object",
    properties: {
      codes: {
        type: "object",
        title: "Inline Codes",
        "ui:widget": "code-finder",
        "ui:presets": {
          "HTML Tags": { rules: [{ pattern: "</?\\w[^>]*>" }], sample: "<b>x</b>" },
        },
      },
    },
  };

  it("renders the code-finder editor with its preset menu", () => {
    const { container } = mount({ schema });
    expect(container.querySelector('[data-slot="code-finder-editor"]')).not.toBeNull();
    expect(buttonByText(container, "Presets")).toBeDefined();
  });

  it("applies a preset, committing its rules + sample to the value", () => {
    const { container, onChange } = mount({ schema });
    click(buttonByText(container, "Presets")!);
    click(buttonByText(container, "HTML Tags")!);
    const v = lastValue(onChange).codes as { rules: Array<{ pattern: string }>; sample?: string };
    expect(v.rules[0].pattern).toBe("</?\\w[^>]*>");
    expect(v.sample).toBe("<b>x</b>");
  });

  it("adds an empty rule via Add rule", () => {
    const { container, onChange } = mount({ schema });
    click(buttonByText(container, "Add rule")!);
    const v = lastValue(onChange).codes as { rules: Array<{ pattern: string }> };
    expect(v.rules.length).toBeGreaterThan(0);
  });
});
