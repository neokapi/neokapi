import { test, expect } from "../fixtures/test";

const SAMPLE_JSON = JSON.stringify({
  greeting: "Hello, world!",
  farewell: "Goodbye!",
  menu: {
    file: "File",
    edit: "Edit",
    view: "View",
  },
});

const SAMPLE_HTML = `<!DOCTYPE html>
<html lang="en">
<head><title>E2E Test Page</title></head>
<body>
  <h1>Welcome</h1>
  <p>This is a test document for end-to-end testing.</p>
  <p>It contains <strong>inline markup</strong> and <a href="#">links</a>.</p>
</body>
</html>`;

test.describe("Project & Translation", () => {
  let wsSlug: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E Projects", `e2e-proj-${Date.now()}`);
    wsSlug = ws.slug;
  });

  test("create project with source and target locales", async ({ api }) => {
    const project = await api.createProject(wsSlug, "JSON App", "en", ["fr", "de"]);
    expect(project.id).toBeTruthy();
    expect(project.name).toBe("JSON App");
  });

  test("upload JSON file and verify project items", async ({ api }) => {
    const project = await api.createProject(wsSlug, "Upload Test", "en", ["fr"]);
    await api.uploadFile(wsSlug, project.id, "strings.json", SAMPLE_JSON);

    const fetched = await api.getProject(wsSlug, project.id);
    expect(fetched.items).toBeTruthy();
    expect(fetched.items!.length).toBeGreaterThan(0);
    const item = fetched.items!.find((i) => i.name === "strings.json");
    expect(item).toBeTruthy();
  });

  test("upload HTML file", async ({ api }) => {
    const project = await api.createProject(wsSlug, "HTML Test", "en", ["fr"]);
    await api.uploadFile(wsSlug, project.id, "page.html", SAMPLE_HTML);

    const fetched = await api.getProject(wsSlug, project.id);
    const item = fetched.items!.find((i) => i.name === "page.html");
    expect(item).toBeTruthy();
  });

  test("pseudo-translate a file", async ({ api }) => {
    const project = await api.createProject(wsSlug, "Pseudo Test", "en", ["fr"]);
    await api.uploadFile(wsSlug, project.id, "strings.json", SAMPLE_JSON);

    const result = await api.pseudoTranslate(wsSlug, project.id, "strings.json", "fr");
    expect(result.total_blocks).toBeGreaterThan(0);
    expect(result.translated_blocks).toBe(result.total_blocks);
  });

  test("list and delete projects", async ({ api }) => {
    const p1 = await api.createProject(wsSlug, "To Delete 1", "en", ["fr"]);
    const p2 = await api.createProject(wsSlug, "To Delete 2", "en", ["de"]);

    let projects = await api.listProjects(wsSlug);
    const ids = projects.map((p) => p.id);
    expect(ids).toContain(p1.id);
    expect(ids).toContain(p2.id);

    await api.deleteProject(wsSlug, p1.id);
    await api.deleteProject(wsSlug, p2.id);

    projects = await api.listProjects(wsSlug);
    const remainingIds = projects.map((p) => p.id);
    expect(remainingIds).not.toContain(p1.id);
    expect(remainingIds).not.toContain(p2.id);
  });

  test("editor loads project in the browser", async ({ api, authenticatedPage }) => {
    const project = await api.createProject(wsSlug, "Browser Test", "en", ["fr"]);
    await api.uploadFile(wsSlug, project.id, "strings.json", SAMPLE_JSON);

    // Navigate to the project's translate view.
    await authenticatedPage.goto(`/${wsSlug}`);
    await expect(authenticatedPage.getByTestId("nav-translate")).toBeVisible({ timeout: 15_000 });
    await authenticatedPage.getByTestId("nav-translate").click();
    // Should see the project name or files listed.
    await expect(
      authenticatedPage.getByText("Browser Test"),
    ).toBeVisible({ timeout: 15_000 });
  });
});
