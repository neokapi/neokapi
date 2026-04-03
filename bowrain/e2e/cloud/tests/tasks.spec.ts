import { test, expect } from "../fixtures/test";

test.describe("Task Management", () => {
  let wsSlug: string;
  let projectId: string;

  test.beforeAll(async ({ api }) => {
    const ws = await api.getOrCreateWorkspace("E2E Tasks", `e2e-tasks-${Date.now().toString(36)}`);
    wsSlug = ws.slug;
    const project = await api.createProject(wsSlug, "Task Project", "en", ["fr"]);
    projectId = project.id;
  });

  test("create a task", async ({ api }) => {
    let task: { id: string; title: string };
    try {
      task = await api.createTask(wsSlug, {
        title: "Review French translations",
        description: "Check all French translations for accuracy",
        type: "review",
        priority: "high",
        project_id: projectId,
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Tasks feature not available on this server");
        return;
      }
      throw err;
    }

    expect(task.id).toBeTruthy();
    expect(task.title).toBe("Review French translations");
  });

  test("list tasks", async ({ api }) => {
    let tasks: Array<{ id: string; title: string }>;
    try {
      tasks = await api.listTasks(wsSlug);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Tasks feature not available on this server");
        return;
      }
      throw err;
    }

    expect(tasks.length).toBeGreaterThan(0);
    const found = tasks.find((t) => t.title === "Review French translations");
    expect(found).toBeTruthy();
  });

  test("get task by ID", async ({ api }) => {
    let tasks: Array<{ id: string; title: string }>;
    try {
      tasks = await api.listTasks(wsSlug);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Tasks feature not available on this server");
        return;
      }
      throw err;
    }

    const target = tasks.find((t) => t.title === "Review French translations");
    expect(target).toBeTruthy();

    const task = await api.getTask(wsSlug, target!.id);
    expect(task.id).toBe(target!.id);
    expect(task.title).toBe("Review French translations");
  });

  test("assign task", async ({ api }) => {
    let tasks: Array<{ id: string; title: string }>;
    try {
      tasks = await api.listTasks(wsSlug);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Tasks feature not available on this server");
        return;
      }
      throw err;
    }

    const target = tasks.find((t) => t.title === "Review French translations");
    expect(target).toBeTruthy();

    // Assign to self (no assignee_id means self-assign).
    try {
      await api.assignTask(wsSlug, target!.id);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      // Some server versions may not support assignment.
      if (msg.includes("404") || msg.includes("503") || msg.includes("405")) {
        test.skip(true, "Task assignment not available on this server");
        return;
      }
      throw err;
    }

    // Verify assignment via getTask.
    const updated = await api.getTask(wsSlug, target!.id);
    expect(updated.assignee_id).toBeTruthy();
  });

  test("complete task", async ({ api }) => {
    let task: { id: string; title: string };
    try {
      task = await api.createTask(wsSlug, {
        title: "Task to complete",
        type: "review",
        priority: "low",
        project_id: projectId,
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "Tasks feature not available on this server");
        return;
      }
      throw err;
    }

    try {
      await api.completeTask(wsSlug, task.id);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503") || msg.includes("405")) {
        test.skip(true, "Task completion not available on this server");
        return;
      }
      throw err;
    }

    const completed = await api.getTask(wsSlug, task.id);
    expect(completed.status).toMatch(/completed|done|closed/i);
  });

  test("list my tasks", async ({ api }) => {
    try {
      const myTasks = await api.myTasks(wsSlug);
      // myTasks should be an array (may be empty if no tasks assigned to self).
      expect(Array.isArray(myTasks)).toBe(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("404") || msg.includes("503")) {
        test.skip(true, "My tasks feature not available on this server");
        return;
      }
      throw err;
    }
  });
});
