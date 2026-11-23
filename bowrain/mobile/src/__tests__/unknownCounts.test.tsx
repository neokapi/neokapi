// A count the app could not load is shown as not loaded, with a retry, and
// never as zero: a failed inbox must not tell a person that nothing waits.
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react-native";

import { createTestQueryClient, withTestProviders } from "./testProviders";
import { inboxKey } from "../lib/home";
import Workspaces from "../app/(tabs)/(home)/workspaces";
import Projects from "../app/(tabs)/(home)/[ws]/index";
import ProjectHome from "../app/(tabs)/(home)/[ws]/[project]/index";
import ReviewInbox from "../app/(tabs)/review/index";

const mockParams: { current: Record<string, string> } = { current: {} };
jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn() },
  Stack: { Screen: () => null },
  Redirect: () => null,
  useLocalSearchParams: () => mockParams.current,
  useFocusEffect: () => undefined,
}));

/** Every call to a client method, and the answer the test gives each method. */
const mockCalls: string[] = [];
const mockAnswers = new Map<string, () => Promise<unknown>>();
jest.mock("../lib/auth", () => {
  const client = new Proxy(
    {},
    {
      get: (_target, method: string, receiver: unknown) =>
        method === "with"
          ? () => receiver
          : () => {
              mockCalls.push(method);
              const answer = mockAnswers.get(method);
              return answer ? answer() : new Promise(() => undefined);
            },
    },
  );
  return { useAuth: () => ({ client }) };
});

const fail = () => Promise.reject(new TypeError("Network request failed"));
const project = {
  id: "p1",
  name: "Website",
  target_languages: ["nb"],
  default_source_language: "en",
};

beforeEach(() => {
  mockCalls.length = 0;
  mockAnswers.clear();
});

describe("a failed inbox", () => {
  it("leaves the workspace cards unloaded, with a retry", async () => {
    mockAnswers.set("workspaces", async () => [{ id: "w1", name: "Acme", slug: "acme" }]);
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<Workspaces />));
    expect(await screen.findByText("Not loaded")).toBeTruthy();
    expect(screen.queryByText("Nothing waiting for you")).toBeNull();
    expect(screen.getByTestId("counts-failed")).toBeTruthy();

    mockAnswers.set("reviewHome", async () => ({ context: [], languages: [], projects: [] }));
    await act(async () => fireEvent.press(screen.getByTestId("counts-retry")));
    await waitFor(() => expect(screen.getByText("Nothing waiting for you")).toBeTruthy());
    expect(screen.queryByTestId("counts-failed")).toBeNull();
    expect(mockCalls.filter((m) => m === "reviewHome").length).toBeGreaterThanOrEqual(2);
  });

  it("leaves the project cards unloaded, with a retry", async () => {
    mockParams.current = { ws: "acme" };
    mockAnswers.set("projects", async () => [project]);
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<Projects />));
    expect(await screen.findByText("Not loaded")).toBeTruthy();
    expect(screen.queryByText("Nothing waiting for you")).toBeNull();
    expect(screen.getByTestId("counts-failed")).toBeTruthy();
  });

  it("leaves a project's language cards unloaded, with a retry", async () => {
    mockParams.current = { ws: "acme", project: "p1" };
    mockAnswers.set("projects", async () => [project]);
    mockAnswers.set("permissions", async () => ({ permissions: ["review"], languages: [] }));
    mockAnswers.set("reviewPlan", async () => ({ next: null }));
    mockAnswers.set("feed", async () => ({ entries: [] }));
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<ProjectHome />));
    // The language card and the context row both say so.
    expect(await screen.findAllByText("Not loaded")).toHaveLength(2);
    expect(screen.queryByText("Nothing waiting for you")).toBeNull();
    expect(await screen.findByTestId("counts-failed")).toBeTruthy();
  });

  it("shows the review tab's load failure with a retry, not that the person is caught up", async () => {
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<ReviewInbox />));
    expect(await screen.findByText("Could not load this. Check your connection.")).toBeTruthy();
    expect(screen.getByText("Try again")).toBeTruthy();
    expect(screen.queryByText("All caught up")).toBeNull();
  });

  it("marks the project badges as unknown, not as nothing waiting", async () => {
    mockParams.current = { ws: "acme" };
    mockAnswers.set("projects", async () => [project]);
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<Projects />));
    expect(
      await screen.findByTestId("badge-nb-unknown", { includeHiddenElements: true }),
    ).toBeTruthy();
  });
});

describe("counts whose refresh failed", () => {
  it("are kept and marked as possibly out of date, with a retry", async () => {
    const qc = createTestQueryClient();
    qc.setQueryData(inboxKey, { context: [], projects: [], languages: [] });
    mockAnswers.set("workspaces", async () => [{ id: "w1", name: "Acme", slug: "acme" }]);
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<Workspaces />, qc));
    expect(await screen.findByTestId("counts-stale")).toBeTruthy();
    expect(screen.getByText("Nothing waiting for you")).toBeTruthy();
  });
});

describe("what a list has shown", () => {
  const second = { ...project, id: "p2", name: "Docs" };
  const waitsInP2 = {
    context: [],
    projects: [],
    languages: [
      {
        workspace: "acme",
        workspace_name: "Acme",
        project: "p2",
        project_name: "Docs",
        stream: "main",
        locale: "nb",
        waiting: 4,
        fast: 4,
        closer: 0,
        desk: 0,
        unclassified: 0,
      },
    ],
  };

  it("keeps its order when a retried inbox would re-sort it", async () => {
    mockParams.current = { ws: "acme" };
    mockAnswers.set("projects", async () => [project, second]);
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<Projects />));
    await screen.findByTestId("counts-failed");
    const order = () => screen.getAllByTestId(/^project-p\d$/).map((n) => n.props.testID);
    expect(order()).toEqual(["project-p1", "project-p2"]);

    // The retry says p2 has work waiting, which would sort it first.
    mockAnswers.set("reviewHome", async () => waitsInP2);
    await act(async () => fireEvent.press(screen.getByTestId("counts-retry")));
    await waitFor(() => expect(screen.getByText("4 translations in 1 language")).toBeTruthy());
    expect(order()).toEqual(["project-p1", "project-p2"]);
  });

  it("adds no next step above the workspaces after a failed first answer", async () => {
    mockAnswers.set("workspaces", async () => [{ id: "w1", name: "Acme", slug: "acme" }]);
    mockAnswers.set("reviewHome", fail);
    await render(withTestProviders(<Workspaces />));
    await screen.findByTestId("counts-failed");
    mockAnswers.set("reviewHome", async () => waitsInP2);
    await act(async () => fireEvent.press(screen.getByTestId("counts-retry")));
    await waitFor(() => expect(screen.getByText("4 translations")).toBeTruthy());
    expect(screen.queryByTestId("home-next")).toBeNull();
  });
});
