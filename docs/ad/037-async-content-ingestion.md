---
id: 037-async-content-ingestion
sidebar_position: 37
title: "AD-037: Async Content Ingestion"
---
# AD-037: Async Content Ingestion

## Context

The sync push endpoint (`POST /sync/push`) currently processes content synchronously: it receives up to 1,000 blocks in a single HTTP request, groups them by item, and writes them to the database using individual INSERT statements within the request lifecycle. This has several production-readiness issues:

- **API server holds DB connections** for the entire write duration (seconds for large pushes)
- **No atomicity across items** — partial failures leave inconsistent state
- **N blocks = N individual SQL statements** — no bulk insert optimization
- **60-80MB peak memory** per large push, multiplied by concurrent requests
- **No rate limiting** — a single user can exhaust server resources
- **GetBlocks returns unbounded results** — no pagination enforced

These issues are manageable at moderate scale but become critical as usage grows.

## Decision

### Async push: blob upload then job processing

Replace the synchronous push with a two-phase async pattern:

```
Current (synchronous):
  Client → API → DB writes (holds connection) → 200 OK

New (async):
  Client → API → Blob Storage → 202 Accepted (with push_id)
  Worker → Read blob → Validate → Bulk INSERT → DB
  Client → Poll /sync/status?push_id=X or receive notification
```

The API server's responsibility shrinks to: validate auth, write blob, enqueue job, return immediately. The heavy DB work moves to the worker, which controls its own memory, batching, and retry behavior.

### Push endpoint changes

`POST /api/v1/projects/:id/sync/push` becomes:

1. **Validate** request (auth, project exists, block count limit)
2. **Write blocks to blob storage** as a JSON file: `pushes/{push_id}.json`
3. **Enqueue** a `sync-push` job with the push_id
4. **Return 202 Accepted** with `{ "push_id": "...", "status": "queued" }`

The existing push status endpoint (`GET /sync/status?push_id=X`) already returns job aggregation — it continues to work.

### Sync push worker

A new job type processed by the existing worker infrastructure:

1. **Read blob** from storage (streaming, not all-in-memory)
2. **Validate** block schema
3. **Group by item** and process in batches
4. **Bulk INSERT** using PostgreSQL `COPY` or multi-value INSERT (10-100x faster)
5. **Single transaction per item** with proper error handling
6. **Publish `EventPushCompleted`** when all items are stored
7. **Delete blob** after successful processing

### Bulk INSERT optimization

Replace individual `INSERT...ON CONFLICT` with batched multi-row inserts:

```sql
-- Current: N individual statements
INSERT INTO blocks (...) VALUES ($1,...) ON CONFLICT DO UPDATE ...
INSERT INTO blocks (...) VALUES ($1,...) ON CONFLICT DO UPDATE ...
-- repeated N times

-- New: batched (50-100 rows per statement)
INSERT INTO blocks (...) VALUES
  ($1,...), ($2,...), ($3,...), ... ($50,...)
ON CONFLICT (project_id, id) DO UPDATE SET ...
```

For PostgreSQL, `COPY` protocol is even faster but doesn't support upsert. Multi-row INSERT with ON CONFLICT is the pragmatic choice.

### Blob storage backend

```go
type BlobStore interface {
    Put(ctx context.Context, key string, data io.Reader) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
}
```

Implementations:
- **Azure Blob Storage** (production) — already deployed, managed identity auth
- **Local filesystem** (dev) — `$TMPDIR/bowrain-blobs/`
- Both already exist in the codebase for media assets

### Rate limiting

Add per-project rate limiting on the push endpoint:

- **Max 10 pushes per minute per project** (configurable)
- **Max 50MB per push** (already enforced)
- **Max 10,000 blocks per push** (increase from current 1,000)
- Returns 429 Too Many Requests when exceeded

### Pull endpoint pagination

Enforce pagination on `GET /sync/blocks`:

- **Default limit: 1,000 blocks** per response
- **Max limit: 10,000 blocks**
- Cursor-based pagination via `offset` parameter
- Response includes `next_offset` when more blocks exist

### Backward compatibility

The CLI and GitHub Action currently expect synchronous push (200 OK with stored block count). The change to 202 Accepted requires client updates:

- **bowrain CLI**: Already has `bowrain sync` which does push → wait → pull. The push step changes from expecting 200 to accepting 202 and polling status.
- **GitHub Action**: Same — poll for completion after push.
- **API clients**: Must handle 202 and poll `/sync/status`.

A **transition period** can support both: if the request includes `?async=true`, use the new path. Default to sync for backward compat until clients are updated.

## Implementation

### Phase 1: Bulk INSERT + atomic transactions
- Replace individual INSERTs with batched multi-row INSERT
- Wrap multi-item push in a single transaction
- Add rate limiting middleware on push endpoint
- Add pagination to GetBlocks handler

### Phase 2: Async push via blob
- Add `sync-push` job type to worker
- Push endpoint writes blob + enqueues job
- Worker reads blob, processes with bulk INSERT
- Support `?async=true` query param for transition

### Phase 3: Client updates
- Update bowrain CLI to handle 202 + poll
- Update GitHub Action
- Make async the default

### Files to modify

| File | Change |
|---|---|
| `server/handlers_sync.go` | Async push path, rate limiting, pagination |
| `store/postgres.go` | Bulk INSERT implementation |
| `store/sqlite.go` | Batched INSERT for dev |
| `jobs/worker.go` | Sync push job type |
| `server/server.go` | Rate limiting middleware |

## Alternatives Considered

- **Keep synchronous but add bulk INSERT**: Improves speed but doesn't solve the API-holds-DB-connection problem. Under high concurrency, the API server still becomes a bottleneck.

- **gRPC streaming for push**: More complex client integration. The blob approach works with any HTTP client and provides natural retry (re-read the blob).

- **Queue the raw request body**: Simpler than blob but limited by queue message size (256KB for Service Bus). Blobs have no practical size limit.

## Operational: Blob Store Alignment

The API server and worker **must use the same blob store**. In production, both use Azure Blob Storage via managed identity. In local dev, both must point to the same filesystem directory (set `BLOB_STORAGE_LOCAL_DIR` / `LOCAL_BLOB_DIR` to the same path).

If they use different stores, the worker cannot find chunks uploaded by the server, and push jobs fail silently with "chunk download failed".

The dev Makefile pins both to `/tmp/bowrain-blobs`. The bowrain-infra `containerapp-worker.bicep` passes `AZURE_STORAGE_ACCOUNT_URL` and `AZURE_STORAGE_CONTAINER` to ensure blob store alignment in Azure.

## Consequences

- Push endpoint returns 202 immediately — API server stays responsive under any load
- Worker processes at its own pace with proper batching — 10-100x faster DB writes
- Blob provides natural durability — if worker crashes, blob is re-processed on retry
- Rate limiting prevents resource exhaustion
- Pagination prevents memory exhaustion on reads
- Slightly more complex flow for clients (poll for completion)
