// Which declared preview host this client can actually open.
//
// The declaration is per collection because a repository publishes one host per
// surface it ships — the desktop app's components and the web app's are two
// Storybooks — and it carries a kind because the hosts differ in how a view is
// found, not in how it renders.
import { describe, it, expect } from "vite-plus/test";

import { canReadInContext, storybookHost } from "../components/editor/previewHost";

describe("storybookHost", () => {
  it("resolves a declared Storybook", () => {
    expect(
      storybookHost({ kind: "storybook", url: "https://neokapi.github.io/storybook/bowrain/" }),
    ).toBe("https://neokapi.github.io/storybook/bowrain/");
  });

  it("offers nothing for a collection that declares no host", () => {
    expect(storybookHost(undefined)).toBeUndefined();
    expect(canReadInContext(undefined)).toBe(false);
  });

  // A kind this client cannot resolve a view within is not a Storybook at an
  // unfamiliar address. Guessing would put an empty iframe in front of a
  // reviewer with nothing to say why it is empty.
  it("offers nothing for a kind it cannot resolve", () => {
    expect(storybookHost({ kind: "ladle", url: "https://example.dev/ladle/" })).toBeUndefined();
    expect(canReadInContext({ kind: "vite", url: "http://localhost:5173/" })).toBe(false);
  });

  it("treats a blank URL as no host", () => {
    expect(storybookHost({ kind: "storybook", url: "   " })).toBeUndefined();
  });
});
