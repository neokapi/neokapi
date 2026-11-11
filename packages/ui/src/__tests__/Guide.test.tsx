// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vite-plus/test";

import { GuideProvider, useGuide, useOptionalGuide } from "../components/guide";
import type { Guide, HelpTopic } from "../components/guide";
import { placeBubble } from "../components/guide/geometry";

beforeAll(() => {
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
  Element.prototype.scrollIntoView ??= () => {};
});

beforeEach(() => localStorage.clear());
afterEach(cleanup);

const STORAGE_KEY = "test.guides";

const tour: Guide = {
  id: "tour",
  title: "Check and fix a file",
  summary: "Run the checks and fix what they find",
  topic: "checks",
  steps: [
    { id: "one", target: '[data-slot="first"]', title: "Pick a file", body: "The **file** list." },
    {
      id: "two",
      target: '[data-slot="second"]',
      title: "Run the checks",
      body: "Press **Check**.",
    },
    { id: "three", title: "You are done", body: <p>All fixed.</p> },
  ],
};

const blocked: Guide = {
  id: "blocked",
  title: "Share a project",
  summary: "Invite someone to the project",
  available: () => ({ ok: false, reason: "Open a project first" }),
  steps: [{ id: "a", title: "Share", body: "Share it." }],
};

const topics: HelpTopic[] = [
  {
    id: "checks",
    title: "Checks",
    summary: "What a check reports",
    body: "## Findings\n\nEach **finding** names a rule.",
    guides: ["tour"],
    moreUrl: "https://example.com/checks",
  },
  { id: "terms", title: "Terms", summary: "Approved words", body: "Terms." },
];

function Controls() {
  const api = useGuide();
  return (
    <div>
      <button type="button" onClick={() => api.start("tour")}>
        start tour
      </button>
      <button type="button" onClick={() => api.openHelp()}>
        open help
      </button>
      <button type="button" onClick={() => api.openHelp("checks")}>
        open checks
      </button>
      <span data-testid="active">
        {api.active ? `${api.active.guide.id}:${api.active.step}` : "none"}
      </span>
      <span data-testid="completed">{[...api.completed].join(",")}</span>
    </div>
  );
}

function setup(extra: Partial<React.ComponentProps<typeof GuideProvider>> = {}) {
  const onOpenUrl = vi.fn();
  render(
    <GuideProvider
      guides={[tour, blocked]}
      topics={topics}
      onOpenUrl={onOpenUrl}
      storageKey={STORAGE_KEY}
      {...extra}
    >
      <div data-slot="first">first</div>
      <div data-slot="second">second</div>
      <Controls />
    </GuideProvider>,
  );
  return { onOpenUrl };
}

async function bubble() {
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect(dialog.getAttribute("data-state")).toBe("ready"));
  return dialog;
}

describe("useGuide", () => {
  it("throws outside the provider", () => {
    function Bare() {
      useGuide();
      return null;
    }
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => render(<Bare />)).toThrow(/GuideProvider/);
    spy.mockRestore();
  });

  it("useOptionalGuide returns null outside the provider", () => {
    let seen: unknown = "unset";
    function Bare() {
      seen = useOptionalGuide();
      return null;
    }
    render(<Bare />);
    expect(seen).toBeNull();
  });
});

describe("tour", () => {
  it("starts, moves forward and back, and stops", async () => {
    setup();
    fireEvent.click(screen.getByText("start tour"));
    let dialog = await bubble();
    expect(within(dialog).getByText("Pick a file")).toBeTruthy();
    expect(within(dialog).getByText("1 of 3")).toBeTruthy();
    expect(within(dialog).queryByText("Back")).toBeNull();
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    const title = within(dialog).getByText("Pick a file");
    expect(dialog.getAttribute("aria-labelledby")).toBe(title.id);

    fireEvent.click(within(dialog).getByText("Next"));
    dialog = await bubble();
    await waitFor(() => expect(within(dialog).getByText("Run the checks")).toBeTruthy());
    expect(screen.getByTestId("active").textContent).toBe("tour:1");

    fireEvent.click(within(dialog).getByText("Back"));
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:0"));

    fireEvent.click(within(dialog).getByLabelText("Skip tour"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTestId("active").textContent).toBe("none");
    expect(screen.getByTestId("completed").textContent).toBe("");
  });

  it("answers the keyboard", async () => {
    setup();
    fireEvent.click(screen.getByText("start tour"));
    await bubble();
    fireEvent.keyDown(document.body, { key: "ArrowRight" });
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:1"));
    fireEvent.keyDown(document.body, { key: "Enter" });
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:2"));
    fireEvent.keyDown(document.body, { key: "ArrowLeft" });
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:1"));
    fireEvent.keyDown(document.body, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("moves focus to the bubble and back when the tour ends", async () => {
    setup();
    const trigger = screen.getByText("start tour");
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = await bubble();
    await waitFor(() => expect(document.activeElement).toBe(dialog));
    fireEvent.keyDown(document.body, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("centres the bubble when the target never appears", async () => {
    const lost: Guide = {
      id: "tour",
      title: "Lost",
      summary: "",
      steps: [
        { id: "x", target: '[data-slot="nowhere"]', title: "Nowhere", body: "Gone.", waitFor: 50 },
      ],
    };
    setup({ guides: [lost] });
    fireEvent.click(screen.getByText("start tour"));
    const dialog = await bubble();
    expect(dialog.getAttribute("data-placement")).toBe("center");
    expect(document.querySelector('[data-slot="guide-cutout"]')).toBeNull();
    expect(document.querySelector('[data-slot="guide-dim"]')).not.toBeNull();
  });

  it("passes over an optional step whose target is not there, both ways", async () => {
    const gaps: Guide = {
      id: "tour",
      title: "Gaps",
      summary: "",
      steps: [
        { id: "a", target: '[data-slot="first"]', title: "First step", body: "One." },
        {
          id: "b",
          target: '[data-slot="nowhere"]',
          title: "Absent step",
          body: "Two.",
          waitFor: 50,
          optional: true,
        },
        { id: "c", target: '[data-slot="second"]', title: "Third step", body: "Three." },
      ],
    };
    setup({ guides: [gaps] });
    fireEvent.click(screen.getByText("start tour"));
    await bubble();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:2"));
    expect((await bubble()).textContent).toContain("Third step");
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:0"));
    expect((await bubble()).textContent).toContain("First step");
  });

  it("runs before() and waits for a target it adds", async () => {
    const late: Guide = {
      id: "tour",
      title: "Late",
      summary: "",
      steps: [
        {
          id: "late",
          target: '[data-slot="late"]',
          title: "Late",
          body: "Here now.",
          before: () => {
            setTimeout(() => {
              const el = document.createElement("div");
              el.setAttribute("data-slot", "late");
              document.body.appendChild(el);
            }, 30);
          },
        },
      ],
    };
    setup({ guides: [late] });
    fireEvent.click(screen.getByText("start tour"));
    const dialog = await bubble();
    expect(dialog.getAttribute("data-placement")).not.toBe("center");
    expect(document.querySelector('[data-slot="guide-cutout"]')).not.toBeNull();
    document.querySelector('[data-slot="late"]')?.remove();
  });

  it("advances when the lit target is clicked", async () => {
    const act1: Guide = {
      id: "tour",
      title: "Act",
      summary: "",
      steps: [
        {
          id: "a",
          target: '[data-slot="first"]',
          title: "Click it",
          body: "Go.",
          advanceOn: "target",
        },
        { id: "b", title: "Done", body: "Yes." },
      ],
    };
    setup({ guides: [act1] });
    fireEvent.click(screen.getByText("start tour"));
    await bubble();
    fireEvent.click(screen.getByText("first"));
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:1"));
  });

  it("marks the guide completed after the last step and keeps it in storage", async () => {
    setup();
    fireEvent.click(screen.getByText("start tour"));
    for (const label of ["Next", "Next", "Done"]) {
      const dialog = await bubble();
      await act(async () => {
        fireEvent.click(within(dialog).getByText(label));
      });
    }
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTestId("completed").textContent).toBe("tour");
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "[]")).toEqual(["tour"]);

    cleanup();
    setup();
    expect(screen.getByTestId("completed").textContent).toBe("tour");
  });

  it("works when storage throws", async () => {
    const get = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    setup({ guides: [{ ...tour, steps: [tour.steps[2]!] }] });
    fireEvent.click(screen.getByText("start tour"));
    const dialog = await bubble();
    fireEvent.click(within(dialog).getByText("Done"));
    await waitFor(() => expect(screen.getByTestId("completed").textContent).toBe("tour"));
    get.mockRestore();
    set.mockRestore();
  });
});

describe("help panel", () => {
  it("lists guides and topics and filters them", async () => {
    setup();
    fireEvent.click(screen.getByText("open help"));
    const panel = await screen.findByRole("dialog");
    expect(within(panel).getByText("Help")).toBeTruthy();
    expect(within(panel).getByText("Check and fix a file")).toBeTruthy();
    expect(within(panel).getByText("Terms")).toBeTruthy();

    // An unavailable guide shows its reason and a disabled Start.
    expect(within(panel).getByText("Open a project first")).toBeTruthy();
    const startShare = within(panel).getByLabelText("Start Share a project") as HTMLButtonElement;
    expect(startShare.disabled).toBe(true);

    fireEvent.change(within(panel).getByLabelText("Filter guides and topics"), {
      target: { value: "approved" },
    });
    expect(within(panel).queryByText("Check and fix a file")).toBeNull();
    expect(within(panel).getByText("Terms")).toBeTruthy();

    fireEvent.change(within(panel).getByLabelText("Filter guides and topics"), {
      target: { value: "zzz" },
    });
    expect(within(panel).getByText("No guides or topics match")).toBeTruthy();
  });

  it("opens a topic, goes back, and opens its website page", async () => {
    const { onOpenUrl } = setup();
    fireEvent.click(screen.getByText("open help"));
    const panel = await screen.findByRole("dialog");
    fireEvent.click(within(panel).getByText("Checks"));
    expect(within(panel).getByText("Findings")).toBeTruthy();
    expect(within(panel).getByLabelText("Start Check and fix a file")).toBeTruthy();

    fireEvent.click(within(panel).getByText("Read more on the website"));
    expect(onOpenUrl).toHaveBeenCalledWith("https://example.com/checks");

    fireEvent.click(within(panel).getByLabelText("Back to help"));
    expect(within(panel).getByText("Terms")).toBeTruthy();
  });

  it("opens straight on a topic", async () => {
    setup();
    fireEvent.click(screen.getByText("open checks"));
    const panel = await screen.findByRole("dialog");
    expect(within(panel).getByText("Findings")).toBeTruthy();
  });

  it("closes before a guide started from it begins", async () => {
    setup();
    fireEvent.click(screen.getByText("open help"));
    const panel = await screen.findByRole("dialog");
    fireEvent.click(within(panel).getByLabelText("Start Check and fix a file"));
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("tour:0"));
    const dialogs = screen.getAllByRole("dialog");
    expect(dialogs).toHaveLength(1);
    expect(within(dialogs[0]!).getByText("Pick a file")).toBeTruthy();
  });
});

describe("placeBubble", () => {
  const viewport = { width: 1000, height: 800 };
  const bubble = { width: 300, height: 150 };

  it("takes the preferred side when it fits", () => {
    const p = placeBubble(
      { top: 100, left: 400, width: 100, height: 40 },
      bubble,
      viewport,
      "bottom",
    );
    expect(p.placement).toBe("bottom");
    expect(p.top).toBe(152);
  });

  it("flips when the preferred side has no room", () => {
    const p = placeBubble(
      { top: 700, left: 400, width: 100, height: 40 },
      bubble,
      viewport,
      "bottom",
    );
    expect(p.placement).toBe("top");
  });

  it("clamps to the window", () => {
    const p = placeBubble({ top: 100, left: 0, width: 20, height: 20 }, bubble, viewport, "bottom");
    expect(p.left).toBe(8);
  });

  it("centres with no target", () => {
    const p = placeBubble(null, bubble, viewport);
    expect(p).toEqual({ top: 325, left: 350, placement: "center" });
  });
});
