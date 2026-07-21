// @vitest-environment jsdom
/**
 * The `useShipStatus` hook: it threads the picker `options` (the label override
 * map) through to `languagePickerModel`, so codes are labelled the same way in
 * React as in the headless transform. The first synchronous render already
 * resolves labels from an empty manifest, so no fetch needs to resolve to
 * observe the labelling.
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { useShipStatus } from "../src/ship/react.tsx";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function Picker() {
  const model = useShipStatus(["fr", "qps"], undefined, { labels: { qps: "Pseudo English" } });
  return (
    <ul>
      {model.map((m) => (
        <li key={m.locale} data-locale={m.locale}>
          {m.label}
        </li>
      ))}
    </ul>
  );
}

describe("useShipStatus", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("derives labels and applies the override map from options", () => {
    // A ship.json that resolves to {} leaves the first-render labels in place.
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 200 })),
    );
    const container = document.createElement("div");
    const root = createRoot(container);
    act(() => {
      root.render(<Picker />);
    });
    const labels = Object.fromEntries(
      Array.from(container.querySelectorAll("li")).map((li) => [
        li.getAttribute("data-locale"),
        li.textContent,
      ]),
    );
    expect(labels.fr).toBe("Français"); // derived endonym
    expect(labels.qps).toBe("Pseudo English"); // override from options.labels
    act(() => root.unmount());
  });
});
