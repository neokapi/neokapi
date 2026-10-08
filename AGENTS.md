<!-- kapi:rules (written by kapi from this project's context; edit outside this section) -->
## Writing rules

These rules hold for every file in this project. Follow them as you write; you do not need to ask kapi first. kapi writes this section from the project's context and replaces it when the context changes, so edit outside it.

Voice: neokapi documentation. Academic, precise register for neokapi user-facing text: docs site, CLI help, UI copy, release notes. Explain what something is and what it does; let the facts stand without selling. Precise, plain, specific, formal register, neutral tone, no humor, active voice, short sentences, second person (you), contractions where natural. State capabilities plainly. Lead with the problem and the mechanism, not adjectives. Every claim must be checkable against the code or a generated artifact. One idea per sentence; short sentences over long ones. The reader is a developer or the person running the language work, not a prospect.

Avoid:
- Marketing superlatives and hype words (such as powerful, blazing, game-changing, cutting-edge, revolutionary, unleash)
- Brochure framing (such as production-proven, everything you need, localize at scale, just point and go)
- Content memory is the store; brand is a coordinate on it, not a kind of memory
- Retired positioning: say what the product does, not which axis it leads with (such as brand-first)
- Retired positioning: kapi converges content; drafting is one step in the loop
- Retired positioning: team framing belongs to the platform, not to kapi (such as for your whole team)
- Retired positioning: this is a content and language engine
- Emoji in committed prose
- Hardcoded counts that the code controls: name categories and link to the generated reference instead (such as tools, providers, filters, languages)
- "magic" as a claim about the product: state the mechanism; the technical sense (magic bytes, magic number) is not this rule
- Extraction, not segmentation, produces blocks; segmentation is an opt-in overlay within a block (such as segmented into blocks)
- termbase: the store is the terms store, and its contents are terms (TBX, the ISO standard, keeps its name)
- glossary: the store is the terms store (XLIFF 2.x's Glossary module keeps its name)
(The voice continues: `kapi context <path>` gives it in full.)

Say this, not that:
- voice profile, not "brand voice": The VoiceProfile bound to a project. Retired spelling: brand voice, which names the common use case, never the mechanism.
- Avoid "easily"
- term list, not "glossary": Simple source-to-target term mapping fed to the translate prompt. The prompt section keeps its wire name `glossary`.
- memory, not "sievepen": The built-in content memory package (memory/). Never translate.
- Avoid "simply"
- terms store, not "termbase" (also "terms"): The store of approved terminology. Retired spelling: termbase.
- text, not "translatable text": Frames the shared engine as translation-only; use neutral wording unless the context really is about translation
- content memory, not "translation memory": The store of source/target pairs kapi reuses; backed by the memory/ package.
- Bowrain: The full-stack platform. Never translate; capitalized product name.
- kapi: The standalone CLI. Never translate; always lowercase.
- kapi-desktop: The desktop GUI companion. Never translate; always lowercase and hyphenated.
- KBF: Kapi Bundle Format. Never translate the acronym.
- neokapi: Project and Go framework name. Never translate; always lowercase.
- Okapi: The upstream Okapi Framework (Java), which neokapi reimagines and which okapi-bridge exposes. Never translate; capitalized product name. Not the animal of the same name.
- okapi-bridge: The Java bridge exposing Okapi Framework filters to neokapi over gRPC, and its repository. Never translate; always lowercase and hyphenated.

Code comments follow a voice of their own: run `kapi context <path> --comments` before you write one.

Some folders have rules of their own, in the AGENTS.md and CLAUDE.md there. Read that file before you edit a file in one of them:
- bowrain/mailer/subjects/: a voice of its own.
- bowrain/web/docs/: a voice of its own.

Before you finish, run `kapi check <file>` on each file you changed and fix what it reports. `kapi context <path>` gives the full answer for one file when you need more than this section.
When the files keep to a name or a word this section does not list, record it with `kapi context note --term <used> --instead-of <avoided> --seen-in <file>`. A person decides what becomes a rule.
<!-- /kapi:rules -->
