import { describe, it, expect } from "vite-plus/test";
import { render } from "@testing-library/react";

import { CollapsedTargetCell } from "../components/editor/GridTargetRenderer";
import type { BlockInfo } from "../types/api";

function makeBlock(overrides: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    source: "Welcome",
    targets: {},
    targets_coded: {},
    translatable: true,
    has_spans: false,
    properties: {},
    ...overrides,
  };
}

describe("CollapsedTargetCell — writing direction", () => {
  it("puts dir/lang on the plain-text branch for an RTL target", () => {
    const block = makeBlock({ targets: { "ar-EG": "مرحباً" } });
    const { getByTestId } = render(
      <CollapsedTargetCell block={block} locale="ar-EG" testId="cell" />,
    );
    const cell = getByTestId("cell");
    expect(cell.getAttribute("dir")).toBe("rtl");
    expect(cell.getAttribute("lang")).toBe("ar-EG");
    expect(cell.textContent).toContain("مرحباً");
  });

  it("puts dir/lang on the chip (has_spans) branch for an RTL target", () => {
    const block = makeBlock({
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
      targets: { "ar-EG": "مرحباً {count}" },
      targets_coded: { "ar-EG": "مرحباً " },
    });
    const { getByTestId } = render(
      <CollapsedTargetCell block={block} locale="ar-EG" testId="cell" />,
    );
    const cell = getByTestId("cell");
    expect(cell.getAttribute("dir")).toBe("rtl");
    expect(cell.getAttribute("lang")).toBe("ar-EG");
  });

  it("puts dir/lang on the plural-preview branch for an RTL target", () => {
    const block = makeBlock({
      targets: { "ar-EG": "{count, plural, one {واحد} other {# عناصر}}" },
    });
    const { getByTestId } = render(
      <CollapsedTargetCell block={block} locale="ar-EG" testId="cell" />,
    );
    const cell = getByTestId("cell");
    expect(cell.getAttribute("dir")).toBe("rtl");
    expect(cell.getAttribute("lang")).toBe("ar-EG");
  });

  it("defaults to ltr for an LTR target locale", () => {
    const block = makeBlock({ targets: { fr: "Bienvenue" } });
    const { getByTestId } = render(<CollapsedTargetCell block={block} locale="fr" testId="cell" />);
    const cell = getByTestId("cell");
    expect(cell.getAttribute("dir")).toBe("ltr");
  });
});
