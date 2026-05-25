/**
 * Render kapi JSON outputs into self-contained HTML "report cards" that read well
 * on screen. Used by the artifact capture step (Playwright screenshots these).
 */

const SHELL = (title: string, body: string) => `<!doctype html><html><head><meta charset="utf-8">
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; font-family: -apple-system, "SF Pro Text", Inter, system-ui, sans-serif;
    background: radial-gradient(1200px 700px at 70% -10%, #1b2440 0%, #0c1020 60%, #080a14 100%);
    color: #e8ecf6; padding: 64px; min-height: 100vh; }
  .wrap { max-width: 1000px; margin: 0 auto; }
  h1 { font-size: 30px; font-weight: 650; margin: 0 0 4px; letter-spacing: -0.01em; }
  .sub { color: #8c98b8; font-size: 16px; margin-bottom: 32px; }
  .card { background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08);
    border-radius: 18px; padding: 28px 32px; margin-bottom: 18px; backdrop-filter: blur(8px); }
  .row { display: flex; align-items: center; gap: 24px; }
  .gauge { width: 132px; height: 132px; border-radius: 50%; display: grid; place-items: center; flex: none; }
  .gauge .num { font-size: 40px; font-weight: 750; }
  .gauge .den { font-size: 15px; color: #aab4d0; }
  .finding { display: flex; gap: 14px; padding: 14px 0; border-top: 1px solid rgba(255,255,255,0.07); }
  .sev { font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em;
    padding: 4px 10px; border-radius: 999px; height: fit-content; }
  .sev.error { background: #4a1620; color: #ff9aa8; }
  .sev.warning { background: #4a3712; color: #ffd479; }
  .sev.info, .sev.suggestion { background: #14304a; color: #8fd0ff; }
  .ftext { font-size: 15px; line-height: 1.5; }
  .ftext .orig { color: #ff9aa8; text-decoration: line-through; }
  .ftext .sug { color: #8fe3a0; }
  table { width: 100%; border-collapse: collapse; font-size: 15px; }
  th, td { text-align: left; padding: 12px 14px; border-bottom: 1px solid rgba(255,255,255,0.07); }
  th { color: #8c98b8; font-weight: 600; font-size: 12px; text-transform: uppercase; letter-spacing: 0.05em; }
  .pill { font-family: ui-monospace, "SF Mono", Menlo, monospace; background: rgba(255,255,255,0.06);
    padding: 2px 8px; border-radius: 6px; font-size: 13px; }
  .ok { color: #8fe3a0; } .bad { color: #ff9aa8; }
  .big { font-size: 56px; font-weight: 750; letter-spacing: -0.02em; }
  .label { color: #8c98b8; font-size: 15px; }
  .brandmark { position: absolute; top: 40px; right: 56px; font-weight: 700; color: #6f7da0; letter-spacing: 0.04em; }
  pre { font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 14px; line-height: 1.55;
    white-space: pre-wrap; word-break: break-word; color: #cfe0ff; margin: 0; }
</style></head>
<body><div class="brandmark">kapi</div><div class="wrap">${body}</div></body></html>`;

function gaugeColor(score: number): string {
  if (score >= 85) return "#1f7a44";
  if (score >= 60) return "#9a7012";
  return "#7a1f2b";
}

function renderBrand(data: any): string {
  const score = Number(data.score ?? data.overall_score ?? 0);
  const findings: any[] = data.findings ?? data.issues ?? [];
  const fhtml = findings
    .slice(0, 8)
    .map((f) => {
      const sev = String(f.severity ?? "info").toLowerCase();
      const orig = f.original_text ?? f.original ?? f.text ?? "";
      const sug = f.suggestion ?? f.replacement ?? "";
      return `<div class="finding"><span class="sev ${sev}">${sev}</span>
        <div class="ftext"><span class="orig">${esc(orig)}</span>${sug ? ` → <span class="sug">${esc(sug)}</span>` : ""}
        ${f.message ? `<div style="color:#aab4d0;margin-top:4px">${esc(f.message)}</div>` : ""}</div></div>`;
    })
    .join("");
  return SHELL(
    "Brand check",
    `<h1>Brand voice check</h1><div class="sub">kapi brand check · deterministic, offline</div>
     <div class="card"><div class="row">
       <div class="gauge" style="background:conic-gradient(${gaugeColor(score)} ${score * 3.6}deg, rgba(255,255,255,0.08) 0)">
         <div class="gauge" style="width:108px;height:108px;background:#0c1020;flex-direction:column">
           <div class="num">${score}</div><div class="den">/ 100</div></div></div>
       <div><div class="label">on-brand score</div>
         <div style="font-size:20px;margin-top:6px">${
           score >= 85 ? "On brand ✓" : score >= 60 ? "Needs a pass" : "Off brand"
         }</div>
         <div class="label" style="margin-top:6px">${findings.length} finding${findings.length === 1 ? "" : "s"}</div></div>
     </div></div>
     ${findings.length ? `<div class="card"><div class="label" style="margin-bottom:6px">Findings</div>${fhtml}</div>` : ""}`,
  );
}

function renderTermCheck(data: any): string {
  const issues: any[] = data.issues ?? data.findings ?? (Array.isArray(data) ? data : []);
  const rows = issues
    .slice(0, 12)
    .map(
      (it) =>
        `<tr><td><span class="pill bad">${esc(it.found ?? it.actual ?? it.term ?? "")}</span></td>
         <td><span class="pill ok">${esc(it.expected ?? it.preferred ?? it.suggestion ?? "")}</span></td>
         <td>${esc(it.message ?? it.rule ?? it.severity ?? "terminology")}</td></tr>`,
    )
    .join("");
  return SHELL(
    "Terminology",
    `<h1>Terminology check</h1><div class="sub">kapi term-check · glossary enforcement</div>
     <div class="card"><table><thead><tr><th>Found</th><th>Preferred</th><th>Note</th></tr></thead>
       <tbody>${rows || `<tr><td colspan="3" class="ok">No terminology issues — all approved terms ✓</td></tr>`}</tbody></table></div>`,
  );
}

function renderWordCount(data: any): string {
  const words = data.words ?? data.word_count ?? data.total_words ?? data.totalWords ?? 0;
  const segs = data.segments ?? data.segment_count ?? data.blocks ?? 0;
  const chars = data.characters ?? data.char_count ?? data.chars ?? 0;
  return SHELL(
    "Word count",
    `<h1>Translation scope</h1><div class="sub">kapi word-count · planning a localization job</div>
     <div class="card"><div style="display:flex;gap:56px">
       <div><div class="big">${Number(words).toLocaleString()}</div><div class="label">translatable words</div></div>
       <div><div class="big">${Number(segs).toLocaleString()}</div><div class="label">segments</div></div>
       ${chars ? `<div><div class="big">${Number(chars).toLocaleString()}</div><div class="label">characters</div></div>` : ""}
     </div></div>`,
  );
}

function renderGlossary(data: any): string {
  const pairs: any[] = Array.isArray(data) ? data : (data.terms ?? data.pairs ?? []);
  const rows = pairs
    .slice(0, 14)
    .map((p) => {
      const src = p.en ?? p.source ?? p.term ?? p.src ?? Object.values(p)[0];
      const tgt = p.fr ?? p.target ?? p.translation ?? p.approved ?? Object.values(p)[1];
      return `<tr><td><span class="pill">${esc(src)}</span></td><td>→</td><td><span class="pill ok">${esc(tgt)}</span></td></tr>`;
    })
    .join("");
  return SHELL(
    "Glossary",
    `<h1>Approved terminology</h1><div class="sub">kapi termbase · the wording every translation must use</div>
     <div class="card"><table><thead><tr><th>Source (en)</th><th></th><th>Approved term</th></tr></thead>
       <tbody>${rows}</tbody></table></div>`,
  );
}

function renderCatalog(data: any, title: string, sub: string): string {
  const entries = Object.entries(data).slice(0, 16);
  const rows = entries
    .map(([k, v]) => `<tr><td><span class="pill">${esc(k)}</span></td><td class="ok">${esc(v)}</td></tr>`)
    .join("");
  return SHELL(
    title,
    `<h1>${esc(title)}</h1><div class="sub">${esc(sub)}</div>
     <div class="card"><table><thead><tr><th>key</th><th>value</th></tr></thead><tbody>${rows}</tbody></table></div>`,
  );
}

function renderJson(data: any, title = "kapi output"): string {
  return SHELL(title, `<h1>${esc(title)}</h1><div class="card"><pre>${esc(JSON.stringify(data, null, 2))}</pre></div>`);
}

/** Lightweight, dependency-free TSX/TS syntax highlighter (single-pass tokenizer). */
function highlightCode(code: string): string {
  const C = { comment: "#6b7794", str: "#9ae6b4", kw: "#c792ea", tag: "#7aa2ff", num: "#f78c6c" };
  const e = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  // Order matters: comments, strings, JSX tags, keywords, numbers.
  const re =
    /(\/\/[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:[^`\\]|\\.)*`|"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')|(<\/?[A-Za-z][\w.-]*|\/?>)|\b(export|default|function|return|const|let|var|import|from|interface|type|extends|implements|new|async|await|if|else|for|of|in|class|null|true|false|void)\b|\b(\d+(?:\.\d+)?)\b/g;
  let out = "";
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(code))) {
    out += e(code.slice(last, m.index));
    const c = m[1] ? C.comment : m[2] ? C.str : m[3] ? C.tag : m[4] ? C.kw : C.num;
    out += `<span style="color:${c}">${e(m[0])}</span>`;
    last = m.index + m[0].length;
  }
  out += e(code.slice(last));
  return out;
}

/** Render a source file as a styled, syntax-highlighted editor card. */
function renderCode(code: string, title: string, sub: string): string {
  return SHELL(
    title,
    `<h1>${esc(title)}</h1>${sub ? `<div class="sub">${esc(sub)}</div>` : ""}
     <div class="card" style="background:#0d1117;border-color:rgba(255,255,255,0.10);padding:0;overflow:hidden">
       <div style="padding:11px 20px;border-bottom:1px solid rgba(255,255,255,0.08);color:#8c98b8;
         font-family:ui-monospace,Menlo,monospace;font-size:13px">${esc(title)}</div>
       <pre style="padding:24px 28px;font-size:15px;line-height:1.75;color:#cdd6f4;white-space:pre;margin:0">${highlightCode(
         code,
       )}</pre>
     </div>`,
  );
}

export { renderCode };

/**
 * Per-line status for a before/after pair via an LCS line diff. Lines present in
 * both (trimmed-equal) are "same"; lines only in `before` are "removed"; lines only
 * in `after` are "added". Lets the diff card tint exactly the lines kapi-react changed.
 */
function lineDiff(before: string[], after: string[]): { beforeStatus: string[]; afterStatus: string[] } {
  const t = (s: string) => s.trim();
  const n = before.length;
  const m = after.length;
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = t(before[i]) === t(after[j]) ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const beforeStatus = new Array(n).fill("removed");
  const afterStatus = new Array(m).fill("added");
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (t(before[i]) === t(after[j])) {
      beforeStatus[i] = "same";
      afterStatus[j] = "same";
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      i++;
    } else {
      j++;
    }
  }
  return { beforeStatus, afterStatus };
}

/** Two source files side by side (before | after) with changed lines highlighted. */
function renderCodeDiff(
  before: string,
  after: string,
  title: string,
  sub: string,
  beforeLabel = "Before",
  afterLabel = "After",
): string {
  const beforeLines = before.replace(/\n$/, "").split("\n");
  const afterLines = after.replace(/\n$/, "").split("\n");
  const { beforeStatus, afterStatus } = lineDiff(beforeLines, afterLines);
  // Render each line as its own row so changed lines can carry a tint + gutter bar.
  const rows = (lines: string[], status: string[], changed: "added" | "removed") => {
    const tint = changed === "added" ? "rgba(63,185,80,0.16)" : "rgba(248,81,73,0.15)";
    const bar = changed === "added" ? "#3fb950" : "#f85149";
    return lines
      .map((line, k) => {
        const hot = status[k] === changed;
        return `<div style="padding:0 22px 0 19px;border-left:3px solid ${hot ? bar : "transparent"};${
          hot ? `background:${tint}` : ""
        }">${highlightCode(line) || "&nbsp;"}</div>`;
      })
      .join("");
  };
  const panel = (label: string, accent: string, body: string) =>
    `<div style="flex:1;min-width:0;background:#0d1117;border:1px solid rgba(255,255,255,0.10);border-radius:14px;overflow:hidden">
       <div style="padding:11px 22px;border-bottom:1px solid rgba(255,255,255,0.08);color:${accent};
         font-family:ui-monospace,Menlo,monospace;font-size:14px;font-weight:600">${esc(label)}</div>
       <div style="padding:18px 0;font-family:ui-monospace,Menlo,monospace;font-size:13px;line-height:1.75;
         color:#cdd6f4;white-space:pre-wrap;word-break:break-word">${body}</div>
     </div>`;
  return SHELL(
    title,
    `<style>.wrap{max-width:1720px}</style>
     <h1>${esc(title)}</h1>${sub ? `<div class="sub">${esc(sub)}</div>` : ""}
     <div style="display:flex;gap:20px;align-items:flex-start">
       ${panel(beforeLabel, "#8c98b8", rows(beforeLines, beforeStatus, "removed"))}
       ${panel(afterLabel, "#8fe3a0", rows(afterLines, afterStatus, "added"))}
     </div>`,
  );
}

export { renderCodeDiff };

/** Wrap a pandoc-produced HTML fragment (from a .docx) in a clean "document page" card. */
export function renderDocxHtml(fragment: string, title: string, sub: string): string {
  return SHELL(
    title,
    `<h1>${esc(title)}</h1>${sub ? `<div class="sub">${esc(sub)}</div>` : ""}
     <div class="card" style="background:#fff;color:#1a1f2e;padding:44px 56px">
       <style>
         .docx h1 { font-size: 30px; margin: 0 0 14px; color:#0c1020; font-weight:700 }
         .docx h2 { font-size: 21px; margin: 26px 0 8px; color:#0c1020; font-weight:650 }
         .docx p  { font-size: 16px; line-height: 1.6; color:#33415a; margin: 10px 0 }
         .docx ul { margin: 8px 0 8px 22px } .docx li { font-size: 16px; line-height: 1.7; color:#33415a }
         .docx strong { color:#0c1020 } .docx a { color:#ff7a45; text-decoration: none; font-weight: 600 }
       </style>
       <div class="docx">${fragment}</div>
     </div>`,
  );
}

/** Minimal Markdown → HTML for rendering a translated doc as a styled article. */
export function renderMarkdownDoc(md: string, title = "Document"): string {
  const lines = md.replace(/\r\n/g, "\n").split("\n");
  const out: string[] = [];
  let inList = false;
  const inline = (s: string) =>
    esc(s)
      .replace(/`([^`]+)`/g, "<code>$1</code>")
      .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
      .replace(/(^|[^*])\*([^*]+)\*/g, "$1<em>$2</em>")
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>');
  const closeList = () => {
    if (inList) {
      out.push("</ul>");
      inList = false;
    }
  };
  for (const raw of lines) {
    const line = raw.trimEnd();
    if (/^###\s+/.test(line)) {
      closeList();
      out.push(`<h3>${inline(line.replace(/^###\s+/, ""))}</h3>`);
    } else if (/^##\s+/.test(line)) {
      closeList();
      out.push(`<h2>${inline(line.replace(/^##\s+/, ""))}</h2>`);
    } else if (/^#\s+/.test(line)) {
      closeList();
      out.push(`<h1 class="doch1">${inline(line.replace(/^#\s+/, ""))}</h1>`);
    } else if (/^[-*]\s+/.test(line)) {
      if (!inList) {
        out.push("<ul>");
        inList = true;
      }
      out.push(`<li>${inline(line.replace(/^[-*]\s+/, ""))}</li>`);
    } else if (/^>\s?/.test(line)) {
      closeList();
      out.push(`<blockquote>${inline(line.replace(/^>\s?/, ""))}</blockquote>`);
    } else if (line.trim() === "") {
      closeList();
    } else {
      closeList();
      out.push(`<p>${inline(line)}</p>`);
    }
  }
  closeList();
  return SHELL(
    title,
    `<div class="card" style="background:#fff;color:#1a1f2e;padding:48px 56px">
       <style>
         .doc h1.doch1 { font-size: 38px; margin-bottom: 18px; color:#0c1020 }
         .doc h2 { font-size: 26px; margin: 26px 0 10px; color:#0c1020 }
         .doc h3 { font-size: 20px; margin: 20px 0 8px; color:#0c1020 }
         .doc p { font-size: 18px; line-height: 1.6; color:#33415a; margin: 10px 0 }
         .doc ul { margin: 10px 0 10px 26px } .doc li { font-size: 18px; line-height: 1.7; color:#33415a }
         .doc code { background:#eef1f7; color:#b5380f; padding:2px 7px; border-radius:6px; font-size:16px }
         .doc strong { color:#0c1020 } .doc a { color:#ff7a45; text-decoration: none; font-weight:600 }
         .doc blockquote { margin: 14px 0; padding: 10px 18px; border-left: 4px solid #ff7a45; background:#fff6f1; color:#5a4030; font-size:17px; line-height:1.55; border-radius:0 8px 8px 0 }
       </style>
       <div class="doc">${out.join("\n")}</div>
     </div>`,
  );
}

function esc(s: unknown): string {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

export function renderReport(kind: string, json: any, opts: { title?: string; sub?: string } = {}): string {
  switch (kind) {
    case "brand":
      return renderBrand(json);
    case "term-check":
      return renderTermCheck(json);
    case "word-count":
      return renderWordCount(json);
    case "glossary":
      return renderGlossary(json);
    case "catalog":
      return renderCatalog(json, opts.title ?? "Translated catalog", opts.sub ?? "kapi · format-aware translation");
    default:
      return renderJson(json, opts.title);
  }
}
