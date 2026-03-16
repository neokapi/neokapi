import { test, expect } from "../fixtures/test";

const SAMPLE_JSON = JSON.stringify({
  title: "Automation Test",
  description: "Testing automation rules",
});

test.describe("Automations", () => {
  let wsSlug: string;
  let projectId: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E Automations", `e2e-auto-${Date.now()}`);
    wsSlug = ws.slug;
    const project = await api.createProject(wsSlug, "Auto Project", "en", ["fr", "de"]);
    projectId = project.id;
    await api.uploadFile(wsSlug, projectId, "app.json", SAMPLE_JSON);
  });

  test("create an automation rule", async ({ api }) => {
    const rule = await api.createAutomationRule(wsSlug, projectId, {
      name: "Auto-translate on upload",
      trigger: "file.uploaded",
      conditions: [],
      actions: [{ type: "pseudo_translate", config: { target_locale: "fr" } }],
      enabled: true,
    });

    expect(rule.id).toBeTruthy();
    expect(rule.name).toBe("Auto-translate on upload");
    expect(rule.enabled).toBe(true);
  });

  test("list automation rules", async ({ api }) => {
    const rules = await api.listAutomationRules(wsSlug, projectId);
    expect(rules.length).toBeGreaterThan(0);
    expect(rules[0].name).toBeTruthy();
  });

  test("update an automation rule", async ({ api }) => {
    const rules = await api.listAutomationRules(wsSlug, projectId);
    const rule = rules[0];

    const updated = await api.updateAutomationRule(wsSlug, projectId, rule.id, {
      ...rule,
      name: "Updated rule name",
      enabled: false,
    });

    expect(updated.name).toBe("Updated rule name");
    expect(updated.enabled).toBe(false);
  });

  test("delete an automation rule", async ({ api }) => {
    const rule = await api.createAutomationRule(wsSlug, projectId, {
      name: "To be deleted",
      trigger: "file.uploaded",
      conditions: [],
      actions: [],
      enabled: false,
    });

    await api.deleteAutomationRule(wsSlug, projectId, rule.id);

    const rules = await api.listAutomationRules(wsSlug, projectId);
    const found = rules.find((r) => r.id === rule.id);
    expect(found).toBeUndefined();
  });

  test("automations page loads in the browser", async ({ api, authenticatedPage }) => {
    await authenticatedPage.goto(`/${wsSlug}/automations`);
    await expect(
      authenticatedPage.getByText(/automation/i),
    ).toBeVisible({ timeout: 15_000 });
  });
});
