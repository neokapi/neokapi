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
    expect(childPaths).toContain("p/$projectId/s/$stream/source");
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

  // Concepts is a list beside a detail pane, and the selected concept is the
  // child segment. Both addresses match the list route first, so moving the
  // selection keeps the page mounted with its search and scroll position.
  it("keeps the Concepts list mounted when a concept is selected", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });

    const list = router.matchRoutes("/acme/context/concepts", undefined);
    expect((list[list.length - 1] as AnyRoute).routeId).toBe("/$workspace/context/concepts");

    const one = router.matchRoutes("/acme/context/concepts/c-checkout", undefined);
    const ids = one.map((m) => (m as AnyRoute).routeId as string);
    expect(ids).toContain("/$workspace/context/concepts");
    expect(ids[ids.length - 1]).toBe("/$workspace/context/concepts/$cid");
    expect((one[one.length - 1] as AnyRoute).params.cid).toBe("c-checkout");

    const href = router.buildLocation({
      to: "/$workspace/context/concepts/$cid",
      params: { workspace: "acme", cid: "c-checkout" },
    }).href;
    expect(href).toBe("/acme/context/concepts/c-checkout");
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

  // The overview of collections and ship readiness has its own address; the
  // file-centric source view is a named sibling.
  it("renders the overview at /overview", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });

    const matches = router.matchRoutes("/acme/p/proj-1/s/main/overview", undefined);
    const leaf = matches[matches.length - 1] as AnyRoute;
    expect(leaf.routeId).toBe("/$workspace/p/$projectId/s/$stream/overview");

    const source = router.matchRoutes("/acme/p/proj-1/s/main/source", undefined);
    expect((source[source.length - 1] as AnyRoute).routeId).toBe(
      "/$workspace/p/$projectId/s/$stream/source",
    );
  });

  // The overview's drill-down is search state on its route, so a link to one
  // collection resolves to the overview rather than a path of its own.
  it("keeps the collection drill-down on the overview route", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    const href = router.buildLocation({
      to: "/$workspace/p/$projectId/s/$stream/overview",
      params: { workspace: "acme", projectId: "proj-1", stream: "main" },
      search: { collection: "docs" },
    }).href;
    expect(href).toBe("/acme/p/proj-1/s/main/overview?collection=docs");

    const matches = router.matchRoutes("/acme/p/proj-1/s/main/overview", { collection: "docs" });
    const leaf = matches[matches.length - 1] as AnyRoute;
    expect(leaf.routeId).toBe("/$workspace/p/$projectId/s/$stream/overview");
    expect(leaf.search.collection).toBe("docs");
  });

  /** The redirect a route's beforeLoad throws, for the params and search given. */
  const redirectOf = (routeId: string, search: Record<string, unknown>) => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    const route = router.routesById[routeId as keyof typeof router.routesById] as unknown as {
      options: { beforeLoad: (opts: unknown) => void };
    };
    try {
      route.options.beforeLoad({
        params: { workspace: "acme", projectId: "proj-1", stream: "main" },
        search,
      });
    } catch (err) {
      return (err as AnyRoute).options;
    }
    return undefined;
  };

  // Links and bookmarks to the overview's former address must not break, and
  // must arrive on the drill-down they named.
  it("redirects the former /dashboard address to the overview, keeping its scope", () => {
    const router = createBowrainRouter({ queryClient: undefined!, api: undefined! });
    const matches = router.matchRoutes("/acme/p/proj-1/s/main/dashboard", { collection: "docs" });
    const leaf = matches[matches.length - 1] as AnyRoute;
    expect(leaf.routeId).toBe("/$workspace/p/$projectId/s/$stream/dashboard");
    expect(leaf.search.collection).toBe("docs");

    const to = redirectOf("/$workspace/p/$projectId/s/$stream/dashboard", { collection: "docs" });
    expect(to, "the former address must redirect, not render").toBeDefined();
    expect(to.to).toBe("/$workspace/p/$projectId/s/$stream/overview");
    expect(to.search).toEqual({ collection: "docs" });
    expect(to.replace).toBe(true);
  });

  // Opening a project lands on the Map. A link to the stream root that carries
  // the overview's drill-down lands on the overview with it.
  it("redirects the stream root to the Map, or to the overview with its drill-down", () => {
    const toMap = redirectOf("/$workspace/p/$projectId/s/$stream", {});
    expect(toMap.to).toBe("/$workspace/p/$projectId/s/$stream/map");
    expect(toMap.replace).toBe(true);

    const toOverview = redirectOf("/$workspace/p/$projectId/s/$stream", { collection: "docs" });
    expect(toOverview.to).toBe("/$workspace/p/$projectId/s/$stream/overview");
    expect(toOverview.search).toEqual({ collection: "docs" });
    expect(toOverview.replace).toBe(true);
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

describe("the project screens' routes", () => {
  const router = () => createBowrainRouter({ queryClient: undefined!, api: undefined! });
  const leaf = (href: string, search?: Record<string, unknown>) => {
    const matches = router().matchRoutes(href, search);
    return matches[matches.length - 1] as AnyRoute;
  };

  it("draws the Map at /map", () => {
    expect(leaf("/acme/p/proj-1/s/main/map").routeId).toBe(
      "/$workspace/p/$projectId/s/$stream/map",
    );
  });

  it("opens the Reader on an item whose path keeps its slashes, at the block it names", () => {
    const href = router().buildLocation({
      to: "/$workspace/p/$projectId/s/$stream/read/$",
      params: { workspace: "acme", projectId: "proj-1", stream: "main", _splat: "docs/a b.md" },
      search: { block: "p2" },
    }).href;
    expect(href).toBe("/acme/p/proj-1/s/main/read/docs/a%20b.md?block=p2");
    const route = leaf("/acme/p/proj-1/s/main/read/docs/a%20b.md", { block: "p2" });
    expect(route.routeId).toBe("/$workspace/p/$projectId/s/$stream/read/$");
    expect(route.params._splat).toBe("docs/a b.md");
    expect(route.search.block).toBe("p2");
  });

  it("opens Rules on the question ?row= names", () => {
    const route = leaf("/acme/p/proj-1/s/main/rules", { row: "suggested:s-1" });
    expect(route.routeId).toBe("/$workspace/p/$projectId/s/$stream/rules");
    expect(route.search.row).toBe("suggested:s-1");
  });

  it("opens Rules on the questions assigned to the member with ?assigned=me", () => {
    expect(leaf("/acme/p/proj-1/s/main/rules", { assigned: "me" }).search.assigned).toBe("me");
    expect(leaf("/acme/p/proj-1/s/main/rules", { assigned: "all" }).search.assigned).toBe(
      undefined,
    );
  });

  it("keeps the Overview at /overview", () => {
    expect(leaf("/acme/p/proj-1/s/main/overview").routeId).toBe(
      "/$workspace/p/$projectId/s/$stream/overview",
    );
  });
});
