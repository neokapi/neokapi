# Samples

Sample projects and datasets that walkthroughs, documentation and evals draw on,
so every surface shows the same fictional companies instead of inventing a
throwaway fixture each time.

| Sample | Shape | What it proves |
| --- | --- | --- |
| [`northsea/`](northsea/) | A complete repository: operator documentation, interface strings, a marketing page. One language, one product, four channels. | **S0**, the monolingual journey. Discover a context graph, ask where you are, gate prose the way tests gate code, land a correction as a decision, converge. No second language, no server, no provider credential. |
| [`compass/`](compass/) | A deployable app: interface catalogs in four languages, a committed review record, and the page that reads them. | **S1**, the multilingual journey as an *extension* of S0: the same point, one more axis. Recycle before AI, review as a change-set, and two ship gates driving the deployed page's language picker, including a language deliberately left mid-loop. |
| [`tidewatch-docs/`](tidewatch-docs/) | A documentation site: four handbook pages, two target languages, a CI workflow, and an i18n tree that is build output. | **S2**, the continuous condition. Coverage reported rather than enforced, front matter converged with the prose, and a build that depends on neither the loop nor a language being ready. |
| [`mart/`](mart/) | A dataset: source strings, a prose document, three target locales, terms and a content memory. | The multilingual material: recycling, partial coverage, plural and select messages, a review cast. Two brand instances (KapiMart, BowMart) share one design. |

The first three are projects you can run a loop in. Compass and Tidewatch are
Northsea with a locale axis, at the delivery edge and in CI respectively, and
Mart is content you can put through one.

## Conventions

- **One fiction per sample, reused everywhere.** Northsea is also the fictional
  company in `scripts/contexteval/`, down to the product names and the domain
  vocabulary, so a term decision means the same thing in the eval corpus and in
  the recording.
- **The sample is the fixture.** A harness demo seeds its sandbox from the
  sample directory with `fixturesFrom:` rather than keeping a private copy, so
  the tree in a recording is the tree a reader clones.
- **In-fiction copy is written in the fiction's own register.** The restrained
  register in `docs/internals/brand-communication.md` governs a sample's README
  and any prose *about* the sample, not the product copy inside it.
- **Nothing here resembles a real customer.** Names, quotations and figures are
  invented.
