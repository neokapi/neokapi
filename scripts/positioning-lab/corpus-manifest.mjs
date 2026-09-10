// Declare development evidence and the labels still needed for comparative work.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const source = 'samples/audience-context';
const files = ['kapi.yaml', '.kapi/voice.yaml', 'service-facts.md', ...['child', 'teen', 'adult', 'older-adult'].map(a => `site/locales/en/${a}.json`)];
const manifest = {
  schema: 'neokapi-evaluation-corpus/v1',
  purpose: 'Development fixture inventory; no held-out quality evaluation has been performed.',
  splitUnit: 'Project/document family. Original, mutations and repairs remain together.',
  partitions: {
    training: { status: 'unused', families: [] },
    development: { status: 'available', families: ['harbor-help-video-appointment'] },
    heldOut: { status: 'requires independently authored task families and blinded human labels', families: [] },
  },
  files: files.map(file => ({ path: `${source}/${file}`, sha256: createHash('sha256').update(readFileSync(join(repo, source, file))).digest('hex') })),
  labels: [
    { case: 'clean', source: 'Authored fixture', deterministicConstraintFindings: 0, humanQuality: 'unmeasured' },
    { case: 'assurance', source: 'harbor-help/no-unsupported-assurance version 1', deterministicConstraintFindings: 1, humanQuality: 'unmeasured' },
    { case: 'repair', source: 'Prepared correction of the planted prohibited phrase', deterministicConstraintFindings: 0, humanQuality: 'unmeasured' },
    { case: 'semantic-contradiction', source: 'service-facts.md#recording-and-control', deterministicConstraintFindings: 0, semanticCoverage: 'unsupported', humanQuality: 'requires independent review' },
    { case: 'mechanical', source: 'Authored repeated-word mutation', deterministicConstraintFindings: 0, humanQuality: 'requires independent review' },
  ],
  limitations: ['Authored labels are not independent human judgments.', 'All four audiences belong to one development family.', 'Rule construction used this fixture; it cannot estimate generalization.'],
};
mkdirSync(join(repo, 'harness/out/positioning-review'), { recursive: true });
writeFileSync(join(repo, 'harness/out/positioning-review/corpus-manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
