# Activity Visualization & Demo Material

## Overview

The primary output of the agentic testing system isn't test results — it's **living activity** visible through Bowrain's own dashboards, feeds, and metrics. This document covers how to visualize, capture, and present that activity for demos, marketing, and internal evaluation.

## Built-in Bowrain Visualization

### Activity Feed

Bowrain's activity feed at `/{workspace}/activities` automatically captures every agent action:

```
14:32  Jean-Pierre Dubois  translated 28 blocks in landing-page.html (fr-FR)
14:15  Katrin Weber         translated 22 blocks in app-strings.json (de-DE)
13:45  Maria Santos         added 5 terminology concepts (cloud, deploy, ...)
13:30  Lisa Chen            created 4 translation tasks for v3.2 release
13:00  Alex Chen            pushed 142 new blocks from upstream v3.2.0
12:45  Taylor Kim           QA passed: 0 issues in fr-FR batch
```

**Key insight:** Because agents use real Bowrain accounts and real API calls, the activity feed is indistinguishable from human team activity. This is the most powerful demo asset.

### Dashboard Metrics

The workspace dashboard (`/{workspace}/dashboard`) shows:

- **Translation progress** — % complete per language per project
- **Words translated** — Running total, velocity chart
- **Active contributors** — Agent personas appear as real team members
- **Recent activity** — Last 24h summary
- **Project health** — Coverage, quality score, overdue tasks

### Translation Editor

The translate view (`/{workspace}/translate`) shows:

- Source/target side-by-side
- Translation status per block (untranslated, AI-translated, reviewed, approved)
- TM match indicators (fuzzy %, exact match)
- Terminology highlights
- Translator attribution ("Last edited by Jean-Pierre Dubois")

### Brand Dashboard

The brand view (`/{workspace}/brand-dashboard`) shows:

- Brand profiles per project
- Tone/style sliders with visual indicators
- Vocabulary lists (preferred, deprecated, forbidden)
- Compliance scores per file/language

## Custom Visualization Layer

Beyond Bowrain's built-in views, build a supplementary dashboard for the agentic system itself.

### Agent Activity Dashboard

A dedicated view showing agent-specific metrics:

```
┌─────────────────────────────────────────────────────────┐
│  Agentic Testing Dashboard                    Live ●     │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Projects: 4 active    Agents: 12 total    Sessions: 847 │
│                                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ Docusaurus   │  │ Gitea       │  │ HA Frontend │     │
│  │ ████████░░   │  │ ██████░░░░  │  │ ████░░░░░░  │     │
│  │ 78% complete │  │ 62% complete│  │ 41% complete│     │
│  │ 6 agents     │  │ 5 agents    │  │ 8 agents    │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│                                                          │
│  Agent Timeline (last 7 days)                            │
│  ─────────────────────────────────────────────────────── │
│  Alex Chen     ▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░           │
│  Maria Santos  ░▓░░░▓░░░▓░░░▓░░░▓░░░▓░░░▓░░           │
│  Jean-Pierre   ░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓           │
│  Katrin Weber  ░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓░░▓▓           │
│  Yuki Tanaka   ░░░░▓▓░░░░▓▓░░░░▓▓░░░░▓▓░░░░           │
│  Lisa Chen     ▓░░░▓░░░▓░░░▓░░░▓░░░▓░░░▓░░░           │
│                Mon  Tue  Wed  Thu  Fri  Sat  Sun         │
│                                                          │
│  Cost Tracking                                           │
│  AI Translation: $12.40 today / $156.80 this month      │
│  LLM Decisions:  $3.20 today  / $38.50 this month       │
│  Total:          $15.60 today / $195.30 this month       │
│                                                          │
│  Quality Metrics                                         │
│  TM Reuse Rate:  34% (growing)                           │
│  Brand Compliance: 92% avg across projects               │
│  QA Pass Rate:    88% first-pass                         │
└─────────────────────────────────────────────────────────┘
```

### Implementation Options

**Option A: Bowrain Web Extension**
- Add an `/agentic` route to the Bowrain web app
- Query Bowrain's own activity/metrics APIs
- Fully integrated experience

**Option B: Standalone Dashboard**
- Separate React app reading from Bowrain API + container logs
- Independent deployment
- Can be shared publicly (demo site)

**Option C: Grafana Dashboard**
- Export metrics to Prometheus
- Build Grafana dashboards
- Rich visualization, time-series analysis
- Heavy infrastructure

**Recommendation:** Start with **Option B** (standalone dashboard) for fast iteration. Migrate key views into Bowrain web app (Option A) once the metrics stabilize.

## Automated Screenshot & Recording Capture

### Periodic Screenshots

A separate Playwright-based screenshot service (or cron job) captures screenshots at key moments:

```typescript
// Screenshots captured automatically
const screenshotTriggers = [
  { event: "content_pushed",         view: "dashboard",     name: "post-push-dashboard" },
  { event: "translation_batch_done", view: "translate",     name: "translation-progress" },
  { event: "terminology_updated",    view: "termbase",      name: "termbase-growth" },
  { event: "brand_profile_created",  view: "brand-dashboard", name: "brand-setup" },
  { event: "qa_complete",            view: "activities",    name: "qa-results" },
  { event: "tasks_created",          view: "tasks",         name: "task-board" },
];

// Scheduled captures (hourly/daily)
const scheduledCaptures = [
  { cron: "0 * * * *",   view: "dashboard",   name: "hourly-dashboard" },
  { cron: "0 9 * * *",   view: "activities",  name: "daily-activity" },
  { cron: "0 0 * * 1",   view: "dashboard",   name: "weekly-overview" },
];
```

### Time-Lapse Generation

Compile periodic screenshots into time-lapse videos showing the platform evolving:

```bash
# Generate time-lapse from daily dashboard screenshots
ffmpeg -framerate 2 -pattern_type glob -i 'data/screenshots/daily-dashboard-*.png' \
  -c:v libx264 -pix_fmt yuv420p output/dashboard-timelapse.mp4
```

### Workflow Recordings

Use Playwright to record complete workflow sequences:

```typescript
// Record a translator work session
async function recordTranslatorSession(page: Page, wsSlug: string) {
  await page.video().start({ dir: "data/recordings" });

  // Inject cursor helper for natural-looking mouse movement
  await injectCursor(page);

  // Navigate to translation editor
  await humanClick(page, `a[href="/${wsSlug}/translate"]`);
  await page.waitForTimeout(1000);

  // Select a file
  await humanClick(page, "[data-testid='file-list'] li:first-child");
  await page.waitForTimeout(500);

  // Edit a translation
  await humanClick(page, "[data-testid='block-0'] .target-cell");
  await humanType(page, "Bienvenue sur notre plateforme");
  await page.waitForTimeout(2000);

  await page.video().stop();
}
```

## Demo Scenarios

### Scenario 1: "A Day in the Life"

Walk through 24 hours of agent activity, showing the natural flow:

```
09:00  Alex pushes upstream changes     → Dashboard shows new content spike
09:30  Lisa creates translation tasks   → Task board populates
10:00  Maria reviews new terms          → Termbase grows
14:00  Jean-Pierre translates (fr)      → Progress bar advances
14:30  Katrin translates (de)           → Second language catches up
20:00  Yuki translates (ja)             → Overnight progress in Asia timezone
06:00  Taylor runs QA checks            → Quality report generated
09:00  Alex pulls completed translations → Commits land in the repo
```

### Scenario 2: "Release Localization"

Show a complete release cycle from source change to localized release:

```
1. Upstream release v3.2.0 detected
2. Fork updated, 142 new/changed blocks pushed
3. 5 new terms identified and added to termbase
4. Brand profile updated for new feature terminology
5. Translation tasks assigned across 3 languages
6. AI generates initial translations (95% coverage in 5 minutes)
7. Human review: 28% edited by translators over 2 days
8. QA catches 3 placeholder issues → fixed
9. All languages at 100% → translations pulled
10. PR created with localized files
```

### Scenario 3: "Translation Memory in Action"

Demonstrate TM growth and cost savings over time:

```
Week 1: TM empty → 100% AI translation → $50 cost
Week 2: TM has 200 entries → 15% reuse → $42 cost
Week 4: TM has 800 entries → 28% reuse → $36 cost
Week 8: TM has 2000 entries → 40% reuse → $30 cost
Week 12: TM has 3500 entries → 52% reuse → $24 cost
```

### Scenario 4: "Multi-Project Brand Consistency"

Show how brand profiles and terminology ensure consistency across projects:

```
Docusaurus (docs):      "Deploy your site" → "Déployez votre site"
Gitea (UI):             "Deploy repository" → "Déployer le dépôt"
Home Assistant (IoT):   "Deploy automation" → "Déployer l'automatisation"

All three use the same term "déployer" because it's in the shared termbase
with status "preferred" across the bowrain-l10n workspace.
```

## Metrics to Track

### Platform Health Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| API response time (p95) | Server performance under agent load | < 500ms |
| Push/pull success rate | CLI reliability | > 99% |
| Auth token refresh rate | Token management health | 0 failures |
| Concurrent agent sessions | Load testing | Up to 10 |
| Database size growth | Storage planning | Linear |

### Translation Quality Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| AI acceptance rate | % of AI translations accepted without edit | 40-70% |
| TM reuse rate | % of blocks matched from TM | Growing over time |
| QA pass rate | % passing quality checks first time | > 85% |
| Brand compliance score | Avg compliance across checked content | > 90% |
| Time to translate | Hours from push to 100% translated | Decreasing |

### Agent Performance Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| Sessions per day | Agent activity level | Per persona config |
| Blocks per session | Throughput | Per persona config |
| Error rate | Agent failures per session | < 5% |
| LLM cost per session | AI spend per agent invocation | < $5 |
| Decision latency | Time for LLM to make translation decision | < 10s |

### Business Value Metrics

| Metric | Description | Demo Value |
|--------|-------------|------------|
| Total words translated | Cumulative throughput | "100k words across 4 projects" |
| Languages supported | Breadth | "3-4 languages per project" |
| File formats processed | Format breadth | "JSON, MD, HTML, YAML, INI, MDX" |
| TM entries created | Asset growth | "3,500 TM entries building over 12 weeks" |
| Terminology concepts | Knowledge base | "150 standardized terms" |
| Cost per word | Efficiency | "$0.02/word with TM, vs $0.05 pure AI" |

## Public Demo Site

### Concept

Host a read-only public demo of the agentic testing system:

- **Live dashboard** showing current agent activity
- **Project pages** with real translation progress
- **Activity timeline** showing weeks/months of activity
- **Metrics dashboard** with cost savings, quality trends
- **Before/after** comparisons of AI vs. human-reviewed translations

### Implementation

```
demo.bowrain.io/
├── /                    # Landing: "Watch AI agents localize real open source projects"
├── /projects            # List of active projects with progress
├── /projects/:id        # Per-project deep dive (embedded Bowrain views)
├── /agents              # Agent profiles with activity stats
├── /timeline            # Chronological activity feed
├── /metrics             # Quality, cost, throughput dashboards
└── /about               # How it works, methodology
```

### Content Strategy

The demo site itself becomes marketing content:

- Blog posts: "How we localized Docusaurus into 3 languages with AI agents"
- Case studies: Per-project deep dives with metrics
- Comparison: "AI-only vs. AI+human review quality scores"
- Time-lapses: Video of dashboards evolving over weeks
