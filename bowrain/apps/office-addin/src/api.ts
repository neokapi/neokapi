// Typed client for the Bowrain add-in REST API (bowrain/addin). The task pane is
// served from the Bowrain web origin, so the API is reached at a same-origin
// relative path — no CORS, no configured host.

const API_BASE = "/api/v1/addin";

export interface Finding {
  category: string;
  severity: string;
  message: string;
  suggestion?: string;
  original_text?: string;
}

export interface CheckResult {
  profile: string;
  score: number;
  findings: Finding[];
}

export interface TermHit {
  term: string;
  status: string; // preferred | forbidden | competitor
  replacement?: string;
  note?: string;
  severity?: string;
}

export interface TermsResult {
  profile: string;
  matches: TermHit[];
}

export interface TranslateResult {
  translation: string;
  source_locale: string;
  target_locale: string;
  provider: string;
}

async function post<T>(path: string, body: unknown, token?: string): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const j = (await res.json()) as { error?: string };
      if (j.error) detail = j.error;
    } catch {
      // non-JSON error body; keep status text
    }
    throw new Error(`Bowrain API ${res.status}: ${detail}`);
  }
  return (await res.json()) as T;
}

export function checkBrand(text: string, token?: string, profile?: string): Promise<CheckResult> {
  return post<CheckResult>("/check", { text, profile }, token);
}

export function lookupTerms(text: string, token?: string, profile?: string): Promise<TermsResult> {
  return post<TermsResult>("/terms", { text, profile }, token);
}

export function translate(
  text: string,
  targetLocale: string,
  token?: string,
  profile?: string,
): Promise<TranslateResult> {
  return post<TranslateResult>("/translate", { text, target_locale: targetLocale, profile }, token);
}
