---
id: 024-streams
sidebar_position: 24
title: "AD-024: Streams"
---
# AD-024: Streams

## Context

Software and documentation projects routinely maintain multiple concurrent versions — a 1.x maintenance branch receiving patches while 2.0 is under active development. Feature branches, pull requests, and experiments create temporary forks of content that need independent translation without polluting the main translation state. Without streams, every push overwrites the same content set, making it impossible to work on multiple versions simultaneously or preview translations for a feature branch before merging.

Localization platforms that lack branching force teams into workarounds: separate projects per version (duplicating config and losing shared translations), manual coordination of "which strings belong to which release", or deferring translation until after code merges (losing the parallelism that makes continuous localization valuable).

Streams solve this by bringing git-like branching to the content layer. A stream is a named, lightweight fork of the project's content that tracks its own changes independently. Creating a stream is O(1) — no content is copied, just a pointer to the parent's state. Only the delta (new or changed blocks) is stored per stream. Translations from the parent stream are inherited automatically, and AI translation is triggered only for genuinely new content introduced by the stream.

## Decision

### Stream Model

A **stream** is a named branch of content within a project. Every project has a `main` stream that is created implicitly. Additional streams branch from a parent stream at a specific point in time (captured as a cursor position in the change log).

```go
type Stream struct {
    ProjectID   string           `json:"project_id"`
    Name        string           `json:"name"`        // "main", "v2.0", "feature/new-ui", "pr/142"
    Parent      string           `json:"parent"`       // parent stream name; empty for "main"
    BaseCursor  int64            `json:"base_cursor"`  // cursor in parent at branch point
    Visibility  StreamVisibility `json:"visibility"`   // "public", "private", "shared"
    Description string           `json:"description"`
    SharedWith  []string         `json:"shared_with"`  // user IDs for "shared" visibility
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

**Key properties:**

- **Implicit main**: The `main` stream always exists and is the default target for all operations. It does not have a parent.
- **Copy-on-write**: Creating a stream records only the parent name and base cursor. No blocks are copied. The stream inherits all content from its parent up to the base cursor.
- **Independent change log**: Each stream has its own change log entries (scoped by `stream` column). Changes in one stream do not affect another.
- **Translation inheritance**: When pulling translations for a stream, blocks that exist in the parent but have not been overridden in the stream inherit the parent's translations. This means creating a feature branch immediately has all existing translations available.
- **Access control**: Streams have a visibility setting — `public` (all project members), `private` (creator only), or `shared` (creator + explicitly added members). This is enforced at the API layer via logical isolation. The `stream_members` table tracks membership for shared streams.
- **TM/Terminology scoping**: Translation memory and terminology entries are scoped to streams. Lookups follow the full parent chain (stream → parent → grandparent → main → workspace), enabling isolated terminology experiments that inherit from the parent while overriding specific entries.
- **Merging**: Merging a stream into its parent applies the stream's changes (new/modified/deleted blocks) to the parent. This is analogous to a git merge — only the delta is applied.

### Stream Resolution (CLI)

The CLI resolves the active stream through a priority chain:

```
1. --stream flag           (explicit per-command)
2. BOWRAIN_STREAM env var  (CI/CD override)
3. config.yaml stream      (project default)
4. $auto detection         (git branch / CI heuristics)
5. "main"                  (ultimate fallback)
```

#### `$auto` Detection

When `stream` is set to `$auto` (or omitted, since `$auto` is the default), the CLI auto-detects the stream name from the environment:

**CI systems** (checked in order):

| CI System | Detection | Stream name |
|---|---|---|
| GitHub Actions | `GITHUB_ACTIONS=true` | PR: `GITHUB_HEAD_REF`; push: `GITHUB_REF_NAME` |
| GitLab CI | `GITLAB_CI=true` | MR: `CI_MERGE_REQUEST_SOURCE_BRANCH_NAME`; push: `CI_COMMIT_BRANCH` |
| CircleCI | `CIRCLECI=true` | `CIRCLE_BRANCH` |
| Azure DevOps | `TF_BUILD=True` | PR: `SYSTEM_PULLREQUEST_SOURCEBRANCH`; push: `BUILD_SOURCEBRANCHNAME` |
| Jenkins | `JENKINS_URL` set | PR: `CHANGE_BRANCH`; push: `BRANCH_NAME` |
| Travis CI | `TRAVIS=true` | PR: `TRAVIS_PULL_REQUEST_BRANCH`; push: `TRAVIS_BRANCH` |
| Buildkite | `BUILDKITE=true` | `BUILDKITE_BRANCH` |

**Local** (no CI detected): `git rev-parse --abbrev-ref HEAD`

**Stream name normalization**: Branch names like `feature/new-ui` or `refs/heads/main` are normalized to valid stream names — slashes are preserved (they're natural grouping), but `refs/heads/` and `refs/tags/` prefixes are stripped.

**Special cases**:
- Detached HEAD (`git rev-parse` returns `HEAD`) → falls back to `main`
- `main` or `master` branch → stream `main` (the default, no stream header sent)
- GitHub PR merge refs (`123/merge`) → not used; `GITHUB_HEAD_REF` is preferred

### Config

The `stream` field is added to `config.yaml`:

```yaml
version: v1

url: https://bowrain.example.com/my-team/abc123

# Stream determines which content stream to sync with.
# Default: $auto (detect from git branch / CI environment)
# Explicit: "main", "v2.0", "feature/new-ui"
stream: $auto

defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE]
```

When `stream` is empty or `$auto`, auto-detection runs on every push/pull. When set to a specific name (e.g., `v2.0`), that stream is always used regardless of the current git branch.

### API Protocol

Streams are communicated via the `X-Bowrain-Stream` HTTP header on push/pull requests:

```
POST /api/v1/projects/:id/sync/push
X-Bowrain-Stream: feature/new-ui

GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr
X-Bowrain-Stream: feature/new-ui
```

When the header is absent or `main`, the server operates on the default stream. The server creates the stream automatically on the first push if it doesn't exist (branching from `main` at the current cursor).

**Stream management endpoints:**

```
GET    /api/v1/projects/:id/streams                    # List streams
POST   /api/v1/projects/:id/streams                    # Create stream explicitly
GET    /api/v1/projects/:id/streams/:name              # Get stream info
DELETE /api/v1/projects/:id/streams/:name              # Archive stream
POST   /api/v1/projects/:id/streams/:name/merge        # Merge into parent
GET    /api/v1/projects/:id/streams/:name/diff          # Diff against parent
```

### Server-Side Storage

Streams are lightweight at the storage level:

**`streams` table:**
```sql
CREATE TABLE streams (
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    parent      TEXT NOT NULL DEFAULT '',
    base_cursor INTEGER NOT NULL DEFAULT 0,
    archived    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    created_by  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, name)
);
```

**`change_log` extension:**
```sql
-- Add stream column to change_log
ALTER TABLE change_log ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);
```

**`blocks` table** — unchanged. Blocks are content-addressed and shared across streams. A block pushed to `feature/new-ui` with the same content hash as one in `main` is stored once.

**Query semantics:**

- **Push to stream X**: Insert blocks normally (shared storage). Append to `change_log` with `stream = X`.
- **Pull from stream X**: Return changes from stream X's change log since the cursor. For blocks that don't have stream-specific entries, inherit from the parent chain.
- **Merge stream X into parent**: Copy stream X's change log entries to the parent stream. The base cursor of X advances to the parent's new position.
- **Diff stream X**: Compare X's blocks against the parent's state at X's base cursor. Returns added/modified/removed blocks — the same `VersionDiff` structure used elsewhere.

### Translation Efficiency

Streams are designed for minimal translation cost:

1. **Shared blocks**: Content-addressed storage means identical strings across streams are stored and translated once.
2. **Inherited translations**: A stream inherits all translations from its parent. Only blocks that are new or modified in the stream need translation.
3. **Targeted AI translation**: When a stream is pushed, the server identifies blocks that are genuinely new (not in parent) and triggers AI translation only for those. This is tracked via the change log diff against the parent.
4. **Merge carries translations**: When a stream is merged, any translations done in the stream are carried to the parent. Translations done in the parent since the branch point are also preserved (conflict resolution prefers the more recent translation).

### CLI Commands

```bash
# Auto-detect stream (default behavior, no changes needed):
bowrain push                          # stream = $auto → git branch
bowrain pull                          # stream = $auto → git branch

# Explicit stream:
bowrain push --stream v2.0
bowrain pull --stream v2.0

# CI with env var:
BOWRAIN_STREAM=pr/142 bowrain push

# Stream management:
bowrain stream list                   # List streams for this project
bowrain stream create v2.0            # Create stream (branches from main)
bowrain stream create hotfix --from v1.0   # Branch from specific stream
bowrain stream merge v2.0             # Merge v2.0 into its parent
bowrain stream diff v2.0              # Show changes vs parent
bowrain stream archive v2.0           # Archive (soft delete)

# Status shows current stream:
bowrain status
# → Stream: feature/new-ui (auto-detected from git branch)
# → Modified: src/locales/en.json
```

## Alternatives Considered

- **Separate projects per version**: Works but duplicates configuration, loses shared translations, and requires manual coordination. Streams within a single project are more natural and efficient.

- **Stream as a query parameter instead of header**: Headers keep the URL clean and avoid encoding issues with stream names containing slashes. The stream is metadata about the request context, not a filter on the resource — headers are semantically correct.

- **Mandatory explicit stream creation before push**: Adds friction. Auto-creating on first push (like `git push -u origin branch`) is more ergonomic, especially for CI/CD where streams are transient.

- **Full content copy on branch**: Wasteful for large projects. Copy-on-write with content-addressed storage means branching is O(1) regardless of project size.

- **Stream names as UUIDs**: Human-readable names (matching git branches) are essential for DX. The name is the primary identifier, scoped to the project.

## Consequences

- Every project implicitly has a `main` stream. Existing projects work without changes — the default stream is `main`.

- `$auto` detection makes streams zero-config for projects that follow git branching conventions. Push from a feature branch → content goes to that stream. Push from main → content goes to main.

- CI/CD pipelines can use `BOWRAIN_STREAM` or `--stream` for explicit control. The `$auto` detection handles GitHub Actions, GitLab CI, CircleCI, Azure DevOps, Jenkins, Travis CI, and Buildkite out of the box.

- Content-addressed block storage ensures streams are storage-efficient. Only the change log entries are per-stream; blocks themselves are shared.

- Translation effort is proportional to the actual content delta, not the total project size. Creating a stream for a one-string change triggers AI translation for one string, not the entire project.

- Stream merging follows the same content-addressed diff model as version comparison — added, modified, and removed blocks are computed from change log diffs.

- The `stream` field in `config.yaml` defaults to `$auto`. Projects that don't use streams never see it. Projects that pin a version can set `stream: v2.0`.

- Streams are scoped to projects within workspaces. Stream names are unique per project but can be reused across projects.

- Archived streams are soft-deleted — their content remains for reference but they no longer accept pushes or appear in listings by default.
