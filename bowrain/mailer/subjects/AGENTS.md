<!-- kapi:rules (written by kapi from this project's context; edit outside this section) -->
## Writing rules for bowrain/mailer/subjects/

These rules hold for the files in bowrain/mailer/subjects/, beside the project's rules in the root AGENTS.md. Where the two differ, follow this file. kapi writes this section from the project's context, so edit outside it.

Voice: bowrain platform. Register for Bowrain's user-facing text: landing, app UI, docs, and email. Addressed to the person accountable for content across a team, not to a developer reading a reference. Explain by contrast and state the outcome. Direct, concrete, assured, neutral register, neutral tone, no humor, active voice, short sentences, second person (you), contractions where natural. Lead with what changes for the reader, then the mechanism that makes it true. Explain by contrast when a distinction carries the point — a legal notice is not a help article. Address the reader as someone answerable for content quality, who has the problem already and does not need convincing it exists. Claims stay checkable: name what the product does, never how it feels to use. Sentences carry one idea.

Avoid:
- Marketing superlatives and hype words (such as powerful, blazing, game-changing, cutting-edge, revolutionary, unleash)
- Brochure framing (such as production-proven, everything you need, localize at scale, just point and go)
- Emoji in committed prose
- Hardcoded counts that the code controls — name categories and link to the generated reference instead (such as tools, providers, filters, languages)
- "magic" as a claim about the product — state the mechanism; the technical sense (magic bytes, magic number) is not this rule
- termbase: the store is the terms store, and its contents are terms (TBX, the ISO standard, keeps its name)
- glossary: the store is the terms store (XLIFF 2.x's Glossary module keeps its name) (such as ies)
- Say multilingual content, or language — and recast the sentence rather than substituting a word for "localize" (such as ing, ation, ations)
- Say multilingual content, or language (the l10n-* make targets are developer-facing internals and are not this rule)
- translation memory: say content memory (such as ies)
- Voice is the mechanism — say voice profile. "Brand voice" belongs in prose about the common use case, never as the name of the thing
- Credibility-by-assertion; show the mechanism instead (such as enterprise-grade, best-in-class, world-class, trusted by)

(The voice continues: `kapi context bowrain/mailer/subjects/<file>` gives it in full.)
This voice takes the place of the project's voice for these files.

Say this, not that:
- Avoid "solution": Says nothing; name the thing the product actually does
- context graph: What Bowrain holds — the relationships that govern content, not a content store
- workspace: The tenant boundary a team works inside; not "account" or "org"

Before you finish, run `kapi check <file>` on each file you changed and fix what it reports. `kapi context bowrain/mailer/subjects/<file>` gives the full answer for one file when you need more than this section.
<!-- /kapi:rules -->
