// A small, dependency-free glob matcher over a flat list of virtual paths —
// matching the shapes the kapi CLI accepts for its content globs (doublestar
// semantics: `*`, `**`, `?`, `{a,b}` alternation, `[abc]` classes). The lab's
// file library is an in-memory list, so we match against strings rather than a
// real filesystem; that keeps this framework- and platform-agnostic.
//
// Supported:
//   *            any run of characters except "/"
//   **           any run of characters including "/" (recursive)
//   ?            any single character except "/"
//   {a,b,c}      alternation
//   [abc] [a-z]  character class
//   literal . /  matched literally
//
// `**/*.json`, `*.{json,xliff}`, `src/**`, `messages.*` all work.

const SPECIAL = /[*?{}[\]]/;

/** Whether a string looks like a glob rather than a literal path. */
export function isGlob(s: string): boolean {
  return SPECIAL.test(s);
}

function escapeLiteral(ch: string): string {
  return ch.replace(/[.+^$()|\\]/g, "\\$&");
}

/**
 * Compile a glob to an anchored RegExp. Alternation is expanded inline; `**`
 * spans path separators while `*` and `?` do not.
 */
export function globToRegExp(pattern: string): RegExp {
  let re = "";
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i];
    if (c === "*") {
      if (pattern[i + 1] === "*") {
        // `**` — cross directory boundaries. Consume an optional trailing slash
        // so `src/**` matches `src/a` and `src/a/b` alike.
        i++;
        if (pattern[i + 1] === "/") i++;
        re += "(?:.*?/)?.*?";
        // Collapse `**` followed by content into a single greedy-enough segment;
        // the lazy quantifiers above keep it well-behaved for our short paths.
      } else {
        re += "[^/]*";
      }
    } else if (c === "?") {
      re += "[^/]";
    } else if (c === "{") {
      const close = pattern.indexOf("}", i);
      if (close === -1) {
        re += "\\{";
      } else {
        const alts = pattern
          .slice(i + 1, close)
          .split(",")
          .map((a) => a.split("").map(escapeLiteral).join(""));
        re += `(?:${alts.join("|")})`;
        i = close;
      }
    } else if (c === "[") {
      const close = pattern.indexOf("]", i);
      if (close === -1) {
        re += "\\[";
      } else {
        re += `[${pattern.slice(i + 1, close)}]`;
        i = close;
      }
    } else {
      re += escapeLiteral(c);
    }
  }
  return new RegExp(`^${re}$`);
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
