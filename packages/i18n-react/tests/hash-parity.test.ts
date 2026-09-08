/**
 * Hash parity regression test.
 *
 * `Block.hash` in the .kbf.json extract output must equal the `hash`
 * argument the plugin transform stamps into every `__t()` / `__tx()`
 * call at build time. If they drift, the neokapi-i18n runtime dict
 * loaded via `loadTranslations()` stops resolving. The test walks a
 * set of representative fixtures through both sides and asserts
 * every extract-side hash exists in the transform output.
 */

import { describe, expect, it } from "vitest";

import { extractDocument } from "../src/extract/index.ts";
import { transform } from "../src/plugin/transform.ts";

const FIXTURES: ReadonlyArray<{ name: string; code: string }> = [
  {
    name: "plain text",
    code: "<h1>Hello World</h1>",
  },
  {
    name: "text with variable",
    code: "<h1>Hello, {name}!</h1>",
  },
  {
    name: "text with member expression",
    code: "<p>Welcome, {user.name}!</p>",
  },
  {
    name: "text with inline element",
    code: '<p>Click <a href="/x">here</a> to continue.</p>',
  },
  {
    name: "attribute translation",
    code: '<input placeholder="Search..." />',
  },
  {
    name: "multiple blocks in one file",
    code: `
      <div>
        <h1>Title</h1>
        <p>Body {count}.</p>
        <button aria-label="Close">Save</button>
      </div>
    `,
  },
  {
    name: "inline child with own text",
    code: "<p>Press <kbd>Cmd</kbd>+<kbd>K</kbd> to search.</p>",
  },
  {
    name: "nested blocks",
    code: "<section><h2>Title</h2><p>Body</p></section>",
  },
  {
    name: "Plural with flat text forms",
    code: `<p><Plural count={n}>
      <One>1 item</One>
      <Other>{n} items</Other>
    </Plural></p>`,
  },
  {
    name: "Plural with inline JSX inside a form",
    code: `<p><Plural count={items.length}>
      <Zero>Your cart is empty</Zero>
      <One>1 item</One>
      <Other><strong>{items.length}</strong> items in your cart</Other>
    </Plural></p>`,
  },
  {
    name: "Select with literal cases",
    code: `<p><Select value={role}>
      <Case when="admin">Admin</Case>
      <Case when="guest">Guest</Case>
      <Other>User</Other>
    </Select></p>`,
  },
  {
    name: "container auto-promoted (div with direct text)",
    code: '<div className="mb-3 text-sm font-medium">Appearance</div>',
  },
  {
    name: "container with inline child auto-promoted",
    code: "<div>Click <strong>here</strong> to continue</div>",
  },
  {
    name: "unmapped PascalCase component with direct text",
    code: '<TabsTrigger value="general">General</TabsTrigger>',
  },
  {
    name: "unmapped component with translatable prop (no children)",
    code: '<PageHeader title="Termbases" />',
  },
  {
    name: "unmapped component with multiple translatable props",
    code: '<PageHeader title="Termbases" subtitle="Glossaries you can use in flows" description="Manage term collections" />',
  },
  {
    name: "user-facing t() call in JS data",
    code: `
      import { t } from '@neokapi/i18n-react/runtime';
      const LANGS = [
        { value: 'en',  label: t('English') },
        { value: 'qps', label: t('Pseudo English (qps)') },
      ];
    `,
  },
  {
    name: "t() with params",
    code: `
      import { t } from '@neokapi/i18n-react/runtime';
      const greeting = t('Hello, {name}!', { name: 'Alice' });
    `,
  },
  {
    name: "t() with context",
    code: `
      import { t } from '@neokapi/i18n-react/runtime';
      const label = t('English', 'UI Language');
    `,
  },
  {
    name: "t() with context and params",
    code: `
      import { t } from '@neokapi/i18n-react/runtime';
      const msg = t('Hello, {name}!', 'greeting', { name: 'Alice' });
    `,
  },
  {
    name: "ternary attribute with string-literal branches",
    code: '<PageHeader title={cond ? "Project Flows" : "Flows"} />',
  },
  {
    name: "unmapped component with self-closing icon child + text",
    code: "<Button><FolderOpen size={12} />Open File...</Button>",
  },
  {
    name: 'translate="no" island beside text',
    code: '<div>Saved to <span translate="no">{path}</span> just now</div>',
  },
  {
    name: 'container whose only text sits in a translate="no" child',
    code: '<SimpleTooltip content={label}><span translate="no">{file}:{key}</span></SimpleTooltip>',
  },
];

/**
 * The parity corpus is generated as well as listed: every parent shape
 * that emits a block, crossed with every child shape that travels
 * inside one. Extraction and the transform descend the same tree, and
 * a shape either side handles alone shows up here as a hash only one
 * of them produced. #2522 was exactly that, unnoticed for as long as
 * the corpus held no conditional.
 */
const PARENTS: ReadonlyArray<{ name: string; wrap: (child: string) => string }> = [
  { name: "paragraph", wrap: (c) => `<p>Saved ${c} now</p>` },
  { name: "paragraph, child last", wrap: (c) => `<p>Saved ${c}</p>` },
  { name: "promoted container", wrap: (c) => `<div>Saved ${c} now</div>` },
  { name: "heading", wrap: (c) => `<h2>Saved ${c}</h2>` },
  { name: "fragment", wrap: (c) => `<><span>Saved</span> ${c} now</>` },
  { name: "beside a paired inline child", wrap: (c) => `<p>Click <a href="/x">here</a> ${c}</p>` },
  { name: "beside a protected code span", wrap: (c) => `<p>Press <kbd>K</kbd> ${c}</p>` },
  { name: "beside a variable", wrap: (c) => `<p>Hello {name}, ${c} now</p>` },
  { name: "unmapped component", wrap: (c) => `<TabsTrigger value="a">Saved ${c}</TabsTrigger>` },
  { name: "list item", wrap: (c) => `<li>Saved ${c}</li>` },
  // The promotion shape behind #2561: the container's own text sits in an
  // inline child, and the child beside it is a sibling of that.
  {
    name: "container whose text sits in an inline child",
    wrap: (c) => `<div className="relative">${c}<span>Saved now</span></div>`,
  },
];

const CHILDREN: ReadonlyArray<{ name: string; code: string }> = [
  { name: "logical and", code: "{cond && <span>a note</span>}" },
  { name: "logical or", code: "{cond || <span>a note</span>}" },
  { name: "nullish", code: "{cond ?? <span>a note</span>}" },
  { name: "ternary", code: "{cond ? <span>yes note</span> : <span>no note</span>}" },
  { name: "ternary with one null branch", code: "{cond ? <span>a note</span> : null}" },
  { name: "map", code: "{items.map((i) => <span key={i}>each item</span>)}" },
  { name: "call argument", code: "{wrapIt(<span>a note</span>)}" },
  { name: "fragment", code: "{cond && <><span>a note</span></>}" },
  { name: "attribute only", code: '{cond && <button aria-label="Close it" />}' },
  { name: "attribute and text", code: '{cond && <span title="Tip text">a note</span>}' },
  { name: "two levels", code: "{a && <span>outer {b && <b>inner</b>}</span>}" },
  { name: "plain variable", code: "{count}" },
  { name: "no JSX at all", code: "{cond && label}" },
  // Identifiers whose value is a React element rather than text (#2561). They
  // look like any other parameter here, which is the point: only the runtime
  // can tell them apart, so both sides must keep naming them the same way.
  { name: "element-valued identifier", code: "{icon}" },
  { name: "element-valued member expression", code: "{row.icon}" },
  { name: "element-valued call", code: "{renderIcon(row)}" },
  // String literals in a conditional's branches (#2581). Each is a block of
  // its own, addressed by slot, and the transform rewrites the literal in
  // place inside whatever the parent splices.
  { name: "ternary of literals", code: '{cond ? "yes note" : "no note"}' },
  { name: "ternary with one literal", code: '{cond ? "a note" : label}' },
  { name: "logical and a literal", code: '{cond && "a note"}' },
  { name: "logical or a literal", code: '{label || "a note"}' },
  { name: "nested ternary of literals", code: '{a ? "a note" : b ? "b note" : "c note"}' },
  { name: "ternary of a literal and an element", code: '{cond ? "a note" : <span>b note</span>}' },
];

/**
 * Inline children that go into the parent's flat template but carry
 * something of their own in an attribute. The block splices the
 * element's source into its call, so the attribute is served there;
 * both sides have to agree on which of them it belongs to (#2523).
 */
const INLINE_CHILDREN: ReadonlyArray<{ name: string; code: string }> = [
  { name: "abbreviation with a title", code: '<abbr title="Content memory">CM</abbr>' },
  { name: "anchor with a title", code: '<a href="/x" title="Go there">here</a>' },
  { name: "image with an alt", code: '<img src="/x.png" alt="the chart" />' },
  { name: "childless component with a label", code: '<Badge label="tag" />' },
  { name: "icon button with an aria-label", code: '<button aria-label="Run it"><Play /></button>' },
  { name: "input with a placeholder", code: '<input placeholder="Jane" />' },
  { name: "ternary attribute", code: '<abbr title={c ? "A one" : "B two"}>CM</abbr>' },
  { name: "attribute two levels down", code: '<a href="/x" title="Tip"><img alt="chart" /></a>' },
  { name: "conditional under an inline child", code: "<strong>bold {c && <b>inner</b>}</strong>" },
  { name: "JSX in a prop of an inline child", code: "<span data-x={<b>note</b>}>text</span>" },
  { name: "translate=no island with a title", code: '<span translate="no" title="Tip">{p}</span>' },
  { name: "machine-facing props only", code: '<button type="submit" name="go">Go</button>' },
];

/** JSX carried in a prop rather than in the children. */
const PROP_FIXTURES: ReadonlyArray<{ name: string; code: string }> = [
  {
    name: "prop JSX on an element that emits its own block",
    code: "<div actions={<Button>Go now</Button>}>Some text here</div>",
  },
  {
    name: "prop JSX on an element that emits nothing",
    code: "<Panel actions={<Button>Go now</Button>}>{children}</Panel>",
  },
  {
    name: "prop JSX beside a translatable attribute",
    code: '<div title="Tip" actions={<Button>Go now</Button>}>Some text here</div>',
  },
];

function hashesFromTransform(code: string): Set<string> {
  const out = transform(code, "Test.tsx", {
    mode: "runtime",
    onWarning: () => {},
  });
  if (!out?.code) return new Set();
  // __t("hash", …)  and  __tx("hash", …)
  const hashes = new Set<string>();
  for (const match of out.code.matchAll(/__tx?\("([^"]+)"/g)) {
    hashes.add(match[1]);
  }
  return hashes;
}

function hashesFromExtract(code: string): Set<string> {
  const doc = extractDocument(code, { filename: "Test.tsx" });
  return new Set(doc?.blocks.map((b) => b.hash) ?? []);
}

describe("hash parity between extract and transform", () => {
  for (const { name, code } of FIXTURES) {
    it(`emits the same hashes for "${name}"`, () => {
      const extracted = hashesFromExtract(code);
      const transformed = hashesFromTransform(code);

      // Every extracted hash must be somewhere in the transform output.
      // (Transform may emit additional hashes the extract skips — e.g.
      // attribute-only elements still trigger extract; symmetry is
      // desirable, so we also check the reverse below.)
      for (const hash of extracted) {
        expect(transformed, `extract hash "${hash}" missing from transform`).toContain(hash);
      }

      // Reverse direction: every hash the transform emits must be one
      // the extractor produced. Catches cases where transform still
      // translates something extract correctly skips (or vice versa).
      for (const hash of transformed) {
        expect(extracted, `transform hash "${hash}" missing from extract`).toContain(hash);
      }
    });
  }

  for (const parent of PARENTS) {
    for (const child of CHILDREN) {
      const code = parent.wrap(child.code);
      it(`emits the same hashes for "${parent.name}" holding "${child.name}"`, () => {
        const extracted = hashesFromExtract(code);
        const transformed = hashesFromTransform(code);
        for (const hash of extracted) {
          expect(transformed, `extract hash "${hash}" missing from transform`).toContain(hash);
        }
        for (const hash of transformed) {
          expect(extracted, `transform hash "${hash}" missing from extract`).toContain(hash);
        }
      });
    }
  }

  for (const parent of PARENTS) {
    for (const child of INLINE_CHILDREN) {
      const code = parent.wrap(child.code);
      it(`emits the same hashes for "${parent.name}" holding "${child.name}"`, () => {
        const extracted = hashesFromExtract(code);
        const transformed = hashesFromTransform(code);
        for (const hash of extracted) {
          expect(transformed, `extract hash "${hash}" missing from transform`).toContain(hash);
        }
        for (const hash of transformed) {
          expect(extracted, `transform hash "${hash}" missing from extract`).toContain(hash);
        }
      });
    }
  }

  for (const { name, code } of PROP_FIXTURES) {
    it(`emits the same hashes for "${name}"`, () => {
      const extracted = hashesFromExtract(code);
      const transformed = hashesFromTransform(code);
      for (const hash of extracted) {
        expect(transformed, `extract hash "${hash}" missing from transform`).toContain(hash);
      }
      for (const hash of transformed) {
        expect(extracted, `transform hash "${hash}" missing from extract`).toContain(hash);
      }
    });
  }

  it("covers every fixture — regression guard", () => {
    // Sanity: we're not shipping an empty fixture set.
    expect(FIXTURES.length).toBeGreaterThan(5);
    expect(PARENTS.length * (CHILDREN.length + INLINE_CHILDREN.length)).toBeGreaterThan(200);
  });
});
