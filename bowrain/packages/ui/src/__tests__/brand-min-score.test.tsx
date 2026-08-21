import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { BrandProfileWizard } from "../brand/BrandProfileWizard";
import { BrandProfileEditor } from "../brand/BrandProfileEditor";
import { complianceBar, DEFAULT_MIN_SCORE } from "../brand/complianceBar";
import { minScoreFieldValue, parseMinScore } from "../brand/minScore";
import type { VoiceProfile } from "../brand/types";

// `min_score` is the profile's own compliant bar: the server excludes a block
// scoring below it from the compliance rate and refuses to auto-approve it. The
// surfaces have READ it since #1771, but the create/update request omitted it,
// so the only bar anyone could actually apply was the default. These cases pin
// the field onto both profile editors and the parse it goes through.

const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "..", "..");

function makeProfile(overrides: Partial<VoiceProfile> = {}): VoiceProfile {
  return {
    id: "prof-1",
    name: "Existing Brand",
    tone: { personality: [], formality: "neutral", emotion: "neutral", humor: "none" },
    style: {
      active_voice: true,
      sentence_length: "medium",
      person_pov: "second",
      contractions: "sometimes",
    },
    vocabulary: {},
    examples: [],
    workspace_id: "ws-1",
    version: 1,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("parseMinScore", () => {
  const cases: { raw: string; value?: number; valid: boolean; why: string }[] = [
    { raw: "", valid: true, why: "a blank field is the default bar, not zero" },
    { raw: "   ", valid: true, why: "whitespace only is still blank" },
    { raw: "0", valid: true, why: "a literal 0 means the default, like the Go zero value" },
    { raw: "90", value: 90, valid: true, why: "a bar in range is carried as a number" },
    { raw: "100", value: 100, valid: true, why: "the top of the band is legal" },
    { raw: "101", valid: false, why: "above the band the server rejects with 400" },
    { raw: "-5", valid: false, why: "a negative bar is not a bar" },
    { raw: "8.5", valid: false, why: "the bar is a whole number" },
    { raw: "eighty", valid: false, why: "text is not a bar" },
  ];
  for (const c of cases) {
    it(`${JSON.stringify(c.raw)}: ${c.why}`, () => {
      const parsed = parseMinScore(c.raw);
      expect(parsed.valid).toBe(c.valid);
      expect(parsed.value).toBe(c.value);
    });
  }

  it("round-trips a profile's bar back into the field", () => {
    expect(minScoreFieldValue(90)).toBe("90");
    expect(parseMinScore(minScoreFieldValue(90)).value).toBe(90);
    expect(minScoreFieldValue(undefined)).toBe("");
    expect(minScoreFieldValue(0)).toBe("");
  });
});

describe("BrandProfileWizard compliant bar", () => {
  it("carries the authored bar into the save payload", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<BrandProfileWizard onSave={onSave} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Name"), "Strict Brand");
    await user.type(screen.getByLabelText("On-brand bar"), "90");
    await user.click(screen.getByRole("button", { name: /Personas/ }));
    await user.click(screen.getByRole("button", { name: "Create Profile" }));

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0][0].min_score).toBe(90);
  });

  it("leaves the bar unset when the field is blank, so the profile takes the default", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<BrandProfileWizard onSave={onSave} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Name"), "Default Brand");
    await user.click(screen.getByRole("button", { name: /Personas/ }));
    await user.click(screen.getByRole("button", { name: "Create Profile" }));

    const saved = onSave.mock.calls[0][0];
    expect(saved.min_score).toBeUndefined();
    expect(complianceBar(saved)).toBe(DEFAULT_MIN_SCORE);
  });

  it("pre-fills an existing profile's bar and can clear it back to the default", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(
      <BrandProfileWizard
        profile={makeProfile({ min_score: 95 })}
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    );

    const field = screen.getByLabelText("On-brand bar");
    expect(field).toHaveValue(95);

    await user.clear(field);
    await user.click(screen.getByRole("button", { name: /Personas/ }));
    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    expect(onSave.mock.calls[0][0].min_score).toBeUndefined();
  });

  it("refuses to save an out-of-band bar rather than sending the server a 400", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<BrandProfileWizard onSave={onSave} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Name"), "Impossible Brand");
    await user.type(screen.getByLabelText("On-brand bar"), "120");
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: /Personas/ }));
    await user.click(screen.getByRole("button", { name: "Create Profile" }));
    expect(onSave).not.toHaveBeenCalled();
  });
});

describe("BrandProfileEditor compliant bar", () => {
  it("carries the authored bar into the save payload", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<BrandProfileEditor onSave={onSave} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Name"), "Strict Brand");
    await user.type(screen.getByLabelText("On-brand bar"), "70");
    await user.click(screen.getByRole("button", { name: "Create Profile" }));

    expect(onSave.mock.calls[0][0].min_score).toBe(70);
  });
});

describe("the write request mirrors the Go request body", () => {
  // A field the read type declares and the write type omits is a value a user
  // can see and never set — exactly what min_score was. Reading the Go source
  // as text keeps the two in step; the extraction is the same shape the
  // enumeration contract tests use.
  const GO_PATH = "bowrain/server/handlers_brand.go";
  const TS_PATH = "bowrain/packages/ui/src/brand/types.ts";

  /** The json tag names of a Go struct's fields, in declaration order. */
  function goJSONTags(source: string, structName: string): Set<string> {
    const start = source.indexOf(`type ${structName} struct {`);
    if (start < 0) throw new Error(`Go struct not found: ${structName}`);
    const end = source.indexOf("\n}", start);
    const body = source.slice(start, end);
    const out = new Set<string>();
    for (const m of body.matchAll(/`json:"([^",]+)/g)) out.add(m[1]);
    return out;
  }

  /** The property names of a TS interface body. */
  function tsInterfaceFields(source: string, name: string): Set<string> {
    const start = source.indexOf(`export interface ${name} {`);
    if (start < 0) throw new Error(`TS interface not found: ${name}`);
    const body = source.slice(start, source.indexOf("\n}", start));
    const out = new Set<string>();
    for (const m of body.matchAll(/^\s{2}([a-z_][\w]*)\??:/gm)) out.add(m[1]);
    return out;
  }

  const goSource = readFileSync(join(REPO_ROOT, GO_PATH), "utf8");
  const tsSource = readFileSync(join(REPO_ROOT, TS_PATH), "utf8");

  it("declares no field the server would silently drop", () => {
    const accepted = goJSONTags(goSource, "BrandProfileRequest");
    const sent = tsInterfaceFields(tsSource, "CreateVoiceProfileRequest");
    expect(sent.size).toBeGreaterThan(1);
    const ignored = [...sent].filter((f) => !accepted.has(f)).sort();
    expect(
      ignored,
      `${TS_PATH} sends ${ignored.join(", ")}, which ${GO_PATH}'s BrandProfileRequest does not bind`,
    ).toEqual([]);
  });

  it("can set the compliant bar it reads", () => {
    expect(goJSONTags(goSource, "BrandProfileRequest")).toContain("min_score");
    expect(tsInterfaceFields(tsSource, "VoiceProfile")).toContain("min_score");
    expect(tsInterfaceFields(tsSource, "CreateVoiceProfileRequest")).toContain("min_score");
  });
});
