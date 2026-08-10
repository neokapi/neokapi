import { describe, it, expect, vi } from "vite-plus/test";
import type { ApiAdapter } from "@neokapi/ui";
import { projectQueryOptions, projectDetailQueryOptions } from "./queries";

/**
 * Reading a project used to mean reading every item in it: the server builds
 * the embedded array with a block read per file, and every project route asked
 * for it — including the editors, which want nothing from it but the project's
 * name. These pin which shape each query asks for, and that the two never share
 * a cache entry.
 */
describe("project queries", () => {
  const adapter = () => {
    const getProject = vi.fn().mockResolvedValue({ id: "p1", name: "Proj" });
    return { getProject } as unknown as ApiAdapter & { getProject: ReturnType<typeof vi.fn> };
  };

  it("reads the summary — no embedded items — by default", async () => {
    const api = adapter();
    const opts = projectQueryOptions(api, "acme", "p1", "main");
    await opts.queryFn!({} as never);

    expect(api.getProject).toHaveBeenCalledWith("acme", "p1", "main", { view: "summary" });
  });

  it("reads the full detail only where the items are rendered", async () => {
    const api = adapter();
    const opts = projectDetailQueryOptions(api, "acme", "p1", "main");
    await opts.queryFn!({} as never);

    expect(api.getProject).toHaveBeenCalledWith("acme", "p1", "main", { view: "full" });
  });

  it("keeps the two shapes on separate cache entries under one prefix", () => {
    const api = adapter();
    const summary = projectQueryOptions(api, "acme", "p1", "main").queryKey;
    const full = projectDetailQueryOptions(api, "acme", "p1", "main").queryKey;

    expect(summary).not.toEqual(full);
    // A mutation invalidates ["project", ws, id] and must reach both.
    expect(summary.slice(0, 3)).toEqual(["project", "acme", "p1"]);
    expect(full.slice(0, 3)).toEqual(["project", "acme", "p1"]);
  });

  it("defaults an absent stream to main on both", () => {
    const api = adapter();
    expect(projectQueryOptions(api, "acme", "p1").queryKey).toEqual([
      "project",
      "acme",
      "p1",
      "main",
    ]);
    expect(projectDetailQueryOptions(api, "acme", "p1").queryKey).toEqual([
      "project",
      "acme",
      "p1",
      "main",
      "full",
    ]);
  });
});
