/**
 * kapi-react extract has two modes:
 *   1. Default — writes per-file .klf under --out.
 *   2. --stream — NDJSON block records to stdout, reads NUL-separated
 *      paths from stdin. This is the exec-format wire protocol.
 *
 * These tests round-trip both modes and confirm they agree on block
 * count + hashes against the same source tree.
 */

import { describe, expect, it } from 'vitest';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Readable, Writable } from 'node:stream';

import type { File } from '@neokapi/kapi-format';
import { runExtract } from '../src/commands/extract.ts';

function tempProject() {
  const dir = mkdtempSync(join(tmpdir(), 'kapi-react-extract-'));
  mkdirSync(join(dir, 'src'));
  writeFileSync(
    join(dir, 'src', 'App.tsx'),
    `
    export default function App() {
      return (
        <main>
          <h1>Hello world</h1>
          <p>One and two.</p>
        </main>
      );
    }
    `,
  );
  return dir;
}

function stringSink(): { stream: Writable; read: () => string } {
  const chunks: Buffer[] = [];
  const stream = new Writable({
    write(chunk, _encoding, cb) {
      chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
      cb();
    },
  });
  return { stream, read: () => Buffer.concat(chunks).toString('utf8') };
}

describe('kapi-react extract', () => {
  it('writes per-file .klf under --out by default', async () => {
    const dir = tempProject();
    const cwd = process.cwd();
    process.chdir(dir);
    try {
      await runExtract(['--src', 'src/**/*.tsx', '--out', 'i18n']);
      const entries = readdirSync(join(dir, 'i18n', 'src'));
      expect(entries).toContain('App.klf');

      const raw = readFileSync(join(dir, 'i18n', 'src', 'App.klf'), 'utf8');
      const file = JSON.parse(raw) as File;
      expect(file.kind).toBe('kapi-localization-format');
      expect(file.documents).toHaveLength(1);
      expect(file.documents[0].path).toBe('src/App.tsx');
      expect(file.documents[0].blocks.length).toBeGreaterThan(0);
    } finally {
      process.chdir(cwd);
    }
  });

  it('emits NDJSON block records that match the KLF-written set', async () => {
    const dir = tempProject();
    const cwd = process.cwd();
    process.chdir(dir);
    try {
      // KLF-default path — reference set of hashes.
      await runExtract(['--src', 'src/**/*.tsx', '--out', 'i18n']);
      const raw = readFileSync(join(dir, 'i18n', 'src', 'App.klf'), 'utf8');
      const file = JSON.parse(raw) as File;
      const klfBlocks = file.documents.flatMap((d) => d.blocks);

      // Stream path — paths on stdin, NDJSON on stdout.
      const sink = stringSink();
      const paths = 'src/App.tsx\0';
      await runExtract(
        ['--stream'],
        { stdin: Readable.from([Buffer.from(paths, 'utf8')]), stdout: sink.stream },
      );

      const streamed = sink
        .read()
        .split('\n')
        .filter((l) => l.startsWith('{'))
        .map((l) => JSON.parse(l) as { type: string; document: string; block: { hash: string } });

      expect(streamed).toHaveLength(klfBlocks.length);
      expect(new Set(streamed.map((r) => r.block.hash))).toEqual(
        new Set(klfBlocks.map((b) => b.hash)),
      );
      for (const rec of streamed) {
        expect(rec.type).toBe('block');
        expect(rec.document).toBe('src/App.tsx');
      }
    } finally {
      process.chdir(cwd);
    }
  });

  it('writes nothing to stdout when --stream is passed with no paths', async () => {
    const sink = stringSink();
    await runExtract(
      ['--stream'],
      { stdin: Readable.from([Buffer.alloc(0)]), stdout: sink.stream },
    );
    expect(sink.read()).toBe('');
  });
});
