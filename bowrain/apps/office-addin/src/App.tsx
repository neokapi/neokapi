import { useCallback, useEffect, useState } from "react";
import { checkBrand, lookupTerms, translate, type CheckResult, type TermsResult } from "./api";
import { getAccessToken, getSelectedText, replaceSelection, waitForOffice } from "./office";

const LANGUAGES: Array<{ label: string; value: string }> = [
  { label: "French (fr)", value: "fr" },
  { label: "German (de)", value: "de" },
  { label: "Spanish (es)", value: "es" },
  { label: "Japanese (ja)", value: "ja" },
  { label: "Portuguese (pt)", value: "pt" },
];

type Status = { kind: "idle" | "busy" | "error" | "ok"; message?: string };

export default function App() {
  const [host, setHost] = useState<string | null>(null);
  const [target, setTarget] = useState("fr");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [check, setCheck] = useState<CheckResult | null>(null);
  const [terms, setTerms] = useState<TermsResult | null>(null);

  useEffect(() => {
    void waitForOffice().then(setHost);
  }, []);

  const scan = useCallback(async () => {
    setStatus({ kind: "busy", message: "Reading selection…" });
    try {
      const text = await getSelectedText();
      if (!text.trim()) {
        setStatus({ kind: "error", message: "Select some text in the document first." });
        return;
      }
      const token = await getAccessToken();
      const [c, t] = await Promise.all([checkBrand(text, token), lookupTerms(text, token)]);
      setCheck(c);
      setTerms(t);
      setStatus({ kind: "ok", message: `Scored ${c.score}/100 against "${c.profile}".` });
    } catch (err) {
      setStatus({ kind: "error", message: (err as Error).message });
    }
  }, []);

  const runTranslate = useCallback(async () => {
    setStatus({ kind: "busy", message: "Translating selection…" });
    try {
      const text = await getSelectedText();
      if (!text.trim()) {
        setStatus({ kind: "error", message: "Select some text to translate." });
        return;
      }
      const token = await getAccessToken();
      const res = await translate(text, target, token);
      await replaceSelection(res.translation);
      setStatus({ kind: "ok", message: `Translated to ${target} (${res.provider}).` });
    } catch (err) {
      setStatus({ kind: "error", message: (err as Error).message });
    }
  }, [target]);

  return (
    <div className="pane">
      <header className="pane-header">
        <h1>Bowrain</h1>
        <p className="subtitle">
          Brand voice · terminology · translation{host ? ` · ${host}` : ""}
        </p>
      </header>

      <section className="actions">
        <button type="button" className="btn btn-secondary" onClick={() => void scan()}>
          Scan selection
        </button>
        <div className="translate-row">
          <select
            aria-label="Target language"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          >
            {LANGUAGES.map((l) => (
              <option key={l.value} value={l.value}>
                {l.label}
              </option>
            ))}
          </select>
          <button type="button" className="btn btn-primary" onClick={() => void runTranslate()}>
            Translate
          </button>
        </div>
      </section>

      {status.message ? <p className={`status status-${status.kind}`}>{status.message}</p> : null}

      {check ? (
        <section className="results">
          <h2>
            Brand voice <span className="score">{check.score}/100</span>
          </h2>
          {check.findings.length === 0 ? (
            <p className="ok-line">No brand-voice issues found.</p>
          ) : (
            <ul className="findings">
              {check.findings.map((f, i) => (
                <li key={i} className={`finding sev-${f.severity}`}>
                  <span className="finding-cat">
                    {f.category} · {f.severity}
                  </span>
                  <span className="finding-msg">{f.message}</span>
                  {f.suggestion ? (
                    <span className="finding-fix">Suggest: {f.suggestion}</span>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </section>
      ) : null}

      {terms && terms.matches.length > 0 ? (
        <section className="results">
          <h2>Terminology</h2>
          <ul className="terms">
            {terms.matches.map((m, i) => (
              <li key={i} className={`term term-${m.status}`}>
                <strong>{m.term}</strong> <em>{m.status}</em>
                {m.replacement ? <span> → {m.replacement}</span> : null}
                {m.note ? <span className="term-note"> ({m.note})</span> : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
