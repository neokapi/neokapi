import { describe, it, expect } from "vite-plus/test";
import { createBowrainRouter, rootRoute } from "./index";

// TanStack Router's internal types are complex; use loose typing for structural tests.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyRoute = any;

describe("route tree", () => {
  it("creates a router with defined routes", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    expect(router).toBeDefined();
    expect(router.routeTree).toBeDefined();
  });

  it("builds a route tree from the root", () => {
    const children = (rootRoute as AnyRoute).children as AnyRoute[];
    expect(children).toBeDefined();
    // Root should have: indexRoute, authLayout, workspaceRoute
    expect(children.length).toBe(3);
  });

  it("contains workspace child routes", () => {
    const children = (rootRoute as AnyRoute).children as AnyRoute[];
    // workspaceRoute is the third child of root
    const workspaceRoute = children[2];
    expect(workspaceRoute.path).toBe("$workspace");
    expect(workspaceRoute.children).toBeDefined();

    const childPaths = (workspaceRoute.children as AnyRoute[]).map(
      (r: AnyRoute) => r.path as string,
    );
    expect(childPaths).toContain("/");
    expect(childPaths).toContain("p/$projectId/s/$stream");
    expect(childPaths).toContain("p/$projectId/s/$stream/$itemName/translate");
    expect(childPaths).toContain("terms");
    expect(childPaths).toContain("memory");
    expect(childPaths).toContain("context");
    expect(childPaths).toContain("settings");
  });

  it("contains context hub child routes", () => {
    const children = (rootRoute as AnyRoute).children as AnyRoute[];
    const workspaceRoute = children[2];
    const contextRoute = (workspaceRoute.children as AnyRoute[]).find(
      (r: AnyRoute) => r.path === "context",
    );
    expect(contextRoute).toBeDefined();

    const childPaths = (contextRoute!.children as AnyRoute[]).map(
      (r: AnyRoute) => r.path as string,
    );
    // One path per Context sub-nav section, so a renamed nav item that lost its
    // route shows up here rather than as a dead click.
    expect(childPaths).toContain("profiles");
    expect(childPaths).toContain("concepts");
    expect(childPaths).toContain("voice");
    expect(childPaths).toContain("memory");
    expect(childPaths).toContain("changes");
    expect(childPaths).toContain("activity");
  });

  // The per-file editor surfaces address an item by its NAME — the coordinate
  // the server resolves — and names carry slashes. The router percent-encodes
  // a path param and decodes it on the way back, so the segment survives the
  // round-trip without the app encoding anything itself; doing so as well
  // would double-encode.
  it("round-trips a nested item name through the editor route", async () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    const itemName = "docs/guide/a.md";

    const href = router.buildLocation({
      to: "/$workspace/p/$projectId/s/$stream/$itemName/translate",
      params: { workspace: "acme", projectId: "proj-1", stream: "main", itemName },
    }).href;

    // One segment, not three: the slashes are encoded, so `$itemName` matches.
    expect(href).toBe("/acme/p/proj-1/s/main/docs%2Fguide%2Fa.md/translate");
    expect(href.split("/")).toHaveLength(8);

    const matches = router.matchRoutes(href, undefined);
    const leaf = matches[matches.length - 1] as AnyRoute;
    expect(leaf.routeId).toBe("/$workspace/p/$projectId/s/$stream/$itemName/translate");
    expect(leaf.params.itemName).toBe(itemName);
  });

  it("contains auth child routes", () => {
    const children = (rootRoute as AnyRoute).children as AnyRoute[];
    // authLayout is the second child of root
    const authLayout = children[1];
    expect(authLayout.children).toBeDefined();

    const childPaths = (authLayout.children as AnyRoute[]).map((r: AnyRoute) => r.path as string);
    expect(childPaths).toContain("join/$code");
    expect(childPaths).toContain("claim/$token");
    expect(childPaths).toContain("device/verify");
    expect(childPaths).toContain("device/authorized");
  });

  it("contains settings child routes", () => {
    const children = (rootRoute as AnyRoute).children as AnyRoute[];
    const workspaceRoute = children[2];
    const settingsRoute = (workspaceRoute.children as AnyRoute[]).find(
      (r: AnyRoute) => r.path === "settings",
    );
    expect(settingsRoute).toBeDefined();
    expect(settingsRoute!.children).toBeDefined();

    const childPaths = (settingsRoute!.children as AnyRoute[]).map(
      (r: AnyRoute) => r.path as string,
    );
    expect(childPaths).toContain("/");
    expect(childPaths).toContain("members");
    expect(childPaths).toContain("providers");
  });
});
