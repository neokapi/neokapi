import { planFromServer, type ReviewPlan } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import { router } from "expo-router";
import type { ReactNode } from "react";

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

type Over = {
  confirmed?: boolean;
  next?: string;
  settle?: string;
  scan?: string;
  /** The context step's scan state, the job's status or a waiting reason. */
  stepScan?: string;
  /** The context step's proposals state: running, or why the job waits. */
  proposals?: string;
  quotaResetsAt?: string;
  keyState?: string;
  waiting?: number;
  concepts?: number;
  auto?: { by?: string; by_name?: string; source?: string; skip?: string };
  last?: { at: string; approved: number; by?: string; by_name?: string; automatic?: boolean };
};

const plan = (over: Over = {}) =>
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
    context: {
      waiting: over.waiting ?? 9,
      concepts: over.concepts ?? 57,
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
        held: 1160,
      },
    ],
    policy: {
      settle: over.settle ?? "first_pass",
      context_open: 29,
      auto: over.auto,
      last: over.last,
    },
    held: "context",
    steps: [
      {
        level: "coordinates",
        state: over.confirmed ? "done" : "open",
        count: over.confirmed ? 0 : 1,
        done: over.confirmed ?? false,
      },
      {
        level: "context",
        state: over.keyState ?? "open",
        count: 29,
        scan: over.stepScan ?? over.scan,
        proposals: over.proposals,
        quota_resets_at: over.quotaResetsAt,
      },
      { level: "exceptions", state: "open", count: 2 },
      { level: "interfaces", state: "open", count: 34 },
      { level: "policy", state: "open", count: 800, settle: over.settle ?? "first_pass" },
    ],
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

  it("shows one row per step and the held translations", async () => {
    mockPlan.mockResolvedValue(plan());
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByTestId("plan-coordinates-next")).toBeTruthy();
    expect(screen.getByText("Not confirmed yet")).toBeTruthy();
    expect(screen.getByText("29 context cards")).toBeTruthy();
    expect(screen.getByText("40 translations fail a check")).toBeTruthy();
    expect(screen.getByText("Read 34 of 300 pages")).toBeTruthy();
    expect(screen.getByText("800 translations clear every check")).toBeTruthy();
    expect(
      screen.getByText(
        "1,160 Norwegian Bokmål translations are held until the context is decided.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("plan-involvement")).toBeNull();
    // Only the next step carries an action.
    expect(screen.getByTestId("plan-coordinates-action")).toBeTruthy();
    expect(screen.queryByTestId("plan-context-action")).toBeNull();
    expect(screen.queryByTestId("plan-policy-action")).toBeNull();
  });

  it("opens the coordinates map", async () => {
    mockPlan.mockResolvedValue(plan());
    await render(wrap(<ReviewPlanScreen />));
    await fireEvent.press(await screen.findByTestId("plan-coordinates-action"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/coordinates",
      params: { ws: "ws", project: "p", languages: "nb" },
    });
    expect(mockConfirm).not.toHaveBeenCalled();
  });

  it("opens the context deck for the context level", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "context" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByTestId("plan-context-next")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("plan-context-action"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/context",
      params: { ws: "ws", project: "p" },
    });
  });

  it("reads the interface sample under Context", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "interfaces" }));
    await render(wrap(<ReviewPlanScreen />));
    await fireEvent.press(await screen.findByTestId("plan-interfaces-action"));
    expect(router.push).toHaveBeenCalledWith({
      pathname: "/[ws]/[project]/[locale]",
      params: { ws: "ws", project: "p", locale: "nb", sample: "true" },
    });
  });

  it("shows how the server settles after each run, with no settle button", async () => {
    mockPlan.mockResolvedValue(
      plan({
        confirmed: true,
        next: "policy",
        auto: { by: "u-ada", by_name: "Ada", source: "involvement" },
        last: {
          at: new Date().toISOString(),
          approved: 412,
          by: "u-ada",
          by_name: "Ada",
          automatic: true,
        },
      }),
    );
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Settles after each run, credited to Ada")).toBeTruthy();
    expect(screen.getByTestId("plan-policy-last").props.children).toMatch(
      /^Last pass after a run, .+: 412 translations established$/,
    );
    expect(screen.queryByTestId("plan-policy-action")).toBeNull();
  });

  it("says why nothing settles after a run", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "policy", auto: { skip: "no_one" } }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByTestId("plan-policy-auto")).toBeTruthy();
    expect(screen.getByTestId("plan-policy-auto").props.children).toMatch(
      /until a manager sets the involvement or confirms the coordinates/,
    );

    mockPlan.mockResolvedValue(
      plan({
        confirmed: true,
        next: "policy",
        auto: { by: "u-bo", by_name: "Bo", skip: "no_permission" },
        last: { at: "2026-11-01T10:00:00Z", approved: 3, by: "u-cy", by_name: "Cy" },
      }),
    );
    await render(wrap(<ReviewPlanScreen />));
    expect(
      await screen.findByText(
        "Nothing settles after a run: Bo no longer has review access to this project",
      ),
    ).toBeTruthy();
    expect(screen.getAllByTestId("plan-policy-last").at(-1)?.props.children).toMatch(
      /^Last pass by Cy, .+: 3 translations established$/,
    );
  });

  it("holds later passes until the context is decided", async () => {
    mockPlan.mockResolvedValue(plan({ confirmed: true, next: "policy", settle: "held" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Waits for the context")).toBeTruthy();
    expect(screen.queryByTestId("plan-policy-auto")).toBeNull();
  });

  const notStarted = {
    confirmed: true,
    next: "context",
    keyState: "not_started",
    waiting: 0,
    concepts: 0,
  };

  it("reads the term scan's state and offers no button to start it", async () => {
    mockPlan.mockResolvedValue(plan({ ...notStarted, stepScan: "waiting_content" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Starts when content arrives")).toBeTruthy();
    expect(screen.queryByTestId("plan-scan")).toBeNull();
    expect(screen.queryByTestId("plan-context-action")).toBeNull();

    mockPlan.mockResolvedValue(plan({ ...notStarted, scan: "processing" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Reading your content, 40% done")).toBeTruthy();

    mockPlan.mockResolvedValue(plan({ ...notStarted, stepScan: "waiting_push" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Starts with the next push")).toBeTruthy();
  });

  it("says when the scan waits for the AI limit to reset", async () => {
    mockPlan.mockResolvedValue(
      plan({
        ...notStarted,
        stepScan: "waiting_quota",
        quotaResetsAt: "2026-12-01T00:00:00Z",
      }),
    );
    await render(wrap(<ReviewPlanScreen />));
    expect(
      await screen.findByText("Waits for the AI limit to reset on December 1, 2026"),
    ).toBeTruthy();
  });

  it("says the server proposes target terms, or why the proposals wait", async () => {
    mockPlan.mockResolvedValue(plan({ proposals: "processing" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(await screen.findByText("Proposing terms in your target languages")).toBeTruthy();

    mockPlan.mockResolvedValue(
      plan({ proposals: "waiting_quota", quotaResetsAt: "2026-12-01T00:00:00Z" }),
    );
    await render(wrap(<ReviewPlanScreen />));
    expect(
      await screen.findByText("Term proposals wait for the AI limit to reset on December 1, 2026"),
    ).toBeTruthy();

    mockPlan.mockResolvedValue(plan({ proposals: "waiting_credits" }));
    await render(wrap(<ReviewPlanScreen />));
    expect(
      await screen.findByText("Term proposals wait for AI credits: this workspace has none left"),
    ).toBeTruthy();

    mockPlan.mockResolvedValue(plan());
    await render(wrap(<ReviewPlanScreen />));
    await screen.findByTestId("plan-list");
    expect(screen.queryByTestId("plan-context-proposals")).toBeNull();
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
