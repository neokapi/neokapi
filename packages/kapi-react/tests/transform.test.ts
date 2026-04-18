import { describe, it, expect } from 'vitest';
import { transform } from '../src/plugin/transform.ts';
import type { PluginOptions } from '../src/types.ts';

function t(code: string, options: Partial<PluginOptions> = {}): string | null {
  return transform(code, 'Test.tsx', { mode: 'runtime', ...options })?.code ?? null;
}

describe('neokapi-react SWC transform', () => {
  describe('dev mode (no-op)', () => {
    it('returns null when no locale or mode set', () => {
      const result = transform('<h1>Hello</h1>', 'Test.tsx', {});
      expect(result).toBeNull();
    });
  });

  describe('runtime mode — text content', () => {
    it('transforms <h1> text to t() call', () => {
      const result = t('<h1>Hello World</h1>');
      expect(result).not.toBeNull();
      expect(result).toContain('__t(');
      expect(result).toContain('"Hello World"');
    });

    it('transforms <button> text', () => {
      const result = t('<button>Save</button>');
      expect(result).toContain('__t(');
      expect(result).toContain('"Save"');
    });

    it('transforms <p> text', () => {
      const result = t('<p>Paragraph</p>');
      expect(result).toContain('__t(');
    });

    it('does NOT transform <code>', () => {
      const result = t('<code>x = 1</code>');
      expect(result).toBeNull();
    });

    it('promotes <div> with direct text', () => {
      const result = t('<div>Just a div</div>');
      expect(result).toContain('__t(');
    });

    it('still skips <div> marked translate="no"', () => {
      expect(t('<div translate="no">skip</div>')).toBeNull();
    });

    it('respects translate="no"', () => {
      const result = t('<h1 translate="no">Skip this</h1>');
      expect(result).toBeNull();
    });
  });

  describe('runtime mode — expressions', () => {
    it('wraps {variable} in t() with params', () => {
      const result = t('<h1>Hello, {name}!</h1>');
      expect(result).toContain('__t(');
      expect(result).toContain('name');
      // Should have template literal fallback
      expect(result).toContain('`');
    });

    it('handles member expressions like {user.name}', () => {
      const result = t('<p>Welcome, {user.name}!</p>');
      expect(result).toContain('user.name');
    });

    // Expression-only children are left untouched — a lone
    // `{variable}` / `{formatDate()}` holds no translator-editable
    // text, and wrapping it in `__t()` coerces React-element values
    // to "[object Object]" at runtime. Mixed content (see below)
    // still transforms normally.
    it('leaves bare identifier `<p>{name}</p>` untransformed', () => {
      expect(t('<p>{name}</p>')).toBeNull();
    });

    it('leaves deep member access `<p>{a.b.c}</p>` untransformed', () => {
      expect(t('<p>{a.b.c}</p>')).toBeNull();
    });

    it('leaves call expression `<p>{formatDate(d)}</p>` untransformed', () => {
      expect(t('<p>{formatDate(d)}</p>')).toBeNull();
    });

    it('handles expression + trailing text: {value}%', () => {
      const result = t('<span>{value}%</span>');
      expect(result).toContain('__t(');
      expect(result).toContain('value');
      expect(result).not.toContain('${}');
    });

    it('handles two expressions separated by text: {current}/{total}', () => {
      const result = t('<span>{current}/{total}</span>');
      expect(result).toContain('__t(');
      expect(result).toContain('current');
      expect(result).toContain('total');
      expect(result).not.toContain('${}');
    });

    it('handles member access with text: {job.fileCount} files', () => {
      const result = t('<p>{job.fileCount} files</p>');
      expect(result).toContain('__t(');
      expect(result).toContain('job.fileCount');
      expect(result).not.toContain('${}');
    });

    // Regression matrix from issue #2 — SWC spans are UTF-8 byte
    // offsets, not UTF-16 code units. Any non-ASCII character shifts
    // subsequent element positions.
    it('handles em-dash in JSX content', () => {
      const result = t('<p>Hello — world</p>');
      expect(result).toContain('__t(');
      expect(result).toContain('Hello — world');
      expect(result).not.toContain('<p>Hello — world</p>');
    });

    it('handles em-dash in a comment before a JSX element', () => {
      const code = [
        '// This file has em-dashes — that break SWC byte offsets —',
        '// when treated as UTF-16 code units.',
        'export function App() {',
        '  return <p>Hello world.</p>;',
        '}',
      ].join('\n');
      const result = t(code);
      expect(result).toContain('__t(');
      expect(result).toContain('Hello world.');
      // Comment must be preserved intact — no splicing into it
      expect(result).toContain('// This file has em-dashes — that break SWC byte offsets —');
      expect(result).toContain('// when treated as UTF-16 code units.');
      // The <p> content must be replaced with the __t() call
      expect(result).not.toContain('<p>Hello world.</p>');
    });

    it('handles curly quotes', () => {
      const result = t('<p>Say \u201Chi\u201D to me</p>');
      expect(result).toContain('__t(');
      expect(result).toContain('\u201Chi\u201D');
    });

    it('handles accented Latin (café)', () => {
      const result = t('<p>café au lait</p>');
      expect(result).toContain('__t(');
      expect(result).toContain('café au lait');
    });

    it('handles CJK characters', () => {
      const result = t('<p>你好世界</p>');
      expect(result).toContain('__t(');
      expect(result).toContain('你好世界');
    });

    it('handles emoji', () => {
      const result = t('<p>🎉 party time</p>');
      expect(result).toContain('__t(');
      expect(result).toContain('🎉 party time');
    });

    it('handles non-ASCII attribute values (naïve)', () => {
      const result = t('<button title="naïve">Click</button>');
      expect(result).toContain('__t(');
      expect(result).toContain('naïve');
      expect(result).toContain('Click');
    });

    it('handles non-ASCII BEFORE a JSX element — the core #2 repro', () => {
      const code = [
        'const label = "français";  // accented',
        'const greeting = "你好";   // CJK',
        'const party = "🎉";        // emoji',
        'export function App() {',
        '  return <p>Hello world.</p>;',
        '}',
      ].join('\n');
      const result = t(code);
      expect(result).toContain('__t(');
      expect(result).toContain('Hello world.');
      // All non-ASCII preludes preserved intact
      expect(result).toContain('"français"');
      expect(result).toContain('"你好"');
      expect(result).toContain('"🎉"');
      // The <p> content was actually replaced, not some random offset
      expect(result).not.toContain('<p>Hello world.</p>');
    });

    it('handles member access + em-dash in a surrounding text context', () => {
      const code = '// — prelude\nexport const X = <p>Hello, {user.name}!</p>;';
      const result = t(code);
      expect(result).toContain('__t(');
      expect(result).toContain('user.name');
      expect(result).toContain('// — prelude');
    });

    it('produces valid JS output for mixed-content expression cases', () => {
      // Expression-only children no longer transform — text must
      // anchor the block. These are the realistic patterns.
      const cases = [
        '<span>{value}%</span>',
        '<span>{current}/{total}</span>',
        '<p>{count} of {total} items</p>',
      ];

      for (const input of cases) {
        const result = t(input);
        expect(result, `Failed for input: ${input}`).not.toBeNull();
        expect(result, `Empty template expr in: ${input}`).not.toContain('${}');
        expect(result, `Unquoted dotted key in: ${input}`).not.toMatch(/\{\s*\w+\.\w+:/);
      }
    });
  });

  describe('runtime mode — attributes', () => {
    it('transforms placeholder to t() call', () => {
      const result = t('<input placeholder="Search..." />');
      expect(result).toContain('__t(');
      expect(result).toContain('Search...');
    });

    it('transforms alt to t() call', () => {
      const result = t('<img alt="Photo" />');
      expect(result).toContain('__t(');
    });

    it('does NOT transform className', () => {
      // className isn't in translatableAttributes; the div's "text"
      // content IS picked up by the auto-promotion rule, but the
      // className literal must stay verbatim.
      const result = t('<div className="foo">text</div>');
      expect(result).toContain('__t(');
      expect(result).toContain('className="foo"');
      expect(result).not.toContain('__t("foo"');
    });
  });

  describe('runtime mode — imports', () => {
    it('adds runtime import when __t() is used', () => {
      const result = t('<h1>Hello</h1>');
      expect(result).toContain("import { __t } from '@neokapi/kapi-react/runtime'");
    });
  });

  // Regression matrix from issue #3 — nested translatable elements
  // used to produce two overlapping ops (outer captured children
  // verbatim, inner replaced its own content) and emit stray closing
  // tags. Walker now skips descendants of consumed blocks; final ops
  // are asserted to be pairwise disjoint.
  describe('runtime mode — nested translatable elements', () => {
    it('captures inline child span verbatim in tx() and does not emit a separate t() for the span', () => {
      const code = [
        'export function App({ count }: { count: number }) {',
        '  return (',
        '    <div>',
        '      <h2>',
        '        Files',
        '        <span className="muted">({count} matched)</span>',
        '      </h2>',
        '    </div>',
        '  );',
        '}',
      ].join('\n');
      const result = t(code);
      expect(result).not.toBeNull();
      // Exactly one __tx and no __t call for the inner span
      const txCount = (result!.match(/__tx\(/g) || []).length;
      const tCount = (result!.match(/__t\("/g) || []).length;
      expect(txCount).toBe(1);
      expect(tCount).toBe(0);
      // The inline span is captured verbatim inside the elements map
      expect(result).toContain('"=m0": <span className="muted">({count} matched)</span>');
      // No stray closing </span> outside any opening <span>
      expect(result).not.toMatch(/\}<\/span>/);
    });

    it('inline child without own text (<button>Delete <strong>{name}</strong></button>)', () => {
      const result = t('<button>Delete <strong>{name}</strong></button>');
      expect(result).not.toBeNull();
      const txCount = (result!.match(/__tx\(/g) || []).length;
      const tCount = (result!.match(/__t\("/g) || []).length;
      expect(txCount).toBe(1);
      expect(tCount).toBe(0);
      expect(result).toContain('<strong>{name}</strong>');
    });

    it('child with its own text stays inside parent tx() (<p>Click <a>here</a></p>)', () => {
      const result = t('<p>Click <a href="/help">here</a> to continue.</p>');
      expect(result).not.toBeNull();
      const txCount = (result!.match(/__tx\(/g) || []).length;
      const tCount = (result!.match(/__t\("/g) || []).length;
      expect(txCount).toBe(1);
      expect(tCount).toBe(0);
      expect(result).toContain('<a href="/help">here</a>');
    });

    it('two sibling inline children (<p>Press <kbd>Cmd</kbd>+<kbd>K</kbd></p>)', () => {
      const result = t('<p>Press <kbd>Cmd</kbd>+<kbd>K</kbd> to search.</p>');
      expect(result).not.toBeNull();
      const txCount = (result!.match(/__tx\(/g) || []).length;
      expect(txCount).toBe(1);
      expect(result).toContain('<kbd>Cmd</kbd>');
      expect(result).toContain('<kbd>K</kbd>');
    });

    it('three levels deep (section > h2 > span > strong)', () => {
      const result = t('<section><h2>Title <span><strong>bold</strong></span></h2></section>');
      expect(result).not.toBeNull();
      // h2 is the outermost translatable block; only one __tx
      const txCount = (result!.match(/__tx\(/g) || []).length;
      const tCount = (result!.match(/__t\("/g) || []).length;
      expect(txCount).toBe(1);
      expect(tCount).toBe(0);
      expect(result).toContain('<span><strong>bold</strong></span>');
    });

    it('mixed text + expression + inline element', () => {
      const result = t('<h2>{title} <span className="muted">({count})</span></h2>');
      expect(result).not.toBeNull();
      const txCount = (result!.match(/__tx\(/g) || []).length;
      const tCount = (result!.match(/__t\("/g) || []).length;
      expect(txCount).toBe(1);
      expect(tCount).toBe(0);
      // param index 0 is {title}, so the span is =m1
      expect(result).toContain('"=m1": <span className="muted">({count})</span>');
      expect(result).toContain('"title": title');
    });
  });

  // A parent whose only child is an expression container has no
  // translator-editable text anchor, so we leave it alone. When the
  // expression evaluates to JSX, the inner translatable elements
  // get picked up by the walker through normal descent.
  describe('runtime mode — conditional JSX in expression containers', () => {
    it('leaves `<p>{ok && <strong>yes</strong>}</p>` untransformed, but `yes` still becomes a `__t()` call', () => {
      const result = t('<p>{ok && <strong>yes</strong>}</p>');
      expect(result).not.toBeNull();
      expect(result).not.toContain('__tx(');
      // Inner <strong> is translatable on its own.
      expect(result).toContain('__t(');
      expect(result).toContain('"yes"');
    });

    it('leaves `<p>{loading ? <Spinner /> : <Done />}</p>` untransformed (no inner text)', () => {
      const result = t('<p>{loading ? <Spinner /> : <Done />}</p>');
      // No translatable text anywhere — JSX renders unchanged.
      expect(result).toBeNull();
    });

    it('mixed text + conditional JSX: parent uses tx(), JSX captured as element token', () => {
      const result = t('<p>Hello {name}, {isAdmin && <Badge>admin</Badge>}!</p>');
      expect(result).not.toBeNull();
      expect(result).toContain('__tx(');
      expect(result).toContain('"name": name');
      // isAdmin && ... is the element token
      expect(result).toContain('"=m1": isAdmin && <Badge>admin</Badge>');
    });

    it('multiple inline conditionals (tag chip repro)', () => {
      const code = [
        '<span data-tag-chip>',
        '  {index !== undefined && <span className="badge">{index}</span>}',
        '  {label}',
        '  {!deletable && <span className="required">*</span>}',
        '</span>',
      ].join('\n');
      const result = t(code, { rules: [{ selector: '[data-tag-chip]', translate: true }] });
      // Outer `data-tag-chip` span has only expression-container
      // children — no translator-editable text — so it's left alone.
      // The inner `<span className="required">*</span>` still has
      // the "*" text anchor and gets a __t() call of its own.
      expect(result).not.toBeNull();
      expect(result).not.toContain('__tx(');
      expect(result).toContain('__t(');
      expect(result).toContain('"*"');
    });

    it('leaves `<p>{name || <em>anonymous</em>}</p>` outer untransformed, inner em translated', () => {
      const result = t('<p>{name || <em>anonymous</em>}</p>');
      expect(result).not.toBeNull();
      expect(result).not.toContain('__tx(');
      expect(result).toContain('"anonymous"');
    });
  });

  describe('runtime mode — componentMap', () => {
    it('maps custom components', () => {
      const result = t('<Button>Submit</Button>', {
        componentMap: { Button: 'button' },
      });
      expect(result).toContain('__t(');
    });

    it('extracts text from unmapped components (auto-promoted)', () => {
      // Unmapped PascalCase components fall through to container
      // semantics so their direct text still becomes translatable.
      // A warning is surfaced so the dev can stabilise hashes by
      // adding the component to componentMap.
      const warnings: string[] = [];
      const result = t('<MyWidget>text</MyWidget>', {
        onWarning: (msg) => warnings.push(msg),
      });
      expect(result).toContain('__t(');
      expect(warnings.join('\n')).toContain('<MyWidget>');
      expect(warnings.join('\n')).toContain('unmapped component');
    });

    it('auto-promotes container elements silently — no warning for <div>Label</div>', () => {
      // `<div>` with direct text is mainstream React; warning on
      // every promotion drowned stderr in noise during normal
      // builds. Promotion still happens; it just doesn't announce
      // itself.
      const warnings: string[] = [];
      const result = t('<div>Label</div>', {
        onWarning: (msg) => warnings.push(msg),
      });
      expect(result).toContain('__t(');
      expect(warnings).toEqual([]);
    });
  });

  describe('runtime mode — rules', () => {
    it('can make a container translatable', () => {
      const result = t('<div className="hero">Big text</div>', {
        rules: [{ selector: '.hero', translate: true }],
      });
      expect(result).toContain('__t(');
    });

    it('can suppress a translatable element', () => {
      const result = t('<p>Technical: ABC-123</p>', {
        rules: [{ selector: 'p', translate: false }],
      });
      expect(result).toBeNull();
    });
  });

  describe('runtime mode — rich JSX (tx)', () => {
    it('emits tx() for elements with inline JSX', () => {
      const result = t('<p>Click <a href="/x">here</a> to continue.</p>');
      expect(result).toContain('__tx(');
      expect(result).toContain('"=m0"');
      expect(result).toContain('<a href="/x">here</a>');
    });

    it('imports tx alongside t when both are needed', () => {
      const result = t(`
        <div>
          <h1>Simple text</h1>
          <p>Click <a href="/x">here</a> now.</p>
        </div>
      `);
      expect(result).toContain('import { __t, __tx }');
    });

    it('emits t() (not tx) for text without inline elements', () => {
      const result = t('<h1>Hello, {name}!</h1>');
      expect(result).toContain('__t(');
      expect(result).not.toContain('__tx(');
    });
  });

  describe('no fbt references anywhere', () => {
    it('output contains zero fbt references', () => {
      const result = t(`
        <div>
          <h1>Welcome, {name}!</h1>
          <p>Click <a href="/x">here</a> to continue.</p>
          <button>Save</button>
          <input placeholder="Search..." />
        </div>
      `);
      expect(result).not.toBeNull();
      expect(result).not.toContain('fbt');
      expect(result).not.toContain('fbs');
      expect(result).toContain("@neokapi/kapi-react/runtime");
    });
  });

  describe('missing translation warnings', () => {
    it('warns on missing translation when strict is "warn"', () => {
      const warnings: string[] = [];
      const origWarn = console.warn;
      console.warn = (msg: string) => warnings.push(msg);

      try {
        transform('<h1>Missing text</h1>', 'Test.tsx', {
          locale: 'de',
          translationsDir: './nonexistent',
          strict: 'warn',
        });
        expect(warnings.some(w => w.includes('Missing translation'))).toBe(true);
      } finally {
        console.warn = origWarn;
      }
    });

    it('throws on missing translation when strict is "error"', () => {
      expect(() => {
        transform('<h1>Missing text</h1>', 'Test.tsx', {
          locale: 'de',
          translationsDir: './nonexistent',
          strict: 'error',
        });
      }).toThrow('Missing translation');
    });

    it('does not warn when strict is false', () => {
      const warnings: string[] = [];
      const origWarn = console.warn;
      console.warn = (msg: string) => warnings.push(msg);

      try {
        transform('<h1>Missing text</h1>', 'Test.tsx', {
          locale: 'de',
          translationsDir: './nonexistent',
          strict: false,
        });
        expect(warnings).toHaveLength(0);
      } finally {
        console.warn = origWarn;
      }
    });
  });

  describe('fallback locale chain', () => {
    it('loads translations from fallback when primary is missing', () => {
      // Create temp translation files
      const fs = require('fs');
      const path = require('path');
      const tmpDir = path.join(__dirname, '__tmp_translations__');
      fs.mkdirSync(tmpDir, { recursive: true });

      // Only write 'de.json', not 'de-AT.json'
      fs.writeFileSync(
        path.join(tmpDir, 'de.json'),
        JSON.stringify({ fallbackHash: 'Deutscher Fallback' }),
      );

      try {
        const result = transform('<h1>Test</h1>', 'Test.tsx', {
          locale: 'de-AT',
          fallbackLocales: ['de'],
          translationsDir: tmpDir,
          strict: false,
        });
        // The transform should have loaded de.json as fallback
        // (we can't easily verify the dict content in the transform output,
        // but we verify it doesn't crash and processes the file)
        expect(result).not.toBeNull();
      } finally {
        fs.rmSync(tmpDir, { recursive: true });
      }
    });
  });
});
