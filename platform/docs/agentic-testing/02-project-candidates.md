# Open Source Project Candidates

## Selection Criteria

| Criterion | Weight | Rationale |
|-----------|--------|-----------|
| Active development | High | Agents need ongoing changes to react to |
| Translatable content variety | High | Showcases Bowrain's format breadth |
| Multiple file formats | High | JSON + Markdown + HTML + YAML minimum |
| Existing i18n | Medium | Enables quality benchmarking |
| Manageable size | Medium | Not so large it overwhelms, not trivial |
| Clear release cadence | High | Enables chronological walkthrough |
| Permissive license | Medium | MIT/Apache preferred for unrestricted forking |

## Recommended Starting Set (Tier 1)

These four projects form the initial cohort — balanced across size, formats, and complexity.

### 1. Docusaurus — Documentation Framework

- **URL:** github.com/facebook/docusaurus
- **License:** MIT
- **Stars:** ~64k

**Why it's ideal:**
Docusaurus is the gold standard for documentation-site i18n. Its own website is translated via Crowdin using a structured folder layout. This gives you JSON translation files (`code.json`, `footer.json`, `navbar.json`), Markdown documentation pages, MDX components, and YAML config — a rich format mix in a single repo.

**What to localize:**
- Theme/UI strings → JSON (`i18n/{locale}/code.json`)
- Documentation pages → Markdown/MDX (`docs/`)
- Blog posts → Markdown (`blog/`)
- Config labels → YAML/JS (`docusaurus.config.js`, `_category_.yml`)

**Estimated volume:** ~200 Markdown docs, ~10 JSON translation files per locale, ~50k words

**Release cadence:** Minor releases every 2-3 months, patches more frequently

**Format coverage:** JSON, Markdown, MDX, YAML

**Challenges:** Monorepo (Lerna) — target `website/` subtree. MDX with JSX interpolation adds complexity (good stress test).

**Agent team:**
- 1 Developer Agent (manages fork, pushes content)
- 1 Brand Manager (technical documentation voice)
- 3 Translator Agents (fr-FR, de-DE, ja-JP)
- 1 PM Agent

---

### 2. Gitea — Self-Hosted Git Forge

- **URL:** github.com/go-gitea/gitea
- **License:** MIT
- **Stars:** ~54k

**Why it's ideal:**
Gitea uses INI-format locale files — a less common format that tests Bowrain's breadth beyond JSON. The English locale file has 4000+ keys. The project also has HTML templates, Markdown docs, and YAML configs. Clear monthly releases with tagged versions make chronological walkthrough straightforward.

**What to localize:**
- UI strings → INI (`options/locale/locale_en-US.ini`)
- Email templates → HTML (`templates/mail/`)
- Documentation → Markdown (`docs/`)
- Config descriptions → INI/YAML

**Estimated volume:** ~4000 translation keys, ~40 locale files, substantial docs

**Release cadence:** Monthly patches (1.25.x), annual major versions

**Format coverage:** INI, HTML, Markdown, YAML

**Challenges:** INI format parsing. The locale file is very large (scalability test). Go template syntax in HTML.

**Agent team:**
- 1 Developer Agent
- 1 Brand Manager (developer tool voice)
- 2 Translator Agents (fr-FR, zh-CN)
- 1 QA Agent (format validation focus)

---

### 3. Home Assistant Frontend — Smart Home UI

- **URL:** github.com/home-assistant/frontend
- **License:** Apache 2.0
- **Stars:** ~5k (frontend); core has 77k+

**Why it's ideal:**
Home Assistant has one of the most mature community-driven translation systems in open source — 60+ languages, 10,000+ translation keys, managed via Lokalise. The deeply nested JSON structure and per-integration `strings.json` files provide excellent format complexity. Strict monthly release calendar (first Wednesday) is perfect for chronological simulation.

**What to localize:**
- UI strings → JSON (`src/translations/en.json`, deeply nested)
- Integration descriptions → JSON (per-component `strings.json`)
- Documentation → Markdown
- Config examples → YAML

**Estimated volume:** ~10,000+ translation keys, 60+ languages, extensive docs

**Release cadence:** Monthly (strict calendar), predictable

**Format coverage:** JSON (nested), YAML, Markdown

**Challenges:** Very large volume — start with a subset (e.g., core UI only, not all integrations). Frontend-only repo (core is separate).

**Agent team:**
- 1 Developer Agent
- 2 Brand Managers (IoT terminology, user-facing UI voice)
- 4 Translator Agents (fr-FR, de-DE, ja-JP, pt-BR)
- 1 QA Agent
- 1 PM Agent

---

### 4. Tolgee — Localization Platform (Meta-Candidate)

- **URL:** github.com/tolgee/tolgee-platform
- **License:** Apache 2.0
- **Stars:** ~4k

**Why it's ideal:**
Localizing a competing localization platform with Bowrain is a compelling demo story. Tolgee's codebase is Kotlin/Spring Boot + React/TypeScript, with JSON translation files, Markdown docs, and YAML config. Small enough to iterate quickly, well-structured, and Apache 2.0 licensed.

**What to localize:**
- Platform UI strings → JSON
- Documentation site → Markdown
- API descriptions → YAML/JSON
- Configuration → YAML

**Estimated volume:** ~1000 translation keys, docs site, smaller but diverse

**Release cadence:** Monthly releases

**Format coverage:** JSON, Markdown, YAML

**Challenges:** Smaller community. Being well-maintained means fewer "interesting" localization problems — but excellent for benchmarking quality.

**Agent team:**
- 1 Developer Agent
- 1 Brand Manager (platform/SaaS voice)
- 2 Translator Agents (fr-FR, de-DE)

---

## Expansion Set (Tier 2)

Add these after Tier 1 is stable to increase diversity and volume.

### 5. Excalidraw — Collaborative Whiteboard

- **URL:** github.com/excalidraw/excalidraw
- **License:** MIT
- **Stars:** ~13k
- **Formats:** JSON (i18next), Markdown
- **Volume:** ~500-800 keys, 20+ languages
- **Why:** Clean React codebase, small and focused, good for testing rapid iterations
- **Agent team:** 1 Developer + 2 Translators (minimal team)

### 6. Immich — Photo Management Platform

- **URL:** github.com/immich-app/immich
- **License:** AGPL-3.0
- **Formats:** JSON, Markdown, YAML, ARB (Flutter)
- **Volume:** ~1500 keys, 20+ languages
- **Why:** Cross-platform (web + mobile), ARB format tests plugin system, extremely active development
- **Agent team:** 1 Developer + 1 Brand Manager + 3 Translators

### 7. Cal.com — Scheduling Platform

- **URL:** github.com/calcom/cal.com
- **License:** AGPL-3.0
- **Formats:** JSON (i18next), Markdown, YAML
- **Volume:** ~2000 keys, 65+ languages
- **Why:** Heavy i18n investment, frequent releases, large translation surface
- **Agent team:** 1 Developer + 1 Brand Manager + 4 Translators + 1 PM

### 8. Grafana — Observability Platform

- **URL:** github.com/grafana/grafana
- **License:** AGPL-3.0
- **Formats:** JSON, YAML, Markdown, HTML
- **Volume:** ~5000 keys, growing language support
- **Why:** Enterprise-grade software, maturing i18n system, complex plugin i18n story
- **Agent team:** 1 Developer + 2 Brand Managers + 3 Translators + 1 QA

## Aspirational Set (Tier 3)

For maximum showcase impact, if resources permit.

### 9. Bitwarden Clients — Security Suite

- Multiple client targets (web, desktop, browser, CLI) from one repo
- 3000+ keys, 50 languages, monthly releases
- Multi-client consistency testing

### 10. n8n — Workflow Automation

- 400+ integration node descriptions as translation targets
- Non-standard localization targets (node metadata)
- Very high release velocity (weekly)

---

## Comparison Matrix

| Project | License | Formats | Keys | Languages | Releases | Complexity |
|---------|---------|---------|------|-----------|----------|------------|
| **Docusaurus** | MIT | JSON, MD, MDX, YAML | ~2k | 20+ | Monthly | Medium |
| **Gitea** | MIT | INI, HTML, MD, YAML | ~4k | 30+ | Monthly | Medium |
| **Home Assistant** | Apache | JSON, YAML, MD | ~10k | 60+ | Monthly | High |
| **Tolgee** | Apache | JSON, MD, YAML | ~1k | 10+ | Monthly | Low |
| Excalidraw | MIT | JSON, MD | ~600 | 20+ | Quarterly | Low |
| Immich | AGPL | JSON, MD, YAML, ARB | ~1.5k | 20+ | Weekly | Medium |
| Cal.com | AGPL | JSON, MD, YAML | ~2k | 65+ | Weekly | Medium |
| Grafana | AGPL | JSON, YAML, MD, HTML | ~5k | 10+ | Monthly | High |

## Fork Strategy

### Mirror vs. Fork

**Option A: GitHub Fork (Recommended for Tier 1)**
- Fork to a Bowrain org (e.g., `bowrain-l10n/docusaurus`)
- Maintain a `bowrain-main` branch tracking upstream `main`
- Create `l10n/*` branches for localization work
- PRs from `l10n/*` → `bowrain-main` show translation diffs

**Option B: Selective Mirror**
- Clone only translatable content (docs, locales, templates)
- Smaller footprint, faster operations
- Loses git history context

**Option C: Release Snapshot**
- Download tagged releases sequentially
- Simulates walking through release history
- Simplest but loses branch/PR workflow

**Recommendation:** Start with Option A for authenticity. The full fork shows Bowrain working with real git workflows. Option C is a good supplement for "time acceleration" demos.

### Release Walkthrough Strategy

To simulate months of activity compressed into shorter timeframes:

1. **Identify release tags:** `git tag --list 'v*' --sort=version:refname`
2. **Walk forward:** For each release N → N+1:
   - Merge upstream changes into `bowrain-main`
   - Developer Agent pushes new/changed content
   - Translators process the delta
   - This creates authentic-looking activity over "time"
3. **Pacing:** Process one release per day (configurable)
4. **Branch creation:** Major versions get streams; patches update the main stream
