// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  DIGEST_KINDS,
  DIGEST_THEMES,
  KIND_ICONS,
  KindIcon,
  THEME_KINDS,
  digestKind,
  kindLabel,
} from "../components/ui/kind-icon";
import type { DigestKind } from "../components/ui/kind-icon";

afterEach(cleanup);

const kinds = Object.keys(KIND_ICONS) as DigestKind[];

describe("KindIcon", () => {
  it("declares an icon for exactly the kinds in DIGEST_KINDS", () => {
    expect([...kinds].sort()).toEqual([...DIGEST_KINDS].sort());
  });

  it("draws every kind as an icon named by its label", () => {
    for (const kind of kinds) {
      const { unmount } = render(<KindIcon kind={kind} />);
      const label = kindLabel(kind);
      expect(label, kind).not.toBe("");
      const icon = screen.getByRole("img", { name: label });
      expect(icon.getAttribute("data-kind")).toBe(kind);
      expect(icon.querySelector("svg"), kind).not.toBeNull();
      unmount();
    }
  });

  it("gives every kind its own label and its own icon", () => {
    expect(new Set(kinds.map(kindLabel)).size).toBe(kinds.length);
    expect(new Set(kinds.map((k) => KIND_ICONS[k].icon)).size).toBe(kinds.length);
  });

  it("draws a disagreement in the warning ink, an established rule in the success ink, and the rest muted", () => {
    const tones = Object.fromEntries(
      kinds.map((kind) => {
        const { container, unmount } = render(<KindIcon kind={kind} />);
        const el = container.querySelector("[data-slot=kind-icon]") as HTMLElement;
        const out = [kind, { tone: el.getAttribute("data-tone"), className: el.className }];
        unmount();
        return out;
      }),
    ) as Record<DigestKind, { tone: string; className: string }>;
    expect(tones.conflict.tone).toBe("warning");
    expect(tones.conflict.className).toContain("text-warning-ink");
    expect(tones.established.tone).toBe("success");
    expect(tones.established.className).toContain("text-success");
    for (const kind of kinds.filter((k) => k !== "conflict" && k !== "established")) {
      expect(tones[kind].tone, kind).toBe("muted");
      expect(tones[kind].className, kind).toContain("text-muted-foreground");
    }
  });

  it("draws the two sizes at 14 and 16 pixels", () => {
    const { container } = render(
      <>
        <KindIcon kind="name" />
        <KindIcon kind="name" size="md" />
      </>,
    );
    const svgs = container.querySelectorAll("svg");
    expect(svgs[0].getAttribute("class")).toContain("size-3.5");
    expect(svgs[1].getAttribute("class")).toContain("size-4");
  });

  it("hides a decorative icon from assistive technology", () => {
    const { container } = render(<KindIcon kind="note" decorative />);
    expect(screen.queryByRole("img")).toBeNull();
    expect(container.querySelector("[data-slot=kind-icon]")?.getAttribute("aria-hidden")).toBe(
      "true",
    );
  });
});

describe("digestKind", () => {
  it("maps every digest theme to a kind", () => {
    for (const theme of DIGEST_THEMES) {
      const kind = THEME_KINDS[theme];
      expect(DIGEST_KINDS, theme).toContain(kind);
      expect(digestKind({ theme, status: "suggested" })).toBe(kind);
    }
    expect(THEME_KINDS).toEqual({
      names: "name",
      words: "word",
      writing: "note",
      memory: "memory",
    });
  });

  it("draws a contested item as a disagreement and an established one as a rule, whatever its theme", () => {
    for (const theme of DIGEST_THEMES) {
      expect(digestKind({ theme, status: "contested" })).toBe("conflict");
      expect(digestKind({ theme, status: "established" })).toBe("established");
    }
  });

  it("reads the subject of an entry that carries no theme", () => {
    expect(digestKind({ status: "suggested", subject: { kind: "memory" } })).toBe("memory");
    expect(digestKind({ status: "suggested", subject: { kind: "note" } })).toBe("note");
    expect(digestKind({ status: "suggested", subject: { kind: "term" } })).toBe("word");
  });

  it("draws anything it cannot place as a word rule, as the digest does", () => {
    expect(digestKind({})).toBe("word");
    expect(digestKind({ theme: "tone" })).toBe("word");
    expect(digestKind({ theme: "constructor" })).toBe("word");
  });
});
