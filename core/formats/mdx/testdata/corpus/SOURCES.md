# MDX corpus — provenance

Genuine `.mdx` files copied verbatim from THIS repository's documentation sites
(`web/` and `bowrain/web/docs/`). Because they are same-repo copies there
is no third-party license concern; they are real shipping docs pages, not
synthetic fixtures.

| Local file | Source path in this repo |
|---|---|
| `bowrain-auth.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-auth.mdx` |
| `bowrain-automation.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-automation.mdx` |
| `bowrain-create-project.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-create-project.mdx` |
| `bowrain-getting-started.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-getting-started.mdx` |
| `bowrain-init.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-init.mdx` |
| `bowrain-overview.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-overview.mdx` |
| `bowrain-serve.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-serve.mdx` |
| `bowrain-web-term-explorer.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-web-term-explorer.mdx` |
| `bowrain-web-translation-editor.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-web-translation-editor.mdx` |
| `bowrain-workspaces.mdx` | `bowrain/web/docs/docs/walkthroughs/bowrain-workspaces.mdx` |
| `kapi-bilingual-workflow.mdx` | `web/docs/walkthroughs/kapi-bilingual-workflow.mdx` |
| `kapi-pseudo-translate.mdx` | `web/docs/walkthroughs/kapi-pseudo-translate.mdx` |
| `kapi-terminology-pretranslation.mdx` | `web/docs/walkthroughs/kapi-terminology-pretranslation.mdx` |
| `kapi-terminology-qa.mdx` | `web/docs/walkthroughs/kapi-terminology-qa.mdx` |
| `kapi-cli-bilingual-workflow.mdx` | `web/docs/kapi-cli/bilingual-workflow.mdx` |
| `server-web-overview.mdx` | `bowrain/web/docs/docs/server/web-overview.mdx` |
| `website-translation.mdx` | `bowrain/web/docs/docs/cli/use-cases/website-translation.mdx` |

## How they were copied

```sh
# From the repo root, verbatim (no edits):
cp web/docs/walkthroughs/kapi-terminology-qa.mdx            corpus/kapi-terminology-qa.mdx
cp web/docs/kapi-cli/bilingual-workflow.mdx                 corpus/kapi-cli-bilingual-workflow.mdx
cp bowrain/web/docs/docs/server/web-overview.mdx                 corpus/server-web-overview.mdx
cp bowrain/web/docs/docs/cli/use-cases/website-translation.mdx   corpus/website-translation.mdx
# … (see the table above for the full set)
```

## Constructs covered

- **YAML frontmatter** — every file opens with a `---` block.
- **ESM imports** — every file imports Docusaurus/walkthrough components.
- **Block-level JSX** — `<ThemedVideo …/>`, `<Callout>…</Callout>`, etc.
- **GFM tables** — `kapi-cli-bilingual-workflow.mdx`, `server-web-overview.mdx`,
  `website-translation.mdx`.
- **Fenced code blocks** — `kapi-cli-bilingual-workflow.mdx`,
  `website-translation.mdx`.
- **Inline code with literal angle brackets** — `bowrain-init.mdx`
  (`` `<dir-name>.kapi` ``) and others, which is the case that distinguishes a
  correct (run-preserving) translation from one that corrupts the MDX (see
  `invariants_test.go::TestInvariantInlineCodeMarkupPreservedUnderTranslation`
  and the consumer-acceptance compile check).
- **Headings, lists, links, inline markup** — throughout.

## Note for maintainers

If a docs page is edited, refreshing the corpus copy is a verbatim `cp`. The
corpus exists to lock byte-faithful round-trip and real-compiler acceptance
against real authored content; keep the copies byte-identical to the source.
