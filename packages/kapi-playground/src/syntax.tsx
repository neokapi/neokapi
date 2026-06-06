// A tiny, dependency-free syntax highlighter for the textual localisation
// formats the playground previews. It is deliberately not a full parser: it
// tokenises line by line with ordered sticky-regex rules, which is robust,
// fast, and good enough to make a YAML/XML/JSON/PO/properties file scannable in
// the raw preview.
//
// The playground cannot import @neokapi/kapi-lab (the dependency direction runs
// kapi-lab → kapi-playground), so this mirrors the ordered-regex approach used
// in packages/kapi-lab/src/highlight.ts but is fully self-contained here.
//
// Tokens render as <span class="kapi-pg-hl-<type>"> elements, themed via the
// playground stylesheet using the host's --ifm-* variables so light and dark
// both read well.

import React from "react";

export type Lang = "json" | "xml" | "yaml" | "properties" | "po" | "markdown" | "text";

export type TokenType =
  | "key"
  | "string"
  | "number"
  | "boolean"
  | "null"
  | "tag"
  | "attr"
  | "comment"
  | "punct"
  | "section"
  | "heading"
  | "meta"
  | "text";

export interface Token {
  type: TokenType;
  text: string;
}

/**
 * Map a filename to a highlighter language (plain text when unknown).
 *
 * Notable mappings the task calls out:
 *   - `.kapi`/`.yaml`/`.yml` → yaml (the .kapi recipe is a YAML document)
 *   - `.tmx`/`.tbx`/`.xliff`/`.xlf`/`.xml`/`.resx` → xml
 *   - `.json`/`.arb`/`.klf` → json
 */
export function detectLang(filename: string): Lang {
  const base = filename.split("/").pop() ?? filename;
  const ext = base.includes(".") ? base.slice(base.lastIndexOf(".") + 1).toLowerCase() : "";
  switch (ext) {
    case "kapi":
    case "yaml":
    case "yml":
      return "yaml";
    case "tmx":
    case "tbx":
    case "xliff":
    case "xlf":
    case "sdlxliff":
    case "mxliff":
    case "xml":
    case "resx":
    case "html":
    case "htm":
    case "svg":
    case "stringsdict":
      return "xml";
    case "json":
    case "jsonc":
    case "json5":
    case "arb":
    case "xcstrings":
    case "klf":
      return "json";
    case "po":
    case "pot":
      return "po";
    case "properties":
    case "ini":
    case "toml":
    case "strings":
      return "properties";
    case "md":
    case "mdx":
    case "markdown":
      return "markdown";
    default:
      return "text";
  }
}

interface Rule {
  type: TokenType;
  re: RegExp; // must be sticky (y)
}

// Generic sticky-rule scanner: at each position try rules in order; on a match
// emit a token and advance, otherwise consume one character as text. Adjacent
// text characters are coalesced so output stays compact.
function scan(line: string, rules: Rule[]): Token[] {
  const out: Token[] = [];
  let i = 0;
  let pending = "";
  const flush = () => {
    if (pending) {
      out.push({ type: "text", text: pending });
      pending = "";
    }
  };
  while (i < line.length) {
    let matched = false;
    for (const rule of rules) {
      rule.re.lastIndex = i;
      const m = rule.re.exec(line);
      if (m && m.index === i && m[0].length > 0) {
        flush();
        out.push({ type: rule.type, text: m[0] });
        i += m[0].length;
        matched = true;
        break;
      }
    }
    if (!matched) {
      pending += line[i];
      i++;
    }
  }
  flush();
  return out;
}

const JSON_RULES: Rule[] = [
  { type: "string", re: /"(?:[^"\\]|\\.)*"/y },
  { type: "number", re: /-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/y },
  { type: "boolean", re: /\b(?:true|false)\b/y },
  { type: "null", re: /\bnull\b/y },
  { type: "punct", re: /[{}[\],:]/y },
];

// A JSON string immediately followed (past whitespace) by ':' is an object key.
function retypeJsonKeys(tokens: Token[]): Token[] {
  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i].type !== "string") continue;
    let j = i + 1;
    while (j < tokens.length && tokens[j].type === "text" && tokens[j].text.trim() === "") j++;
    if (j < tokens.length && tokens[j].type === "punct" && tokens[j].text === ":") {
      tokens[i] = { type: "key", text: tokens[i].text };
    }
  }
  return tokens;
}

const XML_RULES: Rule[] = [
  { type: "comment", re: /<!--.*?(?:-->|$)/y },
  { type: "meta", re: /<[!?][^>]*>?/y },
  { type: "tag", re: /<\/?[A-Za-z][\w:.-]*/y },
  { type: "attr", re: /[A-Za-z_:][\w:.-]*(?=\s*=)/y },
  { type: "string", re: /"[^"]*"|'[^']*'/y },
  { type: "punct", re: /\/?>/y },
];

const YAML_RULES: Rule[] = [
  { type: "comment", re: /#.*/y },
  { type: "string", re: /"(?:[^"\\]|\\.)*"|'[^']*'/y },
  { type: "key", re: /[A-Za-z0-9_.\- ]+?(?=\s*:(?:\s|$))/y },
  { type: "number", re: /-?\d+(?:\.\d+)?\b/y },
  { type: "boolean", re: /\b(?:true|false|null|yes|no|on|off)\b/y },
  { type: "punct", re: /[:\-[\]{},|>]/y },
];

function tokenizeProperties(line: string): Token[] {
  const t = line.trimStart();
  const indent = line.slice(0, line.length - t.length);
  if (t.startsWith("#") || t.startsWith("!") || t.startsWith(";")) {
    return [{ type: "comment", text: line }];
  }
  if (t.startsWith("[") && t.includes("]")) {
    return [{ type: "section", text: line }];
  }
  const m = /^([^=:]+?)(\s*[=:]\s*)(.*)$/.exec(t);
  if (!m) return [{ type: "text", text: line }];
  const out: Token[] = [];
  if (indent) out.push({ type: "text", text: indent });
  out.push({ type: "key", text: m[1] });
  out.push({ type: "punct", text: m[2] });
  if (m[3]) out.push({ type: "string", text: m[3] });
  return out;
}

function tokenizePo(line: string): Token[] {
  const t = line.trimStart();
  if (t.startsWith("#")) return [{ type: "comment", text: line }];
  const m = /^(msgid|msgstr|msgctxt|msgid_plural)(\[\d+\])?(\s+)(.*)$/.exec(t);
  if (m) {
    return [
      { type: "key", text: m[1] + (m[2] ?? "") },
      { type: "text", text: m[3] },
      { type: "string", text: m[4] },
    ];
  }
  if (t.startsWith('"')) return [{ type: "string", text: line }];
  return [{ type: "text", text: line }];
}

function tokenizeMarkdown(line: string): Token[] {
  if (/^\s{0,3}#{1,6}\s/.test(line)) return [{ type: "heading", text: line }];
  if (/^\s{0,3}(?:[-*+]|\d+\.)\s/.test(line)) {
    const m = /^(\s*)([-*+]|\d+\.)(\s)(.*)$/.exec(line)!;
    return [
      { type: "text", text: m[1] },
      { type: "punct", text: m[2] },
      { type: "text", text: m[3] + m[4] },
    ];
  }
  if (line.trimStart().startsWith(">")) return [{ type: "comment", text: line }];
  if (line.trimStart().startsWith("```")) return [{ type: "meta", text: line }];
  return [{ type: "text", text: line }];
}

/** Tokenise text into per-line token arrays for the given language. */
export function tokenize(text: string, lang: Lang): Token[][] {
  const lines = text.split("\n");
  switch (lang) {
    case "json":
      return lines.map((l) => retypeJsonKeys(scan(l, JSON_RULES)));
    case "xml":
      return lines.map((l) => scan(l, XML_RULES));
    case "yaml":
      return lines.map((l) => scan(l, YAML_RULES));
    case "properties":
      return lines.map(tokenizeProperties);
    case "po":
      return lines.map(tokenizePo);
    case "markdown":
      return lines.map(tokenizeMarkdown);
    default:
      return lines.map((l) => [{ type: "text" as TokenType, text: l }]);
  }
}

/**
 * Highlight raw text as a <pre> with per-token <span> classes, language chosen
 * from the filename. Used by the raw-text fallback in FilePreview and curated
 * BlockPreview so unparseable files (e.g. a .kapi YAML recipe, a .tmx) still
 * read as syntax-highlighted source.
 */
export function HighlightedCode({
  text,
  filename,
  className,
}: {
  text: string;
  filename: string;
  className?: string;
}): React.ReactElement {
  const lang = detectLang(filename);
  const lines = tokenize(text, lang);
  return (
    <pre className={className} data-lang={lang}>
      {lines.map((tokens, li) => (
        <React.Fragment key={li}>
          {tokens.map((tok, ti) => (
            <span key={ti} className={`kapi-pg-hl-${tok.type}`}>
              {tok.text}
            </span>
          ))}
          {li < lines.length - 1 ? "\n" : null}
        </React.Fragment>
      ))}
    </pre>
  );
}
