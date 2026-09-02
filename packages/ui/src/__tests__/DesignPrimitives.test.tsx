// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { buttonVariants } from "../components/ui/button";
import { AXES, AXIS_IDS, CoordinateChip, axisMeta } from "../components/ui/coordinate-chip";
import {
  CONTENT_STATUS_LADDER,
  SOURCE_STATUS_LADDER,
  StatusBadge,
  statusMeta,
} from "../components/ui/status-badge";
import { LocaleLabel } from "../components/ui/locale-label";

// React only treats `act` as real inside an act environment; without the flag
// every render logs a warning that buries the assertions.
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement | null = null;
let root: Root | null = null;

function render(el: React.ReactElement) {
  container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    root = createRoot(container!);
    root.render(el);
  });
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = null;
  root = null;
});

describe("Button semantic variants", () => {
  it("fills success from its own token pair", () => {
    const classes = buttonVariants({ variant: "success" });
    expect(classes).toContain("bg-success");
    expect(classes).toContain("text-success-foreground");
    expect(classes).toContain("hover:bg-success/90");
  });

  it("fills warning from its own token pair", () => {
    const classes = buttonVariants({ variant: "warning" });
    expect(classes).toContain("bg-warning");
    expect(classes).toContain("text-warning-foreground");
    expect(classes).toContain("hover:bg-warning/90");
  });

  // The focus ring follows the button's own hue rather than the page's, the way
  // destructive already does, so a keyboard user sees which button is focused.
  it("rings in its own hue in both themes", () => {
    expect(buttonVariants({ variant: "success" })).toContain("focus-visible:ring-success/40");
    expect(buttonVariants({ variant: "success" })).toContain("dark:focus-visible:ring-success/50");
    expect(buttonVariants({ variant: "warning" })).toContain("focus-visible:ring-warning/40");
  });

  it("leaves the other variants alone", () => {
    expect(buttonVariants({ variant: "default" })).toContain("bg-primary");
    expect(buttonVariants({ variant: "destructive" })).toContain("text-destructive");
  });

  it("carries the variant through every size", () => {
    for (const size of ["xs", "sm", "default", "lg"] as const) {
      expect(buttonVariants({ variant: "success", size })).toContain("bg-success");
    }
  });
});

describe("CoordinateChip", () => {
  it("names the axis in its accessible name", () => {
    render(<CoordinateChip axis="channel" value="reference" />);
    const chip = container!.querySelector("[data-slot='coordinate-chip']")!;
    expect(chip.getAttribute("aria-label")).toBe("Channel: reference");
    expect(chip.getAttribute("title")).toBe("Channel: reference");
  });

  it("names every axis in the vocabulary", () => {
    const expected: Record<string, string> = {
      product: "Product",
      channel: "Channel",
      brand: "Brand",
      language: "Language",
    };
    for (const axis of AXIS_IDS) {
      render(<CoordinateChip axis={axis} value="v" />);
      const chip = container!.querySelector("[data-slot='coordinate-chip']")!;
      expect(chip.getAttribute("aria-label")).toBe(`${expected[axis]}: v`);
      act(() => root?.unmount());
      container?.remove();
    }
  });

  // A recipe may declare any axis, so an unknown one is ordinary: it keeps its
  // own id as the name and takes the neutral tint.
  it("falls back to the axis id and the neutral tint", () => {
    render(<CoordinateChip axis="region" value="EMEA" />);
    const chip = container!.querySelector("[data-slot='coordinate-chip']")!;
    expect(chip.getAttribute("aria-label")).toBe("region: EMEA");
    expect(chip.className).toContain("bg-axis-unknown");
    expect(axisMeta("region").token).toBe("--axis-unknown");
  });

  it("draws the value exactly as given", () => {
    render(<CoordinateChip axis="language" value="sr-Latn-RS" />);
    const chip = container!.querySelector("[data-slot='coordinate-chip']")!;
    expect(chip.textContent).toBe("sr-Latn-RS");
    expect(chip.className).not.toContain("uppercase");
    expect(chip.className).not.toContain("capitalize");
  });

  it("gives each axis its own tint pair", () => {
    for (const axis of AXIS_IDS) {
      expect(AXES[axis].className).toBe(`bg-axis-${axis} text-axis-${axis}-foreground`);
      expect(AXES[axis].token).toBe(`--axis-${axis}`);
    }
  });

  it("stays a group when it carries a remove control", () => {
    const onRemove = vi.fn();
    render(<CoordinateChip axis="product" value="kapi" onRemove={onRemove} />);
    const chip = container!.querySelector("[data-slot='coordinate-chip']")!;
    expect(chip.getAttribute("role")).toBe("group");
    const button = chip.querySelector("button")!;
    expect(button.getAttribute("aria-label")).toBe("Remove Product: kapi");
    act(() => {
      button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("is an image when it carries no control", () => {
    render(<CoordinateChip axis="product" value="kapi" />);
    expect(container!.querySelector("[data-slot='coordinate-chip']")!.getAttribute("role")).toBe(
      "img",
    );
  });
});

describe("StatusBadge", () => {
  const contentLabels: Record<string, string> = {
    draft: "Draft",
    translated: "Translated",
    reviewed: "Reviewed",
    "signed-off": "Signed off",
  };
  const contentTones: Record<string, string> = {
    draft: "start",
    translated: "middle",
    reviewed: "earned",
    "signed-off": "settled",
  };

  it("draws every rung of the content ladder", () => {
    for (const status of CONTENT_STATUS_LADDER) {
      render(<StatusBadge ladder="content" status={status} />);
      const badge = container!.querySelector("[data-slot='status-badge']")!;
      expect(badge.textContent).toBe(contentLabels[status]);
      expect(badge.getAttribute("data-tone")).toBe(contentTones[status]);
      expect(badge.getAttribute("data-status")).toBe(status);
      act(() => root?.unmount());
      container?.remove();
    }
  });

  const sourceLabels: Record<string, string> = {
    authored: "Authored",
    checked: "Checked",
    approved: "Approved",
  };
  const sourceTones: Record<string, string> = {
    authored: "start",
    checked: "earned",
    approved: "settled",
  };

  it("draws every rung of the source ladder", () => {
    for (const status of SOURCE_STATUS_LADDER) {
      render(<StatusBadge ladder="source" status={status} />);
      const badge = container!.querySelector("[data-slot='status-badge']")!;
      expect(badge.textContent).toBe(sourceLabels[status]);
      expect(badge.getAttribute("data-tone")).toBe(sourceTones[status]);
      act(() => root?.unmount());
      container?.remove();
    }
  });

  // The two ladders have to agree, or a reader who learnt one misreads the
  // other: checked reads as reviewed, approved reads as signed-off.
  it("puts the matching rungs on the same tone", () => {
    expect(statusMeta("source", "authored").tone).toBe(statusMeta("content", "draft").tone);
    expect(statusMeta("source", "checked").tone).toBe(statusMeta("content", "reviewed").tone);
    expect(statusMeta("source", "approved").tone).toBe(statusMeta("content", "signed-off").tone);
  });

  it("takes warning for a status that is waiting on a person", () => {
    for (const status of ["blocked", "attention"]) {
      expect(statusMeta("content", status).tone).toBe("attention");
      expect(statusMeta("source", status).tone).toBe("attention");
    }
    render(<StatusBadge ladder="content" status="blocked" />);
    expect(container!.querySelector("[data-slot='status-badge']")!.className).toContain(
      "bg-warning",
    );
  });

  // The platform counts a locale with no target text in a bucket below the
  // ladder; without a name for it the badge would draw the wire value.
  it("names the not-started bucket below the bottom rung", () => {
    render(<StatusBadge ladder="content" status="not-started" />);
    const badge = container!.querySelector("[data-slot='status-badge']")!;
    expect(badge.textContent).toBe("Not started");
    expect(badge.getAttribute("data-tone")).toBe("start");
  });

  it("keeps an unstyled status readable", () => {
    render(<StatusBadge ladder="content" status="proofread" />);
    const badge = container!.querySelector("[data-slot='status-badge']")!;
    expect(badge.textContent).toBe("proofread");
    expect(badge.getAttribute("data-tone")).toBe("middle");
  });

  it("shrinks for a dense row", () => {
    render(<StatusBadge ladder="content" status="draft" compact />);
    expect(container!.querySelector("[data-slot='status-badge']")!.className).toContain("h-4");
  });

  it("reads the wire spelling of signed-off", () => {
    expect(CONTENT_STATUS_LADDER).toContain("signed-off");
    expect(statusMeta("content", "signed_off").label).toBe("signed_off");
  });
});

describe("LocaleLabel", () => {
  it("shows the name with the code beside it", () => {
    render(<LocaleLabel locale="fr-FR" />);
    const label = container!.querySelector("[data-slot='locale-label']")!;
    expect(label.textContent).toBe("French (France)fr-FR");
    expect(label.getAttribute("title")).toBe("French (France) (fr-FR)");
  });

  it("shows the code alone in a compact context and keeps the name in the title", () => {
    render(<LocaleLabel locale="pt-BR" compact />);
    const label = container!.querySelector("[data-slot='locale-label']")!;
    expect(label.textContent).toBe("pt-BR");
    expect(label.getAttribute("title")).toBe("Brazilian Portuguese (pt-BR)");
  });

  it("never uppercases the tag", () => {
    render(<LocaleLabel locale="zh-Hant" />);
    const label = container!.querySelector("[data-slot='locale-label']")!;
    expect(label.innerHTML).toContain("zh-Hant");
    expect(label.innerHTML).not.toContain("uppercase");
    expect(label.innerHTML).not.toContain("capitalize");
  });

  it("marks the source language", () => {
    render(<LocaleLabel locale="en-US" source />);
    expect(container!.querySelector("[data-slot='locale-label']")!.textContent).toContain(
      "· source",
    );
  });

  it("drops the region for the short variant", () => {
    render(<LocaleLabel locale="fr-FR" variant="short" hideCode />);
    expect(container!.querySelector("[data-slot='locale-label']")!.textContent).toBe("French");
  });

  it("prefers a caller's own name for the locale", () => {
    render(<LocaleLabel locale="fr-FR" displayName="French (Canada office)" hideCode />);
    expect(container!.querySelector("[data-slot='locale-label']")!.textContent).toBe(
      "French (Canada office)",
    );
  });

  it("names the locale in the UI language it is given", () => {
    render(<LocaleLabel locale="fr-FR" uiLocale="nb" hideCode />);
    expect(container!.querySelector("[data-slot='locale-label']")!.textContent).toBe(
      "fransk (Frankrike)",
    );
  });

  // An unnamed tag would otherwise read "qps qps".
  it("falls back to the code alone when CLDR has no name", () => {
    render(<LocaleLabel locale="qps" />);
    expect(container!.querySelector("[data-slot='locale-label']")!.textContent).toBe("qps");
  });
});
