---
title: Terminology-Assisted Pre-Translation
sidebar_label: Pre-Translation with Terminology
sidebar_position: 2
---

# Pre-Translating Files with a Termbase

You're about to translate a batch of UI strings into French. Your team has spent months building an approved glossary — hundreds of terms covering product features, security concepts, and marketing language. You want those terms used consistently in the translation, not left to chance.

Kapi lets you combine translation memory, AI translation, and terminology in a single pipeline. The termbase guides the translation process: approved terms are looked up automatically and fed into the AI translator as context, so the output uses your preferred terminology from the start.

## The Scenario

Your project has:

- `messages_en.json` — English source strings for a web application
- `glossary.csv` — an approved bilingual glossary (English/French) maintained by your terminology team
- A translation memory from previous releases, stored as `project.tmx`

The goal: produce a French translation that reuses previous translations where possible and follows your approved terminology everywhere else.

## Step 1: Set Up Your Language Assets

Import the glossary and translation memory into named resources so they're available across sessions:

```bash
# Import terminology
kapi termbase import glossary.csv \
  --name product-terms \
  --format csv \
  -s en -t fr

# Import translation memory
kapi tm import project.tmx \
  --name project-tm \
  -s en -t fr
```

Verify both are ready:

```bash
kapi termbase stats --name product-terms
kapi tm stats --name project-tm
```

## Step 2: Pre-Translate with TM Leverage

Start by filling in segments that have exact or fuzzy matches in your translation memory. This reuses proven translations and reduces the volume that needs fresh translation:

```bash
kapi tm-leverage \
  -i messages_en.json \
  -o messages_fr_step1.json \
  --source-lang en \
  --target-lang fr \
  --tm project-tm
```

Segments with exact matches get their previous translations applied automatically. Fuzzy matches (70%+ similarity by default) are also filled in, with the match score recorded so reviewers know which segments may need adjustment.

## Step 3: Translate Remaining Segments with AI + Terminology

Now translate the segments that the TM didn't cover. The `--termbase` flag provides your glossary as context to the AI translator, so it uses "mot de passe" instead of inventing its own translation for "password":

```bash
kapi ai-translate \
  -i messages_fr_step1.json \
  -o messages_fr.json \
  --source-lang en \
  --target-lang fr \
  --termbase product-terms
```

The AI translator receives your approved terms as additional context. When it encounters a source segment containing "encryption", it knows to use "chiffrement" — not "cryptage" or "encodage".

## Step 4: Verify Terminology Compliance

Even with terminology-guided translation, it's worth running a final QA pass to confirm everything landed correctly:

```bash
kapi qa-check \
  -i messages_fr.json \
  -o qa-report.json \
  --source-lang en \
  --target-lang fr \
  --termbase product-terms
```

This catches any segments where the AI chose a different phrasing despite the terminology hints — rare, but possible with creative source content.

## The Full Pipeline at a Glance

Here's the complete workflow from source file to verified translation:

```bash
# 1. Import language assets (one-time setup)
kapi termbase import glossary.csv --name product-terms --format csv -s en -t fr
kapi tm import project.tmx --name project-tm -s en -t fr

# 2. Leverage translation memory
kapi tm-leverage \
  -i messages_en.json -o messages_fr_tm.json \
  --source-lang en --target-lang fr \
  --tm project-tm

# 3. AI-translate remaining segments with terminology guidance
kapi ai-translate \
  -i messages_fr_tm.json -o messages_fr.json \
  --source-lang en --target-lang fr \
  --termbase product-terms

# 4. Run terminology QA
kapi qa-check \
  -i messages_fr.json -o messages_fr_qa.json \
  --source-lang en --target-lang fr \
  --termbase product-terms
```

## Processing Multiple Languages

The same glossary can contain terms in multiple languages. When expanding to additional locales, repeat the flow for each target:

```bash
# Import German terms into the same termbase
kapi termbase import glossary_de.csv --name product-terms --format csv -s en -t de

# Translate to German
kapi ai-translate \
  -i messages_en.json -o messages_de.json \
  --source-lang en --target-lang de \
  --termbase product-terms
```

Since the termbase is concept-oriented, a single database can hold terms for all your target languages. Each concept links "password" (en) to "mot de passe" (fr), "Passwort" (de), and "contrasena" (es) — all under one roof.

## When to Use This Approach

This workflow works best when you have:

- **An established glossary** — even a small one with 50-100 key terms makes a noticeable difference in translation consistency.
- **Recurring content** — product UI strings, documentation, marketing copy that gets updated and re-translated across releases.
- **Multiple target languages** — terminology consistency becomes harder to maintain manually as you add languages. A shared termbase keeps all languages aligned.
- **A mix of new and updated content** — TM leverage handles the recycled segments, and terminology-guided AI handles the rest.

For one-off translations of short documents, `kapi ai-translate` without a termbase may be sufficient. The termbase adds the most value when consistency across files, releases, or languages matters.

## Related

- [Terminology QA](/kapi-cli/use-cases/terminology-qa) — validating terminology compliance in translated files
- [kapi termbase](/commands?id=termbase) — full command reference
- [kapi tm](/commands?id=tm) — translation memory command reference
- [kapi run](/commands?id=run) — run command reference
- [Terminology Management](/features/terminology) — how the terminology system works
- [Translation Memory](/features/translation-memory) — how TM matching works
