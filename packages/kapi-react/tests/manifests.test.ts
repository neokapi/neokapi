import { describe, expect, it } from 'vitest';
import { parseSync } from '@swc/core';

import {
  collectImports,
  parseManifestFromDTS,
  resolveLibraryComponentMap,
} from '../src/plugin/manifests.ts';

function parse(code: string) {
  return parseSync(code, { syntax: 'typescript', tsx: true });
}

describe('collectImports', () => {
  it('groups non-relative imports by source', () => {
    const mod = parse(`
      import { Root as Tabs, Trigger as TabsTrigger } from '@radix-ui/react-tabs';
      import { Button } from '@/components/ui/button';
      import local from './local';
      import sideEffect from '/abs/path';
    `);
    const imports = collectImports(mod);
    expect(Array.from(imports.keys())).toEqual([
      '@radix-ui/react-tabs',
      '@/components/ui/button',
    ]);
    expect(Object.fromEntries(imports.get('@radix-ui/react-tabs')!)).toEqual({
      Root: 'Tabs',
      Trigger: 'TabsTrigger',
    });
  });
});

describe('parseManifestFromDTS', () => {
  it('derives component → HTML element from RefAttributes<HTMLButtonElement>', () => {
    const dts = `
      export declare const TabsTrigger: React.ForwardRefExoticComponent<
        TabsTriggerProps & React.RefAttributes<HTMLButtonElement>
      >;
      export declare const TabsContent: React.ForwardRefExoticComponent<
        TabsContentProps & React.RefAttributes<HTMLDivElement>
      >;
    `;
    const manifest = parseManifestFromDTS(dts);
    expect(manifest?.components).toEqual({
      TabsTrigger: 'button',
      TabsContent: 'div',
    });
  });

  it('tracks `Trigger = typeof TabsTrigger` aliases', () => {
    const dts = `
      export declare const TabsTrigger: React.ForwardRefExoticComponent<
        TabsTriggerProps & React.RefAttributes<HTMLButtonElement>
      >;
      export declare const Trigger: typeof TabsTrigger;
    `;
    const manifest = parseManifestFromDTS(dts);
    expect(manifest?.aliases).toEqual({ Trigger: 'TabsTrigger' });
  });
});

describe('resolveLibraryComponentMap', () => {
  it('returns an empty map when no libraries ship manifests', () => {
    const mod = parse(`import { foo } from 'package-with-no-manifest-whatsoever';`);
    const map = resolveLibraryComponentMap(mod, process.cwd());
    expect(map).toEqual({});
  });

  it('skips relative imports entirely when no filename is provided', () => {
    const mod = parse(`
      import { Foo } from './foo';
      import { Bar } from '../bar';
    `);
    const map = resolveLibraryComponentMap(mod, process.cwd());
    expect(map).toEqual({});
  });

  it('applies the owning package manifest when filename is provided', () => {
    // packages/ui ships its own i18n-manifest.json — passing a file
    // inside it should expose the mappings for relative imports
    // (the library-manifest resolver otherwise skips those).
    const mod = parse(`
      import { Button } from './button';
      import { Label } from './label';
      export function Panel() {
        return <div><Label>Email</Label><Button>Save</Button></div>;
      }
    `);
    const uiFile =
      new URL('../../ui/src/components/ui/dialog.tsx', import.meta.url).pathname;
    const map = resolveLibraryComponentMap(
      mod,
      process.cwd(),
      undefined,
      uiFile,
    );
    expect(map.Button).toBe('button');
    expect(map.Label).toBe('label');
  });

  it('does not cross package boundaries — a file outside the package gets no entries', () => {
    const mod = parse(`import { Button } from './button';`);
    // A path that isn't inside any package with a manifest.
    const map = resolveLibraryComponentMap(
      mod,
      process.cwd(),
      undefined,
      '/tmp/unrelated/file.tsx',
    );
    expect(map).toEqual({});
  });
});
