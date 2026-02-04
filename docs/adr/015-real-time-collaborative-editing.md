---
id: 015-real-time-collaborative-editing
sidebar_position: 15
title: "ADR-015: Real-Time Collaborative Editing"
---
# ADR-015: Real-Time Collaborative Editing

## Context

Bowrain, the gokapi desktop application, needs to support multiple translators working on the same project simultaneously. This is a common workflow in localization where:

- Multiple translators work on different segments of the same document
- Reviewers need to see translator progress in real-time
- Translators may work offline and sync changes later
- Conflicts can occur when two people edit the same segment

This ADR focuses specifically on the collaborative editing architecture. It builds upon the infrastructure established in [ADR-014: Distributed Processing Architecture](./014-distributed-processing-architecture.md), which provides:

- Redis for distributed locking (Phase 2)
- gRPC streaming for real-time sync via `Subscribe` RPC (Phase 3)
- Connection codes for Bowrain-to-server connectivity (Phase 3)

## Open Questions Requiring Decisions

### Q1: Collaboration Technology

**Question:** What technology should power Bowrain's collaborative editing?

| Option | Offline Support | Conflict Resolution | Complexity | Maturity |
|--------|-----------------|---------------------|------------|----------|
| **A. Operational Transform (OT)** | Limited | Server-coordinated | High | High (Google Docs) |
| **B. CRDTs (Yjs)** | Full | Automatic merge | Medium | High (900k+ weekly npm downloads) |
| **C. CRDTs (Automerge)** | Full | Automatic merge | Medium | Medium |
| **D. Last-Write-Wins** | Full | Data loss possible | Low | N/A |
| **E. Segment Locking Only** | No | Prevention-based | Low | Traditional CAT |

**Significance:**

- **If OT (Option A)**: Google Docs-style collaboration. All operations go through a central server that transforms them to maintain consistency.
  - *Pros*: Battle-tested at scale, predictable behavior
  - *Cons*: Requires constant server connectivity, complex implementation, no true offline support
  - *Risk*: Single point of failure, latency impacts UX

- **If Yjs (Option B)**: Modern CRDT library with excellent ecosystem support.
  - *Pros*: Offline-first, automatic conflict resolution, rich editor integrations (ProseMirror, CodeMirror, Monaco), awareness protocol for presence
  - *Cons*: Merge results may occasionally surprise users
  - *Risk*: Learning curve for CRDT mental model

- **If Automerge (Option C)**: JSON-focused CRDT with Rust core.
  - *Pros*: Clean JSON data model, Rust performance, good documentation
  - *Cons*: Smaller ecosystem than Yjs, fewer editor integrations
  - *Risk*: Less community support for edge cases

- **If Last-Write-Wins (Option D)**: Simplest approach—latest timestamp wins.
  - *Pros*: Trivial to implement, predictable
  - *Cons*: Translator work can be silently overwritten
  - *Risk*: Unacceptable data loss in professional translation

- **If Segment Locking Only (Option E)**: Traditional CAT tool model where segments are locked during editing.
  - *Pros*: Simple, no conflicts possible, familiar to translators
  - *Cons*: Blocks concurrent work on same segment, no offline editing
  - *Risk*: Poor UX when translators need same segments

**Recommendation:** Hybrid approach—**Option B (Yjs)** for segment content with **Option E (Locking)** for segment assignment. This provides:
- Optimistic editing with automatic merge for the common case
- Explicit locking when translators want guaranteed exclusive access
- Full offline support with sync on reconnect

---

### Q2: CRDT Granularity

**Question:** At what level should CRDTs operate?

| Option | Merge Quality | Memory Usage | Sync Overhead |
|--------|---------------|--------------|---------------|
| **A. Character-level** | Best | High | High |
| **B. Word-level** | Good | Medium | Medium |
| **C. Segment-level** | Coarse | Low | Low |
| **D. Field-level** | Per-field | Low | Low |

**Significance:**

- **If Character-level**: Every keystroke is tracked. Best for real-time "Google Docs" feel.
  - *Risk*: High memory and bandwidth for large projects

- **If Word-level**: Changes tracked per word. Good balance.
  - *Risk*: Word boundary detection complexity across languages

- **If Segment-level**: Entire segment replaced atomically.
  - *Risk*: Concurrent edits to same segment always conflict

- **If Field-level**: Separate CRDTs for source, target, notes, status.
  - *Risk*: More complex state management

**Recommendation:** Option A (Character-level) for translation text fields using Yjs `Y.Text`, Option D (Field-level) for metadata using `Y.Map`.

---

### Q3: Offline Sync Strategy

**Question:** How should offline changes be handled?

| Option | User Control | Complexity | Data Safety |
|--------|--------------|------------|-------------|
| **A. Auto-sync on reconnect** | Low | Low | Medium |
| **B. Manual sync with preview** | High | Medium | High |
| **C. Background sync with notifications** | Medium | Medium | High |

**Significance:**

- **If Auto-sync**: Changes merge automatically when online. Users may be surprised by results.
- **If Manual sync**: Users review changes before committing. Safer but more friction.
- **If Background sync with notifications**: Auto-sync but notify users of conflicts requiring attention.

**Recommendation:** Option C (Background sync with notifications). Auto-merge non-conflicting changes, surface conflicts for manual resolution.

---

### Q4: Presence Protocol

**Question:** What presence information should be shared?

| Feature | Value | Privacy | Bandwidth |
|---------|-------|---------|-----------|
| Online/offline status | High | Low concern | Minimal |
| Current document | Medium | Low concern | Minimal |
| Current segment | High | Medium concern | Low |
| Cursor position | Medium | Medium concern | Medium |
| Selection highlight | Low | Medium concern | Medium |
| Typing indicator | Low | Low concern | Low |

**Recommendation:** Implement online status, current document, current segment, and typing indicator. Cursor position and selection are lower priority and can be added later.

---

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         BOWRAIN CLIENT                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Editor    │  │  Yjs Doc    │  │  Awareness  │              │
│  │  Component  │◄─┤   Store     │◄─┤  Provider   │              │
│  └─────────────┘  └──────┬──────┘  └──────┬──────┘              │
│                          │                 │                     │
│                   ┌──────┴─────────────────┴──────┐              │
│                   │      Sync Provider            │              │
│                   │  (WebSocket / gRPC-Web)       │              │
│                   └──────────────┬────────────────┘              │
└──────────────────────────────────┼──────────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │        GOKAPI SERVER        │
                    │  ┌─────────────────────┐    │
                    │  │   Collaboration     │    │
                    │  │      Service        │    │
                    │  └──────────┬──────────┘    │
                    │             │               │
                    │  ┌──────────┴──────────┐    │
                    │  │                     │    │
                    │  ▼                     ▼    │
                    │ Redis              PostgreSQL│
                    │ (locks,            (Yjs docs,│
                    │  presence)          history) │
                    └─────────────────────────────┘
```

### Segment Locking

Segment locking provides explicit exclusive access when translators need guaranteed ownership:

```go
// core/collab/locks.go
type SegmentLocker struct {
    redis   *redis.Client
    ttl     time.Duration
}

func (l *SegmentLocker) TryLock(ctx context.Context, projectID, blockID, userID string) (bool, error) {
    key := fmt.Sprintf("lock:%s:%s", projectID, blockID)

    // SET NX with TTL
    ok, err := l.redis.SetNX(ctx, key, userID, l.ttl).Result()
    if err != nil {
        return false, err
    }

    if !ok {
        // Check if we already own the lock
        owner, _ := l.redis.Get(ctx, key).Result()
        return owner == userID, nil
    }

    return true, nil
}

func (l *SegmentLocker) Extend(ctx context.Context, projectID, blockID, userID string) error {
    key := fmt.Sprintf("lock:%s:%s", projectID, blockID)

    // Only extend if we own the lock (Lua script for atomicity)
    script := `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("PEXPIRE", KEYS[1], ARGV[2])
        end
        return 0
    `
    return l.redis.Eval(ctx, script, []string{key}, userID, l.ttl.Milliseconds()).Err()
}

func (l *SegmentLocker) Unlock(ctx context.Context, projectID, blockID, userID string) error {
    key := fmt.Sprintf("lock:%s:%s", projectID, blockID)

    // Only unlock if we own the lock
    script := `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("DEL", KEYS[1])
        end
        return 0
    `
    return l.redis.Eval(ctx, script, []string{key}, userID).Err()
}
```

### Yjs Integration

```typescript
// apps/bowrain/frontend/src/collab/yjs-provider.ts
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import { Awareness } from 'y-protocols/awareness'

interface UserState {
  userId: string
  name: string
  color: string
  currentSegment?: string
  isTyping: boolean
}

export class ProjectSync {
  private doc: Y.Doc
  private provider: WebsocketProvider
  private awareness: Awareness

  constructor(projectId: string, serverUrl: string, token: string) {
    this.doc = new Y.Doc()

    // Connect to sync server
    this.provider = new WebsocketProvider(
      serverUrl,
      `project:${projectId}`,
      this.doc,
      { params: { token } }
    )

    this.awareness = this.provider.awareness
  }

  // Get Y.Text for a specific block's translation
  getBlockText(blockId: string, locale: string): Y.Text {
    const blocks = this.doc.getMap('blocks')
    let block = blocks.get(blockId) as Y.Map<any>

    if (!block) {
      block = new Y.Map()
      blocks.set(blockId, block)
    }

    let translations = block.get('translations') as Y.Map<any>
    if (!translations) {
      translations = new Y.Map()
      block.set('translations', translations)
    }

    let text = translations.get(locale) as Y.Text
    if (!text) {
      text = new Y.Text()
      translations.set(locale, text)
    }

    return text
  }

  // Update local user's presence
  setPresence(state: Partial<UserState>) {
    this.awareness.setLocalStateField('user', {
      ...this.awareness.getLocalState()?.user,
      ...state
    })
  }

  // Subscribe to other users' presence
  onPresenceChange(callback: (states: Map<number, UserState>) => void) {
    this.awareness.on('change', () => {
      const states = new Map<number, UserState>()
      this.awareness.getStates().forEach((state, clientId) => {
        if (state.user && clientId !== this.doc.clientID) {
          states.set(clientId, state.user)
        }
      })
      callback(states)
    })
  }

  // Handle offline/online transitions
  get isOnline(): boolean {
    return this.provider.wsconnected
  }

  onConnectionChange(callback: (connected: boolean) => void) {
    this.provider.on('status', (event: { status: string }) => {
      callback(event.status === 'connected')
    })
  }
}
```

### Conflict Resolution

When CRDTs can't automatically merge (rare with character-level CRDTs, but possible with concurrent structural changes):

```go
// core/collab/conflicts.go
type Conflict struct {
    ID          string    `json:"id"`
    BlockID     string    `json:"block_id"`
    Locale      string    `json:"locale"`
    LocalValue  string    `json:"local_value"`
    RemoteValue string    `json:"remote_value"`
    LocalUser   string    `json:"local_user"`
    RemoteUser  string    `json:"remote_user"`
    DetectedAt  time.Time `json:"detected_at"`
    Status      string    `json:"status"` // pending, resolved_local, resolved_remote, resolved_merged
}

type ConflictResolver struct {
    store ConflictStore
    notifier Notifier
}

func (r *ConflictResolver) Detect(blockID string, local, remote *BlockState) *Conflict {
    // CRDTs handle most merges automatically
    // Conflicts occur when:
    // 1. Both sides changed status (e.g., both marked "approved")
    // 2. Structural changes (segment split/merge)

    if local.Status != remote.Status &&
       local.StatusChangedAt.After(remote.SyncedAt) &&
       remote.StatusChangedAt.After(local.SyncedAt) {
        return &Conflict{
            ID:          uuid.New().String(),
            BlockID:     blockID,
            LocalValue:  local.Status,
            RemoteValue: remote.Status,
            // ...
        }
    }
    return nil
}

func (r *ConflictResolver) Resolve(conflictID string, resolution Resolution) error {
    conflict, _ := r.store.Get(conflictID)

    switch resolution.Choice {
    case "local":
        conflict.Status = "resolved_local"
    case "remote":
        conflict.Status = "resolved_remote"
    case "merged":
        conflict.Status = "resolved_merged"
        // Apply custom merged value
    }

    r.store.Update(conflict)
    r.notifier.NotifyResolution(conflict)
    return nil
}
```

---

## Implementation Roadmap

### Phase 1: Foundation [Size: MEDIUM]

**Scope:** Basic locking and presence without CRDTs.

#### Checklist

**1.1 Segment Locking**
- [ ] Implement `SegmentLocker` with Redis
- [ ] Add lock acquisition with TTL
- [ ] Implement lock extension (heartbeat)
- [ ] Add lock release on completion
- [ ] Create automatic lock expiration
- [ ] Implement lock ownership verification
- [ ] Add lock status in Bowrain UI
- [ ] Test concurrent lock requests

**1.2 Basic Presence**
- [ ] Implement presence tracking in Redis
- [ ] Add online/offline status
- [ ] Create current document tracking
- [ ] Implement current segment tracking
- [ ] Add presence API endpoints
- [ ] Create Bowrain presence display component
- [ ] Test with multiple concurrent users

#### Testing Strategy

```go
func TestSegmentLocking(t *testing.T) {
    // Lock acquisition succeeds for first user
    // Second user's lock attempt fails
    // Lock extension works for owner
    // Expired lock can be acquired by another user
    // Unlock only succeeds for owner
}

func TestPresence(t *testing.T) {
    // User presence appears when connected
    // Presence updates when changing segments
    // Presence disappears on disconnect
    // Multiple users visible simultaneously
}
```

---

### Phase 2: CRDT Integration [Size: LARGE]

**Scope:** Full Yjs integration for real-time collaborative editing.

#### Checklist

**2.1 Yjs Setup**
- [ ] Evaluate Yjs Go port vs JavaScript runtime on server
- [ ] Set up y-websocket server or custom gRPC sync
- [ ] Implement Yjs document model for blocks
- [ ] Create Y.Text bindings for translation content
- [ ] Add Y.Map for block metadata
- [ ] Implement document persistence to PostgreSQL

**2.2 Sync Protocol**
- [ ] Implement sync provider for Bowrain
- [ ] Add awareness protocol for presence
- [ ] Create offline change buffering (IndexedDB)
- [ ] Implement reconnection with state sync
- [ ] Add bandwidth optimization (update batching)
- [ ] Test sync under various network conditions

**2.3 Editor Integration**
- [ ] Integrate Yjs with Bowrain's text editor
- [ ] Add real-time cursor display
- [ ] Implement selection highlighting
- [ ] Create typing indicators
- [ ] Test editor responsiveness under sync load

#### Testing Strategy

```go
func TestCRDTMerge(t *testing.T) {
    // Concurrent character inserts merge correctly
    // Concurrent deletes merge correctly
    // Insert + delete at same position preserves intention
    // Complex multi-user editing scenario
}

func TestOfflineSync(t *testing.T) {
    // User goes offline
    // User makes multiple edits
    // User comes online
    // Changes sync correctly without data loss
    // Concurrent offline edits from multiple users merge
}
```

---

### Phase 3: Conflict Resolution & Polish [Size: MEDIUM]

**Scope:** Handle edge cases and improve UX.

#### Checklist

**3.1 Conflict Resolution**
- [ ] Design conflict detection logic
- [ ] Create conflict notification system
- [ ] Implement manual resolution UI
- [ ] Add automatic resolution rules (configurable)
- [ ] Create conflict history tracking
- [ ] Implement resolution audit log

**3.2 Collaboration UX**
- [ ] Add collaborative editing mode toggle
- [ ] Create "who's editing what" overview panel
- [ ] Implement activity feed
- [ ] Add @mentions in comments
- [ ] Create sync status indicator
- [ ] Add offline mode indicator

#### Testing Strategy

**User Acceptance Tests:**
- [ ] 5 concurrent translators on same project
- [ ] Network interruption and recovery
- [ ] Large project (1000+ blocks) performance
- [ ] Conflict resolution workflow

**Stress Tests:**
```go
func TestHighConcurrency(t *testing.T) {
    // 50 concurrent users
    // 10 updates per second per user
    // Verify no lost updates
    // Verify UI remains responsive
    // Measure sync latency p99 < 200ms
}
```

---

## Decision Summary

| Concern | Decision | Rationale |
|---------|----------|-----------|
| **Collaboration Tech** | Yjs + Segment Locking | Best of both: optimistic CRDT merge + explicit locking when needed |
| **CRDT Granularity** | Character-level for text | Real-time feel, proven with Yjs |
| **Offline Strategy** | Background sync + notifications | Auto-merge with user awareness of conflicts |
| **Presence** | Status + document + segment + typing | High value features without privacy/bandwidth concerns |

---

## References

### CRDTs and Collaborative Editing

- [Yjs Documentation](https://docs.yjs.dev/) — High-performance CRDT for collaborative editing
- [Yjs GitHub](https://github.com/yjs/yjs) — Shared data types for building collaborative software
- [Automerge](https://automerge.org/docs/hello/) — JSON-like CRDT that merges concurrent changes automatically
- [CRDT.tech](https://crdt.tech/) — Resources and implementations for Conflict-free Replicated Data Types
- [Y-websocket](https://github.com/yjs/y-websocket) — WebSocket provider for Yjs
- [Yjs Awareness Protocol](https://docs.yjs.dev/getting-started/adding-awareness) — Presence and cursor sharing

### Operational Transform (Alternative)

- [OT FAQ](https://www3.ntu.edu.sg/scse/staff/czsun/projects/otfaq/) — Comprehensive OT explanation
- [Google Wave OT](https://svn.apache.org/repos/asf/incubator/wave/whitepapers/operational-transform/operational-transform.html) — Google's OT implementation details

### CAT Tool Collaboration Models

- [memoQ Server](https://www.memoq.com/products/memoq-server) — Traditional segment locking model
- [Phrase TMS](https://phrase.com/platform/tms/) — Translation management with collaborative workflows

### Related ADRs

- [ADR-014: Distributed Processing Architecture](./014-distributed-processing-architecture.md) — Infrastructure this ADR builds upon
- [ADR-012: Bowrain Desktop App](./012-bowrain-desktop-app.md) — Bowrain architecture context
