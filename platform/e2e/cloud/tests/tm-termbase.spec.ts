import { test, expect } from "../fixtures/test";

test.describe("Translation Memory & Terminology", () => {
  let wsSlug: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E TM", `e2e-tm-${Date.now()}`);
    wsSlug = ws.slug;
  });

  test("add and search TM entries", async ({ api }) => {
    await api.addTMEntry(wsSlug, "Hello", "Bonjour", "en", "fr");
    await api.addTMEntry(wsSlug, "Goodbye", "Au revoir", "en", "fr");
    await api.addTMEntry(wsSlug, "Hello", "Hallo", "en", "de");

    const results = await api.searchTM(wsSlug, "Hello");
    expect(results.length).toBeGreaterThan(0);
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

    const results = await api.searchTerms(wsSlug, "component");
    expect(results.length).toBeGreaterThan(0);
  });
});
