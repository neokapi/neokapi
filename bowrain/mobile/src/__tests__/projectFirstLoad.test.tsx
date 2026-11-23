// A project's screen fills each section in place as its request answers. The
// header, the plan card, the context row and the language cards hold their
// place while they load; the activity strip, which shows only when there is
// activity, comes below the languages once the sections above it have
// answered.
import { act, render, screen, waitFor } from "@testing-library/react-native";

import { withTestProviders } from "./testProviders";
import ProjectHome from "../app/(tabs)/(home)/[ws]/[project]/index";

jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn() },
  Stack: { Screen: () => null },
  Redirect: () => null,
  useLocalSearchParams: () => ({ ws: "acme", project: "p1" }),
  useFocusEffect: () => undefined,
}));

/** One pending answer per client method, settled by the test. */
const mockPending = new Map<
  string,
  { resolve: (v: unknown) => void; reject: (e: unknown) => void }
>();
jest.mock("../lib/auth", () => {
  const client = new Proxy(
    {},
    {
      get: (_target, method: string) => () =>
        new Promise((resolve, reject) => {
          mockPending.set(method, { resolve, reject });
        }),
    },
  );
  return { useAuth: () => ({ client }) };
});

async function answer(method: string, value: unknown) {
  await waitFor(() => expect(mockPending.has(method)).toBe(true));
  await act(async () => mockPending.get(method)!.resolve(value));
}

async function fail(method: string) {
  await waitFor(() => expect(mockPending.has(method)).toBe(true));
  await act(async () => mockPending.get(method)!.reject(new Error("forbidden")));
}

const project = {
  id: "p1",
  name: "Website",
  target_languages: ["nb", "de"],
  default_source_language: "en",
  block_count: 12,
};
const inbox = {
  projects: [],
  languages: [],
  context: [
    {
      workspace: "acme",
      workspace_name: "ACME",
      project: "p1",
      project_name: "Website",
      waiting: 3,
      by_kind: { note: 3 },
    },
  ],
};
const feedEntry = {
  id: "e1",
  kind: "push",
  actor: "u1",
  actor_name: "Kari",
  occurred_at: new Date().toISOString(),
  summary: "pushed 3 files",
};

beforeEach(() => mockPending.clear());

describe("a project's first load", () => {
  it("holds the header, the plan card and the language cards while they load", async () => {
    await render(withTestProviders(<ProjectHome />));
    expect(
      screen.getByTestId("project-header-loading", { includeHiddenElements: true }),
    ).toBeTruthy();
    expect(screen.getByTestId("project-plan")).toHaveTextContent(/Loading/);

    await answer("projects", [project]);
    expect(await screen.findByTestId("project-header")).toBeTruthy();
    // The permissions have not answered: the project's languages hold the place.
    expect(screen.getByTestId("language-nb")).toBeTruthy();
    expect(screen.getByTestId("language-de")).toBeTruthy();
    // The context row is there from the start, loading in one line.
    expect(screen.getByTestId("project-context")).toHaveTextContent(/Loading/);

    await answer("reviewPlan", { next: null });
    await waitFor(() =>
      expect(screen.getByTestId("project-plan")).toHaveTextContent(/Nothing waits for you/),
    );
  });

  it("says in the plan card when the plan failed to load", async () => {
    await render(withTestProviders(<ProjectHome />));
    await fail("reviewPlan");
    await waitFor(() =>
      expect(screen.getByTestId("project-plan")).toHaveTextContent(/Could not load this/),
    );
  });

  it("fills the context row in place with what waits", async () => {
    await render(withTestProviders(<ProjectHome />));
    await answer("projects", [project]);
    await answer("reviewHome", inbox);
    await waitFor(() =>
      expect(screen.getByTestId("project-context")).toHaveTextContent(/Context: 3 to review/),
    );
    await answer("feed", { entries: [feedEntry] });
    await answer("permissions", { permissions: ["review"], languages: ["nb"] });
    await waitFor(() => expect(screen.queryByTestId("language-de")).toBeNull());
  });

  it("says the context is up to date when nothing waits there", async () => {
    await render(withTestProviders(<ProjectHome />));
    await answer("reviewHome", { projects: [], languages: [], context: [] });
    await waitFor(() =>
      expect(screen.getByTestId("project-context")).toHaveTextContent(/Up to date/),
    );
  });
});
