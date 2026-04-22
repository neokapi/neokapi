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
 * transform would recursively accent the UI's own labels. The
 * kapi-react plugin inherits that setting down the subtree (matching
 * the W3C `translate` attribute spec), so every descendant label is
 * exempt.
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

const SAMPLE_TEXT = "Sample translatable string";

type SetMode = (cfg: PseudoConfig | null) => void;

const ALPHABET_OPTIONS: ReadonlyArray<{
  value: AlphabetName;
  label: string;
  sample: string;
}> = [
  { value: "accented", label: "Accented (uniform)", sample: "Ĥéļļö ŵöŕļđ" },
  {
    value: "wobbly",
    label: "Wobbly (mixed tilt)",
    // "Hello world" rendered with the cycle italic→sans→script, so
    // each letter sits at a visibly different angle.
    sample:
      "\u{1D5A7}\u{1D4BE}\u{1D463}\u{1D5CA}\u{1D4C1} " +
      "\u{1D5C1}\u{1D4C1}\u{1D5BB}\u{1D4BE}\u{1D5BE}",
  },
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

// Paired prefix/suffix presets. The dropdown selects a wrapper
// style; picking one updates both the prefix and suffix inputs.
// Custom values (anything not in the list) are preserved and shown
// as "Custom…" in the select.
const WRAPPER_OPTIONS: ReadonlyArray<{
  value: string;
  label: string;
  prefix: string;
  suffix: string;
}> = [
  { value: "shade", label: "▒ text ▒  Medium shade", prefix: "\u2592 ", suffix: " \u2592" },
  { value: "square", label: "[ text ]  Square brackets", prefix: "[", suffix: "]" },
  { value: "guillemets", label: "« text »  Guillemets", prefix: "\u00ab ", suffix: " \u00bb" },
  {
    value: "single-guillemets",
    label: "‹ text ›  Single guillemets",
    prefix: "\u2039 ",
    suffix: " \u203a",
  },
  { value: "angle", label: "⟨ text ⟩  Angle brackets", prefix: "\u27e8 ", suffix: " \u27e9" },
  { value: "double-angle", label: "⟪ text ⟫  Double angle", prefix: "\u27ea ", suffix: " \u27eb" },
  { value: "math", label: "⟦ text ⟧  White square", prefix: "\u27e6 ", suffix: " \u27e7" },
  { value: "corner", label: "「 text 」  Corner brackets", prefix: "\u300c", suffix: "\u300d" },
  {
    value: "lenticular",
    label: "【 text 】  Black lenticular",
    prefix: "\u3010",
    suffix: "\u3011",
  },
  { value: "pipes", label: "| text |  Pipes", prefix: "| ", suffix: " |" },
  { value: "none", label: "No wrapper (bare)", prefix: "", suffix: "" },
];

function resolveWrapperPreset(prefix: string, suffix: string): string {
  const match = WRAPPER_OPTIONS.find((w) => w.prefix === prefix && w.suffix === suffix);
  return match?.value ?? "__custom__";
}

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
    ? applyPreview(SAMPLE_TEXT, {
        prefix,
        suffix,
        expansion,
        expansionChar,
        alphabet,
      })
    : SAMPLE_TEXT;

  return (
    // translate="no" on the card root opts the whole subtree out of
    // runtime pseudo-translation. Inheritance is rock-solid: the
    // kapi-react plugin walks self + ancestors and the nearest
    // explicit `translate` setting wins, matching the W3C spec. A
    // descendant can opt back in with `translate="yes"`.
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
              Wobbly cycles italic (right slant) → sans (upright) → script (flowy) per letter so
              adjacent characters sit at different angles and weights.
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

          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Wrapper</Label>
            <select
              value={resolveWrapperPreset(prefix, suffix)}
              disabled={!enabled}
              onChange={(e) => {
                if (e.target.value === "__custom__") return;
                const preset = WRAPPER_OPTIONS.find((w) => w.value === e.target.value);
                if (preset) {
                  setPrefix(preset.prefix);
                  setSuffix(preset.suffix);
                }
              }}
              className="h-8 w-full rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {WRAPPER_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
              <option value="__custom__">Custom…</option>
            </select>
            <p className="mt-1 text-[10px] text-muted-foreground">
              Paired brackets or markers around every pseudo-translated string. Pick a preset or
              edit prefix / suffix directly below for anything custom.
            </p>
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
// Mirror of the WOBBLY map in kapi-react/runtime/pseudo. Cycles
// three Mathematical Alphanumeric blocks per letter (italic → sans
// → script) so the preview renders with visibly mixed angles and
// weights. Reserved codepoints use their Letterlike-Symbols
// substitutes. Astral escapes iterate correctly via `for…of`.
const WOBBLY: Record<string, string> = {
  a: "\u{1D44E}",
  b: "\u{1D5BB}",
  c: "\u{1D4B8}",
  d: "\u{1D451}",
  e: "\u{1D5BE}",
  f: "\u{1D4BB}",
  g: "\u{1D454}",
  h: "\u{1D5C1}",
  i: "\u{1D4BE}",
  j: "\u{1D457}",
  k: "\u{1D5C4}",
  l: "\u{1D4C1}",
  m: "\u{1D45A}",
  n: "\u{1D5C7}",
  o: "\u{2134}",
  p: "\u{1D45D}",
  q: "\u{1D5CA}",
  r: "\u{1D4C7}",
  s: "\u{1D460}",
  t: "\u{1D5CD}",
  u: "\u{1D4CA}",
  v: "\u{1D463}",
  w: "\u{1D5D0}",
  x: "\u{1D4CD}",
  y: "\u{1D466}",
  z: "\u{1D5D3}",
  A: "\u{1D434}",
  B: "\u{1D5A1}",
  C: "\u{1D49E}",
  D: "\u{1D437}",
  E: "\u{1D5A4}",
  F: "\u{2131}",
  G: "\u{1D43A}",
  H: "\u{1D5A7}",
  I: "\u{2110}",
  J: "\u{1D43D}",
  K: "\u{1D5AA}",
  L: "\u{2112}",
  M: "\u{1D440}",
  N: "\u{1D5AD}",
  O: "\u{1D4AA}",
  P: "\u{1D443}",
  Q: "\u{1D5B0}",
  R: "\u{211B}",
  S: "\u{1D446}",
  T: "\u{1D5B3}",
  U: "\u{1D4B0}",
  V: "\u{1D449}",
  W: "\u{1D5B6}",
  X: "\u{1D4B3}",
  Y: "\u{1D44C}",
  Z: "\u{1D5B9}",
};
const ALPHABETS: Record<AlphabetName, Record<string, string>> = {
  accented: ACCENTED,
  wobbly: WOBBLY,
  none: {},
};

// Mirror of `pseudoTransform` in kapi-react/runtime/pseudo. Kept in
// sync manually (one file, ~40 lines) so live preview updates
// without forcing a dynamic import on every keystroke.
function applyPreview(text: string, cfg: Required<PseudoConfig>): string {
  const alphabet = ALPHABETS[cfg.alphabet];
  const expansion = Math.max(0, Math.min(100, cfg.expansion));
  const chars: string[] = [];
  const isLetter: boolean[] = [];
  const isSpace: boolean[] = [];
  let depth = 0;
  for (const ch of text) {
    if (ch === "{") {
      depth++;
      chars.push(ch);
      isLetter.push(false);
      isSpace.push(false);
      continue;
    }
    if (ch === "}") {
      if (depth > 0) depth--;
      chars.push(ch);
      isLetter.push(false);
      isSpace.push(false);
      continue;
    }
    if (depth > 0) {
      chars.push(ch);
      isLetter.push(false);
      isSpace.push(false);
      continue;
    }
    const ws = ch === " " || ch === "\t" || ch === "\n";
    chars.push(alphabet[ch] ?? ch);
    isLetter.push(!ws);
    isSpace.push(ws);
  }

  let body = "";
  if (expansion === 0 || !cfg.expansionChar) {
    body = chars.join("");
  } else if (expansion >= 100) {
    const rate = expansion / 100;
    let accum = 0;
    for (let i = 0; i < chars.length; i++) {
      body += chars[i];
      if (!isLetter[i]) continue;
      accum += rate;
      while (accum >= 1) {
        body += cfg.expansionChar;
        accum -= 1;
      }
    }
  } else {
    const letterCount = isLetter.reduce((n, b) => n + (b ? 1 : 0), 0);
    const wanted = Math.round((letterCount * expansion) / 100);
    const spaceIndices: number[] = [];
    for (let i = 0; i < isSpace.length; i++) if (isSpace[i]) spaceIndices.push(i);
    const insertAfter: number[] = Array.from({ length: chars.length }, () => 0);
    let leading = 0;
    let trailing = 0;
    if (spaceIndices.length === 0) {
      leading = Math.ceil(wanted / 2);
      trailing = wanted - leading;
    } else if (wanted <= spaceIndices.length) {
      for (let k = 0; k < wanted; k++) {
        const idx = spaceIndices[Math.floor((k * spaceIndices.length) / wanted)];
        insertAfter[idx] += 1;
      }
    } else {
      for (const idx of spaceIndices) insertAfter[idx] += 1;
      const remaining = wanted - spaceIndices.length;
      leading = Math.ceil(remaining / 2);
      trailing = remaining - leading;
    }
    body += cfg.expansionChar.repeat(leading);
    for (let i = 0; i < chars.length; i++) {
      body += chars[i];
      if (insertAfter[i] > 0) body += cfg.expansionChar.repeat(insertAfter[i]);
    }
    body += cfg.expansionChar.repeat(trailing);
  }
  return cfg.prefix + body + cfg.suffix;
}
