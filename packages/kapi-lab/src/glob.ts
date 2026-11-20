// Project globs use slash-separated paths, recursive directory wildcards,
// brace alternatives and character classes.

const SPECIAL = /[*?{}[\]]/;

/** Whether a string looks like a glob rather than a literal path. */
export function isGlob(s: string): boolean {
  return SPECIAL.test(s);
}

function escapeLiteral(ch: string): string {
  return ch.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** Compile a project glob. An incomplete pattern matches no path. */
export function globToRegExp(pattern: string): RegExp {
  const never = /(?!)/;
  let source = "";
  let braces = 0;
  for (let i = 0; i < pattern.length; i++) {
    const ch = pattern[i];
    switch (ch) {
      case "*": {
        const atStart = i === 0 || "/{,".includes(pattern[i - 1]);
        const start = i;
        while (pattern[i + 1] === "*") i++;
        const next = pattern[i + 1];
        if (atStart && i === start + 1 && next === "/") {
          source += "(?:[^/]+/)*";
          i++;
        } else if (atStart && i === start + 1 && (!next || (braces && ",}".includes(next)))) {
          if (source.endsWith("/")) {
            source = source.slice(0, -1) + "(?:/.*)?";
          } else {
            source += ".*";
          }
        } else {
          source += "[^/]*";
        }
        break;
      }
      case "?":
        source += "[^/]";
        break;
      case "{":
        braces++;
        source += "(?:";
        break;
      case "}":
        if (braces) {
          braces--;
          source += ")";
        } else {
          source += "\\}";
        }
        break;
      case ",":
        source += braces ? "|" : ",";
        break;
      case "[": {
        const cls = /^\[([!^]?)((?:\\.|[^\]\\])+)\]/su.exec(pattern.slice(i));
        if (!cls) return never;
        const chars = cls[2].replace(
          /\\(.)/gsu,
          (_, c: string) => `\\u{${c.codePointAt(0)!.toString(16)}}`,
        );
        source += `[${cls[1] ? "^" : ""}${chars}]`;
        i += cls[0].length - 1;
        break;
      }
      case "\\":
        if (++i === pattern.length) return never;
        source += escapeLiteral(pattern[i]);
        break;
      default:
        source += escapeLiteral(ch);
    }
  }
  if (braces) return never;
  try {
    return new RegExp(`^${source}$`, "su");
  } catch {
    return never;
  }
}

/** The subset of paths matching the glob, in input order. */
export function matchGlob(pattern: string, paths: string[]): string[] {
  if (!pattern.trim()) return [];
  const re = globToRegExp(pattern.trim());
  return paths.filter((p) => re.test(p));
}

/** Match a single path against a glob. */
export function globMatches(pattern: string, path: string): boolean {
  return globToRegExp(pattern.trim()).test(path);
}
