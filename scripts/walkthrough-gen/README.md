# walkthrough-gen

One authored **scene spec** drives both the recorded VHS video and the
interactive in-browser playground embed for a kapi walkthrough, so they cannot
drift (neokapi epic #658, W3 / issue #661).

## The single source

`web/docs/walkthroughs/<id>.scene.yaml` — ordered steps plus a per-walkthrough
`mode`. See [`kapi-overview.scene.yaml`](../../web/docs/walkthroughs/kapi-overview.scene.yaml)
for the fully-commented schema. In short:

```yaml
id: kapi-word-count # matches <id>.md prompt + docs/walkthroughs/<id>.mdx
scene: word-count # the single terminal scene (file prefix 01)
mode: interactive # interactive → embed is primary; video → ThemedVideo primary
tape: { width, height, theme, padding, fontSize }
seed: [messages.json] # built-in kit fixtures (packages/kapi-playground/src/fixtures.ts)
files: [{ path, content }] # inline files not covered by the kit fixtures
steps:
  - comment: "..." # a visible "# ..." beat (tape only)
  - command: kapi word-count messages.json # a runnable command
    narration: "..." # shown beside the step in the embed rail
    offline: true # default; runs in the wasm embed
    smoke: true # default(=offline); add to the .md smoke_contract (W7)
```

`offline: false` marks a step that needs AI / network / SQLite — it appears in
the tape + narration but never runs in the embed. `smoke: false` keeps a step
out of the W7 smoke_contract (use it for chained steps that consume a file
produced by an earlier step — the W7 verifier runs each command in isolation).

## What the generator emits

```
node --experimental-strip-types scripts/walkthrough-gen/gen.ts <id> [<id>...]
node --experimental-strip-types scripts/walkthrough-gen/gen.ts --all
node --experimental-strip-types scripts/walkthrough-gen/gen.ts --check   # CI: fail if stale
```

From one `<id>.scene.yaml` it writes, deterministically:

1. **The VHS tape** `web/docs/scenes/<id>/01-<scene>.tape` (interactive
   walkthroughs only — video walkthroughs keep their hand-curated recording
   tape). For interactive walkthroughs it also materializes the `seed`/`files`
   into the scene dir at bare paths so `cd web/docs/scenes/<id> && vhs 01-*.tape`
   records against the exact bytes the embed seeds.
2. **The embed config** `web/docs/src/components/KapiPlayground/embeds/<id>.embed.ts`
   (plus `types.ts` and the `index.ts` registry). `KapiGuidedEmbed` reads these
   to seed + script the kit's `<KapiEmbed>` and render the guided-steps rail.
   Emitted pre-formatted (via `vp fmt`) so `vp fmt --check` stays green.
3. **Keeps the `smoke_contract:` array in sync** in `web/docs/walkthroughs/<id>.md`
   front matter, so the W7 verifier (`make docs-verify-snippets`) keeps
   re-running these commands against the wasm build.

Reproducible: same spec → byte-identical output (`--check` is the CI gate).

## The published page

`web/docs/docs/walkthroughs/<id>.mdx` renders `<KapiGuidedEmbed id="<id>" />`
as the primary artifact for interactive walkthroughs, with `<ThemedVideo>` kept
as the fallback for video walkthroughs (or for individual steps that need
AI/network). The MDX prose is authored by hand around the embed.

## Relationship to the walkthrough-\* skills

This generator slots into the existing `walkthroughs/ → scenes/ → docs/`
pipeline that the `walkthrough-scenes` / `walkthrough-doc` / `walkthrough-verify`
skills describe. The `.scene.yaml` is the structured source the `walkthrough-scenes`
skill's terminal-tape template is generated from; the smoke_contract it keeps in
sync is what `walkthrough-verify` re-runs.
