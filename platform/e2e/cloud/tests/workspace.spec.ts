import { test, expect } from "../fixtures/test";

test.describe("Workspace CRUD", () => {
  const slug = `e2e-ws-${Date.now()}`;

  test("create a workspace", async ({ api }) => {
    const ws = await api.createWorkspace("E2E Test Workspace", slug);
    expect(ws.slug).toBe(slug);
    expect(ws.name).toBe("E2E Test Workspace");
    expect(ws.id).toBeTruthy();
  });

  test("get workspace by slug", async ({ api }) => {
    const ws = await api.getWorkspace(slug);
    expect(ws.slug).toBe(slug);
  });

  test("list workspaces includes the new workspace", async ({ api }) => {
    const workspaces = await api.listWorkspaces();
    const found = workspaces.find((w) => w.slug === slug);
    expect(found).toBeTruthy();
  });

  test("workspace appears in the UI", async ({ authenticatedPage }) => {
    await authenticatedPage.goto(`/${slug}`);
    await expect(
      authenticatedPage.getByText("E2E Test Workspace").or(
        authenticatedPage.getByTestId("nav-translate"),
      ),
    ).toBeVisible({ timeout: 15_000 });
  });
});
