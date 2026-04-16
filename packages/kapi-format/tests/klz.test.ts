import { describe, expect, it } from 'vitest';
import { unzipSync } from 'fflate';

import {
  flattenRuns,
  Kind,
  KlzReader,
  KlzWriter,
  MANIFEST_PATH,
  ManifestVersion,
  type File,
  marshalFile,
  newFile,
  SchemaVersion,
  validatePartPath,
} from '../src/index.ts';
import { filesHeading } from '../examples/files-heading.ts';
import { tagChip } from '../examples/tag-chip.ts';
import { shoppingCart } from '../examples/shopping-cart.ts';

// A throwaway block fixture. We don't require a full document for
// writer tests — path safety, manifest structure, and SHA integrity
// are independent of block contents.
const sampleFile: File = newFile({
  generator: { id: '@neokapi/kapi-react', version: '0.1.0' },
  project: { id: 'sample-app', sourceLocale: 'en', targetLocales: ['de', 'qps'] } as never,
  documents: [
    {
      id: 'src/App.tsx',
      documentType: 'jsx',
      path: 'src/App.tsx',
      blocks: [filesHeading, tagChip, shoppingCart],
    },
  ],
});

describe('marshalFile', () => {
  it('produces deterministic bytes with trailing newline', () => {
    const bytes1 = marshalFile(sampleFile);
    const bytes2 = marshalFile(sampleFile);
    expect(bytes1).toEqual(bytes2);
    expect(new TextDecoder().decode(bytes1)).toMatch(/\n$/);
  });

  it('emits schemaVersion/kind in canonical order', () => {
    const text = new TextDecoder().decode(marshalFile(sampleFile));
    const parsed = JSON.parse(text) as Record<string, unknown>;
    const keys = Object.keys(parsed);
    expect(keys[0]).toBe('schemaVersion');
    expect(keys[1]).toBe('kind');
    expect(parsed.schemaVersion).toBe(SchemaVersion);
    expect(parsed.kind).toBe(Kind);
  });
});

describe('KlzWriter.build', () => {
  it('emits a valid ZIP with manifest first and every part hashed', () => {
    const writer = new KlzWriter({
      generator: { id: '@neokapi/kapi-react', version: '0.1.0' },
      project: { id: 'sample-app', sourceLocale: 'en', targetLocales: ['de', 'qps'] },
      created: '2026-04-16T00:00:00Z',
    });
    writer.addDocument('documents/App.klf', sampleFile);
    writer.addSkeleton('skeletons/App.skl', new TextEncoder().encode('skeleton-opaque'));
    writer.addAsset(
      'assets/logo.png',
      new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    );

    const zip = writer.build();
    const entries = unzipSync(zip);

    // Manifest is present and parseable.
    const manifestBytes = entries[MANIFEST_PATH];
    expect(manifestBytes).toBeDefined();
    const manifest = JSON.parse(new TextDecoder().decode(manifestBytes)) as {
      kapiLocalizationFormat: string;
      parts: Array<{ path: string; sha256: string; size: number; role: string }>;
    };
    expect(manifest.kapiLocalizationFormat).toBe(ManifestVersion);
    expect(manifest.parts).toHaveLength(3);

    // Every manifest entry round-trips to a real ZIP part with the
    // declared SHA and size.
    for (const part of manifest.parts) {
      const payload = entries[part.path];
      expect(payload, `part ${part.path}`).toBeDefined();
      expect(payload.length).toBe(part.size);
      expect(part.sha256).toMatch(/^[0-9a-f]{64}$/);
    }

    // Roles are classified correctly.
    const byPath = Object.fromEntries(manifest.parts.map((p) => [p.path, p.role]));
    expect(byPath['documents/App.klf']).toBe('document');
    expect(byPath['skeletons/App.skl']).toBe('skeleton');
    expect(byPath['assets/logo.png']).toBe('asset');
  });

  it('rejects duplicate part paths', () => {
    const writer = new KlzWriter({
      generator: { id: 'x', version: '1' },
      project: { id: 'p', sourceLocale: 'en' },
    });
    writer.addSkeleton('skeletons/a.skl', new Uint8Array([1]));
    expect(() => writer.addSkeleton('skeletons/a.skl', new Uint8Array([2]))).toThrow(/duplicate/);
  });

  it('rejects a part path colliding with the manifest', () => {
    const writer = new KlzWriter({
      generator: { id: 'x', version: '1' },
      project: { id: 'p', sourceLocale: 'en' },
    });
    expect(() => writer.addAsset(MANIFEST_PATH, new Uint8Array([0]))).toThrow(/reserved/);
  });

  it('produces stable bytes for identical inputs', () => {
    const make = () => {
      const w = new KlzWriter({
        generator: { id: '@neokapi/kapi-react', version: '0.1.0' },
        project: { id: 'sample-app', sourceLocale: 'en' },
      });
      w.addDocument('documents/App.klf', sampleFile);
      w.addSkeleton('skeletons/App.skl', new TextEncoder().encode('opaque'));
      return w.build();
    };

    // Byte-for-byte identical across invocations (DEFLATE + zeroed
    // mtime + stable part order + deterministic manifest).
    expect(make()).toEqual(make());
  });
});

describe('KlzReader', () => {
  it('round-trips Writer output and exposes documents', () => {
    const w = new KlzWriter({
      generator: { id: '@neokapi/kapi-react', version: '0.1.0' },
      project: { id: 'sample-app', sourceLocale: 'en' },
    });
    w.addDocument('documents/App.klf', sampleFile);
    w.addSkeleton('skeletons/App.skl', new TextEncoder().encode('opaque'));
    const archive = w.build();

    const r = new KlzReader(archive);
    expect(r.manifest.generator.id).toBe('@neokapi/kapi-react');
    const docs = r.documents();
    expect(docs).toHaveLength(1);
    expect(docs[0].documents[0].blocks.map((b) => b.id)).toEqual([
      'files-heading',
      'tag-chip',
      'shopping-cart-plural',
    ]);
  });

  it('detects hash tampering', () => {
    const w = new KlzWriter({
      generator: { id: 'g', version: '1' },
      project: { id: 'p', sourceLocale: 'en' },
    });
    w.addDocument('documents/App.klf', sampleFile);
    const archive = w.build();

    // Tamper with the document inside the archive by rebuilding the
    // ZIP with altered bytes but the original manifest.
    const r1 = new KlzReader(archive, { verifyHashes: false });
    const tamperedDoc = new TextDecoder().decode(r1.read('documents/App.klf')).replace(
      'files-heading',
      'tampered',
    );
    // Can't easily modify zipSync output byte-in-place, so reconstruct:
    const w2 = new KlzWriter({
      generator: r1.manifest.generator,
      project: r1.manifest.project,
    });
    w2.addDocumentBytes('documents/App.klf', new TextEncoder().encode(tamperedDoc));
    const tamperedArchive = w2.build();

    // A second reader reads the tampered archive fine (manifest was
    // re-hashed by the Writer), but if we splice the original
    // manifest's SHA into the new archive, read() must fail.
    const legit = new KlzReader(archive);
    const legitSha = legit.manifest.parts[0].sha256;

    const tamperedReader = new KlzReader(tamperedArchive);
    tamperedReader.manifest.parts[0].sha256 = legitSha;
    expect(() => tamperedReader.read('documents/App.klf')).toThrow(/hash mismatch/);
  });

  it('iterates blocks via the generator', () => {
    const w = new KlzWriter({
      generator: { id: 'g', version: '1' },
      project: { id: 'p', sourceLocale: 'en' },
    });
    w.addDocument('documents/App.klf', sampleFile);
    const r = new KlzReader(w.build());
    const ids: string[] = [];
    for (const { block } of r.blocks()) ids.push(block.id);
    expect(ids).toEqual(['files-heading', 'tag-chip', 'shopping-cart-plural']);
  });
});

describe('flattenRuns', () => {
  it('flattens text + placeholders + paired codes to the runtime dict shape', () => {
    expect(
      flattenRuns([
        { text: 'Files ' },
        { pcOpen: { id: '1', type: 'jsx:element', data: '<span>', equiv: 'muted' } },
        { text: '(' },
        { ph: { id: '2', type: 'jsx:var', data: '{count}', equiv: 'count' } },
        { text: ' matched)' },
        { pcClose: { id: '1', type: 'jsx:element', data: '</span>', equiv: 'muted' } },
      ]),
    ).toBe('Files {=m1}({count} matched){/=m1}');
  });

  it('emits ICU for plural constructs', () => {
    expect(
      flattenRuns([
        {
          plural: {
            pivot: 'count',
            forms: {
              one: [{ text: '1 item' }],
              other: [{ ph: { id: '1', type: 'jsx:var', data: '{count}', equiv: 'count' } }, { text: ' items' }],
            },
          },
        },
      ]),
    ).toBe('{count, plural, one {1 item} other {{count} items}}');
  });
});

describe('validatePartPath', () => {
  it('accepts well-formed POSIX paths', () => {
    expect(validatePartPath('documents/a.klf')).toBe('documents/a.klf');
    expect(validatePartPath('targets/de/home.klf')).toBe('targets/de/home.klf');
  });

  it('rejects ZIP-slip and other unsafe shapes', () => {
    const cases: Array<[string, RegExp]> = [
      ['', /empty/],
      ['/documents/a.klf', /leading slash/],
      ['documents\\a.klf', /backslash/],
      ['documents/../secret', /canonical/],
      ['../outside', /canonical|escapes/],
      ['documents//a.klf', /canonical|empty component/],
    ];
    for (const [input, matcher] of cases) {
      expect(() => validatePartPath(input), input).toThrow(matcher);
    }
  });

  it('NFC-normalizes non-canonical unicode', () => {
    // "é" as U+0065 U+0301 (NFD) normalises to U+00E9 (NFC).
    const nfd = 'documents/cafe\u0301.klf';
    const normalized = validatePartPath(nfd);
    expect(normalized).toBe('documents/caf\u00e9.klf');
  });
});
