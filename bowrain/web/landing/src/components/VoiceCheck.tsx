import { useState, useEffect } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { captureLandingEvent } from "../analytics";
import { markEngaged } from "../sectionSignals";

// A working miniature of the voice check, running in the page.
//
// The four profiles are coordinate bundles, not company types, and each carries
// both a draft and the rules in force where that draft sits. What separates
// them is severity, not vocabulary size: the API reference forbids the new name
// outright — the rename table's third row, enforced — where the help centre has
// no rule about it at all. Selecting a point loads that point's draft; editing
// re-scores whatever is in the box against the point currently selected.
//
// The arithmetic is the product's, not an approximation of it: findings carry a
// severity, severity carries a penalty weight (neutral 0, minor 1, major 5,
// critical 25), the roll-up is 100 − Σpenalty clamped to [0,100], and each of
// the five dimensions scores 100 − its own penalty. See core/profile/scoring.go.
//
// label/coordinates are user-visible and translatable; id is a programmatic key
// for rule lookup. The rule patterns, reasons, suggestions and sample texts
// analyze *English* input by design, so they are deliberately not marked for
// translation.

type Dimension = "Tone" | "Style" | "Vocabulary" | "Clarity" | "Brand";
type Severity = "minor" | "major" | "critical";

/** The framework's severity weights (core/check). */
const SEVERITY_WEIGHT: Record<Severity, number> = { minor: 1, major: 5, critical: 25 };

const DIMENSIONS: Dimension[] = ["Tone", "Style", "Vocabulary", "Clarity", "Brand"];

const PROFILES = [
  {
    id: "help",
    label: t("End-user help"),
    coordinates: t("customers · help centre · plain"),
  },
  {
    id: "reference",
    label: t("Developer reference"),
    coordinates: t("developers · API reference · terse"),
  },
  {
    id: "disclosure",
    label: t("Regulated disclosure"),
    coordinates: t("all customers · legal notice · formal"),
  },
  {
    id: "campaign",
    label: t("Launch campaign"),
    coordinates: t("prospects · marketing site · warm"),
  },
];

interface Rule {
  pattern: RegExp;
  reason: string;
  suggestion: string;
  dimension: Dimension;
  severity: Severity;
}

interface Finding extends Rule {
  word: string;
}

const RULES: Record<string, Rule[]> = {
  help: [
    {
      pattern: /\bsimply\b/gi,
      reason: "Assumes it is simple",
      suggestion: "(remove)",
      dimension: "Clarity",
      severity: "minor",
    },
    {
      pattern: /\bjust\b/gi,
      reason: "Diminishing",
      suggestion: "(remove)",
      dimension: "Clarity",
      severity: "minor",
    },
    {
      pattern: /\bobviously\b/gi,
      reason: "Assumes knowledge the reader came here without",
      suggestion: "(remove)",
      dimension: "Clarity",
      severity: "major",
    },
    {
      pattern: /\beas(y|ily)\b/gi,
      reason: "Subjective",
      suggestion: "(remove)",
      dimension: "Clarity",
      severity: "minor",
    },
    {
      pattern: /\bvery\b/gi,
      reason: "Vague intensifier",
      suggestion: "(remove or be specific)",
      dimension: "Clarity",
      severity: "minor",
    },
    {
      pattern: /\bplease\b/gi,
      reason: "Unnecessary in instructions",
      suggestion: "(remove)",
      dimension: "Style",
      severity: "minor",
    },
    {
      pattern: /\butilize\b/gi,
      reason: "Unnecessarily complex",
      suggestion: "use",
      dimension: "Vocabulary",
      severity: "minor",
    },
  ],
  reference: [
    {
      pattern: /\bworkspaces?\b/gi,
      reason: "The wire field is named project; the reference keeps the shipped name",
      suggestion: "project",
      dimension: "Brand",
      severity: "critical",
    },
    {
      pattern: /\bpowerful\b/gi,
      reason: "Superlative; state the capability",
      suggestion: "(remove)",
      dimension: "Tone",
      severity: "major",
    },
    {
      pattern: /\bsimply\b/gi,
      reason: "Assumes it is simple",
      suggestion: "(remove)",
      dimension: "Clarity",
      severity: "minor",
    },
    {
      pattern: /\beas(y|ily)\b/gi,
      reason: "Subjective",
      suggestion: "(remove)",
      dimension: "Clarity",
      severity: "minor",
    },
    {
      pattern: /\bour\b/gi,
      reason: "A reference describes the API, not its authors",
      suggestion: "the",
      dimension: "Style",
      severity: "minor",
    },
  ],
  disclosure: [
    {
      pattern: /\b\w+n't\b|\bwe'll\b|\byou're\b/gi,
      reason: "Contractions are out of register for a notice",
      suggestion: "write it out",
      dimension: "Style",
      severity: "major",
    },
    {
      pattern: /\bguarantee(d|s)?\b/gi,
      reason: "Unbounded commitment; legal review has not approved one",
      suggestion: "(remove)",
      dimension: "Brand",
      severity: "critical",
    },
    {
      pattern: /\b(never|always)\b/gi,
      reason: "Absolute claim in a disclosure",
      suggestion: "state the actual condition",
      dimension: "Brand",
      severity: "major",
    },
    {
      pattern: /!/g,
      reason: "Exclamation in a notice",
      suggestion: ".",
      dimension: "Tone",
      severity: "major",
    },
  ],
  campaign: [
    {
      pattern: /\bleverage\b/gi,
      reason: "Overused jargon",
      suggestion: "use",
      dimension: "Vocabulary",
      severity: "minor",
    },
    {
      pattern: /\butilize\b/gi,
      reason: "Unnecessarily complex",
      suggestion: "use",
      dimension: "Vocabulary",
      severity: "minor",
    },
    {
      pattern: /\bsynergy\b/gi,
      reason: "Discouraged term",
      suggestion: "working together",
      dimension: "Brand",
      severity: "major",
    },
    {
      pattern: /\b(cutting.?edge|revolutionary|game.?changer)\b/gi,
      reason: "Superlative; name the change instead",
      suggestion: "(be specific)",
      dimension: "Tone",
      severity: "major",
    },
    {
      pattern: /!!+/g,
      reason: "Over-enthusiastic punctuation",
      suggestion: "!",
      dimension: "Tone",
      severity: "minor",
    },
  ],
};

const SAMPLE_TEXTS: Record<string, string> = {
  help: `Just simply open the settings panel. It's obviously very easy. Please note that you can utilize the search box to find anything.`,
  reference: `Simply rename your workspace with a PATCH request. Our powerful endpoint makes renaming a workspace easy.`,
  disclosure: `We'll never share your data, guaranteed! You aren't giving up any rights, and we always delete backups on request.`,
  campaign: `We leverage cutting-edge AI to utilize synergy across your whole workflow. It's a real game-changer!!`,
};

interface Analysis {
  findings: Finding[];
  overall: number;
  perDimension: Record<Dimension, { penalty: number; issues: number }>;
}

/** Mirrors core/profile.CalculateScore: penalties by dimension, roll-up over all. */
function analyze(text: string, profileId: string): Analysis {
  const findings: Finding[] = [];
  for (const rule of RULES[profileId] ?? []) {
    for (const match of text.matchAll(rule.pattern)) {
      findings.push({ ...rule, word: match[0] });
    }
  }

  const perDimension = Object.fromEntries(
    DIMENSIONS.map((d) => [d, { penalty: 0, issues: 0 }]),
  ) as Analysis["perDimension"];
  let total = 0;
  for (const f of findings) {
    const weight = SEVERITY_WEIGHT[f.severity];
    perDimension[f.dimension].penalty += weight;
    perDimension[f.dimension].issues += 1;
    total += weight;
  }
  return { findings, overall: Math.max(0, Math.min(100, 100 - total)), perDimension };
}

const DIMENSION_COLORS: Record<Dimension, string> = {
  Tone: "text-warning bg-warning/10",
  Style: "text-prism-5 bg-prism-5/10",
  Vocabulary: "text-primary bg-primary/10",
  Clarity: "text-prism-2 bg-prism-2/10",
  Brand: "text-destructive bg-destructive/10",
};

export function VoiceCheck() {
  const [profile, setProfile] = useState(PROFILES[0].id);
  const [text, setText] = useState(SAMPLE_TEXTS[PROFILES[0].id]);
  const [analysis, setAnalysis] = useState<Analysis>(() => analyze(SAMPLE_TEXTS.help, "help"));
  const [reported, setReported] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setAnalysis(analyze(text, profile)), 200);
    return () => clearTimeout(timer);
  }, [text, profile]);

  // One engagement signal per visit, whichever way the reader touched it.
  function reportUse(action: string, nextProfile: string) {
    markEngaged("voice-check");
    if (reported) return;
    setReported(true);
    captureLandingEvent("voice_check_used", { action, profile: nextProfile });
  }

  function selectProfile(id: string) {
    setProfile(id);
    setText(SAMPLE_TEXTS[id]);
    reportUse("profile", id);
  }

  const scoreColor =
    analysis.overall >= 80
      ? "text-success"
      : analysis.overall >= 50
        ? "text-warning"
        : "text-destructive";
  const barColor =
    analysis.overall >= 80
      ? "bg-success"
      : analysis.overall >= 50
        ? "bg-warning"
        : "bg-destructive";

  // Hoisted so neokapi-i18n extracts the sentence as one block with named
  // placeholders instead of an opaque expression.
  const active = PROFILES.find((p) => p.id === profile);
  const activeLabel = active?.label;
  const activeCoordinates = active?.coordinates;

  return (
    <div className="mt-10 grid gap-6 lg:grid-cols-[1fr_320px]">
      {/* The draft under check */}
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2.5">
          <div className="flex flex-wrap gap-2">
            {PROFILES.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => selectProfile(p.id)}
                aria-pressed={p.id === profile}
                className={`rounded-md px-3 py-1 text-xs transition ${
                  profile === p.id
                    ? "bg-primary/10 text-primary"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {p.label}
              </button>
            ))}
          </div>
          <button
            type="button"
            onClick={() => {
              setText(SAMPLE_TEXTS[profile]);
              reportUse("sample", profile);
            }}
            className="text-xs text-muted-foreground/70 transition hover:text-muted-foreground"
          >
            {t("Reset the draft")}
          </button>
        </div>
        <textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            reportUse("edit", profile);
          }}
          rows={8}
          aria-label={t("Draft under check")}
          className="w-full resize-none bg-transparent p-4 text-sm outline-none placeholder:text-muted-foreground/50"
          placeholder={t("Write or paste content here")}
        />
        {analysis.findings.length > 0 && (
          <div className="border-t border-border p-4">
            <div className="flex flex-wrap gap-2">
              {analysis.findings.map((f, i) => (
                <span
                  key={`${f.word}-${i}`}
                  title={`${f.reason} · ${f.severity}`}
                  className={`inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs ${DIMENSION_COLORS[f.dimension]}`}
                >
                  <span className="font-medium">{f.word}</span>
                  <span className="opacity-60">→ {f.suggestion}</span>
                </span>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* The score the same arithmetic produces in the product */}
      <div className="space-y-4">
        <div className="rounded-xl border border-border bg-card p-6 text-center">
          <div className="text-xs uppercase tracking-wider text-muted-foreground">
            {t("Voice compliance score")}
          </div>
          <div translate="no" className={`mt-2 text-5xl font-bold ${scoreColor}`}>
            {analysis.overall}
          </div>
          <div translate="no" className="mt-1 text-sm text-muted-foreground">
            / 100
          </div>
          <div className="mt-4 h-2 overflow-hidden rounded-full bg-secondary">
            <div
              className={`h-full rounded-full transition-all duration-500 ${barColor}`}
              style={{ width: `${analysis.overall}%` }}
            />
          </div>
        </div>

        <div className="rounded-xl border border-border bg-card p-4">
          <div className="mb-3 text-xs uppercase tracking-wider text-muted-foreground">
            {t("Penalty by dimension")}
          </div>
          <div className="space-y-2">
            {DIMENSIONS.map((dim) => {
              const { penalty, issues } = analysis.perDimension[dim];
              return (
                <div key={dim} className="flex items-center justify-between text-sm">
                  <span translate="no" className="text-muted-foreground">
                    {dim}
                  </span>
                  <span
                    translate="no"
                    className={
                      penalty > 0 ? "font-mono text-warning" : "font-mono text-muted-foreground/50"
                    }
                  >
                    {penalty > 0 ? `${issues} · −${penalty}` : "·"}
                  </span>
                </div>
              );
            })}
          </div>
        </div>

        <div className="rounded-xl border border-border bg-card p-4">
          <div className="mb-2 text-xs uppercase tracking-wider text-muted-foreground">
            {t("Point in force")}
          </div>
          <div className="text-sm font-medium">{activeLabel}</div>
          <div translate="no" className="mt-1 font-mono text-xs text-muted-foreground">
            {activeCoordinates}
          </div>
        </div>
      </div>
    </div>
  );
}
