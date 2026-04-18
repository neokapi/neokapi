/**
 * Round-trip: run kapi-react extract with --blocks-stream, parse the
 * NDJSON output, assert the stream + the archive-writing path agree
 * on block count + block hashes. Guards the exec-extractor contract
 * expected by core/plugin/extractor (NUL-separated paths on stdin,
 * NDJSON block records on stdout).
 */

import { describe, expect, it } from 'vitest';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Readable, Writable } from 'node:stream';

import { KlzReader } from '@neokapi/kapi-format/klz';
import { runExtract } from '../src/commands/extract.ts';

function tempProject() {
  const dir = mkdtempSync(join(tmpdir(), 'kapi-react-blocks-stream-'));
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

describe('kapi-react extract --blocks-stream', () => {
  it('emits NDJSON block records that match the archive-written set', async () => {
    const dir = tempProject();
    const cwd = process.cwd();
    process.chdir(dir);
    try {
      // 1. Archive-writing path — reference set of hashes.
      await runExtract(['--src', 'src/**/*.tsx', '--out', 'i18n/reference.klz']);
      const archiveBytes = readFileSync(join(dir, 'i18n', 'reference.klz'));
      const reader = new KlzReader(new Uint8Array(archiveBytes));
      const archiveBlocks = reader
        .documents()
        .flatMap((file) => file.documents.flatMap((doc) => doc.blocks));

      // 2. Blocks-stream path — paths on stdin, NDJSON on stdout.
      const sink = stringSink();
      const paths = 'src/App.tsx\0';
      await runExtract(
        ['--blocks-stream'],
        { stdin: Readable.from([Buffer.from(paths, 'utf8')]), stdout: sink.stream },
      );

      const streamed = sink
        .read()
        .split('\n')
        .filter((l) => l.startsWith('{'))
        .map((l) => JSON.parse(l) as { type: string; document: string; block: { hash: string } });

      // 3. Same block count, same hashes, same document attribution.
      expect(streamed).toHaveLength(archiveBlocks.length);
      expect(new Set(streamed.map((r) => r.block.hash))).toEqual(
        new Set(archiveBlocks.map((b) => b.hash)),
      );
      for (const rec of streamed) {
        expect(rec.type).toBe('block');
        expect(rec.document).toBe('src/App.tsx');
      }
    } finally {
      process.chdir(cwd);
    }
  });

  it('writes nothing to stdout when no paths are supplied', async () => {
    const sink = stringSink();
    await runExtract(
      ['--blocks-stream'],
      { stdin: Readable.from([Buffer.alloc(0)]), stdout: sink.stream },
    );
    expect(sink.read()).toBe('');
  });
});
