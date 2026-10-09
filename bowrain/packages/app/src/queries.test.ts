import { describe, it, expect, vi } from "vite-plus/test";
import { QueryClient } from "@tanstack/react-query";
import type { ApiAdapter } from "@neokapi/ui";
import {
  invalidateProjectQueries,
  projectQueryOptions,
  projectDetailQueryOptions,
  projectsQueryOptions,
} from "./queries";

/**
 * The workspace home cards and the Files stat read each project's item and
 * word counts from the projects list, not from the project's own entry. A
 * mutation that refreshed only the project left the cards on the previous
 * counts until the list's stale time ran out (#746).
 */
describe("invalidateProjectQueries", () => {
  const seeded = async () => {
    const api = {
      getProject: vi.fn().mockResolvedValue({ id: "p1", name: "Proj", item_count: 0 }),
      listProjects: vi.fn().mockResolvedValue([{ id: "p1", name: "Proj", item_count: 0 }]),
    } as unknown as ApiAdapter;
    const client = new QueryClient();
    await client.fetchQuery(projectQueryOptions(api, "acme", "p1", "main"));
    await client.fetchQuery(projectDetailQueryOptions(api, "acme", "p1", "main"));
    await client.fetchQuery(projectsQueryOptions(api, "acme"));
    await client.fetchQuery(projectsQueryOptions(api, "other"));
    return client;
  };

  it("marks the project's entries and the workspace's project list stale", async () => {
    const client = await seeded();

    invalidateProjectQueries(client, "acme", "p1");

    expect(client.getQueryState(["project", "acme", "p1", "main"])?.isInvalidated).toBe(true);
    expect(client.getQueryState(["project", "acme", "p1", "main", "full"])?.isInvalidated).toBe(
      true,
    );
    expect(client.getQueryState(["projects", "acme"])?.isInvalidated).toBe(true);
  });

  it("leaves another workspace's project list alone", async () => {
    const client = await seeded();

    invalidateProjectQueries(client, "acme", "p1");

    expect(client.getQueryState(["projects", "other"])?.isInvalidated).toBe(false);
  });
});

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
