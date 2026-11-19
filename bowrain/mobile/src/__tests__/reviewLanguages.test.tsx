import {
  inboxProjectFromServer,
  planFromServer,
  reviewLanguagesFromServer,
  type ReviewHome,
  type ReviewLanguages,
} from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import { router } from "expo-router";
import type { ReactNode } from "react";
import { Alert } from "react-native";

import i18n from "../lib/i18n";
import ReviewInbox from "../app/(tabs)/review/index";
import ReviewLanguagesScreen from "../app/[ws]/[project]/languages";
import { followedChipLabel } from "../lib/languages";

const mockParams: { current: Record<string, string> } = { current: {} };
jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn() },
  Stack: { Screen: () => null },
  useLocalSearchParams: () => mockParams.current,
  useFocusEffect: () => undefined,
}));

const mockHome = jest.fn<Promise<ReviewHome>, unknown[]>();
const mockLanguages = jest.fn<Promise<ReviewLanguages>, unknown[]>();
const mockSetFollowed = jest.fn<Promise<ReviewLanguages>, unknown[]>();
const mockSetProject = jest.fn<Promise<ReviewLanguages>, unknown[]>();
const mockPlan = jest.fn();
const mockInvolve = jest.fn();
jest.mock("../lib/auth", () => ({
  useAuth: () => ({
    client: {
      reviewHome: (...a: unknown[]) => mockHome(...a),
      reviewLanguages: (...a: unknown[]) => mockLanguages(...a),
      setFollowedLanguages: (...a: unknown[]) => mockSetFollowed(...a),
      setProjectLanguage: (...a: unknown[]) => mockSetProject(...a),
      reviewPlan: (...a: unknown[]) => mockPlan(...a),
      setInvolvement: (...a: unknown[]) => mockInvolve(...a),
    },
  }),
}));

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const project = (saved: boolean) =>
  inboxProjectFromServer({
    workspace: "ws",
    workspace_name: "WS",
    project: "p",
    project_name: "neokapi",
    followed: ["nb"],
    saved,
    can_manage: true,
    waiting: 40,
    plan: {
      involvement: "context",
      next: "key_language",
      steps: [
        { level: "coordinates", state: "done", count: 0, progress: 100, done: true },
        { level: "key_language", state: "not_started", count: 0, progress: 0 },
        { level: "exceptions", state: "open", count: 3, progress: 50 },
        { level: "interfaces", state: "open", count: 34, progress: 0 },
        { level: "policy", state: "open", count: 800, progress: 0, settle: "held" },
      ],
      held: [{ locale: "nb", count: 1200, reason: "key_language" }],
    },
  });

const languages = (over: Partial<Parameters<typeof reviewLanguagesFromServer>[0]> = {}) =>
  reviewLanguagesFromServer({
    followed: ["nb", "sv"],
    saved: false,
    allowed: ["nb", "sv"],
    source: "en",
    project: ["nb", "sv"],
    can_manage: false,
    ...over,
  });

describe("the Review tab plan card", () => {
  beforeEach(() => jest.clearAllMocks());

  it("shows one next step, the later steps and the held translations", async () => {
    mockHome.mockResolvedValue({ context: [], languages: [], projects: [project(true)] });
    await render(wrap(<ReviewInbox />));
    expect(await screen.findByTestId("plan-card-p")).toBeTruthy();
    expect(screen.getByText("Step 2 of 5")).toBeTruthy();
    expect(screen.getByText("Decide the key language")).toBeTruthy();
    expect(screen.getByText("Not started: scan for terms")).toBeTruthy();
    expect(screen.queryByTestId("plan-card-p-coordinates")).toBeNull();
    expect(screen.getByLabelText("Exceptions, 3")).toBeTruthy();
    expect(screen.getByLabelText("Pages to read, 34")).toBeTruthy();
    expect(screen.getByLabelText("Settle the rest, Waits")).toBeTruthy();
    expect(
      screen.getByText(
        "1,200 Norwegian Bokmål translations are held until the key language is decided.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("plan-card-p-setup")).toBeNull();

    await fireEvent.press(screen.getByTestId("plan-card-p-next"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/plan",
      params: { ws: "ws", project: "p", step: "key_language" },
    });
    await fireEvent.press(screen.getByTestId("plan-card-p-exceptions"));
    expect(router.push).toHaveBeenLastCalledWith({
      pathname: "/[ws]/[project]/plan",
      params: { ws: "ws", project: "p", step: "exceptions" },
    });
  });

  it("opens the map when the coordinates changed after the team confirmed them", async () => {
    const p = project(true);
    p.plan!.next = "coordinates";
    p.plan!.steps[0] = {
      level: "coordinates",
      state: "attention",
      count: 1,
      progress: 100,
      done: false,
      changed: 1,
    };
    mockHome.mockResolvedValue({ context: [], languages: [], projects: [p] });
    await render(wrap(<ReviewInbox />));
    expect(await screen.findByText("Step 1 of 5")).toBeTruthy();
    expect(screen.getByText("1 change since the last confirmation")).toBeTruthy();
    expect(screen.getByLabelText("Key language, Not started")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-card-p-next"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/coordinates",
      params: { ws: "ws", project: "p", languages: "nb" },
    });
  });

  it("opens the setup first while no languages are saved", async () => {
    mockHome.mockResolvedValue({ context: [], languages: [], projects: [project(false)] });
    await render(wrap(<ReviewInbox />));
    await fireEvent.press(await screen.findByTestId("plan-card-p-interfaces"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/languages",
      params: { ws: "ws", project: "p", setup: "1", next: "interfaces" },
    });
    await fireEvent.press(screen.getByTestId("plan-card-p-setup"));
    expect(router.push).toHaveBeenLastCalledWith({
      pathname: "/[ws]/[project]/languages",
      params: { ws: "ws", project: "p", setup: "1" },
    });
  });
});

describe("the languages screen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams.current = { ws: "ws", project: "p" };
  });

  it("lets a manager set how involved the reviewers are", async () => {
    mockLanguages.mockResolvedValue(languages({ can_manage: true }));
    mockPlan.mockResolvedValue(
      planFromServer({
        stream: "main",
        involvement: "context",
        next: "",
        coordinates: { source_language: "en", confirmed: true },
        key_language: { waiting: 0, concepts: 0 },
        exceptions: {},
      }),
    );
    mockInvolve.mockResolvedValue(undefined);
    await render(wrap(<ReviewLanguagesScreen />));
    await fireEvent.press(await screen.findByTestId("follow-involvement-every"));
    expect(mockInvolve).toHaveBeenCalledWith({ workspace: "ws", project: "p" }, "every");
  });

  it("shows a reviewer only their languages and saves the choice", async () => {
    mockLanguages.mockResolvedValue(languages());
    mockSetFollowed.mockResolvedValue(languages({ followed: ["nb"], saved: true }));
    await render(wrap(<ReviewLanguagesScreen />));
    expect(await screen.findByTestId("follow-mine-sv")).toBeTruthy();
    expect(screen.queryByTestId("follow-project-nb")).toBeNull();
    expect(screen.queryByTestId("follow-coordinates")).toBeNull();
    expect(mockPlan).not.toHaveBeenCalled();

    await fireEvent.press(screen.getByTestId("follow-mine-sv"));
    await fireEvent.press(screen.getByTestId("follow-save"));
    expect(mockSetFollowed).toHaveBeenCalledWith("ws", "p", ["nb"]);
    expect(router.back).toHaveBeenCalled();
  });

  it("goes on to the step asked for after the setup", async () => {
    mockParams.current = { ws: "ws", project: "p", setup: "1", next: "exceptions" };
    mockLanguages.mockResolvedValue(languages());
    mockSetFollowed.mockResolvedValue(languages({ saved: true }));
    await render(wrap(<ReviewLanguagesScreen />));
    await fireEvent.press(await screen.findByTestId("follow-save"));
    expect(router.replace).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/plan",
      params: { ws: "ws", project: "p", step: "exceptions" },
    });
  });

  it("lets a manager turn a project language on as a recipe change", async () => {
    mockLanguages.mockResolvedValue(languages({ can_manage: true, pending_project: ["nb"] }));
    mockPlan.mockReturnValue(new Promise(() => undefined));
    mockSetProject.mockResolvedValue(languages({ can_manage: true }));
    jest.spyOn(Alert, "alert").mockImplementation((_t, _m, buttons) => {
      buttons?.find((b) => b.style !== "cancel")?.onPress?.();
    });
    await render(wrap(<ReviewLanguagesScreen />));
    const sv = await screen.findByTestId("follow-project-sv");
    expect(sv.props.value).toBe(false);
    expect(screen.getByText("After the next sync")).toBeTruthy();
    expect(screen.getByTestId("follow-coordinates")).toBeTruthy();

    await fireEvent(sv, "valueChange", true);
    expect(mockSetProject).toHaveBeenCalledWith("ws", "p", "sv", true);
  });
});

describe("followedChipLabel", () => {
  const t = i18n.t.bind(i18n);
  const label = (c: string) => c.toUpperCase();
  it("names one language, or the first and how many more", () => {
    expect(followedChipLabel([], label, t)).toBe("No languages");
    expect(followedChipLabel([["nb"], ["nb"]], label, t)).toBe("NB");
    expect(followedChipLabel([["nb"], ["sv", "de"]], label, t)).toBe("NB +2");
  });
});
