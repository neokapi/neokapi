# Brand Voice Compliance Dashboard

The compliance dashboard provides real-time visibility into brand voice adherence across projects.

## Overview

The dashboard shows:

- **Overall Brand Compliance Score** (0-100) with trend over time
- **Per-dimension breakdown** (Tone, Style, Vocabulary, Clarity, Brand Compliance)
- **Issue density** (issues per 1,000 words)
- **Recent findings** sorted by severity

## Score Model

Scores use an MQM-inspired model with five dimensions and four severity levels:

| Severity | Weight | Meaning                 |
| -------- | ------ | ----------------------- |
| Neutral  | 0      | Observation, no penalty |
| Minor    | 1      | Slight deviation        |
| Major    | 5      | Clear violation         |
| Critical | 25     | Serious violation       |

**Score = max(0, 100 - total_penalty)**

## API Endpoints

```
GET /workspaces/:ws/projects/:id/brand-voice/scores           # All scores
GET /workspaces/:ws/projects/:id/brand-voice/scores/:locale    # By locale
GET /workspaces/:ws/projects/:id/brand-voice/trends            # Trends
POST /workspaces/:ws/projects/:id/brand-voice/corrections      # Record correction
GET /workspaces/:ws/brand-voice/suggested-rules                # Suggested rules
```

## UI Components

The dashboard is built with shared React components in `packages/ui/src/brand/`:

- **BrandScoreGauge** — circular SVG gauge with color coding
- **BrandDimensionBreakdown** — horizontal bar chart per dimension
- **BrandFindingsList** — severity-badged findings list
- **BrandProfileCard** — profile summary card
- **BrandExamplePair** — before/after display

## Pages

- **Profile List** (`/brand-profiles`) — grid of profile cards with CRUD actions
- **Profile Editor** (`/brand-profiles/:id/edit`) — tabbed editor for tone, style, vocabulary, examples
- **Compliance Dashboard** (`/brand-dashboard`) — scores, trends, and findings
- **MCP Connection Guide** (`/brand-mcp-guide`) — configuration templates
