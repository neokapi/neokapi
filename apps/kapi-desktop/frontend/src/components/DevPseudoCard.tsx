/**
 * Dev-only control panel for runtime pseudo-translation.
 *
 * Talks to `@neokapi/kapi-react/runtime/pseudo` via a dynamic import
 * so the accent map never ships in production builds. Renders
 * nothing at all outside `import.meta.env.DEV`.
 *
 * State shape lives in localStorage under the same key the toggle in
 * `main.tsx` writes (`kapi.dev.pseudo`), so the console handle
 * (`window.kapi.pseudo(...)`) and this UI stay in sync across reloads.
 */
import { useCallback, useEffect, useState } from "react";
import { FlaskConical } from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";
import { Card, CardContent, Switch, Label, Input } from "@neokapi/ui-primitives";

interface PseudoConfig {
  prefix?: string;
  suffix?: string;
  expansion?: number;
  accent?: boolean;
}

const STORAGE_KEY = "kapi.dev.pseudo";
const DEFAULT_PREFIX = "\u2592 "; // ▒ + space
const DEFAULT_SUFFIX = " \u2592";

type SetMode = (cfg: PseudoConfig | null) => void;

function readStoredConfig(): PseudoConfig | null {
  if (typeof localStorage === "undefined") return null;
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as PseudoConfig;
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
  const [accent, setAccent] = useState(initial?.accent ?? true);
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
    apply({ prefix, suffix, expansion, accent });
  }, [setMode, enabled, prefix, suffix, expansion, accent, apply]);

  const previewSample = enabled
    ? applyPreview("Sample translatable string", { prefix, suffix, expansion, accent })
    : "Sample translatable string";

  return (
    <Card>
      <CardContent className="space-y-4 p-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-sm font-medium">
              <FlaskConical size={14} className="text-muted-foreground" />
              {t("Runtime pseudo-translation")}
              <span
                className="rounded bg-muted px-1.5 py-0.5 text-[9px] uppercase tracking-wider text-muted-foreground"
                translate="no"
              >
                dev
              </span>
            </div>
            <p className="mt-1 text-[10px] text-muted-foreground">
              {t(
                "Applies pseudo to every translatable string on the fly — works without a loaded catalog. Useful for QAing layout expansion and spotting strings that bypass translation.",
              )}
            </p>
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label={t("Enable runtime pseudo")}
          />
        </div>

        <div
          className={`space-y-4 transition-opacity ${enabled ? "opacity-100" : "pointer-events-none opacity-40"}`}
        >
          <div>
            <div className="mb-2 flex items-center justify-between">
              <Label className="text-xs text-muted-foreground">{t("Expansion")}</Label>
              <span className="text-[10px] font-medium tabular-nums text-foreground" translate="no">
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
              aria-label={t("Expansion percent")}
            />
            <p className="mt-1 text-[10px] text-muted-foreground">
              {t("Extra padding appended to each string, relative to its length.")}
            </p>
          </div>

          <div className="flex items-start justify-between gap-4">
            <div className="flex-1">
              <Label className="mb-1 block text-xs text-muted-foreground">
                {t("Accent ASCII letters")}
              </Label>
              <p className="text-[10px] text-muted-foreground">
                {t("Replace a–z / A–Z with accented equivalents. Turn off for padding-only mode.")}
              </p>
            </div>
            <Switch
              checked={accent}
              onCheckedChange={setAccent}
              aria-label={t("Toggle accent pass")}
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">{t("Prefix")}</Label>
              <Input
                value={prefix}
                onChange={(e) => setPrefix(e.target.value)}
                disabled={!enabled}
                className="font-mono text-xs"
                translate="no"
              />
            </div>
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">{t("Suffix")}</Label>
              <Input
                value={suffix}
                onChange={(e) => setSuffix(e.target.value)}
                disabled={!enabled}
                className="font-mono text-xs"
                translate="no"
              />
            </div>
          </div>

          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">{t("Preview")}</Label>
            <div
              className="rounded-md border border-border bg-muted/40 p-2 font-mono text-xs"
              translate="no"
            >
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
const ACCENT_MAP: Record<string, string> = {
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

function applyPreview(text: string, cfg: Required<PseudoConfig>): string {
  let out = "";
  let depth = 0;
  let textLen = 0;
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
    out += cfg.accent ? (ACCENT_MAP[ch] ?? ch) : ch;
    textLen++;
  }
  if (cfg.expansion > 0 && textLen > 0) {
    const pad = Math.floor((textLen * cfg.expansion) / 100);
    if (pad > 0) out += " " + "~".repeat(pad);
  }
  return cfg.prefix + out + cfg.suffix;
}
