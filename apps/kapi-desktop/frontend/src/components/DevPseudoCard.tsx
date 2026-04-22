/**
 * Dev-only control panel for runtime pseudo-translation.
 *
 * Talks to `@neokapi/kapi-react/runtime/pseudo` via a dynamic import
 * so the accent maps never ship in production builds. Renders
 * nothing at all outside `import.meta.env.DEV`.
 *
 * State shape lives in localStorage under the same key the toggle in
 * `main.tsx` writes (`kapi.dev.pseudo`), so the console handle
 * (`window.kapi.pseudo(...)`) and this UI stay in sync across reloads.
 *
 * The whole panel is `translate="no"` — without that, the pseudo
 * transform would recursively accent the UI's own labels and make
 * them wiggle with whatever expansion / alphabet is active.
 */
import { useCallback, useEffect, useState } from "react";
import { FlaskConical } from "lucide-react";
import { Card, CardContent, Switch, Label, Input } from "@neokapi/ui-primitives";

type AlphabetName = "accented" | "wobbly" | "none";

interface PseudoConfig {
  prefix?: string;
  suffix?: string;
  expansion?: number;
  expansionChar?: string;
  alphabet?: AlphabetName;
}

const STORAGE_KEY = "kapi.dev.pseudo";
const DEFAULT_PREFIX = "\u2592 "; // ▒ + space
const DEFAULT_SUFFIX = " \u2592";
const DEFAULT_EXPANSION_CHAR = "\u00b7"; // ·

type SetMode = (cfg: PseudoConfig | null) => void;

const ALPHABET_OPTIONS: ReadonlyArray<{
  value: AlphabetName;
  label: string;
  sample: string;
}> = [
  { value: "accented", label: "Accented (uniform)", sample: "Ĥéļļö ŵöŕļđ" },
  { value: "wobbly", label: "Wobbly (varied)", sample: "Ḧěľľǒ ŵǒřľď" },
  { value: "none", label: "None (plain)", sample: "Hello world" },
];

const EXPANSION_CHAR_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: "\u00b7", label: "· Middle dot" },
  { value: "\u2022", label: "• Bullet" },
  { value: "\u2219", label: "∙ Bullet operator" },
  { value: "\u2003", label: "␣ Em space" },
  { value: "\u2009", label: "▪ Thin space" },
  { value: "\u2500", label: "─ Box line" },
  { value: "~", label: "~ Tilde" },
  { value: "-", label: "- Hyphen" },
  { value: "_", label: "_ Underscore" },
];

function readStoredConfig(): PseudoConfig | null {
  if (typeof localStorage === "undefined") return null;
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as PseudoConfig & { accent?: boolean };
    // Migrate older `accent: false` stored configs to `alphabet: "none"`.
    if (parsed.accent === false && !parsed.alphabet) parsed.alphabet = "none";
    delete parsed.accent;
    return parsed;
  } catch {
    localStorage.removeItem(STORAGE_KEY);
    return null;
  }
}

function persist(cfg: PseudoConfig | null): void {
  if (cfg === null) localStorage.removeItem(STORAGE_KEY);
  else localStorage.setItem(STORAGE_KEY, JSON.stringify(cfg));
}

export function DevPseudoCard() {
  // Production builds get nothing — both the UI and the runtime
  // pseudo module are stripped by the bundler.
  if (!import.meta.env.DEV) return null;

  const initial = readStoredConfig();
  const [enabled, setEnabled] = useState(initial !== null);
  const [prefix, setPrefix] = useState(initial?.prefix ?? DEFAULT_PREFIX);
  const [suffix, setSuffix] = useState(initial?.suffix ?? DEFAULT_SUFFIX);
  const [expansion, setExpansion] = useState(initial?.expansion ?? 0);
  const [expansionChar, setExpansionChar] = useState(
    initial?.expansionChar ?? DEFAULT_EXPANSION_CHAR,
  );
  const [alphabet, setAlphabet] = useState<AlphabetName>(initial?.alphabet ?? "accented");
  const [setMode, setSetMode] = useState<SetMode | null>(null);

  // Dynamic import so the accent map (+ pseudo-translate code) only
  // loads in the dev bundle. Runs once per SettingsPage mount;
  // repeated mounts hit the module cache.
  useEffect(() => {
    let alive = true;
    void import("@neokapi/kapi-react/runtime/pseudo").then((mod) => {
      if (alive) setSetMode(() => mod.setPseudoMode);
    });
    return () => {
      alive = false;
    };
  }, []);

  const apply = useCallback(
    (next: PseudoConfig | null) => {
      if (!setMode) return;
      setMode(next);
      persist(next);
    },
    [setMode],
  );

  // Push live updates whenever any of the tuning controls changes
  // AND pseudo is enabled. When disabled, we clear.
  useEffect(() => {
    if (!setMode) return;
    if (!enabled) {
      apply(null);
      return;
    }
    apply({ prefix, suffix, expansion, expansionChar, alphabet });
  }, [setMode, enabled, prefix, suffix, expansion, expansionChar, alphabet, apply]);

  const previewSample = enabled
    ? applyPreview("Sample translatable string", {
        prefix,
        suffix,
        expansion,
        expansionChar,
        alphabet,
      })
    : "Sample translatable string";

  return (
    // translate="no" on the card root keeps the pseudo transform
    // from recursively accenting the panel's own labels.
    <Card translate="no">
      <CardContent className="space-y-4 p-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-sm font-medium">
              <FlaskConical size={14} className="text-muted-foreground" />
              Runtime pseudo-translation
              <span className="rounded bg-muted px-1.5 py-0.5 text-[9px] uppercase tracking-wider text-muted-foreground">
                dev
              </span>
            </div>
            <p className="mt-1 text-[10px] text-muted-foreground">
              Applies pseudo to every translatable string on the fly — works without a loaded
              catalog. Useful for QAing layout expansion and spotting strings that bypass
              translation.
            </p>
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label="Enable runtime pseudo"
          />
        </div>

        <div
          className={`space-y-4 transition-opacity ${
            enabled ? "opacity-100" : "pointer-events-none opacity-40"
          }`}
        >
          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Alphabet</Label>
            <select
              value={alphabet}
              disabled={!enabled}
              onChange={(e) => setAlphabet(e.target.value as AlphabetName)}
              className="h-8 w-full rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {ALPHABET_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label} — {opt.sample}
                </option>
              ))}
            </select>
            <p className="mt-1 text-[10px] text-muted-foreground">
              Wobbly uses a varied mix of diacritics per letter — good for noticing layout that
              depends on consistent letter heights.
            </p>
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between">
              <Label className="text-xs text-muted-foreground">Expansion</Label>
              <span className="text-[10px] font-medium tabular-nums text-foreground">
                +{expansion}%
              </span>
            </div>
            <input
              type="range"
              min={0}
              max={100}
              step={5}
              value={expansion}
              disabled={!enabled}
              onChange={(e) => setExpansion(parseInt(e.target.value, 10))}
              className="w-full accent-primary"
              aria-label="Expansion percent"
            />
            <p className="mt-1 text-[10px] text-muted-foreground">
              Inserts a filler character between source characters so the added length shows up
              inside words, not just tacked on the end.
            </p>
          </div>

          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Expansion character</Label>
            <div className="flex gap-2">
              <select
                value={
                  EXPANSION_CHAR_OPTIONS.some((o) => o.value === expansionChar)
                    ? expansionChar
                    : "__custom__"
                }
                disabled={!enabled}
                onChange={(e) => {
                  if (e.target.value !== "__custom__") setExpansionChar(e.target.value);
                }}
                className="h-8 flex-1 rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {EXPANSION_CHAR_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
                <option value="__custom__">Custom…</option>
              </select>
              <Input
                value={expansionChar}
                onChange={(e) => setExpansionChar(e.target.value)}
                disabled={!enabled}
                className="w-20 text-center font-mono text-xs"
                aria-label="Expansion character override"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">Prefix</Label>
              <Input
                value={prefix}
                onChange={(e) => setPrefix(e.target.value)}
                disabled={!enabled}
                className="font-mono text-xs"
              />
            </div>
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">Suffix</Label>
              <Input
                value={suffix}
                onChange={(e) => setSuffix(e.target.value)}
                disabled={!enabled}
                className="font-mono text-xs"
              />
            </div>
          </div>

          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Preview</Label>
            <div className="rounded-md border border-border bg-muted/40 p-2 font-mono text-xs">
              {previewSample}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

// Mirror of the transform in @neokapi/kapi-react/runtime/pseudo —
// duplicated here only to render the preview without forcing a
// dynamic import on every keystroke. Stays in lockstep with the
// real transform because both use the same constants + approach.
const ACCENTED: Record<string, string> = {
  a: "\u00e0",
  b: "\u0183",
  c: "\u00e7",
  d: "\u0111",
  e: "\u00e9",
  f: "\u0192",
  g: "\u011d",
  h: "\u0125",
  i: "\u00ee",
  j: "\u0135",
  k: "\u0137",
  l: "\u013c",
  m: "\u1e3f",
  n: "\u00f1",
  o: "\u00f6",
  p: "\u00fe",
  q: "\u01eb",
  r: "\u0155",
  s: "\u0161",
  t: "\u0163",
  u: "\u00fc",
  v: "\u1e7d",
  w: "\u0175",
  x: "\u1e8b",
  y: "\u00fd",
  z: "\u017e",
  A: "\u00c0",
  B: "\u0182",
  C: "\u00c7",
  D: "\u0110",
  E: "\u00c9",
  F: "\u0191",
  G: "\u011c",
  H: "\u0124",
  I: "\u00ce",
  J: "\u0134",
  K: "\u0136",
  L: "\u013b",
  M: "\u1e3e",
  N: "\u00d1",
  O: "\u00d6",
  P: "\u00de",
  Q: "\u01ea",
  R: "\u0154",
  S: "\u0160",
  T: "\u0162",
  U: "\u00dc",
  V: "\u1e7c",
  W: "\u0174",
  X: "\u1e8a",
  Y: "\u00dd",
  Z: "\u017d",
};
const WOBBLY: Record<string, string> = {
  a: "\u0101",
  b: "\u1e03",
  c: "\u0109",
  d: "\u010f",
  e: "\u011b",
  f: "\u1e1f",
  g: "\u01e7",
  h: "\u1e27",
  i: "\u01d0",
  j: "\u0135",
  k: "\u01e9",
  l: "\u013a",
  m: "\u1e41",
  n: "\u01f9",
  o: "\u01d2",
  p: "\u1e55",
  q: "\u024b",
  r: "\u0159",
  s: "\u0161",
  t: "\u0165",
  u: "\u01d4",
  v: "\u1e7d",
  w: "\u0175",
  x: "\u1e8d",
  y: "\u1ef3",
  z: "\u017e",
  A: "\u0100",
  B: "\u1e02",
  C: "\u0108",
  D: "\u010e",
  E: "\u011a",
  F: "\u1e1e",
  G: "\u01e6",
  H: "\u1e26",
  I: "\u01cf",
  J: "\u0134",
  K: "\u01e8",
  L: "\u0139",
  M: "\u1e40",
  N: "\u01f8",
  O: "\u01d1",
  P: "\u1e54",
  Q: "\u024a",
  R: "\u0158",
  S: "\u0160",
  T: "\u0164",
  U: "\u01d3",
  V: "\u1e7c",
  W: "\u0174",
  X: "\u1e8c",
  Y: "\u1ef2",
  Z: "\u017d",
};
const ALPHABETS: Record<AlphabetName, Record<string, string>> = {
  accented: ACCENTED,
  wobbly: WOBBLY,
  none: {},
};

function applyPreview(text: string, cfg: Required<PseudoConfig>): string {
  const alphabet = ALPHABETS[cfg.alphabet];
  const rate = Math.max(0, Math.min(100, cfg.expansion)) / 100;
  let out = "";
  let depth = 0;
  let accum = 0;
  for (const ch of text) {
    if (ch === "{") {
      depth++;
      out += ch;
      continue;
    }
    if (ch === "}") {
      if (depth > 0) depth--;
      out += ch;
      continue;
    }
    if (depth > 0) {
      out += ch;
      continue;
    }
    out += alphabet[ch] ?? ch;
    if (rate > 0 && cfg.expansionChar) {
      accum += rate;
      while (accum >= 1) {
        out += cfg.expansionChar;
        accum -= 1;
      }
    }
  }
  return cfg.prefix + out + cfg.suffix;
}
