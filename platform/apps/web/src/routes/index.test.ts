import { describe, it, expect } from "vitest";
import { router, rootRoute } from "./index";

// TanStack Router's internal types are complex; use loose typing for structural tests.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyRoute = any;

describe("route tree", () => {
  it("creates a router with defined routes", () => {
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
    expect(childPaths).toContain("project/$projectId/stream/$stream");
    expect(childPaths).toContain("project/$projectId/stream/$stream/translate/$fileName");
    expect(childPaths).toContain("termbase");
    expect(childPaths).toContain("memory");
    expect(childPaths).toContain("settings");
  });

  it("contains auth child routes", () => {
    const children = (rootRoute as AnyRoute).children as AnyRoute[];
    // authLayout is the second child of root
    const authLayout = children[1];
    expect(authLayout.children).toBeDefined();

    const childPaths = (authLayout.children as AnyRoute[]).map(
      (r: AnyRoute) => r.path as string,
    );
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
