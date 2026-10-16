// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ActorMark, KNOWN_ACTORS, actorInitials, actorLabel } from "../components/ui/actor-mark";

afterEach(cleanup);

/** The actors drawn with a hue and two letters of their own. */
const ASSISTANTS = ["claude-code", "claude-desktop", "codex", "cursor", "vscode"] as const;

describe("ActorMark", () => {
  it("names every known actor", () => {
    expect(Object.fromEntries(KNOWN_ACTORS.map((a) => [a, actorLabel(a)]))).toEqual({
      "claude-code": "Claude Code",
      "claude-desktop": "Claude Desktop",
      codex: "Codex",
      cursor: "Cursor",
      vscode: "VS Code",
      person: "You",
      sync: "Sync",
    });
  });

  it("draws every known actor with initials or an icon, named by its label", () => {
    for (const actor of KNOWN_ACTORS) {
      const { unmount } = render(<ActorMark actor={actor} />);
      const mark = screen.getByRole("img", { name: actorLabel(actor) });
      expect(mark.getAttribute("data-actor")).toBe(actor);
      const initials = actorInitials(actor);
      if (initials) {
        expect(initials).toMatch(/^[A-Z]{2}$/);
        expect(mark.textContent).toBe(initials);
      } else {
        expect(mark.querySelector("svg"), actor).not.toBeNull();
      }
      unmount();
    }
  });

  it("gives every assistant its own two letters and its own hue", () => {
    const initials = ASSISTANTS.map((a) => actorInitials(a));
    expect(initials).toEqual(["CC", "CD", "CX", "CU", "VS"]);
    const hues = ASSISTANTS.map((actor) => {
      const { container, unmount } = render(<ActorMark actor={actor} />);
      const hue = (
        container.querySelector("[data-slot=actor-mark]") as HTMLElement
      ).style.getPropertyValue("--mark-hue");
      unmount();
      return hue;
    });
    expect(hues.every((h) => h !== "")).toBe(true);
    expect(new Set(hues).size).toBe(ASSISTANTS.length);
  });

  it("draws a person and a sync on the neutral tint", () => {
    for (const actor of ["person", "sync"]) {
      const { container, unmount } = render(<ActorMark actor={actor} />);
      const mark = container.querySelector("[data-slot=actor-mark]") as HTMLElement;
      expect(mark.className).toContain("bg-axis-unknown");
      expect(mark.style.getPropertyValue("--mark-hue")).toBe("");
      unmount();
    }
  });

  it("falls back to initials from the string and the neutral tint for an unknown actor", () => {
    render(<ActorMark actor="lab-agent" />);
    const mark = screen.getByRole("img", { name: "lab-agent" });
    expect(mark.textContent).toBe("LA");
    expect(mark.className).toContain("bg-axis-unknown");
    expect(actorLabel("lab-agent")).toBe("lab-agent");
    expect(actorInitials("claude@studio")).toBe("CS");
    expect(actorInitials("settle")).toBe("SE");
  });

  it("takes a person's initials and name from the name it is given", () => {
    render(<ActorMark actor="person" name="Ingrid Solberg" />);
    const mark = screen.getByRole("img", { name: "Ingrid Solberg" });
    expect(mark.textContent).toBe("IS");
    expect(actorInitials("person", "Ola")).toBe("OL");
    expect(actorInitials("person", "Anne-Marie de la Cruz")).toBe("AC");
    expect(actorInitials("person", "  ")).toBeNull();
  });

  it("draws a person with no name as a person icon named You", () => {
    render(<ActorMark actor="person" />);
    const mark = screen.getByRole("img", { name: "You" });
    expect(mark.textContent).toBe("");
    expect(mark.querySelector("svg")).not.toBeNull();
  });

  it("keeps a known assistant's product name when a record's own name is passed", () => {
    render(<ActorMark actor="claude-code" name="claude-code" />);
    expect(screen.getByRole("img", { name: "Claude Code" }).textContent).toBe("CC");
  });

  it("reads a known actor whatever its case", () => {
    render(<ActorMark actor="Codex" />);
    expect(screen.getByRole("img", { name: "Codex" }).getAttribute("data-actor")).toBe("codex");
  });

  it("hides a decorative mark from assistive technology", () => {
    const { container } = render(<ActorMark actor="codex" decorative />);
    expect(screen.queryByRole("img")).toBeNull();
    const mark = container.querySelector("[data-slot=actor-mark]");
    expect(mark?.getAttribute("aria-hidden")).toBe("true");
    expect(mark?.textContent).toBe("CX");
  });

  it("draws the three sizes at 18, 22 and 28 pixels", () => {
    const sizes = (["sm", "md", "lg"] as const).map((size) => {
      const { container, unmount } = render(<ActorMark actor="codex" size={size} />);
      const cls = (container.querySelector("[data-slot=actor-mark]") as HTMLElement).className;
      unmount();
      return cls;
    });
    expect(sizes[0]).toContain("size-[18px]");
    expect(sizes[1]).toContain("size-[22px]");
    expect(sizes[2]).toContain("size-7");
  });
});
