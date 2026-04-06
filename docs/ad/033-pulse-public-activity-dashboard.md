---
id: 033-pulse-public-activity-dashboard
sidebar_position: 33
title: "AD-033: Pulse — Public Activity Dashboard"
---

# AD-033: Pulse — Public Activity Dashboard

## Context

Open-source and community-driven localization projects need a public-facing
dashboard where potential contributors can discover translation progress, find
languages that need help, explore terminology, and see who is contributing.
Platforms like Crowdin and Weblate offer public project pages; Bowrain needs an
equivalent that showcases its strengths — brand voice, structured terminology,
stream-based workflows — while being lightweight enough to handle unauthenticated
traffic at scale.

Key use cases:

- **Open-source maintainers** share a link (`pulse.bowrain.cloud/my-project`)
  in their README so contributors can see translation status at a glance.
- **Community translators** discover which languages need help, who the top
  contributors are, and what terminology is expected.
- **Project managers** embed progress badges and link to a richer dashboard
  without exposing the full Bowrain workspace.

The existing badge endpoint (`GET /api/v1/badges/projects/:id`) proves demand
for public project visibility, but it only returns a single shields.io JSON
response. Pulse is the full-page evolution of that idea.

## Decision

### Separate SPA on a dedicated subdomain

Pulse is a **standalone single-page application** deployed at
`pulse.bowrain.cloud` (production) and `pulse.dev.bowrain.cloud` (staging). It
lives in the monorepo at `platform/apps/pulse/` and shares components from
`@neokapi/ui` via the existing npm workspace.

**Alternatives considered:**

| Approach                                 | Pros                                                        | Cons                                                                         |
| ---------------------------------------- | ----------------------------------------------------------- | ---------------------------------------------------------------------------- |
| Routes in the main web app               | Single build, shared auth                                   | Heavy bundle for public visitors; auth bypass complexity; harder CDN caching |
| Separate microservice                    | Full isolation                                              | Operational overhead; duplicated types/queries                               |
| **Standalone SPA, same server (chosen)** | Light bundle; no CORS; independent caching; clean public UX | One more app in the monorepo                                                 |

The bowrain-server detects the `Host` header and serves the Pulse SPA's
embedded static files when the hostname matches `pulse.*`. API calls from the
SPA hit `/api/v1/pulse/*` endpoints on the same origin — no CORS configuration
needed. This follows the same embed-and-serve pattern used by kapi-web.

### Visibility controls

Both workspaces and projects use a three-way `DashboardVisibility` enum:

```go
type DashboardVisibility string

const (
    DashboardPrivate  DashboardVisibility = "private"  // workspace members only
    DashboardUnlisted DashboardVisibility = "unlisted" // accessible via direct URL, not indexed
    DashboardPublic   DashboardVisibility = "public"   // listed, indexed, discoverable
)
```

- `Workspace.DashboardVisibility` — workspace-level, default `private`
- `Project.DashboardVisibility` — project-level, default `private`

Access rules:

| Workspace              | Project    | Result                                                                                                                    |
| ---------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------- |
| `public`               | `public`   | Fully discoverable — listed on any future directory, indexed by search engines                                            |
| `public` or `unlisted` | `unlisted` | Accessible via direct URL only — not listed on the workspace overview's project grid unless the visitor has the exact URL |
| `private`              | any        | 404 for unauthenticated visitors                                                                                          |
| any                    | `private`  | Project hidden from Pulse; 404 for direct project URL                                                                     |

**Unlisted** is the "share with a link" model — the workspace or project's
Pulse URL works, but it won't appear in search engines (served with
`X-Robots-Tag: noindex`) or any public directory. This is ideal for:

- Early-stage projects not yet ready for public attention
- Internal projects shared with specific external contributors via URL
- Testing the dashboard before going fully public

Non-accessible workspaces/projects return HTTP 404 (not 403) to prevent
enumeration.

When the workspace is `private` but the requester has a valid JWT and workspace
membership, the dashboard is still accessible — this lets workspace members
preview the dashboard before making it public or unlisted.

### Contributor identities

Pulse shows **public contributor identities** — real names, avatars, and
activity counts — the same way GitHub shows contributor graphs. This is the
expected behavior for open-source community dashboards where recognition
motivates contributions.

Individual contributors can opt out via their profile settings (a
`pulse_visible` boolean on the user profile, default `true`). When opted out,
their contributions still count toward aggregate stats but their name/avatar is
replaced with "Anonymous Contributor" on leaderboards and activity feeds.

### Terminology explorer scope

The terminology explorer's data sources are **configurable per workspace**. A
new `PulseTermSources` field on the workspace controls which concept sources
are exposed:

```go
type PulseTermSources struct {
    Terminology     bool `json:"terminology"`      // standard glossary terms
    BrandVocabulary bool `json:"brand_vocabulary"`  // product names, taglines, do/don't
}
```

Default: `terminology = true`, `brand_vocabulary = false`. This lets
open-source projects share their glossary without exposing proprietary brand
voice rules, while projects that want to publish brand guidelines can opt in.

The workspace settings UI shows checkboxes for each source under the Pulse
configuration section.

### URL-first filtering

All filter state lives in the URL. Every view's filters are encoded as query
parameters so that links are **shareable, bookmarkable, and navigable** with
browser back/forward.

URL structure:

```
/:workspace?project=my-app&language=fr&time=30d&type=translation&q=search+term
/:workspace/projects/:pid?language=fr&time=30d
/:workspace/leaderboard?time=30d&tab=contributors
/:workspace/terms?domain=brand&q=search
```

The frontend `PulseFilterContext` reads initial state from
`useLocation().search` on mount and calls `navigate({ search: ... }, { replace: true })` on every filter change. The FilterBar component renders
active filters as removable badges (tokens) and provides:

- **Filter keys**: `project`, `language`, `contributor`, `time`, `type`, `q`
- **Quick presets**: "This week", "Needs help" (< 50% complete), "Most active"
- **Autocomplete**: Suggested values fetched from the overview endpoint

This matches the UX pattern established by the agentic testing dashboard's
`FilterContext` (URL-synced tokens with popover suggestions and presets).

### Caching strategy

Public dashboards must handle load from unauthenticated visitors without
hammering the database. The caching approach mirrors the existing
`dashboardCache` in `platform/server/editor.go`:

| Endpoint           | Cache TTL | Rationale                                   |
| ------------------ | --------- | ------------------------------------------- |
| Workspace overview | 5 min     | Aggregates across projects; expensive query |
| Leaderboard        | 10 min    | Heaviest computation (period comparisons)   |
| Activity feed      | 1 min     | Near-realtime feel                          |
| Terminology        | 15 min    | Rarely changes                              |
| Project detail     | 2 min     | Per-project stats                           |

Cache keys include workspace slug, endpoint path, and normalized query
parameters. The event bus subscriber invalidates relevant cache entries on
content-changing events (block stored, project updated, term added).

For extreme traffic, an HTTP `Cache-Control` header with `public,
max-age=60` allows CDN edge caching of overview responses.

## Architecture

### API Endpoints

All under `/api/v1/pulse/` — public, no auth required (gated by
`public_dashboard` flag):

```
GET /api/v1/pulse/:workspace                              → workspace overview
GET /api/v1/pulse/:workspace/projects                     → project list
GET /api/v1/pulse/:workspace/projects/:pid                → project detail
GET /api/v1/pulse/:workspace/projects/:pid/lang/:locale   → locale detail
GET /api/v1/pulse/:workspace/activity                     → activity feed
GET /api/v1/pulse/:workspace/leaderboard                  → leaderboard
GET /api/v1/pulse/:workspace/terms                        → terminology browser
GET /api/v1/pulse/:workspace/terms/:cid                   → concept detail
```

Common query parameters:

- `time` — period filter: `today`, `this-week`, `this-month`, `30d`, `90d`, `all`
- `project` — filter by project name/ID
- `language` — filter by BCP-47 locale
- `contributor` — filter by contributor name
- `type` — activity type prefix filter
- `q` — free-text search
- `cursor` — pagination cursor for activity feed
- `limit` — page size (default 20, max 100)

### Key Response Types

```go
type PulseOverview struct {
    Workspace      PulseWorkspaceInfo    `json:"workspace"`
    Projects       []PulseProjectSummary `json:"projects"`
    TopLanguages   []PulseLanguageRank   `json:"top_languages"`
    TopContribs    []PulseContributor    `json:"top_contributors"`
    RisingStars    []PulseRisingStar     `json:"rising_stars"`
    RecentActivity []PulseActivity       `json:"recent_activity"`
    Stats          PulseGlobalStats      `json:"stats"`
}

type PulseGlobalStats struct {
    TotalProjects    int `json:"total_projects"`
    TotalLanguages   int `json:"total_languages"`
    TotalContributors int `json:"total_contributors"`
    TotalWords       int `json:"total_words"`
    TranslatedWords  int `json:"translated_words"`
    OverallPercent   float64 `json:"overall_percent"`
}

type PulseRisingStar struct {
    Name     string  `json:"name"`
    Type     string  `json:"type"` // "user", "language", "project"
    Growth   float64 `json:"growth"`
    Current  int     `json:"current"`
    Previous int     `json:"previous"`
}

type PulseContributor struct {
    Name          string `json:"name"`
    AvatarURL     string `json:"avatar_url,omitempty"`
    Translations  int    `json:"translations"`
    Reviews       int    `json:"reviews"`
    Languages     []string `json:"languages"`
}

type PulseLanguageRank struct {
    Locale          string  `json:"locale"`
    TranslatedWords int     `json:"translated_words"`
    TotalWords      int     `json:"total_words"`
    Percentage      float64 `json:"percentage"`
    Contributors    int     `json:"contributors"`
    RecentActivity  int     `json:"recent_activity"`
}
```

### Frontend Component Architecture

New shared components in `@neokapi/ui` (`platform/packages/ui/src/components/pulse/`):

| Component              | Purpose                                                |
| ---------------------- | ------------------------------------------------------ |
| `PulseOverview`        | Main workspace overview layout                         |
| `LanguageProgressGrid` | Grid of locale cards with circular completion rings    |
| `CompletionRing`       | SVG circular progress indicator                        |
| `ContributorBoard`     | Leaderboard with avatars, counts, growth badges        |
| `RisingStarBadge`      | Growth indicator (arrow + percentage)                  |
| `TrendAreaChart`       | Recharts area chart for activity over time             |
| `TermExplorerPublic`   | Read-only terminology browser with search              |
| `PulseProjectCard`     | Project summary card for overview grid                 |
| `PulseHeader`          | Minimal branded header with theme toggle               |
| `PulseFilterBar`       | Pulse-specific filter bar extending shared `FilterBar` |

Reused from existing `@neokapi/ui`:

- `ChartContainer`, `LocaleCompletionChart`, `WordCountChart`, `CollectionHeatmap`, `FileProgressTable`
- `FilterBar` (base token component), `ActivityFeed`
- All shadcn primitives (`Card`, `Badge`, `Button`, `Tabs`, etc.)

### SPA Routing

```
/:workspace                          → WorkspaceOverview
/:workspace/projects/:pid            → ProjectDetail
/:workspace/projects/:pid/lang/:loc  → LocaleDetail
/:workspace/leaderboard              → Leaderboard
/:workspace/terms                    → TerminologyExplorer
/:workspace/terms/:cid               → ConceptDetail
```

### Server Integration

```
platform/server/
├── handlers_pulse.go        (new) API handlers
├── pulse_cache.go           (new) TTL-based cache with event invalidation
├── middleware_pulse.go      (new) public_dashboard access gate
└── server.go                (modified) register pulse route group
```

The pulse route group uses `PulseAccessMiddleware` but no `AuthMiddleware` —
the middleware resolves the workspace by slug, checks `dashboard_visibility`,
and optionally validates JWT if the workspace is `private`. For `unlisted`
workspaces, no auth is required but `X-Robots-Tag: noindex` is set. For
project-level endpoints, the project's own visibility is also checked. The
overview endpoint only includes projects whose visibility is `public` or
`unlisted` (never `private` projects, even when the workspace is public).

### Database Changes

```sql
-- Workspace (auth store)
ALTER TABLE workspaces ADD COLUMN dashboard_visibility TEXT NOT NULL DEFAULT 'private';
-- CHECK (dashboard_visibility IN ('private', 'unlisted', 'public'))

-- Project (content store)
ALTER TABLE projects ADD COLUMN dashboard_visibility TEXT NOT NULL DEFAULT 'private';
-- CHECK (dashboard_visibility IN ('private', 'unlisted', 'public'))
```

Both SQLite and PostgreSQL migration paths.

## Development Workflow

### Storybook-first

All Pulse components are developed in Storybook before integration:

- Stories in `platform/packages/ui/src/stories/pulse/`
- Fixtures in `pulse-fixtures.ts` with realistic mock data
- Use existing `decorators.tsx` (`withProviders`) for theme/context wrapping

### Vitest

Unit tests in `platform/packages/ui/src/__tests__/pulse/`:

- Component rendering (empty states, populated states, error states)
- Filter token management (add, remove, presets, URL serialization)
- Chart data transformations
- Accessibility checks

### Backend tests

- `handlers_pulse_test.go` — access control (public vs private, 404 on non-public)
- `pulse_cache_test.go` — TTL expiry, event-driven invalidation
- Aggregation query tests with SQLite test store

## Implementation Phases

### Phase 1: Backend foundation

1. Add `dashboard_visibility` (private/unlisted/public) to workspace/project models + migrations
2. Create `handlers_pulse.go` — overview and project detail endpoints
3. Create `pulse_cache.go` — caching layer
4. Create `middleware_pulse.go` — access control
5. Register routes in `server.go`
6. Backend tests

### Phase 2: Core UI components (Storybook-driven)

1. Scaffold `platform/apps/pulse/` (package.json, vite, index.html)
2. Build primitives: `CompletionRing`, `RisingStarBadge`, `PulseHeader`
3. Build composites: `LanguageProgressGrid`, `ContributorBoard`, `TrendAreaChart`
4. Build `PulseOverview` composing the above
5. Storybook stories + vitest tests

### Phase 3: Full pages, routing & URL-based filtering

1. React Router setup with all routes
2. `PulseFilterBar` + `PulseFilterContext` with URL sync
3. Project detail page (reusing existing chart components)
4. Terminology explorer
5. Leaderboard page
6. Locale detail page

### Phase 4: Integration & polish

1. Connect to live API, end-to-end testing
2. Workspace settings toggle UI
3. Makefile targets (`pulse-build`, `pulse-dev`)
4. Server-side SPA serving for pulse subdomain (Host header routing)
5. Dark/light theme, responsive design
6. CDN cache headers for production

## Files Changed

| File                                           | Change                                                |
| ---------------------------------------------- | ----------------------------------------------------- |
| `platform/core/auth/types.go`                  | Add `DashboardVisibility` to Workspace                |
| `platform/core/store/types.go`                 | Add `DashboardVisibility` to Project, new Pulse types |
| `platform/server/server.go`                    | Register pulse route group                            |
| `platform/server/handlers_pulse.go`            | **New** — all pulse API handlers                      |
| `platform/server/pulse_cache.go`               | **New** — caching layer                               |
| `platform/server/middleware_pulse.go`          | **New** — access gate middleware                      |
| `platform/store/sqlite.go`                     | Migration for `dashboard_visibility`                  |
| `platform/store/postgres.go`                   | Migration for `dashboard_visibility`                  |
| `platform/auth/sqlite.go`                      | Migration for workspace `dashboard_visibility`        |
| `platform/auth/migrations_pg.go`               | Migration for workspace `dashboard_visibility`        |
| `platform/package.json`                        | Add `apps/pulse` to workspaces                        |
| `platform/apps/pulse/**`                       | **New** — Pulse SPA                                   |
| `platform/packages/ui/src/components/pulse/**` | **New** — shared Pulse components                     |
| `platform/packages/ui/src/stories/pulse/**`    | **New** — Storybook stories                           |
| `platform/packages/ui/src/__tests__/pulse/**`  | **New** — vitest tests                                |
| `Makefile`                                     | Add `pulse-build`, `pulse-dev` targets                |
