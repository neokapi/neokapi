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
jest.mock("../lib/auth", () => ({
  useAuth: () => ({
    client: {
      reviewPlan: (...a: unknown[]) => mockPlan(...a),
      confirmCoordinates: (...a: unknown[]) => mockConfirm(...a),
      permissions: (...a: unknown[]) => mockPermissions(...a),
    },
  }),
}));

const plan = (over: { confirmed?: boolean; next?: string } = {}) =>
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
    key_language: { waiting: 9, concepts: 57, untranslated: { nb: 20 } },
    exceptions: { source_findings: 593 },
    languages: [{ locale: "nb", waiting: 1200, failing: 40, pages: 300, settleable: 800 }],
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
});
