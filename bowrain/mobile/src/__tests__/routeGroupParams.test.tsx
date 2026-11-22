// A Home screen reached with a route group's name as its workspace or project
// waits for a real one: it builds no request from the name and leads back to
// the workspace list. Right after sign-in the app once asked the server for
// GET /api/v1/(tabs)/projects (#3652).
import { render, waitFor } from "@testing-library/react-native";

import { withTestProviders } from "./testProviders";
import Projects from "../app/(tabs)/(home)/[ws]/index";
import ProjectHome from "../app/(tabs)/(home)/[ws]/[project]/index";
import { routeValue } from "../lib/routeParams";

const mockParams: { current: Record<string, string> } = { current: {} };
const mockRedirect = jest.fn<null, [{ href: string }]>(() => null);

jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn() },
  Stack: { Screen: () => null },
  Redirect: (props: { href: string }) => mockRedirect(props),
  useLocalSearchParams: () => mockParams.current,
  useFocusEffect: () => undefined,
}));

// Every client call is recorded and left pending, so the screens render their
// loading state and the test reads which requests they built.
const mockCalls: { method: string; args: unknown[] }[] = [];
jest.mock("../lib/auth", () => {
  const client = new Proxy(
    {},
    {
      get:
        (_target, method: string) =>
        (...args: unknown[]) => {
          mockCalls.push({ method, args });
          return new Promise(() => undefined);
        },
    },
  );
  return { useAuth: () => ({ client }) };
});

const groupArgs = () =>
  mockCalls.filter((c) => c.args.some((a) => typeof a === "string" && /^\(.*\)$/.test(a)));

beforeEach(() => {
  mockCalls.length = 0;
  mockRedirect.mockClear();
});

describe("routeValue", () => {
  it("reads a group name, an empty value or a list as no value", () => {
    expect(routeValue("(tabs)")).toBeUndefined();
    expect(routeValue("(home)")).toBeUndefined();
    expect(routeValue("")).toBeUndefined();
    expect(routeValue(undefined)).toBeUndefined();
    expect(routeValue(["acme"])).toBeUndefined();
    expect(routeValue("acme")).toBe("acme");
    expect(routeValue("Ncew8DW5")).toBe("Ncew8DW5");
  });
});

describe("the projects screen", () => {
  it("builds no request from a group name and leads to the workspaces", async () => {
    mockParams.current = { ws: "(tabs)" };
    await render(withTestProviders(<Projects />));
    expect(groupArgs()).toEqual([]);
    expect(mockCalls.some((c) => c.method === "projects")).toBe(false);
    expect(mockRedirect).toHaveBeenCalledWith({ href: "/workspaces" });
  });

  it("asks for the projects of a workspace", async () => {
    mockParams.current = { ws: "acme" };
    await render(withTestProviders(<Projects />));
    await waitFor(() => expect(mockCalls).toContainEqual({ method: "projects", args: ["acme"] }));
    expect(mockRedirect).not.toHaveBeenCalled();
  });
});

describe("the languages screen", () => {
  it("builds no request from a group name and leads to the workspaces", async () => {
    mockParams.current = { ws: "(tabs)", project: "(home)" };
    await render(withTestProviders(<ProjectHome />));
    expect(groupArgs()).toEqual([]);
    expect(mockRedirect).toHaveBeenCalledWith({ href: "/workspaces" });
  });

  it("asks for the permissions in a project", async () => {
    mockParams.current = { ws: "acme", project: "p1" };
    await render(withTestProviders(<ProjectHome />));
    await waitFor(() =>
      expect(mockCalls).toContainEqual({ method: "permissions", args: ["acme", "p1"] }),
    );
    expect(mockRedirect).not.toHaveBeenCalled();
  });
});
