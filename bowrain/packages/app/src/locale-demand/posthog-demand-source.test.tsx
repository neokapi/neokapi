import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import type { PostHogConnectorConfig, PostHogDemandResponse } from "@neokapi/ui";
import {
  PostHogDemandSource,
  mapPostHogDemand,
  parsePostHogApiError,
} from "./posthog-demand-source";
import {
  PostHogConnectCard,
  serverErrorToFormErrors,
  validateConnectForm,
  type PostHogConnectFormValues,
} from "./PostHogConnectCard";
import { LocaleDemandPage, type LocaleDemandApi } from "./LocaleDemandPage";
import { demandProvenanceLabel } from "./locale-demand-fixtures";

// ---------------------------------------------------------------------------
// Wire fixture — shaped exactly like the server's PostHogDemandResponse.
// ---------------------------------------------------------------------------

const wire: PostHogDemandResponse = {
  range: "30d",
  generated_at: "2026-07-03T08:41:00Z",
  total_sessions: 1000,
  countries: [
    {
      country_code: "BR",
      sessions: 600,
      languages: [
        { tag: "pt-BR", sessions: 540 },
        { tag: "en-US", sessions: 60 },
      ],
    },
    {
      country_code: "DE",
      sessions: 300,
      languages: [
        { tag: "de-DE", sessions: 240 },
        { tag: "unknown", sessions: 60 },
      ],
    },
    // Not a resolvable ISO code: stays in totals but falls off the map.
    { country_code: "UNKNOWN", sessions: 100, languages: [{ tag: "en-US", sessions: 100 }] },
  ],
  languages: [
    {
      tag: "pt-BR",
      sessions: 540,
      trend: [
        { week_start: "2026-06-15", sessions: 120 },
        { week_start: "2026-06-22", sessions: 180 },
        { week_start: "2026-06-29", sessions: 240 },
      ],
    },
    { tag: "de-DE", sessions: 240, trend: [] }, // sparse: no trend rows
    { tag: "en-US", sessions: 160 },
    { tag: "unknown", sessions: 60 },
  ],
  served_locale_hit_rate: 0.66,
  warnings: ["served-locale query failed: timeout"],
  source: { kind: "posthog", host_label: "eu.posthog.com", posthog_project_id: "12345" },
  cached_at: "2026-07-03T09:12:00Z",
  cached: true,
};

const notConfiguredError = () =>
  new Error(
    `404: ${JSON.stringify({ error: "PostHog connector is not configured", code: "not_configured" })}`,
  );

describe("mapPostHogDemand", () => {
  const snapshot = mapPostHogDemand(wire, ["en", "de-DE"]);

  it("joins countries onto world-atlas numeric ids with names and flags", () => {
    const br = snapshot.countries.find((c) => c.code === "BR")!;
    expect(br.numericId).toBe("076");
    expect(br.name).toBe("Brazil");
    expect(br.flag).toBe("🇧🇷");
    expect(br.share).toBeCloseTo(0.6);
  });

  it("keeps unresolvable country codes in the data but off the map", () => {
    const unknown = snapshot.countries.find((c) => c.code === "UNKNOWN")!;
    expect(unknown.numericId).toBe(""); // no TopoJSON feature joins on ""
    expect(snapshot.totalSessions).toBe(1000);
  });

  it("computes per-country language shares and drops the unknown tag", () => {
    const de = snapshot.countries.find((c) => c.code === "DE")!;
    expect(de.languages).toEqual([{ code: "de-DE", share: 0.8 }]);
  });

  it("marks served rate as underivable (null), never zero", () => {
    for (const c of snapshot.countries) expect(c.servedRate).toBeNull();
    for (const l of snapshot.languages) expect(l.servedRate).toBeNull();
  });

  it("maps language trends and leaves sparse trends empty (no data ≠ zero demand)", () => {
    const ptBR = snapshot.languages.find((l) => l.code === "pt-BR")!;
    expect(ptBR.trend).toEqual([120, 180, 240]);
    const deDE = snapshot.languages.find((l) => l.code === "de-DE")!;
    expect(deDE.trend).toEqual([]);
    const enUS = snapshot.languages.find((l) => l.code === "en-US")!;
    expect(enUS.trend).toEqual([]); // trend omitted on the wire
  });

  it("derives coverage from the project locales by base language", () => {
    expect(snapshot.languages.find((l) => l.code === "en-US")!.coverage.status).toBe("covered");
    expect(snapshot.languages.find((l) => l.code === "de-DE")!.coverage.status).toBe("covered");
    expect(snapshot.languages.find((l) => l.code === "pt-BR")!.coverage.status).toBe("not-covered");
    // No plan machinery in phase 0 — no estimates on gaps.
    expect(snapshot.languages.find((l) => l.code === "pt-BR")!.planEstimate).toBeUndefined();
  });

  it("inverts country splits into per-language origins", () => {
    const ptBR = snapshot.languages.find((l) => l.code === "pt-BR")!;
    expect(ptBR.countries).toEqual([{ code: "BR", share: 1 }]);
    const enUS = snapshot.languages.find((l) => l.code === "en-US")!;
    expect(enUS.countries[0]).toEqual({ code: "UNKNOWN", share: 0.625 });
  });

  it("filters the unknown language tag from the table", () => {
    expect(snapshot.languages.find((l) => l.code === "unknown")).toBeUndefined();
  });

  it("carries PostHog provenance for the footer", () => {
    expect(snapshot.provenance).toEqual({
      kind: "posthog",
      hostLabel: "eu.posthog.com",
      posthogProjectId: "12345",
      cachedAt: "2026-07-03T09:12:00Z",
    });
    expect(demandProvenanceLabel(snapshot.provenance)).toBe(
      "Demand data: PostHog (eu.posthog.com · project 12345 · cached 09:12 UTC)",
    );
    expect(snapshot.warnings).toEqual(["served-locale query failed: timeout"]);
  });

  it("tolerates null countries/languages (Go nil slices)", () => {
    const empty = mapPostHogDemand(
      { ...wire, countries: null, languages: null, total_sessions: 0 },
      [],
    );
    expect(empty.countries).toEqual([]);
    expect(empty.languages).toEqual([]);
  });
});

describe("PostHogDemandSource", () => {
  it("fetches through the adapter and maps the response", async () => {
    const getPostHogDemand = vi.fn().mockResolvedValue(wire);
    const source = new PostHogDemandSource({ getPostHogDemand }, "acme", "proj-1", ["en"]);
    const snap = await source.getSnapshot("7d");
    expect(getPostHogDemand).toHaveBeenCalledWith("acme", "proj-1", "7d");
    expect(snap.provenance.kind).toBe("posthog");
  });
});

describe("parsePostHogApiError", () => {
  it("extracts the classified code from an adapter error", () => {
    expect(parsePostHogApiError(new Error('400: {"error":"nope","code":"bad_key"}'))).toEqual({
      code: "bad_key",
      message: "nope",
    });
    expect(parsePostHogApiError(notConfiguredError()).code).toBe("not_configured");
  });

  it("falls back to unknown for unclassified errors", () => {
    expect(parsePostHogApiError(new Error("network down")).code).toBe("unknown");
    expect(parsePostHogApiError(new Error('500: {"error":"boom"}')).code).toBe("unknown");
  });
});

describe("connect-card validation", () => {
  const base: PostHogConnectFormValues = {
    hostChoice: "us",
    selfHostedUrl: "",
    projectId: "12345",
    apiKey: "phx_secret",
  };

  it("accepts a complete cloud config", () => {
    expect(validateConnectForm(base, false)).toEqual({});
  });

  it("requires a URL for self-hosted", () => {
    expect(validateConnectForm({ ...base, hostChoice: "self-hosted" }, false).host).toMatch(
      /self-hosted/i,
    );
    expect(
      validateConnectForm(
        { ...base, hostChoice: "self-hosted", selfHostedUrl: "posthog.example.com" },
        false,
      ).host,
    ).toMatch(/http/);
    expect(
      validateConnectForm(
        { ...base, hostChoice: "self-hosted", selfHostedUrl: "https://posthog.example.com" },
        false,
      ),
    ).toEqual({});
  });

  it("requires a numeric project id", () => {
    expect(validateConnectForm({ ...base, projectId: "" }, false).projectId).toBeDefined();
    expect(validateConnectForm({ ...base, projectId: "abc" }, false).projectId).toBeDefined();
  });

  it("requires a key only when none is on file", () => {
    expect(validateConnectForm({ ...base, apiKey: "" }, false).apiKey).toBeDefined();
    expect(validateConnectForm({ ...base, apiKey: "" }, true)).toEqual({});
  });

  it("maps classified server errors onto their fields", () => {
    expect(serverErrorToFormErrors({ code: "bad_host", message: "h" })).toEqual({ host: "h" });
    expect(serverErrorToFormErrors({ code: "bad_key", message: "k" })).toEqual({ apiKey: "k" });
    expect(serverErrorToFormErrors({ code: "bad_project", message: "p" })).toEqual({
      projectId: "p",
    });
    expect(serverErrorToFormErrors({ code: "query_error", message: "q" })).toEqual({
      general: "q",
    });
  });
});

describe("PostHogConnectCard", () => {
  it("shows field errors before calling the server", async () => {
    const onSave = vi.fn();
    render(<PostHogConnectCard onSave={onSave} />);
    fireEvent.change(screen.getByTestId("posthog-project-id"), { target: { value: "abc" } });
    fireEvent.click(screen.getByTestId("posthog-test-save"));
    expect(await screen.findByTestId("posthog-error-project")).toBeInTheDocument();
    expect(screen.getByTestId("posthog-error-key")).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("surfaces the server's bad-key classification on the key field", async () => {
    const onSave = vi
      .fn()
      .mockRejectedValue(
        new Error(
          `400: ${JSON.stringify({ error: "personal API key was rejected", code: "bad_key" })}`,
        ),
      );
    render(<PostHogConnectCard onSave={onSave} />);
    fireEvent.change(screen.getByTestId("posthog-project-id"), { target: { value: "12345" } });
    fireEvent.change(screen.getByTestId("posthog-api-key"), { target: { value: "phx_bad" } });
    fireEvent.click(screen.getByTestId("posthog-test-save"));
    expect(await screen.findByTestId("posthog-error-key")).toHaveTextContent(
      "personal API key was rejected",
    );
  });

  it("shows the masked key and keeps it when left empty", async () => {
    const config: PostHogConnectorConfig = {
      configured: true,
      host: "eu",
      host_label: "eu.posthog.com",
      posthog_project_id: "12345",
      api_key_masked: "••••9fk2",
    };
    const onSave = vi.fn().mockResolvedValue(config);
    const onSaved = vi.fn();
    render(<PostHogConnectCard config={config} onSave={onSave} onSaved={onSaved} />);
    expect(screen.getByTestId("posthog-api-key")).toHaveAttribute(
      "placeholder",
      expect.stringContaining("••••9fk2"),
    );
    fireEvent.click(screen.getByTestId("posthog-test-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    // Empty key field on re-connect = keep the stored key (never resent).
    expect(onSave).toHaveBeenCalledWith({
      host: "eu",
      posthog_project_id: "12345",
      api_key: undefined,
    });
  });
});

describe("LocaleDemandPage source selection", () => {
  it("falls back to the labeled sample dataset when unconnected", async () => {
    const api: LocaleDemandApi = {
      getPostHogConnector: vi.fn().mockResolvedValue({ configured: false }),
      savePostHogConnector: vi.fn(),
      getPostHogDemand: vi.fn().mockRejectedValue(notConfiguredError()),
    };
    render(
      <LocaleDemandPage api={api} workspaceSlug="acme" projectId="p1" projectLocales={["en"]} />,
    );
    const notice = await screen.findByTestId("sample-data-notice");
    expect(notice).toHaveTextContent("Sample data: connect PostHog to see your own");
    expect(within(notice).getByTestId("posthog-connect-open")).toHaveTextContent("Connect PostHog");
    expect(screen.getByTestId("page-demand-provenance")).toHaveTextContent(
      /sample dataset \(web beacon \+ app telemetry ingest\)/i,
    );
  });

  it("shows live PostHog data and provenance when connected", async () => {
    const api: LocaleDemandApi = {
      getPostHogConnector: vi.fn(),
      savePostHogConnector: vi.fn(),
      getPostHogDemand: vi.fn().mockResolvedValue(wire),
    };
    render(
      <LocaleDemandPage api={api} workspaceSlug="acme" projectId="p1" projectLocales={["en"]} />,
    );
    expect(await screen.findByTestId("live-data-badge")).toBeInTheDocument();
    expect(screen.getByTestId("page-demand-provenance")).toHaveTextContent(
      "Demand data: PostHog (eu.posthog.com · project 12345 · cached 09:12 UTC)",
    );
    expect(screen.queryByTestId("sample-data-notice")).not.toBeInTheDocument();
  });

  it("degrades to the sample dataset with the error surfaced when the source breaks", async () => {
    const api: LocaleDemandApi = {
      getPostHogConnector: vi.fn(),
      savePostHogConnector: vi.fn(),
      getPostHogDemand: vi
        .fn()
        .mockRejectedValue(
          new Error(
            `502: ${JSON.stringify({ error: "personal API key was rejected", code: "bad_key" })}`,
          ),
        ),
    };
    render(
      <LocaleDemandPage api={api} workspaceSlug="acme" projectId="p1" projectLocales={["en"]} />,
    );
    const notice = await screen.findByTestId("sample-data-notice");
    expect(notice).toHaveTextContent(/PostHog could not be read/i);
    expect(notice).toHaveTextContent("personal API key was rejected");
    expect(within(notice).getByTestId("posthog-connect-open")).toHaveTextContent("Fix connection");
  });
});
