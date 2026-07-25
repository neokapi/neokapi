// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";

import {
  RunErrorNotice,
  type RunErrorNoticeProps,
  type RunErrorView,
} from "../components/ui/run-error-notice";

function render(props: RunErrorNoticeProps): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(createElement(RunErrorNotice, props));
  });
  return container;
}

const modelMissing: RunErrorView = {
  kind: "model-not-installed",
  headline: "Model gemma4:e2b isn't installed",
  remediation: "Pull the model, then run again.",
  actions: [
    { kind: "command", label: "Pull the model", command: "ollama pull gemma4:e2b" },
    { kind: "open-settings", label: "Choose another model", target: "settings" },
    { kind: "open-url", label: "Install Ollama", url: "https://ollama.com" },
  ],
  locale: "nb",
  file: "notes/release-notes.md",
  provider: "ollama",
  model: "gemma4:e2b",
  raw: "converge nb: flow execution error: notes/release-notes.md: execute flow: tool translate: translate: ollama: model not installed",
};

describe("RunErrorNotice", () => {
  it("leads with the headline and remediation, not the raw chain", () => {
    const c = render({ error: modelMissing });
    expect(c.querySelector('[data-slot="run-error-headline"]')?.textContent).toBe(
      "Model gemma4:e2b isn't installed",
    );
    expect(c.querySelector('[data-slot="run-error-remediation"]')?.textContent).toBe(
      "Pull the model, then run again.",
    );
    // The wrapped chain is available but not shown until asked for.
    expect(c.textContent).not.toContain("flow execution error");
    expect(c.querySelector('[data-slot="run-error-details-toggle"]')).not.toBeNull();
    expect(c.querySelector('[data-slot="run-error-raw"]')).toBeNull();
  });

  it("reveals the raw chain behind Details", () => {
    const c = render({ error: modelMissing });
    const toggle = c.querySelector<HTMLButtonElement>('[data-slot="run-error-details-toggle"]')!;
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    act(() => toggle.click());
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(c.querySelector('[data-slot="run-error-raw"]')?.textContent).toContain(
      "flow execution error",
    );
  });

  it("shows the affected scope as metadata, isolated and LTR", () => {
    const c = render({ error: modelMissing });
    const scope = c.querySelector('[data-slot="run-error-scope"]')!;
    expect(scope.textContent).toContain("nb");
    expect(scope.textContent).toContain("notes/release-notes.md");
    expect(scope.textContent).toContain("gemma4:e2b");
    // Identifiers must not be reordered by an RTL UI language.
    for (const bdi of scope.querySelectorAll("bdi")) {
      expect(bdi.getAttribute("dir")).toBe("ltr");
    }
  });

  it("renders the remedy as an action carrying the concrete command", () => {
    const c = render({ error: modelMissing });
    const cmd = c.querySelector('[data-action-kind="command"]');
    expect(cmd?.textContent).toContain("ollama pull gemma4:e2b");
  });

  it("hands non-command actions to the host", () => {
    const onAction = vi.fn();
    const c = render({ error: modelMissing, onAction });
    const settings = c.querySelector<HTMLButtonElement>('[data-action-kind="open-settings"]')!;
    act(() => settings.click());
    expect(onAction).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "open-settings", target: "settings" }),
    );
  });

  it("omits actions the host cannot service rather than rendering them inert", () => {
    const c = render({ error: modelMissing, actionKinds: ["command", "open-url"] });
    expect(c.querySelector('[data-action-kind="command"]')).not.toBeNull();
    expect(c.querySelector('[data-action-kind="open-url"]')).not.toBeNull();
    expect(c.querySelector('[data-action-kind="open-settings"]')).toBeNull();
  });

  it("drops the actions row entirely when none can be serviced", () => {
    const c = render({ error: modelMissing, actionKinds: [] });
    expect(c.querySelector('[data-slot="run-error-actions"]')).toBeNull();
    // …but the headline and remediation still stand on their own.
    expect(c.querySelector('[data-slot="run-error-headline"]')).not.toBeNull();
  });

  it("is announced as an alert and tagged with its kind", () => {
    const c = render({ error: modelMissing });
    const root = c.querySelector('[data-slot="run-error-notice"]')!;
    expect(root.getAttribute("role")).toBe("alert");
    expect(root.getAttribute("data-kind")).toBe("model-not-installed");
  });

  it("copes with a failure that has no remedy at all", () => {
    const c = render({
      error: { kind: "unknown", headline: "something novel broke", raw: "something novel broke" },
    });
    expect(c.querySelector('[data-slot="run-error-headline"]')?.textContent).toBe(
      "something novel broke",
    );
    expect(c.querySelector('[data-slot="run-error-actions"]')).toBeNull();
    expect(c.querySelector('[data-slot="run-error-scope"]')).toBeNull();
  });
});
