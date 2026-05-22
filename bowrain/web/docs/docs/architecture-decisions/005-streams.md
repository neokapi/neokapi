---
id: 005-streams
sidebar_position: 5
title: "AD-005: Streams"
---

# AD-005: Streams

## Summary

Streams bring git-like branching to Bowrain's content layer. A stream is a
named, lightweight fork of a project's content with its own change log.
Streams use copy-on-write over content-addressed blocks: branching is
O(1), translations inherit along the stream graph, and the CLI auto-detects
the active stream from the current git branch or CI system context.

## Context

Software and documentation projects routinely maintain multiple concurrent
versions. A 1.x maintenance branch receives patches while 2.0 is in active
development. Feature branches, pull requests, and experiments create
temporary forks that need independent translation without polluting the
main translation state.

Without branching, every push overwrites the same content set, which makes
working on multiple versions simultaneously impractical and breaks the
"translate the feature before merge" workflow that makes continuous
localization valuable. Teams work around the gap with separate projects
per version (duplicating config, losing shared translations) or by
deferring translation until after code merges (losing parallelism).

Streams solve the gap at the content layer, so the rest of the stack —
connectors, flows, TM, terminology, automation — operates unchanged.

## Decision

### Stream Model

A **stream** is a named branch of content within a project. Every project
has a `main` stream that exists implicitly. Additional streams branch from
a parent stream at a specific point, captured as a cursor position in the
change log.

```go
type Stream struct {
    ProjectID   string           `json:"project_id"`
    Name        string           `json:"name"`        // "main", "v2.0", "feature/new-ui", "pr/142"
    Parent      string           `json:"parent"`       // parent stream name; empty for main
    BaseCursor  int64            `json:"base_cursor"`  // cursor in parent at branch point
    Visibility  StreamVisibility `json:"visibility"`   // "public", "private", "shared"
    Description string           `json:"description"`
    SharedWith  []string         `json:"shared_with"`
    CreatedAt   time.Time        `json:"created_at"`
    CreatedBy   string           `json:"created_by"`
    Archived    bool             `json:"archived"`
}

type StreamVisibility string

const (
    StreamPublic  StreamVisibility = "public"   // visible to all project members
    StreamPrivate StreamVisibility = "private"  // visible only to the creator
    StreamShared  StreamVisibility = "shared"   // visible to creator + explicit members
)
```

Key properties:

- **Implicit main.** The `main` stream always exists and is the default
  target for every operation. It has no parent.
- **Copy-on-write.** Creating a stream records the parent name and base
  cursor. No blocks are copied. The stream inherits all content from its
  parent up to the base cursor.
- **Independent change log.** Each stream has its own change log entries
  (scoped by the `stream` column). Changes in one stream do not affect
  another.
- **Translation inheritance.** When pulling translations for a stream,
  blocks that exist in the parent but have not been overridden in the
  stream inherit the parent's translations. Creating a feature branch
  immediately has all existing translations available.
- **Visibility.** Streams can be `public` (all project members), `private`
  (creator only), or `shared` (creator plus an explicit `stream_members`
  list). Visibility is enforced at the API layer.
- **TM and terminology scoping.** TM and terminology lookups walk the
  parent chain (stream → parent → grandparent → main → workspace),
  enabling isolated terminology experiments that inherit from the parent
  while overriding specific entries.
- **Merging.** Merging a stream into its parent applies its delta — the
  added, modified, and removed blocks — to the parent. This is
  analogous to a git merge.

### Stream Resolution (CLI)

The bowrain CLI resolves the active stream through a priority chain:

```
1. --stream flag           (explicit per-command)
2. BOWRAIN_STREAM env var  (CI/CD override)
3. server.stream on recipe (project default)
4. $auto detection         (git branch / CI heuristics)
5. "main"                  (ultimate fallback)
```

### `$auto` Detection

When `stream` is set to `$auto` (the default), the CLI auto-detects the
stream name from the environment.

CI systems (checked in order):

| CI System      | Detection             | Stream name                                                           |
| -------------- | --------------------- | --------------------------------------------------------------------- |
| GitHub Actions | `GITHUB_ACTIONS=true` | PR: `GITHUB_HEAD_REF`; push: `GITHUB_REF_NAME`                        |
| GitLab CI      | `GITLAB_CI=true`      | MR: `CI_MERGE_REQUEST_SOURCE_BRANCH_NAME`; push: `CI_COMMIT_BRANCH`   |
| CircleCI       | `CIRCLECI=true`       | `CIRCLE_BRANCH`                                                       |
| Azure DevOps   | `TF_BUILD=True`       | PR: `SYSTEM_PULLREQUEST_SOURCEBRANCH`; push: `BUILD_SOURCEBRANCHNAME` |
| Jenkins        | `JENKINS_URL` set     | PR: `CHANGE_BRANCH`; push: `BRANCH_NAME`                              |
| Travis CI      | `TRAVIS=true`         | PR: `TRAVIS_PULL_REQUEST_BRANCH`; push: `TRAVIS_BRANCH`               |
| Buildkite      | `BUILDKITE=true`      | `BUILDKITE_BRANCH`                                                    |

Local (no CI detected): `git rev-parse --abbrev-ref HEAD`.

Normalization: branch names like `feature/new-ui` are preserved (slashes
are natural grouping), but `refs/heads/` and `refs/tags/` prefixes are
stripped. Detached HEAD falls back to `main`. The `main` or `master`
branch maps to stream `main` (and no stream header is sent). GitHub PR
merge refs (`123/merge`) are not used — `GITHUB_HEAD_REF` is preferred.

### Config

The `stream` field on the `server:` block of the recipe:

```yaml
version: v1
name: my-app

defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE]

server:
  url: https://bowrain.example.com/my-team/abc123
  # Stream determines which content stream to sync with.
  # Default: $auto (detect from git branch / CI environment)
  # Explicit: "main", "v2.0", "feature/new-ui"
  stream: $auto
```

When `server.stream` is empty or `$auto`, detection runs on every push and pull.
When set to a specific name (e.g. `v2.0`), that stream is always used
regardless of the current git branch — useful for CI pipelines that build
a single release line.

### API

Streams are communicated via the `X-Bowrain-Stream` HTTP header on push
and pull requests:

```
POST /api/v1/projects/:id/sync/push
X-Bowrain-Stream: feature/new-ui

GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr
X-Bowrain-Stream: feature/new-ui
```

When the header is absent or `main`, the server operates on the default
stream. The server auto-creates a stream on first push if it doesn't
exist, branching from `main` at the current cursor — analogous to
`git push -u origin branch`.

Stream management endpoints:

```
GET    /api/v1/projects/:id/streams                 # List streams
POST   /api/v1/projects/:id/streams                 # Create stream explicitly
GET    /api/v1/projects/:id/streams/:name           # Get stream info
DELETE /api/v1/projects/:id/streams/:name           # Archive stream
POST   /api/v1/projects/:id/streams/:name/merge     # Merge into parent
GET    /api/v1/projects/:id/streams/:name/diff      # Diff against parent
```

### Server-Side Storage

Streams are lightweight at the storage level.

**`streams` table:**

```sql
CREATE TABLE streams (
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    parent      TEXT NOT NULL DEFAULT '',
    base_cursor BIGINT NOT NULL DEFAULT 0,
    archived    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL,
    created_by  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, name)
);
```

**`change_log.stream` column** scopes every entry to its branch:

```sql
ALTER TABLE change_log ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);
```

**`blocks` table** is unchanged. Blocks are content-addressed and shared
across streams. A block pushed to `feature/new-ui` with the same content
hash as one in `main` is stored once.

Query semantics:

- **Push to stream X.** Insert blocks into shared storage. Append to
  `change_log` with `stream = X`.
- **Pull from stream X.** Return changes from stream X's change log since
  the cursor. For blocks without stream-specific entries, inherit from
  the parent chain.
- **Merge stream X into parent.** Copy stream X's change log entries to
  the parent stream. The base cursor of X advances to the parent's new
  position.
- **Diff stream X.** Compare X's blocks against the parent's state at
  X's base cursor. Returns added, modified, and removed blocks — the
  same `VersionDiff` structure used elsewhere (see
  [AD-004: Content Store and Versioning](004-content-store.md)).

### Merkle Hash Optimization

The Merkle tree hash negotiation in the sync protocol is stream-aware —
see [AD-009: Sync Protocol](009-sync-protocol.md). Item-level hashes are
computed per stream; an unchanged subtree (item hash matches) signals the
server to skip block-level diff entirely. This makes pull from a mature
stream O(changes since cursor) regardless of project size.

### Translation Efficiency

Streams are designed for minimal translation cost:

1. **Shared blocks.** Content-addressed storage stores and translates
   identical strings across streams once.
2. **Inherited translations.** A stream inherits all translations from
   its parent chain. Only genuinely new or modified blocks need
   translation.
3. **Targeted AI translation.** When a stream is pushed, the server
   identifies blocks that are new relative to the parent (via change log
   diff) and triggers AI translation only for those.
4. **Merge carries translations.** When a stream merges back, its
   translations carry forward. Translations done in the parent since
   the branch point are also preserved; conflict resolution prefers the
   more recent translation.

### Integration with Tags and Brand Voice

Stream-scoped content participates in graph validity scoping (see
[AD-006: Graph Concept Storage](006-graph-concept-storage.md)). Brand
voice profiles, tag dimensions, and temporal validity on graph edges all
respect stream boundaries when resolving effective terminology and voice
rules.

### CLI Commands

```bash
# Auto-detect stream (default behavior, no changes needed):
kapi push                          # stream = $auto → git branch
kapi pull                          # stream = $auto → git branch

# Explicit stream:
kapi push --stream v2.0
kapi pull --stream v2.0

# CI with env var:
BOWRAIN_STREAM=pr/142 kapi push

# Stream management:
kapi stream list                        # List streams for this project
kapi stream create v2.0                 # Create stream (branches from main)
kapi stream create hotfix --from v1.0   # Branch from specific stream
kapi stream merge v2.0                  # Merge v2.0 into its parent
kapi stream diff v2.0                   # Show changes vs parent
kapi stream archive v2.0                # Archive (soft delete)

# Status shows current stream:
kapi status
# → Stream: feature/new-ui (auto-detected from git branch)
# → Modified: src/locales/en.json
```

## Consequences

- Every project implicitly has a `main` stream. Projects that don't use
  streams never see them — they stay on main.
- `$auto` detection makes streams zero-config for git-aligned workflows.
  Push from a feature branch lands content in that stream; push from main
  lands in main.
- CI/CD pipelines can use `BOWRAIN_STREAM` or `--stream` for explicit
  control. `$auto` handles GitHub Actions, GitLab CI, CircleCI, Azure
  DevOps, Jenkins, Travis CI, and Buildkite out of the box.
- Content-addressed storage keeps streams storage-efficient: only change
  log entries are per-stream; blocks themselves are shared.
- Translation effort is proportional to the actual content delta, not the
  total project size. A stream for a one-string change triggers AI
  translation for one string.
- Stream merging follows the same content-addressed diff model as version
  comparison.
- Archived streams are soft-deleted: their content remains for reference
  but they no longer accept pushes or appear in listings by default.

## Related

- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-006: Graph Concept Storage](006-graph-concept-storage.md)
- [AD-009: Sync Protocol](009-sync-protocol.md)
