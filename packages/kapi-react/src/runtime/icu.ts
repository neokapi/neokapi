/**
 * Lightweight ICU MessageFormat resolver for plural and select.
 *
 * Handles:
 *   {count, plural, one {# message} other {# messages}}
 *   {gender, select, male {his} female {her} other {their}}
 *   Nested token substitution within branches
 *
 * Uses Intl.PluralRules (built into all modern browsers, zero polyfill).
 * ~1KB minified — compare to @formatjs/intl-messageformat at 20KB+.
 */

/**
 * Resolve an ICU MessageFormat string with the given parameters.
 * Returns the resolved string with all plural/select branches evaluated.
 */
export function resolveICU(
  text: string,
  params: Record<string, string | number> | undefined,
  locale?: string,
): string {
  if (!params) return text;
  return parseAndResolve(text, params, locale || 'en');
}

function parseAndResolve(
  text: string,
  params: Record<string, string | number>,
  locale: string,
): string {
  let result = '';
  let i = 0;

  while (i < text.length) {
    if (text[i] === '{') {
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
  params: Record<string, string | number>,
  locale: string,
): { value: string; end: number } {
  // Skip opening {
  let i = start + 1;

  // Read the variable name
  const varStart = i;
  while (i < text.length && text[i] !== ',' && text[i] !== '}') i++;
  const varName = text.slice(varStart, i).trim();

  if (text[i] === '}') {
    // Simple substitution: {varName}
    const value = varName === '#'
      ? String(params._count ?? '')
      : String(params[varName] ?? `{${varName}}`);
    return { value, end: i + 1 };
  }

  // Skip comma
  i++;
  // Read the type (plural, select, selectordinal)
  while (i < text.length && text[i] === ' ') i++;
  const typeStart = i;
  while (i < text.length && text[i] !== ',') i++;
  const type = text.slice(typeStart, i).trim();
  i++; // skip comma

  // Parse the branches
  const branches: Record<string, string> = {};
  while (i < text.length && text[i] !== '}') {
    // Skip whitespace
    while (i < text.length && (text[i] === ' ' || text[i] === '\n')) i++;
    if (text[i] === '}') break;

    // Read branch key (one, other, male, female, =0, etc.)
    const keyStart = i;
    while (i < text.length && text[i] !== ' ' && text[i] !== '{') i++;
    const key = text.slice(keyStart, i).trim();

    // Skip whitespace
    while (i < text.length && text[i] === ' ') i++;

    // Read branch value (balanced braces)
    if (text[i] === '{') {
      const branchContent = readBalancedBraces(text, i);
      branches[key] = branchContent.content;
      i = branchContent.end;
    }
  }

  // Skip closing }
  if (i < text.length && text[i] === '}') i++;

  // Resolve the correct branch
  const paramValue = params[varName];
  let selectedBranch: string;

  if (type === 'plural' || type === 'selectordinal') {
    const count = typeof paramValue === 'number' ? paramValue : Number(paramValue);
    // Check for exact match first (=0, =1, =2, etc.)
    if (branches[`=${count}`] !== undefined) {
      selectedBranch = branches[`=${count}`];
    } else {
      // Use Intl.PluralRules
      const rules = new Intl.PluralRules(locale, {
        type: type === 'selectordinal' ? 'ordinal' : 'cardinal',
      });
      const category = rules.select(count);
      selectedBranch = branches[category] ?? branches['other'] ?? '';
    }
    // Resolve # as the count value
    selectedBranch = selectedBranch.replaceAll('#', String(count));
  } else {
    // select: exact string match
    selectedBranch = branches[String(paramValue)] ?? branches['other'] ?? '';
  }

  // Recursively resolve any nested expressions
  const resolved = parseAndResolve(selectedBranch, params, locale);

  return { value: resolved, end: i };
}

/**
 * Read content inside balanced braces: {content}
 * Handles nested braces.
 */
function readBalancedBraces(
  text: string,
  start: number,
): { content: string; end: number } {
  let depth = 0;
  let i = start;

  while (i < text.length) {
    if (text[i] === '{') depth++;
    else if (text[i] === '}') {
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
