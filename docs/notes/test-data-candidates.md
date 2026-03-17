# Realistic Test Data: Open Source Project Candidates

## Goal

Identify open-source projects suitable for forking/mirroring to create realistic test data for neokapi. Ideal candidates have:

1. **Multi-modality** — branding and translation needs across app, website, docs, CLI
2. **Format diversity** — use multiple localization formats that neokapi supports
3. **Scale** — enough strings to stress-test pipelines (thousands, not dozens)
4. **Permissive license** — allows forking and redistributing derivative test data
5. **Active translation community** — real-world messy data (partial translations, plurals, placeholders, context)

## Tier 1 Candidates (Best Fit)

### 1. Home Assistant

| Attribute | Detail |
|---|---|
| License | Apache 2.0 |
| Repo | `home-assistant/core`, `home-assistant/frontend` |
| Modalities | Web app, mobile app (companion), docs site, CLI |
| Formats | JSON (`strings.json` per integration), YAML (`services.yaml`), Markdown (docs) |
| Scale | 2000+ integrations, each with its own `strings.json`; 60+ languages |
| Why | Nested JSON with placeholders (`{count}`), cross-references (`[%key:...]`), platform-specific files (`strings.sensor.json`). YAML service definitions. Markdown documentation site. Exercises JSON, YAML, and Markdown readers simultaneously. |

**Approach:** Fork `home-assistant/core` and `home-assistant/frontend`. Extract `homeassistant/components/*/strings.json` + `services.yaml` as a corpus. Use the docs repo for Markdown content. Create flows that process all three modalities in a single pipeline.

### 2. Nextcloud

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 (server), various per app |
| Repo | `nextcloud/server` + 200+ app repos |
| Modalities | Web app, desktop client, mobile apps (iOS/Android), docs site |
| Formats | PO/POT → JSON/JS, PHP, HTML, Markdown |
| Scale | 30+ apps, each with `translationfiles/` (PO) and `l10n/` (JSON); 100+ languages |
| Why | Classic gettext workflow (POT templates → PO → compiled JSON). Multiple apps share a common l10n pattern. HTML templates. PHP content files. Exercises PO, JSON, HTML, PHP, and Markdown readers. |

**Approach:** Fork `nextcloud/server`. The `translationfiles/` directories contain PO/POT files; `l10n/` has the compiled JSON. Use both as parallel test inputs. Website/docs provide HTML and Markdown content. The multi-app structure tests batch processing.

### 3. Mastodon

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 |
| Repo | `mastodon/mastodon` |
| Modalities | Web app (Rails), mobile apps (iOS/Android separate repos), docs site |
| Formats | YAML (`config/locales/*.yml` — Rails i18n), JSON (API responses), Markdown (docs) |
| Scale | 80+ locale YAML files, deeply nested keys, pluralization rules |
| Why | Rails YAML i18n is a rich format with nested namespaces, plurals (one/other/many/few), and HTML-in-YAML. Mobile apps use separate formats (Strings/XML). Docs are Markdown. Good for testing YAML reader with complex structures. |

**Approach:** Fork `mastodon/mastodon`. The `config/locales/` directory is the primary corpus. Cross-reference with the iOS repo (`.strings` files) and Android repo (XML) for multi-platform coverage.

### 4. GitLab (CE)

| Attribute | Detail |
|---|---|
| License | MIT (CE) |
| Repo | `gitlab-org/gitlab` |
| Modalities | Web app, CLI (`glab`), docs site, API docs |
| Formats | PO/POT (gettext), compiled to JSON for frontend, Markdown (docs), YAML (config) |
| Scale | Massive — 50,000+ translatable strings, 80+ languages |
| Why | Largest PO corpus in open source. Real-world complexity: string interpolation, plurals, context markers. Frontend JSON compilation. Huge Markdown docs site. Tests PO reader at scale. |

**Approach:** Clone the `locale/` directory from GitLab CE. The `.pot` template alone is a stress test. Pair with the docs site (Markdown) for multi-modality.

## Tier 2 Candidates (Good Supplementary Data)

### 5. Bitwarden

| Attribute | Detail |
|---|---|
| License | GPL-3.0 (clients), AGPL-3.0 (server) |
| Modalities | Web vault, desktop app, mobile apps, browser extension, CLI |
| Formats | JSON (web/desktop), RESX/XML (.NET mobile), Markdown (docs) |
| Why | Same product across 6 platforms with different i18n formats per platform. Tests cross-platform consistency scenarios. |

### 6. WordPress

| Attribute | Detail |
|---|---|
| License | GPL-2.0 |
| Modalities | Web CMS, REST API, docs, thousands of plugins/themes |
| Formats | POT/PO/MO (gettext), JSON, PHP |
| Scale | Core has 10,000+ strings; plugin ecosystem is enormous |
| Why | The canonical gettext project. POT→PO→MO workflow. Paired JSON for JS frontend. Real translator comments, fuzzy markers, plural forms. |

### 7. Grafana

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 |
| Modalities | Web dashboard, CLI, docs site |
| Formats | JSON (i18next), Markdown (docs) |
| Why | Modern i18next JSON with namespaces and interpolation. Large dashboard UI with data visualization terminology. |

### 8. Signal

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 (clients) |
| Modalities | Desktop (Electron), Android, iOS |
| Formats | JSON (desktop), Android XML (Android), .strings (iOS) |
| Why | Same UX across three platforms, three different i18n formats. Security/privacy terminology is specialized. Good for testing format conversion flows. |

## Approaches

### A. Curated Subset Fork

**Best for: getting started quickly**

1. Pick 2-3 Tier 1 projects
2. Fork repos, extract only the localization-relevant files
3. Create a `testdata/projects/` directory structure:
   ```
   testdata/projects/
   ├── home-assistant/
   │   ├── strings/          # JSON strings.json files
   │   ├── services/         # YAML service definitions
   │   └── docs/             # Markdown documentation
   ├── nextcloud/
   │   ├── po/               # PO/POT translation files
   │   ├── l10n/             # Compiled JSON
   │   └── templates/        # HTML/PHP templates
   └── mastodon/
       ├── locales/          # YAML locale files
       └── docs/             # Markdown docs
   ```
4. Include 3-5 languages per project (en + 2 complete + 2 partial)
5. Version-pin to specific commits for reproducibility

### B. Git Submodule Mirror

**Best for: staying current with upstream**

1. Add candidate repos as git submodules under `testdata/upstream/`
2. Write extraction scripts that pull relevant files into a normalized structure
3. CI job periodically updates submodules and re-extracts
4. Pro: always fresh data. Con: larger repo, extraction scripts to maintain.

### C. Synthetic Augmentation

**Best for: edge case coverage**

1. Start with real project data (Approach A or B)
2. Use neokapi's own tools to generate synthetic variants:
   - Pseudo-translate to create target locales
   - Inject known errors for QA tool testing
   - Generate XLIFF/TMX from PO sources (format conversion testing)
   - Create partial translations (remove random % of strings)
3. This creates a ground-truth dataset where expected outputs are known

### D. "Brand Kit" Test Scenario

**Best for: demonstrating neokapi's value proposition**

1. Create a fictional brand (e.g., "Acme Cloud") that mirrors a real project's structure
2. Source content from multiple candidates to populate:
   - `app/` — JSON/YAML UI strings (from Home Assistant pattern)
   - `website/` — HTML marketing pages + Markdown docs (from Nextcloud pattern)
   - `desktop/` — Properties/RESX files (from Bitwarden pattern)
   - `cli/` — PO/gettext strings (from GitLab pattern)
   - `media/` — SRT/VTT subtitles for product videos
3. This tests the full neokapi pipeline across all modalities in one coherent scenario

## Recommended Starting Point

Combine **Approach A** (curated subsets) with **Approach D** (brand kit):

1. **Phase 1:** Extract curated subsets from Home Assistant + Nextcloud + Mastodon
2. **Phase 2:** Build a synthetic "Acme Cloud" brand kit that stitches together content patterns from all three, covering JSON, YAML, PO, HTML, Markdown, and subtitles
3. **Phase 3:** Add GitLab PO corpus for scale testing

This gives you both realistic organic data and a controlled multi-modal test scenario.

## Format Coverage Matrix

| Format | Home Assistant | Nextcloud | Mastodon | GitLab | Bitwarden | WordPress |
|---|---|---|---|---|---|---|
| JSON | ● | ● | | ● | ● | ● |
| YAML | ● | | ● | ● | | |
| PO/POT | | ● | | ● | | ● |
| HTML | | ● | | | | |
| Markdown | ● | ● | ● | ● | ● | |
| Properties | | | | | | |
| RESX/XML | | | | | ● | |
| PHP | | ● | | | | ● |
| .strings | | | ● (iOS) | | ● (iOS) | |
| Android XML | | | ● | | ● | |
| SRT/VTT | | | | | | |

neokapi supports all of these formats natively (44 formats total).

## Monolingual Candidates (English-Only Projects)

These are popular open-source projects that are **currently English-only** or have only nascent/incomplete i18n. They represent the "greenfield localization" use case — where neokapi adds the most value by enabling translation from scratch.

### Why Monolingual Projects Matter for Test Data

1. **Greenfield scenario** — Tests neokapi's ability to extract, pseudo-translate, and round-trip content that was never designed for i18n
2. **Format extraction challenge** — Hardcoded strings in HTML/Markdown/code need format-aware extraction (neokapi's core value)
3. **Branding consistency** — Same product across app + website + docs with no existing translation workflow
4. **Realistic customer profile** — Most neokapi users will be adding localization to existing monolingual products, not improving already-translated ones

### Tier 1: Fully Monolingual (No i18n Infrastructure)

#### 1. Plausible Analytics

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 |
| Repo | `plausible/analytics` |
| Stars | 22k+ |
| Stack | Elixir/Phoenix + React |
| Modalities | Web dashboard, email reports, docs site (Markdown) |
| i18n status | **English only.** Multiple community requests ([Discussion #471](https://github.com/plausible/analytics/discussions/471), [#354](https://github.com/plausible/analytics/discussions/354), [#868](https://github.com/plausible/analytics/discussions/868), [#1478](https://github.com/plausible/analytics/discussions/1478)) but no implementation. Half their subscribers are EU-based. |
| Content types | Phoenix templates (HTML), React components (JSX), email templates, Markdown docs |
| Why | Clean, small UI with well-defined strings. Perfect for demonstrating "add i18n to an existing product" workflow. Email reports + dashboard + docs = multi-modal. |

#### 2. Papermark

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 |
| Repo | `mfts/papermark` |
| Stars | 5k+ |
| Stack | Next.js + TypeScript |
| Modalities | Web app, document viewer, email notifications, marketing site |
| i18n status | **English only.** No i18n infrastructure, no translation files, no open issues requesting it. |
| Content types | TSX components with hardcoded strings, HTML email templates, Markdown docs |
| Why | DocSend alternative with rich document sharing UI. Multi-modal: app, viewer, emails, marketing. Small enough to fully localize as a demo. |

#### 3. Infisical

| Attribute | Detail |
|---|---|
| License | MIT (with enterprise features) |
| Repo | `Infisical/infisical` |
| Stars | 18k+ |
| Stack | Next.js + TypeScript + Node.js |
| Modalities | Web dashboard, CLI, docs site, API docs |
| i18n status | **English only.** No i18n infrastructure found. Developer-focused tool with growing enterprise adoption where localization would matter. |
| Content types | React components (TSX), CLI output strings, Markdown documentation, YAML configs |
| Why | Secrets management platform with dashboard + CLI + docs. Good example of developer tool needing enterprise-grade localization. CLI strings test a different modality than web UI. |

#### 4. Trigger.dev

| Attribute | Detail |
|---|---|
| License | Apache 2.0 |
| Repo | `triggerdotdev/trigger.dev` |
| Stars | 10k+ |
| Stack | Next.js + TypeScript |
| Modalities | Web dashboard, CLI, docs site |
| i18n status | **English only.** No i18n system. Developer-facing workflow automation platform. |
| Content types | React TSX, CLI output, Markdown docs |
| Why | AI workflow platform with dashboard UI + CLI + extensive docs. Growing enterprise user base. |

### Tier 2: Nascent i18n (Started But Incomplete)

#### 5. Documenso

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 |
| Repo | `documenso/documenso` |
| Stars | 10k+ |
| Stack | Next.js + TypeScript |
| Modalities | Web app, document signing flow, email notifications, docs |
| i18n status | **Feature requested** ([Issue #885](https://github.com/documenso/documenso/issues/885), [Backlog #22](https://github.com/documenso/backlog/issues/22)). Goal: "support every language on earth." Some initial work with ParaglideJS proposed but not complete. Toast messages not marked for translation ([Issue #2273](https://github.com/documenso/documenso/issues/2273)). |
| Content types | React components, PDF signing templates, HTML email, Markdown docs |
| Why | DocuSign alternative. Document signing has legal/regulatory localization requirements. Email + PDF + web app = rich multi-modal scenario. The "partially started" state is realistic for many customers. |

#### 6. Plane

| Attribute | Detail |
|---|---|
| License | AGPL-3.0 |
| Repo | `makeplane/plane` |
| Stars | 33k+ |
| Stack | Next.js + TypeScript + Django |
| Modalities | Web app, mobile app (Android), docs site |
| i18n status | **In progress.** Recently added `@plane/i18n` package with JSON translation files for en, es, fr, ja. Community contributing zh-CN, ka-ge, ru. Many components still have hardcoded strings. ~2400 translatable strings identified. |
| Content types | React TSX, JSON translation files (nascent), Python backend strings, Markdown docs |
| Why | Jira/Linear alternative with 33k stars. The "partially localized" state is the most common real-world scenario. JSON translation files already exist but are incomplete — perfect for testing TM leverage, gap analysis, and pseudo-translation of untranslated segments. |

### Approach for Monolingual Projects

#### E. String Extraction + Pseudo-Translation Pipeline

**Best for: demonstrating neokapi's greenfield value**

1. Fork a monolingual project (Plausible or Papermark recommended — small, clean)
2. Use neokapi to:
   a. **Extract** translatable strings from HTML/Markdown/JSX source
   b. **Generate** initial resource files (JSON, PO, or XLIFF)
   c. **Pseudo-translate** to create test locales (qps-ploc)
   d. **Round-trip** — verify the pseudo-translated output renders correctly
3. This creates a before/after showcase of neokapi's localization enablement

#### F. "Day Zero" Localization Kit

**Best for: end-to-end demo of the full workflow**

1. Pick one monolingual project (e.g., Plausible)
2. Create a complete localization kit:
   ```
   testdata/greenfield/plausible/
   ├── source/
   │   ├── dashboard/          # Extracted HTML/JSX strings → JSON
   │   ├── emails/             # Email templates → HTML
   │   └── docs/               # Markdown documentation
   ├── tm/
   │   └── analytics-tm.tmx    # Seeded TM from similar projects
   ├── termbase/
   │   └── analytics-terms.csv # Domain terminology (pageview, bounce rate, etc.)
   ├── flows/
   │   ├── extract.yaml        # String extraction flow
   │   ├── pseudo.yaml         # Pseudo-translation flow
   │   ├── translate.yaml      # MT-assisted translation flow
   │   └── qa.yaml             # Quality assurance flow
   └── targets/
       ├── fr/                 # French pseudo-translated output
       ├── de/                 # German pseudo-translated output
       └── ja/                 # Japanese pseudo-translated output
   ```
3. This demonstrates the complete neokapi value chain: extract → TM seed → translate → QA

## Updated Recommendation

### Combined Strategy

| Phase | Source | Use Case |
|---|---|---|
| **Phase 1** | Plausible Analytics (monolingual) | Greenfield: extract + pseudo-translate + round-trip |
| **Phase 2** | Home Assistant + Mastodon (already translated) | Mature: format diversity, scale, TM leverage |
| **Phase 3** | Plane (partially translated) | In-progress: gap analysis, incremental translation |
| **Phase 4** | GitLab CE (massive corpus) | Scale: stress-test with 50k+ strings |
| **Phase 5** | Synthetic "Acme Cloud" brand kit | Multi-modal: stitch together all formats in one scenario |

This progression covers the full customer journey: greenfield → partial → mature → scale.
