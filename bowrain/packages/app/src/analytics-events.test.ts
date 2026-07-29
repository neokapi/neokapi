import { describe, it, expect } from "vite-plus/test";
import { featureFromRoutePattern } from "./analytics-events";

describe("featureFromRoutePattern", () => {
  it("derives workspace-level features", () => {
    expect(featureFromRoutePattern("/$workspace")).toBe("dashboard");
    expect(featureFromRoutePattern("/$workspace/")).toBe("dashboard");
    expect(featureFromRoutePattern("/$workspace/memory")).toBe("memory");
    expect(featureFromRoutePattern("/$workspace/locale-demand")).toBe("locale_demand");
    expect(featureFromRoutePattern("/$workspace/auditlog")).toBe("auditlog");
    expect(featureFromRoutePattern("/$workspace/activities")).toBe("activities");
    expect(featureFromRoutePattern("/$workspace/tasks")).toBe("tasks");
    expect(featureFromRoutePattern("/$workspace/bin")).toBe("bin");
    expect(featureFromRoutePattern("/$workspace/user-settings")).toBe("user_settings");
  });

  it("derives settings sections", () => {
    expect(featureFromRoutePattern("/$workspace/settings")).toBe("settings");
    expect(featureFromRoutePattern("/$workspace/settings/")).toBe("settings");
    expect(featureFromRoutePattern("/$workspace/settings/languages")).toBe("settings_languages");
    expect(featureFromRoutePattern("/$workspace/settings/members")).toBe("settings_members");
    expect(featureFromRoutePattern("/$workspace/settings/billing")).toBe("settings_billing");
  });

  it("derives project-scoped features without leaking params", () => {
    const base = "/$workspace/p/$projectId/s/$stream";
    expect(featureFromRoutePattern(base)).toBe("project_overview");
    expect(featureFromRoutePattern(`${base}/dashboard`)).toBe("translation_dashboard");
    expect(featureFromRoutePattern(`${base}/settings`)).toBe("project_settings");
    expect(featureFromRoutePattern(`${base}/automations`)).toBe("automations");
    expect(featureFromRoutePattern(`${base}/runs`)).toBe("runs");
    expect(featureFromRoutePattern(`${base}/connectors`)).toBe("connectors");
    expect(featureFromRoutePattern(`${base}/$itemId/translate`)).toBe("translate");
    expect(featureFromRoutePattern(`${base}/$itemId/review`)).toBe("review");
    expect(featureFromRoutePattern(`${base}/$itemId/pre-process`)).toBe("pre_process");
  });

  // The hub moved from /brand to /context, so the derived feature names move
  // with it (brand_concepts → context_concepts). Deliberate: the feature name
  // is derived from the route pattern, and a shim that kept the old names would
  // make the taxonomy disagree with the URL it is supposed to describe.
  it("derives context hub features", () => {
    expect(featureFromRoutePattern("/$workspace/context/concepts")).toBe("context_concepts");
    expect(featureFromRoutePattern("/$workspace/context/concepts/$cid")).toBe("context_concepts");
    expect(featureFromRoutePattern("/$workspace/context/voice")).toBe("context_voice");
    expect(featureFromRoutePattern("/$workspace/context/voice/$profileId")).toBe("context_voice");
    expect(featureFromRoutePattern("/$workspace/context/voice/review/$profileId")).toBe(
      "context_voice_review",
    );
    expect(featureFromRoutePattern("/$workspace/context/voice/mcp-guide")).toBe(
      "context_voice_mcp_guide",
    );
    expect(featureFromRoutePattern("/$workspace/context/changes")).toBe("context_changes");
    expect(featureFromRoutePattern("/$workspace/context/changes/$id")).toBe("context_changes");
    expect(featureFromRoutePattern("/$workspace/context/memory")).toBe("context_memory");
    expect(featureFromRoutePattern("/$workspace/context/activity")).toBe("context_activity");
    expect(featureFromRoutePattern("/$workspace/context/scan/$jobId")).toBe("context_scan");
    expect(featureFromRoutePattern("/$workspace/context/dashboard")).toBe("context_dashboard");
  });

  it("derives auth/entry features and drops the bare root", () => {
    expect(featureFromRoutePattern("/")).toBeNull();
    expect(featureFromRoutePattern("/welcome")).toBe("welcome");
    expect(featureFromRoutePattern("/pricing")).toBe("pricing");
    expect(featureFromRoutePattern("/join/$code")).toBe("join");
    expect(featureFromRoutePattern("/claim/$token")).toBe("claim");
    expect(featureFromRoutePattern("/device/verify")).toBe("device_verify");
    expect(featureFromRoutePattern("/account/confirm-email")).toBe("account_confirm_email");
  });

  it("never emits param placeholders or slashes", () => {
    for (const pattern of [
      "/$workspace/p/$projectId/s/$stream/$itemId/translate",
      "/$workspace/context/voice/$profileId",
      "/join/$code",
    ]) {
      const feature = featureFromRoutePattern(pattern);
      expect(feature).not.toBeNull();
      expect(feature).not.toMatch(/[$/]/);
    }
  });
});
