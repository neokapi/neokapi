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

  it("derives brand hub features", () => {
    expect(featureFromRoutePattern("/$workspace/brand/concepts")).toBe("brand_concepts");
    expect(featureFromRoutePattern("/$workspace/brand/concepts/$cid")).toBe("brand_concepts");
    expect(featureFromRoutePattern("/$workspace/brand/voice")).toBe("brand_voice");
    expect(featureFromRoutePattern("/$workspace/brand/voice/$profileId")).toBe("brand_voice");
    expect(featureFromRoutePattern("/$workspace/brand/voice/review/$profileId")).toBe(
      "brand_voice_review",
    );
    expect(featureFromRoutePattern("/$workspace/brand/voice/mcp-guide")).toBe(
      "brand_voice_mcp_guide",
    );
    expect(featureFromRoutePattern("/$workspace/brand/experiments/$id")).toBe("brand_experiments");
    expect(featureFromRoutePattern("/$workspace/brand/scan/$jobId")).toBe("brand_scan");
    expect(featureFromRoutePattern("/$workspace/brand/dashboard")).toBe("brand_dashboard");
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
      "/$workspace/brand/voice/$profileId",
      "/join/$code",
    ]) {
      const feature = featureFromRoutePattern(pattern);
      expect(feature).not.toBeNull();
      expect(feature).not.toMatch(/[$/]/);
    }
  });
});
