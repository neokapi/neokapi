// @vitest-environment jsdom
//
// The keyed reading of a catalog file: keys beside the text, every kind of run
// visible, and the same block-id contract the document reading carries.
//
// The case this exists for is `messages.json` in review. Read as a document it
// is a column of French with nothing to say which string is which, and a unit
// whose source carries a line break reads as "Première ligneDeuxième ligne",
// because a break contributes no literal text and a chip beside the words does
// not end the line.
import { describe, it, expect } from "vitest";
import { createElement as h, act } from "react";
import { createRoot } from "react-dom/client";

import KeyedTable from "../components/preview/KeyedTable";
import DataPreview from "../components/preview/DataPreview";
import FormatPreview from "../components/preview/FormatPreview";
import type { ContentNode, ContentTree, Run } from "../components/preview/types";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

function block(name: string, source: Run[], targets?: Record<string, Run[]>): ContentNode {
  return {
    kind: "block",
    id: `b:${name}`,
    name,
    source,
    ...(targets ? { targets } : {}),
  };
}

function tree(nodes: ContentNode[], format = "json"): ContentTree {
  return {
    format,
    root: nodes,
    stats: { layers: 0, groups: 0, blocks: nodes.length, data: 0, media: 0, runs: 0 },
  };
}

/** A line break as the engine emits it: a placeholder run in the break vocabulary. */
const BREAK: Run = { ph: { id: "1", type: "struct:break", data: "<br/>" } };

describe("KeyedTable", () => {
  it("shows the key beside the text, grouped by nesting", () => {
    const el = render(
      h(KeyedTable, {
        tree: tree([
          block("errors.network.timeout", [{ text: "Request timed out" }]),
          block("title", [{ text: "Kapimart" }]),
        ]),
      }),
    );

    const groups = [...el.querySelectorAll("[data-group-path]")].map((n) =>
      n.getAttribute("data-group-path"),
    );
    expect(groups).toEqual(["errors", "errors.network"]);

    const rows = [...el.querySelectorAll("tr[data-block-id]")];
    expect(rows.map((r) => r.getAttribute("data-key-path"))).toEqual([
      "title",
      "errors.network.timeout",
    ]);
    expect(rows[1].textContent).toContain("timeout");
    expect(rows[1].textContent).toContain("Request timed out");
  });

  it("shows the target column only when a locale is in view", () => {
    const t = tree([block("greeting", [{ text: "Hello" }], { fr: [{ text: "Bonjour" }] })]);

    const sourceOnly = render(h(KeyedTable, { tree: t }));
    expect(sourceOnly.querySelectorAll("thead th")).toHaveLength(2);
    expect(sourceOnly.textContent).not.toContain("Bonjour");

    const withTarget = render(h(KeyedTable, { tree: t, locale: "fr" }));
    expect(withTarget.querySelectorAll("thead th")).toHaveLength(3);
    expect(withTarget.textContent).toContain("Bonjour");
    expect(withTarget.textContent).toContain("Hello");
  });

  it("renders a line-break run as a break, not as a run-on string", () => {
    const el = render(
      h(KeyedTable, {
        tree: tree([
          block("intro", [{ text: "First line" }, BREAK, { text: "Second line" }], {
            fr: [{ text: "Première ligne" }, BREAK, { text: "Deuxième ligne" }],
          }),
        ]),
        locale: "fr",
      }),
    );

    const target = el.querySelectorAll("tr[data-block-id] td")[1];
    expect(target.querySelectorAll("br")).toHaveLength(1);
    expect(target.textContent).not.toContain("Première ligneDeuxième ligne");
    // The two lines are still both present, in order, on either side of the break.
    const html = target.innerHTML;
    expect(html.indexOf("Première ligne")).toBeLessThan(html.indexOf("<br"));
    expect(html.indexOf("<br")).toBeLessThan(html.indexOf("Deuxième ligne"));
  });

  it("renders a placeholder as a chip rather than dropping it", () => {
    const el = render(
      h(KeyedTable, {
        tree: tree([
          block("credits", [
            { text: "Your credits reset on " },
            { ph: { id: "1", type: "code:variable", data: "{date}", equiv: "date" } },
            { text: "." },
          ]),
        ]),
      }),
    );
    const chips = el.querySelectorAll("[data-inline-code]");
    expect(chips).toHaveLength(1);
    expect(el.textContent).toContain("Your credits reset on");
  });

  it("renders a paired code as two chips", () => {
    const el = render(
      h(KeyedTable, {
        tree: tree([
          block("bold", [
            { pcOpen: { id: "1", type: "fmt:bold", data: "<b>" } },
            { text: "Sale" },
            { pcClose: { id: "1", type: "fmt:bold", data: "</b>" } },
          ]),
        ]),
      }),
    );
    expect(el.querySelectorAll('[data-inline-code="opening"]')).toHaveLength(1);
    expect(el.querySelectorAll('[data-inline-code="closing"]')).toHaveLength(1);
    expect(el.textContent).toContain("Sale");
  });

  it("marks a plural as a plural rather than drawing an empty cell", () => {
    // The reading is the shared one: the branch the engine measures (`other`,
    // else the first present), behind a chip saying it is one form of several,
    // so a reader never mistakes it for the whole unit. A cell that flattened
    // the plural away drew an empty line instead.
    const el = render(
      h(KeyedTable, {
        tree: tree([
          block("cart", [
            {
              plural: {
                pivot: "count",
                forms: { one: [{ text: "One item" }], other: [{ text: "Many items" }] },
              },
            },
          ]),
        ]),
      }),
    );
    expect(el.textContent).toContain("Many items");
    const chip = el.querySelector("[data-inline-code]");
    expect(chip?.textContent).toBe("plural");
    expect(chip?.getAttribute("title")).toContain("{count, plural}");
  });

  it("pins the key LTR beside a right-to-left value", () => {
    const el = render(
      h(KeyedTable, {
        tree: tree([block("app.title", [{ text: "Kapimart" }], { ar: [{ text: "مرحبا" }] })]),
        locale: "ar",
      }),
    );
    // <bdi> isolates; the explicit dir pins the identifier LTR so it never
    // renders mirrored beside its Arabic value.
    const key = el.querySelector("tr[data-block-id] th bdi");
    expect(key?.getAttribute("dir")).toBe("ltr");
    expect(key?.textContent).toBe("title");
    const value = el.querySelectorAll("tr[data-block-id] td")[1];
    expect(value.querySelector("[dir='rtl']")?.getAttribute("lang")).toBe("ar");
  });

  it("carries the block-id contract, selection and host decoration", () => {
    const selected: string[] = [];
    const el = render(
      h(KeyedTable, {
        tree: tree([block("title", [{ text: "Kapimart" }])]),
        selectedBlockId: "b:title",
        onSelectBlock: (id: string) => selected.push(id),
        blockAttrs: () => ({ className: "host-class", "data-status": "translated" }),
      }),
    );

    // The host finds the row exactly as it finds a block in the document view.
    const row = el.querySelector('[data-block-id="b:title"]') as HTMLElement;
    expect(row).not.toBeNull();
    expect(row.tagName).toBe("TR");
    expect(row.getAttribute("aria-current")).toBe("true");
    expect(row.getAttribute("data-status")).toBe("translated");
    expect(row.className).toContain("host-class");
    expect(row.getAttribute("role")).toBe("button");

    act(() => {
      row.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(selected).toEqual(["b:title"]);
  });
});

describe("DataPreview", () => {
  const t = tree([block("title", [{ text: "Kapimart" }])]);

  it("shows the table alone when the host supplies no written-back file", () => {
    const el = render(h(DataPreview, { tree: t }));
    expect(el.querySelector('[data-preview="keyed-table"]')).not.toBeNull();
    expect(el.textContent).not.toContain("File");
  });

  it("offers the code view when the host supplies one", () => {
    const el = render(
      h(DataPreview, {
        tree: t,
        code: { text: '{\n  "title": "Kapimart"\n}\n', filename: "messages.json" },
      }),
    );
    const tabs = [...el.querySelectorAll("button")].map((b) => b.textContent);
    expect(tabs).toEqual(["Keys", "File"]);

    const file = [...el.querySelectorAll("button")].find((b) => b.textContent === "File")!;
    act(() => {
      file.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(el.querySelector('[data-preview="keyed-table"]')).toBeNull();
    expect(el.textContent).toContain('"title"');
  });

  it("asks the host for the file the first time the code view opens", () => {
    let asked = 0;
    const el = render(h(DataPreview, { tree: t, code: { onRequest: () => (asked += 1) } }));
    const file = [...el.querySelectorAll("button")].find((b) => b.textContent === "File")!;
    act(() => {
      file.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(asked).toBe(1);
    expect(el.textContent).toContain("not loaded");
  });
});

describe("FormatPreview routing", () => {
  it("reads a catalog format as a key table", () => {
    const el = render(
      h(FormatPreview, { tree: tree([block("title", [{ text: "Kapimart" }])], "json") }),
    );
    expect(el.querySelector('[data-preview="keyed-table"]')).not.toBeNull();
  });

  it("reads a prose format as a document", () => {
    const el = render(
      h(FormatPreview, { tree: tree([block("", [{ text: "A paragraph." }])], "markdown") }),
    );
    expect(el.querySelector('[data-preview="keyed-table"]')).toBeNull();
    expect(el.textContent).toContain("A paragraph.");
  });

  it("honours an explicit override either way", () => {
    const asDoc = render(
      h(FormatPreview, {
        tree: tree([block("title", [{ text: "Kapimart" }])], "json"),
        keyed: false,
      }),
    );
    expect(asDoc.querySelector('[data-preview="keyed-table"]')).toBeNull();

    const asKeys = render(
      h(FormatPreview, {
        tree: tree([block("intro", [{ text: "A paragraph." }])], "markdown"),
        keyed: true,
      }),
    );
    expect(asKeys.querySelector('[data-preview="keyed-table"]')).not.toBeNull();
  });
});
