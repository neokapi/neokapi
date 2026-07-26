import { test, expect } from "../fixtures/test";

test.describe("Content Memory & Terminology", () => {
  let wsSlug: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E content memory", `e2e-tm-${Date.now()}`);
    wsSlug = ws.slug;
  });

  test("add and search content-memory entries", async ({ api }) => {
    await api.addMemoryEntry(wsSlug, "Hello", "Bonjour", "en", "fr");
    await api.addMemoryEntry(wsSlug, "Goodbye", "Au revoir", "en", "fr");
    await api.addMemoryEntry(wsSlug, "Hello", "Hallo", "en", "de");

    const results = (await api.searchMemory(wsSlug, "Hello")) as any;
    // Response may be an array or { entries: [...], total: N }
    const entries = Array.isArray(results) ? results : (results.entries ?? results.results ?? []);
    expect(entries.length).toBeGreaterThan(0);
  });

  test("add and search terminology concepts", async ({ api }) => {
    await api.addConcept(wsSlug, {
      domain: "software",
      definition: "A reusable software element",
      terms: [
        { text: "component", locale: "en", status: "preferred" },
        { text: "composant", locale: "fr", status: "preferred" },
        { text: "Komponente", locale: "de", status: "preferred" },
      ],
    });

    await api.addConcept(wsSlug, {
      domain: "software",
      definition: "A visual interface element",
      terms: [
        { text: "widget", locale: "en", status: "preferred" },
        { text: "widget", locale: "fr", status: "admitted" },
      ],
    });

    // Verify the add succeeded (no error thrown) and search returns a response.
    const results = await api.searchTerms(wsSlug, "component");
    expect(results).toBeTruthy();
  });
});
