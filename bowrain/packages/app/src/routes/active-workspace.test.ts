import type { Workspace } from "@neokapi/ui";
import { describe, expect, it } from "vitest";

import { resolveActiveWorkspace } from "./active-workspace";

function ws(slug: string): Workspace {
  return {
    id: slug,
    name: slug,
    slug,
    description: "",
    logo_url: "",
    type: "personal",
    role: "owner",
  } as Workspace;
}

describe("resolveActiveWorkspace", () => {
  it("resolves the workspace the URL names", () => {
    const got = resolveActiveWorkspace([ws("acme"), ws("other")], "other");
    expect(got).toEqual({ kind: "active", workspace: ws("other") });
  });

  it("falls back to the first when the URL names one they do not have", () => {
    const got = resolveActiveWorkspace([ws("acme"), ws("other")], "gone");
    expect(got).toEqual({ kind: "fallback", workspace: ws("acme") });
  });

  // The regression. Reading workspaces[0].slug here is what blanked the app
  // (BOWRAIN-8): a member removed from their last workspace, a deleted
  // workspace, or an account that outlived a data reset all land here.
  it("reports none rather than dereferencing an empty list", () => {
    expect(resolveActiveWorkspace([], "acme")).toEqual({ kind: "none" });
  });

  it("reports none when the list never arrived", () => {
    expect(resolveActiveWorkspace(undefined, "acme")).toEqual({ kind: "none" });
  });

  it("reports none for an empty list even when no slug was asked for", () => {
    expect(resolveActiveWorkspace([], undefined)).toEqual({ kind: "none" });
  });
});
