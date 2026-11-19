import { planFromServer, type ReviewPlan } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import { router } from "expo-router";
import type { ReactNode } from "react";
import { Alert } from "react-native";

import "../lib/i18n";
import ReviewPlanScreen from "../app/[ws]/[project]/plan";

jest.mock("expo-router", () => ({
  router: { push: jest.fn() },
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ ws: "ws", project: "p" }),
}));

const mockPlan = jest.fn<Promise<ReviewPlan>, unknown[]>();
const mockConfirm = jest.fn<Promise<void>, unknown[]>();
const mockPermissions = jest.fn();
const mockSettle = jest.fn();
const mockScan = jest.fn();
jest.mock("../lib/auth", () => ({
  useAuth: () => ({
    client: {
      reviewPlan: (...a: unknown[]) => mockPlan(...a),
      confirmCoordinates: (...a: unknown[]) => mockConfirm(...a),
      permissions: (...a: unknown[]) => mockPermissions(...a),
      settlePlan: (...a: unknown[]) => mockSettle(...a),
      startTermScan: (...a: unknown[]) => mockScan(...a),
    },
  }),
}));

const plan = (over: { confirmed?: boolean; next?: string; settle?: string; scan?: string } = {}) =>
  planFromServer({
    stream: "main",
    involvement: "context",
    next: over.next ?? "coordinates",
    coordinates: {
      source_language: "en",
      target_languages: ["nb"],
      points: [
        { collection: "neokapi-cli", coordinates: { product: "neokapi", channel: "cli" } },
        { collection: "neokapi-docs", coordinates: { product: "neokapi", channel: "docs" } },
      ],
      confirmed: over.confirmed ?? false,
    },
    key_language: {
      waiting: 9,
      concepts: 57,
      untranslated: { nb: 20 },
      scan: over.scan
        ? { id: "j1", status: over.scan, progress: 40, created_at: "", updated_at: "" }
        : null,
    },
    exceptions: {
      source_findings: 593,
      by_rule: {
        source: [
          {
            key: "terms.vocabulary:utilize",
            rule: "terms.vocabulary",
            subject: "utilize",
            message: 'Use "use" instead of "utilize"',
            fails: true,
            count: 210,
          },
        ],
        targets: {
          nb: [
            {
              key: "terms.vocabulary:verksted",
              rule: "terms.vocabulary",
              subject: "verksted",
              message: '"verksted" is not to be used in nb',
              fails: true,
              count: 31,
            },
          ],
        },
      },
    },
    languages: [
      {
        locale: "nb",
        waiting: 1200,
        failing: 40,
        pages: 34,
        pages_total: 300,
        pages_with_findings: 5,
        settleable: 800,
      },
    ],
    policy: { settle: over.settle ?? "first_pass", key_language_open: 29 },
  });

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("review plan", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPermissions.mockResolvedValue({
      permissions: ["view_content", "review", "manage_project"],
    });
  });

  it("starts at the coordinates and lists each level's work", async () => {
    mockPlan.mockResolvedValue(plan());
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByTestId("plan-coordinates-next")).toBeTruthy();
    expect(screen.getByText("neokapi: cli, docs")).toBeTruthy();
    expect(screen.getByText("9 terms and voice rules to decide")).toBeTruthy();
    expect(screen.getByText("20 terms have no Norwegian Bokmål term yet")).toBeTruthy();
    expect(screen.getByText("593 findings in the source")).toBeTruthy();
    expect(screen.getByText("800 translations clear every check")).toBeTruthy();
  });

  it("confirms the coordinates after asking", async () => {
    mockPlan.mockResolvedValue(plan());
    mockConfirm.mockResolvedValue();
    const alert = jest.spyOn(Alert, "alert").mockImplementation((_t, _m, buttons) => {
      buttons?.find((b) => b.style !== "cancel")?.onPress?.();
    });
    await render(wrap(<ReviewPlanScreen />));
    await fireEvent.press(await screen.findByTestId("plan-coordinates"));
    expect(alert).toHaveBeenCalled();
    expect(mockConfirm).toHaveBeenCalledWith({ workspace: "ws", project: "p" });
  });

  it("opens the context deck for the key language", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "key_language" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByTestId("plan-key_language-next")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-key_language"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/context",
      params: { ws: "ws", project: "p" },
    });
  });

  it("reads the interface sample under Context", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "interfaces" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(
      await screen.findByText(
        "Read 34 of 300 pages: one in ten from each channel, and every page with a failing check.",
      ),
    ).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-interfaces"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/[locale]",
      params: { ws: "ws", project: "p", locale: "nb", sample: "true" },
    });
  });

  it("settles a first pass on the checks alone", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "policy" }));
    mockSettle.mockResolvedValue({ approved: 800, skipped: 0, firstPass: true });
    jest.spyOn(Alert, "alert").mockImplementation((_t, _m, buttons) => {
      buttons?.find((b) => b.style !== "cancel")?.onPress?.();
    });
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText(/First pass: this settles on the checks alone/)).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-policy"));
    expect(mockSettle).toHaveBeenCalledWith({ workspace: "ws", project: "p" }, ["nb"]);
  });

  it("holds later passes until the key language is decided", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, settle: "held" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(
      await screen.findByText(
        "Held until the key language is decided: 29 decisions are still open.",
      ),
    ).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-policy"));
    expect(mockSettle).not.toHaveBeenCalled();
  });

  it("starts a scan for terms, and shows one running", async () => {
    mockPermissions.mockResolvedValue({ permissions: ["review", "manage_voice"] });
    mockPlan.mockResolvedValue(plan({ confirmed: true }));
    mockScan.mockResolvedValue({ id: "j1", status: "queued" });
    jest.spyOn(Alert, "alert").mockImplementation((_t, _m, buttons) => {
      buttons?.find((b) => b.style !== "cancel")?.onPress?.();
    });
    await render(wrap(<ReviewPlanScreen />));
    await fireEvent.press(await screen.findByTestId("plan-scan"));
    expect(mockScan).toHaveBeenCalledWith({ workspace: "ws", project: "p" });

    mockPlan.mockResolvedValue(plan({ confirmed: true, scan: "processing" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Scanning the project for terms, 40% done")).toBeTruthy();
  });

  it("lists the exceptions by rule and opens the deck narrowed to one", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "exceptions" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText('Use "use" instead of "utilize"')).toBeTruthy();
    expect(screen.getByText("210 findings")).toBeTruthy();
    expect(screen.getByText("Norwegian Bokmål, by rule")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-rules-nb-terms.vocabulary:verksted"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/[locale]",
      params: {
        ws: "ws",
        project: "p",
        locale: "nb",
        rule: "terms.vocabulary:verksted",
        ruleName: '"verksted" is not to be used in nb',
      },
    });
  });
});
