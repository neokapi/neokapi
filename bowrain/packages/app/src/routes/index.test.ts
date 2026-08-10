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
    expect(childPaths).toContain("p/$projectId/s/$stream/translate/$");
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
  // the server resolves — and names carry slashes. The name is therefore the
  // route's trailing SPLAT, carried as real path segments: a percent-encoded
  // `%2F` inside a single `$itemName` segment matched in dev but not behind the
  // production CDN, so nothing may depend on `%2F` surviving the round trip.
  it("round-trips a nested item name through the editor route", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    const itemName = "apps/kapi-desktop/frontend/category.kbf.json";

    const href = router.buildLocation({
      to: "/$workspace/p/$projectId/s/$stream/translate/$",
      params: { workspace: "acme", projectId: "proj-1", stream: "main", _splat: itemName },
    }).href;

    // Real slashes, no percent-encoding: the name spans as many segments as it
    // has parts, and the surface segment leads so the splat can trail.
    expect(href).toBe(
      "/acme/p/proj-1/s/main/translate/apps/kapi-desktop/frontend/category.kbf.json",
    );
    expect(href).not.toContain("%2F");

    const matches = router.matchRoutes(href, undefined);
    const leaf = matches[matches.length - 1] as AnyRoute;
    expect(leaf.routeId).toBe("/$workspace/p/$projectId/s/$stream/translate/$");
    expect(leaf.params._splat).toBe(itemName);
  });

  it("matches a flat item name on each editor surface", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    for (const surface of ["translate", "review", "pre-process"]) {
      const matches = router.matchRoutes(
        `/acme/p/proj-1/s/main/${surface}/about-us.html`,
        undefined,
      );
      const leaf = matches[matches.length - 1] as AnyRoute;
      expect(leaf.routeId).toBe(`/$workspace/p/$projectId/s/$stream/${surface}/$`);
      expect(leaf.params._splat).toBe("about-us.html");
    }
  });

  // `review` is two routes: the project-level session at the bare path, and the
  // per-file surface that trails a name. The exact path has to outrank the
  // splat, or opening the review session lands in an editor for no file.
  it("keeps the project-level review session ahead of the per-file splat", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });

    const session = router.matchRoutes("/acme/p/proj-1/s/main/review", undefined);
    expect((session[session.length - 1] as AnyRoute).routeId).toBe(
      "/$workspace/p/$projectId/s/$stream/review",
    );

    const perFile = router.matchRoutes("/acme/p/proj-1/s/main/review/docs/a.md", undefined);
    const leaf = perFile[perFile.length - 1] as AnyRoute;
    expect(leaf.routeId).toBe("/$workspace/p/$projectId/s/$stream/review/$");
    expect(leaf.params._splat).toBe("docs/a.md");
  });

  // A name whose segments need escaping (a space, a hash) still round-trips:
  // each segment is encoded on its own, and the slashes between them are not.
  it("encodes within segments but never the separators", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    const itemName = "docs/release notes/v1#2.md";

    const href = router.buildLocation({
      to: "/$workspace/p/$projectId/s/$stream/translate/$",
      params: { workspace: "acme", projectId: "proj-1", stream: "main", _splat: itemName },
    }).href;

    expect(href).not.toContain("%2F");
    const matches = router.matchRoutes(href, undefined);
    expect((matches[matches.length - 1] as AnyRoute).params._splat).toBe(itemName);
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
