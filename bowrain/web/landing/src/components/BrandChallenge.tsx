import { useState, useEffect } from "react";
import { t } from "@neokapi/kapi-react/runtime";

// label/tone are user-visible and translatable; id and formality are
// programmatic keys (rule lookup) and stay literal. The rule patterns,
// suggestions, and sample texts below analyze *English* input by design,
// so they are deliberately not marked for translation.
const PROFILES = [
  {
    id: "b2b",
    label: t("Professional B2B"),
    tone: t("authoritative, precise"),
    formality: "formal",
  },
  { id: "dtc", label: t("Friendly DTC"), tone: t("warm, conversational"), formality: "casual" },
  { id: "docs", label: t("Technical Docs"), tone: t("clear, concise"), formality: "neutral" },
  {
    id: "support",
    label: t("Customer Support"),
    tone: t("empathetic, helpful"),
    formality: "warm",
  },
];

interface Violation {
  word: string;
  reason: string;
  suggestion: string;
  dimension: "Tone" | "Vocabulary" | "Style" | "Clarity" | "Brand";
}

const VIOLATION_RULES: Record<
  string,
  Array<{ pattern: RegExp; violation: Omit<Violation, "word"> }>
> = {
  b2b: [
    {
      pattern: /\bleverage\b/gi,
      violation: { reason: "Overused jargon", suggestion: "use", dimension: "Vocabulary" },
    },
    {
      pattern: /\butilize\b/gi,
      violation: { reason: "Unnecessarily complex", suggestion: "use", dimension: "Vocabulary" },
    },
    {
      pattern: /\bsynergy\b/gi,
      violation: { reason: "Forbidden term", suggestion: "collaboration", dimension: "Brand" },
    },
    {
      pattern: /\bkick off\b/gi,
      violation: { reason: "Too casual for B2B", suggestion: "start", dimension: "Tone" },
    },
    {
      pattern: /\bgame.?changer\b/gi,
      violation: { reason: "Cliché", suggestion: "significant improvement", dimension: "Style" },
    },
    {
      pattern: /\bbasically\b/gi,
      violation: { reason: "Filler word", suggestion: "(remove)", dimension: "Clarity" },
    },
    {
      pattern: /!!+/g,
      violation: { reason: "Too enthusiastic for B2B", suggestion: ".", dimension: "Tone" },
    },
    {
      pattern: /\bstuff\b/gi,
      violation: { reason: "Too informal", suggestion: "features", dimension: "Tone" },
    },
    {
      pattern: /\bcommence\b/gi,
      violation: { reason: "Too formal", suggestion: "start", dimension: "Vocabulary" },
    },
    {
      pattern: /\bemploy\b/gi,
      violation: { reason: "Unnecessarily complex", suggestion: "use", dimension: "Vocabulary" },
    },
  ],
  dtc: [
    {
      pattern: /\bcommence\b/gi,
      violation: { reason: "Too formal for DTC", suggestion: "start", dimension: "Tone" },
    },
    {
      pattern: /\binitiate\b/gi,
      violation: { reason: "Too corporate", suggestion: "start", dimension: "Tone" },
    },
    {
      pattern: /\bpurchase\b/gi,
      violation: { reason: "Too transactional", suggestion: "get", dimension: "Vocabulary" },
    },
    {
      pattern: /\bfacilitate\b/gi,
      violation: { reason: "Corporate jargon", suggestion: "help", dimension: "Vocabulary" },
    },
    {
      pattern: /\bhereby\b/gi,
      violation: { reason: "Way too formal", suggestion: "(remove)", dimension: "Style" },
    },
    {
      pattern: /\bsynergy\b/gi,
      violation: { reason: "Forbidden", suggestion: "teamwork", dimension: "Brand" },
    },
  ],
  docs: [
    {
      pattern: /\bjust\b/gi,
      violation: { reason: "Diminishing", suggestion: "(remove)", dimension: "Clarity" },
    },
    {
      pattern: /\bsimply\b/gi,
      violation: { reason: "Assumes simplicity", suggestion: "(remove)", dimension: "Clarity" },
    },
    {
      pattern: /\bobviously\b/gi,
      violation: { reason: "Assumes knowledge", suggestion: "(remove)", dimension: "Clarity" },
    },
    {
      pattern: /\bplease\b/gi,
      violation: { reason: "Unnecessary in docs", suggestion: "(remove)", dimension: "Style" },
    },
    {
      pattern: /\beasily\b/gi,
      violation: { reason: "Subjective", suggestion: "(remove)", dimension: "Clarity" },
    },
    {
      pattern: /\bvery\b/gi,
      violation: {
        reason: "Vague intensifier",
        suggestion: "(remove or be specific)",
        dimension: "Clarity",
      },
    },
  ],
  support: [
    {
      pattern: /\byou must\b/gi,
      violation: { reason: "Too demanding", suggestion: "please", dimension: "Tone" },
    },
    {
      pattern: /\bthat's wrong\b/gi,
      violation: { reason: "Blaming", suggestion: "let's fix this", dimension: "Tone" },
    },
    {
      pattern: /\byou failed\b/gi,
      violation: {
        reason: "Blaming language",
        suggestion: "this didn't work as expected",
        dimension: "Tone",
      },
    },
    {
      pattern: /\bcan't\b/gi,
      violation: {
        reason: "Negative framing",
        suggestion: "let me find another way",
        dimension: "Tone",
      },
    },
    {
      pattern: /\bsynergy\b/gi,
      violation: { reason: "Corporate jargon", suggestion: "working together", dimension: "Brand" },
    },
  ],
};

const SAMPLE_TEXTS: Record<string, string> = {
  b2b: `We leverage cutting-edge AI to utilize synergy across your entire workflow. Kick off your journey with our game-changer platform — basically, it's the stuff that modern enterprises need to commence their digital transformation!!`,
  dtc: `We hereby initiate the process to facilitate your purchase of our premium subscription. Synergy between our teams will commence immediately.`,
  docs: `Just simply run the command below. It's obviously very easy. Please note that you can easily configure the settings.`,
  support: `You must restart your computer. That's wrong — you failed to follow the instructions. We can't help if you don't read the docs.`,
};

function analyzeText(text: string, profileId: string): { violations: Violation[]; score: number } {
  const rules = VIOLATION_RULES[profileId] || [];
  const violations: Violation[] = [];

  for (const rule of rules) {
    const matches = text.matchAll(rule.pattern);
    for (const match of matches) {
      violations.push({
        word: match[0],
        ...rule.violation,
      });
    }
  }

  const wordCount = text.split(/\s+/).filter(Boolean).length;
  if (wordCount === 0) return { violations: [], score: 0 };

  const penalty = violations.length * 12;
  const score = Math.max(0, Math.min(100, 100 - penalty));
  return { violations, score };
}

const DIMENSION_COLORS: Record<string, string> = {
  Tone: "text-warning bg-warning/10",
  Vocabulary: "text-primary bg-primary/10",
  Style: "text-prism-5 bg-prism-5/10",
  Clarity: "text-prism-2 bg-prism-2/10",
  Brand: "text-destructive bg-destructive/10",
};

export function BrandChallenge() {
  const [profile, setProfile] = useState("b2b");
  const [text, setText] = useState(SAMPLE_TEXTS.b2b);
  const [analysis, setAnalysis] = useState<{ violations: Violation[]; score: number }>({
    violations: [],
    score: 0,
  });

  useEffect(() => {
    const timer = setTimeout(() => {
      setAnalysis(analyzeText(text, profile));
    }, 200);
    return () => clearTimeout(timer);
  }, [text, profile]);

  function loadSample() {
    setText(SAMPLE_TEXTS[profile]);
  }

  const scoreColor =
    analysis.score >= 80
      ? "text-success"
      : analysis.score >= 50
        ? "text-warning"
        : "text-destructive";

  // Hoisted so kapi-react extracts "Tone: {activeProfileTone}" as one block
  // with a named placeholder instead of an opaque expression.
  const activeProfile = PROFILES.find((p) => p.id === profile);
  const activeProfileLabel = activeProfile?.label;
  const activeProfileTone = activeProfile?.tone;

  return (
    <section id="brand-challenge" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
          TRY IT — RUNS LOCALLY WITH KAPI
        </div>
        <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
          The On-Brand Challenge
        </h2>
        <p className="mt-3 text-muted-foreground">
          Pick a style profile. Write or paste content. See it scored against the profile live, and
          fix the violations to reach 100. The check runs in kapi; Bowrain is where the profile is
          shared and governed across your projects — and your team when you have one.
        </p>
      </div>

      <div className="mt-10 grid gap-6 lg:grid-cols-[1fr_320px]">
        {/* Editor */}
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          <div className="flex items-center justify-between border-b border-border px-4 py-2.5">
            <div className="flex gap-2">
              {PROFILES.map((p) => (
                <button
                  key={p.id}
                  onClick={() => {
                    setProfile(p.id);
                    setText(SAMPLE_TEXTS[p.id]);
                  }}
                  className={`rounded-md px-3 py-1 text-xs transition ${
                    profile === p.id
                      ? "bg-primary/10 text-primary"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                  translate="no"
                >
                  {p.label}
                </button>
              ))}
            </div>
            <button
              onClick={loadSample}
              className="text-xs text-muted-foreground/70 transition hover:text-muted-foreground"
            >
              Load sample
            </button>
          </div>
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            rows={8}
            className="w-full resize-none bg-transparent p-4 text-sm outline-none placeholder:text-muted-foreground/50"
            placeholder="Write or paste your content here..."
          />
          {/* Inline violation highlights */}
          {analysis.violations.length > 0 && (
            <div className="border-t border-border p-4">
              <div className="flex flex-wrap gap-2">
                {analysis.violations.map((v, i) => (
                  <span
                    key={i}
                    className={`inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs ${DIMENSION_COLORS[v.dimension]}`}
                  >
                    <span className="font-medium">"{v.word}"</span>
                    <span className="opacity-60">→ {v.suggestion}</span>
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Score panel */}
        <div className="space-y-4">
          <div className="rounded-xl border border-border bg-card p-6 text-center">
            <div className="text-xs uppercase tracking-wider text-muted-foreground">
              Brand Compliance Score
            </div>
            <div className={`mt-2 text-5xl font-bold ${scoreColor}`}>{analysis.score}</div>
            <div className="mt-1 text-sm text-muted-foreground">/ 100</div>
            {/* Score bar */}
            <div className="mt-4 h-2 overflow-hidden rounded-full bg-secondary">
              <div
                className={`h-full rounded-full transition-all duration-500 ${
                  analysis.score >= 80
                    ? "bg-success"
                    : analysis.score >= 50
                      ? "bg-warning"
                      : "bg-destructive"
                }`}
                style={{ width: `${analysis.score}%` }}
              />
            </div>
          </div>

          {/* Dimension breakdown */}
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="mb-3 text-xs uppercase tracking-wider text-muted-foreground">
              Violations by dimension
            </div>
            <div className="space-y-2">
              {(["Tone", "Style", "Vocabulary", "Clarity", "Brand"] as const).map((dim) => {
                const count = analysis.violations.filter((v) => v.dimension === dim).length;
                return (
                  <div key={dim} className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">{dim}</span>
                    <span
                      className={
                        count > 0 ? "font-mono text-warning" : "font-mono text-muted-foreground/50"
                      }
                    >
                      {count}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Profile info */}
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="mb-2 text-xs uppercase tracking-wider text-muted-foreground">
              Active Profile
            </div>
            <div className="text-sm font-medium">{activeProfileLabel}</div>
            <div className="mt-1 text-xs text-muted-foreground">Tone: {activeProfileTone}</div>
          </div>
        </div>
      </div>

      <div className="mt-6 text-center text-sm text-muted-foreground">
        Edit the text and watch the score update in real time. Remove the violations to reach 100.
      </div>
    </section>
  );
}
