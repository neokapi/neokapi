---
title: Terminology QA
sidebar_label: Terminology QA
sidebar_position: 1
---

# Catching Terminology Mistakes Before They Ship

You've just received translated files from a vendor. The translations read well, but are the right terms actually being used? Did the translator write "contrasenya" instead of your approved "contrassenya"? Did they use a deprecated term your team phased out last quarter?

Manually scanning hundreds of segments for terminology compliance is tedious and error-prone. Kapi can automate this: import your glossary, run a terminology QA check, and get a clear report of every violation — all from the command line.

## The Scenario

Your team maintains a product glossary in a spreadsheet. You've exported it as a CSV:

```csv
source,target,domain,status
password,mot de passe,security,preferred
username,nom d'utilisateur,security,preferred
log in,se connecter,ui,preferred
sign in,se connecter,ui,deprecated
dashboard,tableau de bord,ui,preferred
settings,paramètres,ui,preferred
encryption,chiffrement,security,preferred
```

A translator has delivered `messages_fr.json` — your French UI strings. You want to verify they used the approved terms consistently.

## Step 1: Import Your Glossary

First, create a named termbase so you can reuse it across projects:

```bash
kapi termbase import glossary.csv \
  --name product-terms \
  --format csv \
  -s en -t fr \
  --domain general
```

This stores the glossary in `~/.config/kapi/termbases/product-terms.db`. You only need to do this once — the termbase persists between sessions.

Verify the import:

```bash
kapi termbase stats --name product-terms
```

## Step 2: Spot-Check a Term

Before running a full QA pass, you can look up individual terms to confirm the termbase is set up correctly:

```bash
kapi termbase lookup "log in" --name product-terms -s en -t fr
```

This shows the approved French translation along with its status. If "sign in" also appears as deprecated, you'll see that too — useful for understanding what the translator should and shouldn't use.

For a broader search:

```bash
kapi termbase search "connect" --name product-terms -s en -t fr
```

## Step 3: Run Terminology QA on Translated Files

Now run the `qa-check` flow with your termbase attached. The `--termbase` flag tells kapi to include terminology enforcement in the QA pipeline:

```bash
kapi qa-check \
  -i messages_fr.json \
  -o qa-report.json \
  --source-lang en \
  --target-lang fr \
  --termbase product-terms
```

Kapi reads the translated file, checks every segment against your glossary, and writes the results. Segments where the source contains a known term but the translation doesn't use the approved equivalent are flagged as violations.

## What Gets Checked

The terminology QA process works in two stages:

1. **Term discovery** — kapi scans each source segment and identifies terms that appear in your glossary.
2. **Term enforcement** — for each discovered term, kapi checks whether the translation contains the approved or preferred target term.

A violation is raised when:

- The source contains "password" (a glossary term)
- But the French translation uses "code secret" instead of the approved "mot de passe"

Terms marked as `preferred` or `approved` are enforced by default. `Deprecated` and `forbidden` terms trigger warnings if they appear in the target.

## Scaling Up: QA Across Multiple Files

For larger deliveries, pass multiple input files:

```bash
kapi qa-check \
  -i translations/fr/*.json \
  -o qa-output/ \
  --source-lang en \
  --target-lang fr \
  --termbase product-terms \
  -j 4
```

The `-j 4` flag processes four files in parallel — helpful when you're checking hundreds of resource files from a large delivery.

## Keeping Your Termbase Current

As your product evolves, update the termbase:

```bash
# Import additional terms (new terms are added, existing ones updated)
kapi termbase import new-terms.csv --name product-terms --format csv -s en -t fr

# Export to share with translators
kapi termbase export --name product-terms --format csv -s en -t fr -o glossary-for-vendors.csv
```

You can also export as JSON for a full backup that preserves all metadata (domains, statuses, definitions):

```bash
kapi termbase export --name product-terms --format json -o glossary-backup.json
```

## Tips for Localization Engineers

- **Start small.** Import your most critical terms first — brand names, product features, UI actions. You can always expand later.
- **Use domains** to organize terms by area (security, marketing, UI). This makes it easier to apply the right glossary to the right content.
- **Share the glossary** with translators before they start. Export it as CSV and include it in your translation kit. Prevention is cheaper than correction.
- **Automate in CI.** Add terminology QA to your build pipeline so term violations are caught on every pull request, not just during manual review.

## Related

- [kapi termbase](/commands?id=termbase) — full command reference
- [kapi run](/commands?id=run) — run command reference
- [Terminology Management](/features/terminology) — how the terminology system works
- [Terminology-Assisted Pre-Translation](/kapi-cli/use-cases/terminology-pretranslation) — using your termbase to guide AI translation
