/**
 * Lightweight ICU MessageFormat resolver.
 *
 * Handles:
 *   {count, plural, one {# message} other {# messages}}
 *   {gender, select, male {his} female {her} other {their}}
 *   {n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}
 *   {price, number}  {pct, number, percent}  {n, number, integer}
 *   {total, number, currency/EUR}
 *   {when, date}  {when, date, short|medium|long|full}
 *   {when, time}  {when, time, short|medium|long|full}
 *   Nested token substitution within branches
 *
 * Formatting goes through Intl.PluralRules / Intl.NumberFormat /
 * Intl.DateTimeFormat (built into all modern engines, zero polyfill).
 * Still ~1.5KB minified — compare to @formatjs/intl-messageformat at
 * 20KB+. `#` inside plural branches is locale-number-formatted.
 */

export type ICUParamValue = string | number | Date;

/**
 * True when `text` contains an ICU argument this resolver handles.
 * The runtime uses this to decide whether a lookup result needs the
 * resolver at all — keep it cheap.
 */
export function hasICUSyntax(text: string): boolean {
  return /\{[^{},]+,\s*(plural|select|selectordinal|number|date|time)\s*[,}]/.test(text);
}

/**
 * Resolve an ICU MessageFormat string with the given parameters.
 * Returns the resolved string with all plural/select branches
 * evaluated and number/date/time arguments formatted for `locale`.
 */
export function resolveICU(
  text: string,
  params: Record<string, ICUParamValue> | undefined,
  locale?: string,
): string {
  if (!params) return text;
  return parseAndResolve(text, params, locale || "en");
}

function parseAndResolve(
  text: string,
  params: Record<string, ICUParamValue>,
  locale: string,
): string {
  let result = "";
  let i = 0;

  while (i < text.length) {
    if (text[i] === "{") {
      const parsed = parseExpression(text, i, params, locale);
      result += parsed.value;
      i = parsed.end;
    } else {
      result += text[i];
      i++;
    }
  }

  return result;
}

function parseExpression(
  text: string,
  start: number,
  params: Record<string, ICUParamValue>,
  locale: string,
): { value: string; end: number } {
  // Skip opening {
  let i = start + 1;

  // Read the variable name
  const varStart = i;
  while (i < text.length && text[i] !== "," && text[i] !== "}") i++;
  const varName = text.slice(varStart, i).trim();

  if (text[i] === "}") {
    // Simple substitution: {varName}
    const value =
      varName === "#" ? String(params._count ?? "") : String(params[varName] ?? `{${varName}}`);
    return { value, end: i + 1 };
  }

  // Skip comma
  i++;
  // Read the type (plural, select, selectordinal, number, date, time)
  while (i < text.length && text[i] === " ") i++;
  const typeStart = i;
  while (i < text.length && text[i] !== "," && text[i] !== "}") i++;
  const type = text.slice(typeStart, i).trim();

  // ── Formatting arguments: no branches, an optional style ──
  if (type === "number" || type === "date" || type === "time") {
    let style = "";
    if (text[i] === ",") {
      i++;
      const styleStart = i;
      while (i < text.length && text[i] !== "}") i++;
      style = text.slice(styleStart, i).trim();
    }
    if (text[i] === "}") i++;
    const raw = params[varName];
    if (raw === undefined) return { value: `{${varName}}`, end: i };
    return { value: formatValue(type, raw, style, locale), end: i };
  }

  if (text[i] === ",") i++; // skip comma before branches

  // Parse the branches
  const branches: Record<string, string> = {};
  while (i < text.length && text[i] !== "}") {
    // Skip whitespace
    while (i < text.length && (text[i] === " " || text[i] === "\n")) i++;
    if (text[i] === "}") break;

    // Read branch key (one, other, male, female, =0, etc.)
    const keyStart = i;
    while (i < text.length && text[i] !== " " && text[i] !== "{") i++;
    const key = text.slice(keyStart, i).trim();

    // Skip whitespace
    while (i < text.length && text[i] === " ") i++;

    // Read branch value (balanced braces)
    if (text[i] === "{") {
      const branchContent = readBalancedBraces(text, i);
      branches[key] = branchContent.content;
      i = branchContent.end;
    }
  }

  // Skip closing }
  if (i < text.length && text[i] === "}") i++;

  // Resolve the correct branch
  const paramValue = params[varName];
  let selectedBranch: string;

  if (type === "plural" || type === "selectordinal") {
    const count = typeof paramValue === "number" ? paramValue : Number(paramValue);
    // Check for exact match first (=0, =1, =2, etc.)
    if (branches[`=${count}`] !== undefined) {
      selectedBranch = branches[`=${count}`];
    } else {
      // Use Intl.PluralRules
      const rules = new Intl.PluralRules(locale, {
        type: type === "selectordinal" ? "ordinal" : "cardinal",
      });
      const category = rules.select(count);
      selectedBranch = branches[category] ?? branches["other"] ?? "";
    }
    // Resolve # as the locale-formatted count value
    selectedBranch = selectedBranch.replaceAll("#", formatNumber(count, "", locale));
  } else {
    // select: exact string match
    selectedBranch = branches[String(paramValue)] ?? branches["other"] ?? "";
  }

  // Recursively resolve any nested expressions
  const resolved = parseAndResolve(selectedBranch, params, locale);

  return { value: resolved, end: i };
}

// ─── Intl formatting ─────────────────────────────────────────

function formatValue(
  type: "number" | "date" | "time",
  raw: ICUParamValue,
  style: string,
  locale: string,
): string {
  if (type === "number") {
    const n = typeof raw === "number" ? raw : Number(raw);
    if (Number.isNaN(n)) return String(raw);
    return formatNumber(n, style, locale);
  }
  const date = raw instanceof Date ? raw : new Date(typeof raw === "number" ? raw : String(raw));
  if (Number.isNaN(date.getTime())) return String(raw);
  const styleValue = isDateTimeStyle(style) ? style : "medium";
  try {
    return new Intl.DateTimeFormat(
      locale,
      type === "date" ? { dateStyle: styleValue } : { timeStyle: styleValue },
    ).format(date);
  } catch {
    return String(raw);
  }
}

function isDateTimeStyle(style: string): style is "short" | "medium" | "long" | "full" {
  return style === "short" || style === "medium" || style === "long" || style === "full";
}

function formatNumber(n: number, style: string, locale: string): string {
  try {
    if (style === "") return new Intl.NumberFormat(locale).format(n);
    if (style === "integer") {
      return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(n);
    }
    if (style === "percent") {
      return new Intl.NumberFormat(locale, { style: "percent" }).format(n);
    }
    // `currency/EUR` (also tolerated with the `::` skeleton prefix).
    const currency = /^(?:::)?currency\/([A-Za-z]{3})$/.exec(style);
    if (currency) {
      return new Intl.NumberFormat(locale, {
        style: "currency",
        currency: currency[1].toUpperCase(),
      }).format(n);
    }
    return new Intl.NumberFormat(locale).format(n);
  } catch {
    return String(n);
  }
}

/**
 * Read content inside balanced braces: {content}
 * Handles nested braces.
 */
function readBalancedBraces(text: string, start: number): { content: string; end: number } {
  let depth = 0;
  let i = start;

  while (i < text.length) {
    if (text[i] === "{") depth++;
    else if (text[i] === "}") {
      depth--;
      if (depth === 0) {
        return {
          content: text.slice(start + 1, i),
          end: i + 1,
        };
      }
    }
    i++;
  }

  // Unclosed brace — return what we have
  return { content: text.slice(start + 1), end: text.length };
}
