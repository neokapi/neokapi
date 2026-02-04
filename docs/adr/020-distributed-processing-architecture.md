---
id: 020-distributed-processing-architecture
sidebar_position: 20
title: "ADR-020: Distributed Processing Architecture"
---
# ADR-020: Distributed Processing Architecture

## Context

Gokapi needs to evolve from a single-machine, single-flow processing system to support:
1. Efficient processing of large document collections
2. Distributed processing across multiple machines (Kubernetes clusters)
3. AI translation with rate limiting and worker pools
4. Binary asset handling (images, audio, video)
5. KAZ as both file format and API protocol
6. Thread-safe concurrent access to shared resources (TM, glossaries)
7. Real-time collaboration in Bowrain editor

This ADR synthesizes research on distributed processing patterns, modern TMS architectures, and Go concurrency best practices to design a comprehensive solution.

---

## Open Questions Requiring Decisions

This section documents architectural decisions that need stakeholder input. Each question includes options with their significance and trade-offs.

### Q1: Work Queue Technology

**Question:** Which message queue/work distribution system should we use?

| Option | Pros | Cons | Complexity |
|--------|------|------|------------|
| **A. Redis Streams** | Simple ops, KEDA native, atomic consumer groups, good Go support | Single point of failure (without Cluster), no built-in priorities | Low |
| **B. NATS JetStream** | Cloud-native, excellent Go support, built-in KV store, lightweight | Less mature tooling, smaller community | Medium |
| **C. Apache Kafka** | Battle-tested at scale, excellent durability, rich ecosystem | Heavy ops burden, overkill for small deployments | High |
| **D. Temporal.io** | Full workflow orchestration, retries/timeouts built-in, visibility | Additional infrastructure, learning curve | High |

**Significance:**
- **If Redis Streams**: Simplest path, but need to implement workflow logic ourselves. Good for teams familiar with Redis. Risk: Must handle priority queues manually.
- **If NATS JetStream**: Modern, lightweight alternative. Built-in work-queue retention policy. Risk: Less ecosystem tooling.
- **If Kafka**: Enterprise-grade durability and exactly-once semantics. Risk: Operational complexity inappropriate for small teams.
- **If Temporal**: Eliminates need to build workflow orchestration. Risk: Additional service dependency, steeper learning curve.

**Recommendation:** Redis Streams for initial implementation (Option A), with abstraction layer allowing migration to Temporal for complex workflows.

---

### Q2: Block Identity Strategy

**Question:** How should we uniquely identify blocks across extract-merge cycles?

| Option | Stability | Performance | Migration Complexity |
|--------|-----------|-------------|---------------------|
| **A. Content Hash Only** | High for unchanged content | O(1) lookup | Low |
| **B. Content + Context Hash** | Higher (survives reordering) | O(1) lookup | Medium |
| **C. Structural Path** | Format-dependent | Varies by format | High |
| **D. Hybrid (Hash + Path hints)** | Highest | O(1) + hints | Medium |

**Significance:**
- **If Content Hash Only**: Simple, but reordering same content creates new IDs. TM leverage breaks if paragraph order changes.
- **If Content + Context**: More robust to structural changes. Risk: Context changes when adjacent blocks change.
- **If Structural Path**: XPath/JSONPath provides human-readable IDs. Risk: Brittle to structural refactoring.
- **If Hybrid**: Best of both worlds. Content hash for identity, path for human reference. Risk: More complex migration logic.

**Sub-questions:**
- Should we normalize whitespace before hashing? (Affects: "Hello  World" vs "Hello World")
- Should inline markup affect hash? (Affects: `<b>Hello</b>` vs `Hello`)
- How do we handle the "Ship of Theseus" problem when content gradually changes?

**Recommendation:** Option D (Hybrid) with configurable normalization rules per format.

---

### Q3: Multi-Locale Processing Model

**Question:** How should flows handle multiple target locales?

| Option | Throughput | Resource Usage | Complexity |
|--------|------------|----------------|------------|
| **A. Sequential (current)** | 1 locale at a time | Low | Low |
| **B. Parallel Flows** | N locales parallel | N× memory | Medium |
| **C. Block-level Fanout** | Highest | Efficient | High |
| **D. Locale-aware Batching** | High | Moderate | Medium |

**Significance:**
- **If Sequential**: Simple, but slow for many locales. 10 locales = 10× time.
- **If Parallel Flows**: Good parallelism but N copies of document in memory. Risk: Memory pressure with large docs.
- **If Block-level Fanout**: Each block translated to all locales before moving to next. Best for AI batching. Risk: Complex state management.
- **If Locale-aware Batching**: Group blocks by locale, process in batches. Balances throughput and memory.

**Sub-questions:**
- Should TM lookup happen once (source) or per-locale?
- How do we handle locale-specific formatting rules?
- Should AI translation use multi-locale prompts (translate to 5 languages at once)?

**Recommendation:** Option D (Locale-aware Batching) for AI translation, Option B (Parallel Flows) for TM-only workflows.

---

### Q4: Checkpoint Granularity

**Question:** At what level should we checkpoint for resumable processing?

| Option | Recovery Speed | Storage Cost | Implementation |
|--------|----------------|--------------|----------------|
| **A. No Checkpointing** | Restart from beginning | None | Current |
| **B. Document-level** | Resume from last doc | Low | Small |
| **C. Block-level** | Resume from last block | Medium | Medium |
| **D. Tool-level** | Resume mid-pipeline | High | Large |

**Significance:**
- **If No Checkpointing**: Acceptable for small projects. Risk: Large projects lose hours of work on failure.
- **If Document-level**: Good balance. Complete documents saved, failed doc restarts. Risk: Large single documents still problematic.
- **If Block-level**: Fine-grained recovery. Risk: More I/O, complex state reconstruction.
- **If Tool-level**: Maximum recovery. Risk: Must serialize tool internal state, not always possible.

**Sub-questions:**
- Where should checkpoints be stored? (Local disk, Redis, S3?)
- How long should checkpoints be retained?
- Should checkpoints include TM cache state?

**Recommendation:** Option B (Document-level) for Phase 1, Option C (Block-level) for Phase 2.

---

### Q5: TM Architecture for Scale

**Question:** How should Translation Memory scale for distributed workers?

| Option | Read Latency | Write Consistency | Operational Complexity |
|--------|--------------|-------------------|------------------------|
| **A. Shared SQLite (current)** | ~1ms | Strong (single writer) | Low |
| **B. PostgreSQL + Read Replicas** | ~5ms | Eventual (replicas) | Medium |
| **C. Redis + PostgreSQL** | ~0.5ms (cached) | Eventual | Medium |
| **D. Dedicated TM Service** | ~2ms (RPC) | Configurable | High |

**Significance:**
- **If Shared SQLite**: Works for single-machine. Risk: Cannot scale horizontally.
- **If PostgreSQL + Replicas**: Standard scaling pattern. Risk: Replica lag may cause stale matches.
- **If Redis + PostgreSQL**: Cache hot segments, persist all. Risk: Cache invalidation complexity.
- **If Dedicated TM Service**: Clean separation, can optimize independently. Risk: Additional service to operate.

**Sub-questions:**
- What is the expected TM size? (10K, 100K, 1M+ entries?)
- What fuzzy match algorithm? (Levenshtein, n-gram, embedding-based?)
- Should TM be project-scoped or global?

**Recommendation:** Option C (Redis + PostgreSQL) with singleflight for deduplication.

---

### Q6: Real-Time Collaboration Technology

**Question:** What technology should power Bowrain's collaborative editing?

| Option | Offline Support | Conflict Resolution | Complexity |
|--------|-----------------|---------------------|------------|
| **A. OT (Operational Transform)** | Limited | Server-coordinated | High |
| **B. CRDTs (Yjs/Automerge)** | Full | Automatic merge | Medium |
| **C. Last-Write-Wins** | Full | Data loss possible | Low |
| **D. Segment Locking** | No | Prevention-based | Low |

**Significance:**
- **If OT**: Google Docs-style. Requires central server coordination. Risk: Complex implementation, no offline.
- **If CRDTs**: Offline-first, automatic merge. Risk: Merge results may surprise users.
- **If Last-Write-Wins**: Simple but lossy. Risk: Translator work can be overwritten.
- **If Segment Locking**: Traditional CAT tool model. Risk: Blocks concurrent work on same segment.

**Sub-questions:**
- Is offline translation a requirement?
- How many concurrent translators per project?
- Should we support real-time cursor sharing?

**Recommendation:** Option B (CRDTs via Yjs) for segment content, Option D (Locking) for segment assignment.

---

### Q7: Asset Storage Strategy

**Question:** How should binary assets be stored and distributed?

| Option | Deduplication | Latency | Cost |
|--------|---------------|---------|------|
| **A. Embedded in KAZ** | Per-package | Local fast | Package bloat |
| **B. Content-Addressable (S3)** | Global | Network | Storage efficient |
| **C. Hybrid (small embedded, large external)** | Partial | Variable | Balanced |
| **D. CDN with Signed URLs** | Global | Edge-cached | Higher |

**Significance:**
- **If Embedded**: Self-contained packages. Risk: 100MB image duplicated across packages.
- **If Content-Addressable**: Global dedup via hash. Risk: Requires network access.
- **If Hybrid**: Best of both for common case. Risk: Complex logic for size thresholds.
- **If CDN**: Best performance for distributed teams. Risk: Cost, CDN dependency.

**Sub-questions:**
- What is the size threshold for external storage? (100KB? 1MB?)
- Should assets be versioned or immutable?
- How do we handle asset cleanup when no longer referenced?

**Recommendation:** Option C (Hybrid) with 1MB threshold, content-addressable for external.

---

### Q8: API Protocol Choice

**Question:** What protocol should the Gokapi service API use?

| Option | Streaming | Browser Support | Tooling |
|--------|-----------|-----------------|---------|
| **A. REST + WebSocket** | Separate channels | Native | Excellent |
| **B. gRPC** | Native bidirectional | Via proxy | Good |
| **C. gRPC-Web** | Client streaming only | Native | Growing |
| **D. GraphQL + Subscriptions** | Via WebSocket | Native | Excellent |

**Significance:**
- **If REST + WebSocket**: Universal compatibility. Risk: Two protocols to maintain.
- **If gRPC**: Efficient binary protocol, great for service-to-service. Risk: Browser needs Envoy proxy.
- **If gRPC-Web**: Browser-native gRPC subset. Risk: No server streaming from browser.
- **If GraphQL**: Flexible queries, good for Bowrain. Risk: Complexity for simple operations.

**Sub-questions:**
- Is browser-direct access required, or always via Bowrain desktop?
- Do we need to support third-party integrations?
- What's the expected request volume?

**Recommendation:** Option B (gRPC) for service-to-service, REST gateway for external integrations.

---

## Part 1: Processing Pipeline Architecture

### 1.1 Current State Analysis

The current pipeline follows a simple streaming model:

```
RawDocument → FormatReader → [Tool₁] → [Tool₂] → ... → FormatWriter → Output
                                ↕           ↕
                          chan *Part    chan *Part
```

**Strengths:**
- Clean channel-based streaming with backpressure (64-element buffers)
- Per-document parallelism via semaphore-bounded errgroup
- Tool isolation via ToolFactories for parallel execution
- Context cancellation propagates to all stages

**Gaps:**
- No checkpointing or resumable processing
- Block IDs are positional (unstable across source changes)
- Single target locale per flow execution
- No batching for external API calls
- No distributed coordination

### 1.2 Proposed Multi-Tier Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           COORDINATION TIER                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Flow      │  │   Work      │  │   State     │  │   Event     │        │
│  │ Orchestrator│  │   Queue     │  │   Store     │  │   Bus       │        │
│  │   (Leader)  │  │ (Redis/NATS)│  │  (etcd/S3)  │  │ (NATS/Redis)│        │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┼─────────────────┐
                    ↓                 ↓                 ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│                            WORKER TIER                                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Worker    │  │   Worker    │  │   Worker    │  │   Worker    │        │
│  │   Pool A    │  │   Pool B    │  │   Pool C    │  │   Pool N    │        │
│  │ (AI Trans)  │  │ (TM Lookup) │  │ (QA Check)  │  │ (Format)    │        │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
┌─────────────────────────────────────────────────────────────────────────────┐
│                           RESOURCE TIER                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Translation │  │  Glossary   │  │   Asset     │  │   Config    │        │
│  │   Memory    │  │   Service   │  │   Storage   │  │   Store     │        │
│  │  (Postgres) │  │  (Postgres) │  │  (S3/MinIO) │  │   (etcd)    │        │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.3 Work Unit Granularity

Define three levels of work units:

| Level | Unit | Parallelism | State |
|-------|------|-------------|-------|
| **Project** | KAZ package | Sequential coordination | Project manifest |
| **Document** | Single file within project | Parallel across workers | Document state |
| **Block** | Translation unit | Parallel for AI/TM | Block state per locale |

```go
// core/distributed/work.go
type WorkUnit struct {
    ID          string          `json:"id"`
    Type        WorkUnitType    `json:"type"`     // Project, Document, Block
    ProjectID   string          `json:"project_id"`
    DocumentID  string          `json:"document_id,omitempty"`
    BlockID     string          `json:"block_id,omitempty"`
    Operation   string          `json:"operation"` // extract, translate, qa, merge
    Priority    int             `json:"priority"`
    Locale      model.LocaleID  `json:"locale,omitempty"`
    Checkpoint  *Checkpoint     `json:"checkpoint,omitempty"`
    CreatedAt   time.Time       `json:"created_at"`
}

type WorkUnitType int
const (
    WorkUnitProject WorkUnitType = iota
    WorkUnitDocument
    WorkUnitBlock
    WorkUnitBlockBatch  // Batch of blocks for efficient AI calls
)
```

---

## Part 2: Extract-Merge vs Source-Based Processing

### 2.1 Skeleton Strategies

Support three skeleton modes:

```go
// core/model/skeleton.go (enhanced)
type SkeletonStrategy int
const (
    SkeletonFragmentBased SkeletonStrategy = iota  // Current: stores structure
    SkeletonReparse                                 // Current: re-reads source
    SkeletonContentAddressable                      // New: hash-based identity
)

type Skeleton struct {
    Strategy    SkeletonStrategy
    Parts       []SkeletonPart
    SourceURI   string              // For reparse strategy
    SourceHash  string              // SHA-256 of source content
    Version     int64               // For optimistic locking
}
```

### 2.2 Stable Block Identity

Replace positional IDs with content-addressable identity:

```go
// core/model/block_identity.go
type BlockIdentity struct {
    // Stable identity (survives source changes if content unchanged)
    ContentHash  string  // SHA-256 of normalized source text
    ContextHash  string  // Hash of surrounding context (prev/next blocks)

    // Positional hints (for display, not identity)
    SourcePath   string  // XPath, JSON path, or line number
    SequenceNum  int     // Order in document

    // Lifecycle
    Version      int64   // Increments on content change
    PreviousID   string  // Link to previous version if migrated
}

func ComputeBlockIdentity(source string, context BlockContext) BlockIdentity {
    normalized := normalizeForHashing(source)
    return BlockIdentity{
        ContentHash: sha256Hex(normalized),
        ContextHash: sha256Hex(context.Previous + "|" + context.Next),
    }
}
```

### 2.3 Incremental Extraction

Detect changes without full re-extraction:

```go
// core/extract/incremental.go
type IncrementalExtractor struct {
    baseIndex    *kaz.BlockIndex
    sourceHashes map[string]string  // blockID -> contentHash
}

func (e *IncrementalExtractor) Extract(ctx context.Context, doc *model.RawDocument) (*ExtractionResult, error) {
    result := &ExtractionResult{
        Added:    []*model.Block{},
        Modified: []*model.Block{},
        Deleted:  []string{},
        Unchanged: []string{},
    }

    // Parse current document
    current := parseDocument(doc)

    for _, block := range current.Blocks {
        identity := ComputeBlockIdentity(block.SourceText(), block.Context)

        if existing, ok := e.sourceHashes[identity.ContentHash]; ok {
            // Content unchanged - preserve translations
            result.Unchanged = append(result.Unchanged, existing)
        } else if migrated := e.findByContext(identity.ContextHash); migrated != nil {
            // Content changed but context matches - mark for review
            block.Annotations["migration"] = &MigrationAnnotation{
                PreviousID: migrated.ID,
                ChangeType: "content_modified",
            }
            result.Modified = append(result.Modified, block)
        } else {
            // New block
            result.Added = append(result.Added, block)
        }
    }

    return result, nil
}
```

---

## Part 3: Concurrency and Parallelism

### 3.1 AI Worker Pool with Rate Limiting

```go
// ai/pool/pool.go
type AIWorkerPool struct {
    mu          sync.Mutex
    provider    provider.LLMProvider
    limiter     *rate.Limiter       // Token bucket
    breaker     *gobreaker.CircuitBreaker
    sem         *semaphore.Weighted // Concurrent request limit
    metrics     *PoolMetrics

    // Configuration
    maxConcurrent   int
    requestsPerSec  float64
    burstSize       int
    retryConfig     RetryConfig
}

type RetryConfig struct {
    MaxAttempts     int
    InitialBackoff  time.Duration
    MaxBackoff      time.Duration
    BackoffFactor   float64
    Jitter          float64
}

func NewAIWorkerPool(provider provider.LLMProvider, cfg PoolConfig) *AIWorkerPool {
    return &AIWorkerPool{
        provider:       provider,
        limiter:        rate.NewLimiter(rate.Limit(cfg.RequestsPerSec), cfg.BurstSize),
        sem:            semaphore.NewWeighted(int64(cfg.MaxConcurrent)),
        breaker:        gobreaker.NewCircuitBreaker(gobreaker.Settings{
            Name:        "ai-provider",
            MaxRequests: 3,
            Interval:    60 * time.Second,
            Timeout:     30 * time.Second,
            ReadyToTrip: func(counts gobreaker.Counts) bool {
                return counts.ConsecutiveFailures > 5
            },
        }),
        retryConfig:    cfg.Retry,
    }
}

func (p *AIWorkerPool) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
    // Acquire semaphore slot
    if err := p.sem.Acquire(ctx, 1); err != nil {
        return nil, err
    }
    defer p.sem.Release(1)

    // Wait for rate limit
    if err := p.limiter.Wait(ctx); err != nil {
        return nil, err
    }

    // Execute with circuit breaker and retry
    var lastErr error
    for attempt := 0; attempt <= p.retryConfig.MaxAttempts; attempt++ {
        result, err := p.breaker.Execute(func() (interface{}, error) {
            return p.provider.Translate(ctx, req.ToProviderRequest())
        })

        if err == nil {
            return result.(*TranslateResponse), nil
        }

        lastErr = err
        if !isRetryable(err) {
            break
        }

        backoff := calculateBackoff(attempt, p.retryConfig)
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(backoff):
        }
    }

    return nil, lastErr
}
```

### 3.2 Batch Processing for AI Calls

```go
// ai/batch/batcher.go
type BlockBatcher struct {
    pool        *AIWorkerPool
    batchSize   int
    flushInterval time.Duration

    mu          sync.Mutex
    pending     []*BatchItem
    flushTimer  *time.Timer
}

type BatchItem struct {
    Block    *model.Block
    Locale   model.LocaleID
    Response chan BatchResult
}

func (b *BlockBatcher) Submit(ctx context.Context, block *model.Block, locale model.LocaleID) (*model.Block, error) {
    item := &BatchItem{
        Block:    block,
        Locale:   locale,
        Response: make(chan BatchResult, 1),
    }

    b.mu.Lock()
    b.pending = append(b.pending, item)
    shouldFlush := len(b.pending) >= b.batchSize
    b.mu.Unlock()

    if shouldFlush {
        b.flush()
    }

    select {
    case result := <-item.Response:
        return result.Block, result.Error
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (b *BlockBatcher) flush() {
    b.mu.Lock()
    items := b.pending
    b.pending = nil
    b.mu.Unlock()

    if len(items) == 0 {
        return
    }

    // Build batch request
    texts := make([]string, len(items))
    for i, item := range items {
        texts[i] = item.Block.SourceText()
    }

    // Single API call for batch
    results, err := b.pool.TranslateBatch(context.Background(), BatchRequest{
        Texts:        texts,
        SourceLocale: items[0].Block.Source[0].Content.Locale,
        TargetLocale: items[0].Locale,
    })

    // Distribute results
    for i, item := range items {
        if err != nil {
            item.Response <- BatchResult{Error: err}
        } else {
            block := item.Block.Clone()
            block.SetTargetText(item.Locale, results.Translations[i])
            item.Response <- BatchResult{Block: block}
        }
        close(item.Response)
    }
}
```

### 3.3 TM Flow Integration

Pre-fill translations with TM matches before AI translation:

```go
// lib/sievepen/tm_flow.go
type TMLeverageFlow struct {
    tm          TranslationMemory
    minMatch    float64
    cache       *singleflight.Group
    cacheStore  *xsync.MapOf[string, []TMMatch]
}

func (f *TMLeverageFlow) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-in:
            if !ok {
                return nil
            }

            if part.Type == model.PartBlock {
                block := part.Resource.(*model.Block)
                part = f.leverageBlock(ctx, part, block)
            }

            select {
            case out <- part:
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
}

func (f *TMLeverageFlow) leverageBlock(ctx context.Context, part *model.Part, block *model.Block) *model.Part {
    sourceText := block.SourceText()
    cacheKey := fmt.Sprintf("%s:%s:%s", sourceText, f.sourceLocale, f.targetLocale)

    // Deduplicate concurrent lookups for same source
    matches, err, _ := f.cache.Do(cacheKey, func() (interface{}, error) {
        return f.tm.Lookup(sourceText, f.sourceLocale, f.targetLocale, LookupOptions{
            MinScore:   f.minMatch,
            MaxResults: 5,
        })
    })

    if err != nil || len(matches.([]TMMatch)) == 0 {
        return part
    }

    best := matches.([]TMMatch)[0]
    block.SetTargetText(f.targetLocale, best.Entry.Target)
    block.Annotations["tm_match"] = &TMMatchAnnotation{
        Score:     best.Score,
        MatchType: best.MatchType,
        EntryID:   best.Entry.ID,
    }

    return part
}
```

---

## Part 4: Kubernetes Scaling

### 4.1 Deployment Architecture

```yaml
# k8s/gokapi-cluster.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gokapi-coordinator
spec:
  replicas: 1  # Single leader via lease
  selector:
    matchLabels:
      app: gokapi-coordinator
  template:
    spec:
      containers:
      - name: coordinator
        image: gokapi/coordinator:latest
        env:
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: gokapi-secrets
              key: redis-url
        - name: ETCD_ENDPOINTS
          value: "etcd-0.etcd:2379,etcd-1.etcd:2379,etcd-2.etcd:2379"
        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gokapi-worker
spec:
  replicas: 3  # Scaled by KEDA
  selector:
    matchLabels:
      app: gokapi-worker
  template:
    spec:
      containers:
      - name: worker
        image: gokapi/worker:latest
        env:
        - name: WORKER_TYPE
          value: "general"  # or "ai", "tm", "format"
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: gokapi-secrets
              key: redis-url
        resources:
          requests:
            cpu: "1"
            memory: "2Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
---
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: gokapi-worker-scaler
spec:
  scaleTargetRef:
    name: gokapi-worker
  minReplicaCount: 1
  maxReplicaCount: 50
  triggers:
  - type: redis
    metadata:
      address: redis:6379
      listName: gokapi:work:pending
      listLength: "5"  # Scale up when >5 items per worker
```

### 4.2 Work Distribution with Redis Streams

```go
// core/distributed/queue.go
type WorkQueue struct {
    client    *redis.Client
    stream    string
    group     string
    consumer  string
}

func (q *WorkQueue) Submit(ctx context.Context, work *WorkUnit) error {
    data, _ := json.Marshal(work)
    return q.client.XAdd(ctx, &redis.XAddArgs{
        Stream: q.stream,
        Values: map[string]interface{}{
            "data":     data,
            "priority": work.Priority,
        },
    }).Err()
}

func (q *WorkQueue) Consume(ctx context.Context, handler func(*WorkUnit) error) error {
    for {
        streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    q.group,
            Consumer: q.consumer,
            Streams:  []string{q.stream, ">"},
            Count:    10,
            Block:    5 * time.Second,
        }).Result()

        if err == redis.Nil {
            continue
        }
        if err != nil {
            return err
        }

        for _, stream := range streams {
            for _, msg := range stream.Messages {
                var work WorkUnit
                json.Unmarshal([]byte(msg.Values["data"].(string)), &work)

                if err := handler(&work); err != nil {
                    // Move to dead-letter queue after retries
                    q.handleFailure(ctx, msg.ID, &work, err)
                } else {
                    q.client.XAck(ctx, q.stream, q.group, msg.ID)
                    q.client.XDel(ctx, q.stream, msg.ID)
                }
            }
        }
    }
}

func (q *WorkQueue) ClaimAbandoned(ctx context.Context, minIdle time.Duration) error {
    // Claim work from failed workers
    messages, _, _ := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
        Stream:   q.stream,
        Group:    q.group,
        Consumer: q.consumer,
        MinIdle:  minIdle,
        Count:    10,
    }).Result()

    for _, msg := range messages {
        // Reprocess claimed messages
    }
    return nil
}
```

### 4.3 Leader Election

```go
// core/distributed/leader.go
type LeaderElection struct {
    client    *clientv3.Client
    session   *concurrency.Session
    election  *concurrency.Election
    leaderKey string
}

func (l *LeaderElection) Run(ctx context.Context, onElected func(context.Context)) error {
    session, err := concurrency.NewSession(l.client, concurrency.WithTTL(10))
    if err != nil {
        return err
    }
    defer session.Close()

    l.session = session
    l.election = concurrency.NewElection(session, l.leaderKey)

    // Campaign blocks until we become leader
    if err := l.election.Campaign(ctx, l.nodeID); err != nil {
        return err
    }

    // We are the leader
    defer l.election.Resign(context.Background())

    onElected(ctx)
    return nil
}
```

---

## Part 5: Binary Asset Handling

### 5.1 Asset Model

```go
// core/model/asset.go
type Asset struct {
    ID           string            `json:"id"`
    ContentHash  string            `json:"content_hash"`  // SHA-256
    MIMEType     string            `json:"mime_type"`
    Size         int64             `json:"size"`
    URI          string            `json:"uri"`           // Storage location

    // Localization metadata
    Localizable  bool              `json:"localizable"`
    AltText      map[LocaleID]string `json:"alt_text,omitempty"`
    Transcripts  map[LocaleID]string `json:"transcripts,omitempty"`

    // Locale variants (different image for different locales)
    LocaleVariants map[LocaleID]string `json:"locale_variants,omitempty"` // locale -> assetID

    // Extracted content (for text in images, audio transcription)
    ExtractedText map[LocaleID]*ExtractedContent `json:"extracted_text,omitempty"`
}

type ExtractedContent struct {
    Text       string    `json:"text"`
    Confidence float64   `json:"confidence"`
    Regions    []Region  `json:"regions,omitempty"`  // For images: text bounding boxes
}
```

### 5.2 Content-Addressable Storage

```go
// core/storage/cas.go
type ContentAddressableStorage struct {
    backend  StorageBackend  // S3, MinIO, local filesystem
    index    *AssetIndex
}

func (s *ContentAddressableStorage) Store(ctx context.Context, r io.Reader, mime string) (*Asset, error) {
    // Stream to temp file while computing hash
    hasher := sha256.New()
    temp, _ := os.CreateTemp("", "asset-*")
    defer os.Remove(temp.Name())

    size, _ := io.Copy(io.MultiWriter(temp, hasher), r)
    hash := hex.EncodeToString(hasher.Sum(nil))

    // Check if already exists (deduplication)
    if existing := s.index.GetByHash(hash); existing != nil {
        return existing, nil
    }

    // Upload to backend
    temp.Seek(0, 0)
    uri := fmt.Sprintf("assets/%s/%s", hash[:2], hash)
    if err := s.backend.Upload(ctx, uri, temp); err != nil {
        return nil, err
    }

    asset := &Asset{
        ID:          uuid.New().String(),
        ContentHash: hash,
        MIMEType:    mime,
        Size:        size,
        URI:         uri,
    }
    s.index.Store(asset)

    return asset, nil
}

func (s *ContentAddressableStorage) Get(ctx context.Context, hash string) (io.ReadCloser, error) {
    asset := s.index.GetByHash(hash)
    if asset == nil {
        return nil, ErrNotFound
    }
    return s.backend.Download(ctx, asset.URI)
}
```

### 5.3 KAZ Package with Assets

```
project.kaz (ZIP)
├── manifest.yaml           # Project metadata
├── blocks/                 # Block indices per document
│   └── doc1.json
├── preview/                # HTML previews
│   └── doc1.html
├── items/                  # Original source documents
│   └── doc1.html
├── assets/                 # Binary assets (content-addressed)
│   ├── ab/
│   │   └── ab3f7c...      # Image file (stored by hash prefix)
│   └── cd/
│       └── cd8e2a...      # Another asset
├── assets.json             # Asset manifest with metadata
└── tm/                     # Optional embedded TM
    └── project.tmx
```

### 5.4 Chunked Upload for Large Files

```go
// core/transport/chunked.go
type ChunkedUploader struct {
    storage   *ContentAddressableStorage
    chunkSize int64  // Default 5MB
}

func (u *ChunkedUploader) InitUpload(ctx context.Context, totalSize int64, mime string) (*UploadSession, error) {
    session := &UploadSession{
        ID:         uuid.New().String(),
        TotalSize:  totalSize,
        ChunkSize:  u.chunkSize,
        MIMEType:   mime,
        Chunks:     make(map[int]string),  // index -> hash
        CreatedAt:  time.Now(),
        ExpiresAt:  time.Now().Add(24 * time.Hour),
    }
    // Store session in Redis
    return session, nil
}

func (u *ChunkedUploader) UploadChunk(ctx context.Context, sessionID string, index int, data io.Reader) error {
    session := u.getSession(sessionID)

    // Store chunk
    hash := sha256Stream(data)
    u.storage.backend.Upload(ctx, fmt.Sprintf("chunks/%s/%d", sessionID, index), data)

    session.Chunks[index] = hash
    session.UploadedSize += chunkSize

    return nil
}

func (u *ChunkedUploader) Complete(ctx context.Context, sessionID string) (*Asset, error) {
    session := u.getSession(sessionID)

    // Concatenate chunks into final asset
    readers := make([]io.Reader, len(session.Chunks))
    for i := 0; i < len(session.Chunks); i++ {
        r, _ := u.storage.backend.Download(ctx, fmt.Sprintf("chunks/%s/%d", sessionID, i))
        readers[i] = r
    }

    combined := io.MultiReader(readers...)
    return u.storage.Store(ctx, combined, session.MIMEType)
}
```

---

## Part 6: KAZ as API Protocol

### 6.1 Gokapi Service API

```protobuf
// api/proto/gokapi.proto
syntax = "proto3";
package gokapi.v1;

service GokapiService {
    // Project management
    rpc CreateProject(CreateProjectRequest) returns (Project);
    rpc GetProject(GetProjectRequest) returns (Project);
    rpc ListProjects(ListProjectsRequest) returns (ListProjectsResponse);

    // Document operations
    rpc AddDocument(stream AddDocumentRequest) returns (Document);
    rpc GetDocument(GetDocumentRequest) returns (Document);
    rpc StreamBlocks(StreamBlocksRequest) returns (stream Block);

    // Translation operations
    rpc UpdateTranslation(UpdateTranslationRequest) returns (Block);
    rpc BatchUpdateTranslations(stream UpdateTranslationRequest) returns (BatchUpdateResponse);

    // Flow execution
    rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgress);

    // Asset operations
    rpc UploadAsset(stream UploadAssetRequest) returns (Asset);
    rpc DownloadAsset(DownloadAssetRequest) returns (stream AssetChunk);

    // Real-time collaboration
    rpc Subscribe(SubscribeRequest) returns (stream Event);
}

message Project {
    string id = 1;
    string name = 2;
    string source_locale = 3;
    repeated string target_locales = 4;
    map<string, Document> documents = 5;
    int64 version = 6;
}

message Block {
    string id = 1;
    string document_id = 2;
    string content_hash = 3;
    string source = 4;
    map<string, Translation> translations = 5;  // locale -> translation
    map<string, string> annotations = 6;
    int64 version = 7;
}

message Translation {
    string text = 1;
    string status = 2;  // draft, reviewed, approved
    string translator_id = 3;
    google.protobuf.Timestamp updated_at = 4;
}
```

### 6.2 Bowrain Quick Connect

"Paste a code" workflow for connecting Bowrain to central server:

```go
// apps/bowrain/backend/connect.go
type ConnectionCode struct {
    ServerURL   string `json:"server_url"`
    ProjectID   string `json:"project_id"`
    AccessToken string `json:"access_token"`
    ExpiresAt   time.Time `json:"expires_at"`
}

func GenerateConnectionCode(projectID string, ttl time.Duration) (string, error) {
    code := &ConnectionCode{
        ServerURL:   config.ServerURL,
        ProjectID:   projectID,
        AccessToken: generateSecureToken(),
        ExpiresAt:   time.Now().Add(ttl),
    }

    // Encode as compact string (base62 encoded JSON + HMAC)
    data, _ := json.Marshal(code)
    signature := hmacSign(data)
    encoded := base62Encode(append(data, signature...))

    // Format as groups for easy typing: XXXX-XXXX-XXXX-XXXX
    return formatCode(encoded), nil
}

func (b *BowrainBackend) ConnectWithCode(code string) error {
    conn, err := parseConnectionCode(code)
    if err != nil {
        return err
    }

    // Establish gRPC connection
    client, err := grpc.Dial(conn.ServerURL,
        grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
        grpc.WithPerRPCCredentials(&tokenAuth{token: conn.AccessToken}),
    )
    if err != nil {
        return err
    }

    // Fetch project
    project, err := b.service.GetProject(ctx, &GetProjectRequest{Id: conn.ProjectID})
    if err != nil {
        return err
    }

    // Start real-time sync
    go b.startSync(client, project)

    return nil
}
```

### 6.3 Real-Time Sync Protocol

```go
// core/sync/sync.go
type SyncManager struct {
    client    GokapiServiceClient
    localStore *LocalStore
    conflicts  chan Conflict
}

func (s *SyncManager) StartSync(ctx context.Context, projectID string) error {
    // Subscribe to server events
    stream, err := s.client.Subscribe(ctx, &SubscribeRequest{
        ProjectId: projectID,
        Events:    []string{"block.updated", "document.added", "translation.changed"},
    })
    if err != nil {
        return err
    }

    for {
        event, err := stream.Recv()
        if err != nil {
            return err
        }

        switch e := event.Payload.(type) {
        case *Event_BlockUpdated:
            s.handleRemoteBlockUpdate(e.BlockUpdated)
        case *Event_TranslationChanged:
            s.handleRemoteTranslation(e.TranslationChanged)
        }
    }
}

func (s *SyncManager) handleRemoteBlockUpdate(update *BlockUpdated) {
    local := s.localStore.GetBlock(update.BlockId)

    if local == nil {
        // New block from server
        s.localStore.StoreBlock(update.Block)
        return
    }

    if local.Version >= update.Block.Version {
        // Local is same or newer, ignore
        return
    }

    if s.localStore.HasPendingChanges(update.BlockId) {
        // Conflict - local changes not yet synced
        s.conflicts <- Conflict{
            BlockID: update.BlockId,
            Local:   local,
            Remote:  update.Block,
        }
        return
    }

    // Apply remote update
    s.localStore.StoreBlock(update.Block)
}
```

---

## Part 7: Thread Safety and Concurrency Control

### 7.1 TM with Optimistic Locking

```go
// lib/sievepen/tm_distributed.go
type DistributedTM struct {
    db        *sql.DB
    cache     *xsync.MapOf[string, []TMMatch]
    cacheTTL  time.Duration
    singleflight *singleflight.Group
}

func (tm *DistributedTM) Add(ctx context.Context, entry TMEntry) error {
    // Use optimistic locking with version check
    for attempt := 0; attempt < 3; attempt++ {
        current, _ := tm.getEntryByID(ctx, entry.ID)

        var version int64 = 1
        if current != nil {
            if current.Version != entry.Version {
                return ErrVersionConflict
            }
            version = current.Version + 1
        }

        result, err := tm.db.ExecContext(ctx, `
            INSERT INTO tm_entries (id, source, target, source_locale, target_locale, version, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6, NOW())
            ON CONFLICT (id) DO UPDATE SET
                target = EXCLUDED.target,
                version = EXCLUDED.version,
                updated_at = NOW()
            WHERE tm_entries.version = $7
        `, entry.ID, entry.Source, entry.Target, entry.SourceLocale, entry.TargetLocale, version, entry.Version)

        if err != nil {
            return err
        }

        rows, _ := result.RowsAffected()
        if rows > 0 {
            // Invalidate cache
            tm.invalidateCache(entry.Source, entry.SourceLocale, entry.TargetLocale)
            return nil
        }

        // Version conflict, retry
    }

    return ErrVersionConflict
}

func (tm *DistributedTM) Lookup(ctx context.Context, source string, srcLocale, tgtLocale LocaleID, opts LookupOptions) ([]TMMatch, error) {
    cacheKey := fmt.Sprintf("%x:%s:%s", sha256.Sum256([]byte(source)), srcLocale, tgtLocale)

    // Check cache first
    if cached, ok := tm.cache.Load(cacheKey); ok {
        return cached, nil
    }

    // Deduplicate concurrent lookups
    result, err, _ := tm.singleflight.Do(cacheKey, func() (interface{}, error) {
        matches := tm.performLookup(ctx, source, srcLocale, tgtLocale, opts)

        // Cache result
        tm.cache.Store(cacheKey, matches)
        go func() {
            time.Sleep(tm.cacheTTL)
            tm.cache.Delete(cacheKey)
        }()

        return matches, nil
    })

    if err != nil {
        return nil, err
    }
    return result.([]TMMatch), nil
}
```

### 7.2 Segment Locking for Concurrent Editing

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

    // Only extend if we own the lock
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

---

## Part 8: Implementation Roadmap

### Phase 1: Foundation [Size: MEDIUM]

**Scope:** Core improvements to existing single-machine processing that enable future distribution.

**Prerequisites:** None (builds on current codebase)

**Dependencies:** None external

#### Checklist

**1.1 Stable Block Identity**
- [ ] Design `BlockIdentity` struct with content hash + context hash
- [ ] Implement `normalizeForHashing()` with configurable rules
- [ ] Add `ComputeBlockIdentity()` function
- [ ] Update all format readers to use new identity system
- [ ] Add `PreviousID` tracking for block migrations
- [ ] Create migration tool for existing KAZ files
- [ ] Update KAZ block index schema for new identity fields
- [ ] Document normalization rules per format

**1.2 TM Optimistic Locking**
- [ ] Add `Version` field to `TMEntry` struct
- [ ] Implement version check in `Add()` method
- [ ] Add retry logic with configurable attempts
- [ ] Create `ErrVersionConflict` error type
- [ ] Update SQLite schema with version column
- [ ] Implement cache invalidation on write
- [ ] Add `singleflight` for concurrent lookup deduplication
- [ ] Create TM migration script for version field

**1.3 AI Worker Pool**
- [ ] Implement `AIWorkerPool` struct with semaphore
- [ ] Add `golang.org/x/time/rate` limiter integration
- [ ] Integrate `sony/gobreaker` circuit breaker
- [ ] Implement `RetryConfig` with exponential backoff
- [ ] Add jitter to backoff calculation
- [ ] Create `PoolMetrics` for observability
- [ ] Implement provider-specific rate limit detection (429 handling)
- [ ] Add pool configuration via environment variables

**1.4 Block Batching**
- [ ] Implement `BlockBatcher` with configurable batch size
- [ ] Add flush timer for time-based batching
- [ ] Create `BatchItem` response channel pattern
- [ ] Implement `TranslateBatch()` in provider interface
- [ ] Add batch support to Anthropic provider
- [ ] Add batch support to OpenAI provider
- [ ] Handle partial batch failures gracefully
- [ ] Add metrics for batch efficiency

#### Testing Strategy for Phase 1

**Unit Tests:**
```go
// block_identity_test.go
func TestBlockIdentityStability(t *testing.T) {
    // Same content produces same hash
    // Different content produces different hash
    // Whitespace normalization works correctly
    // Context hash changes when neighbors change
}

func TestTMOptimisticLocking(t *testing.T) {
    // Concurrent writes with same version fail correctly
    // Version increments on successful update
    // Stale version rejected
    // Retry logic works with backoff
}

func TestAIWorkerPool(t *testing.T) {
    // Rate limiting enforced (use mock time)
    // Semaphore limits concurrent requests
    // Circuit breaker opens after failures
    // Retry with backoff works correctly
}

func TestBlockBatcher(t *testing.T) {
    // Batch flushes at size threshold
    // Batch flushes at time threshold
    // Results distributed to correct callers
    // Partial failures handled per-item
}
```

**Integration Tests:**
```go
func TestBlockIdentityAcrossFormats(t *testing.T) {
    // Extract HTML, modify whitespace, re-extract
    // Verify unchanged blocks keep same identity
    // Verify changed blocks get new identity with migration link
}

func TestTMConcurrentAccess(t *testing.T) {
    // 100 goroutines reading simultaneously
    // 10 goroutines writing simultaneously
    // Verify no data corruption
    // Verify singleflight deduplication
}

func TestAIPoolUnderLoad(t *testing.T) {
    // Submit 1000 requests with pool size 10
    // Verify rate limit not exceeded
    // Verify all requests eventually complete
    // Measure p50/p99 latency
}
```

**Benchmark Tests:**
```go
func BenchmarkBlockIdentityComputation(b *testing.B) {
    // Measure hash computation overhead
    // Compare with/without normalization
}

func BenchmarkTMLookupWithCache(b *testing.B) {
    // 100K entry TM, measure lookup latency
    // Compare cached vs uncached
    // Measure singleflight effectiveness
}
```

---

### Phase 2: Distribution [Size: LARGE]

**Scope:** Enable processing across multiple machines with work distribution and coordination.

**Prerequisites:** Phase 1 complete

**Dependencies:** Redis, etcd (or alternatives per Q1 decision)

#### Checklist

**2.1 Redis Streams Work Queue**
- [ ] Implement `WorkQueue` struct with Redis client
- [ ] Add consumer group creation on startup
- [ ] Implement `Submit()` with priority support
- [ ] Implement `Consume()` with blocking read
- [ ] Add `ClaimAbandoned()` for failed worker recovery
- [ ] Create dead-letter queue handling
- [ ] Implement work unit serialization/deserialization
- [ ] Add queue metrics (depth, processing time, failures)
- [ ] Create queue health check endpoint

**2.2 Leader Election**
- [ ] Implement `LeaderElection` with etcd client
- [ ] Add session management with TTL
- [ ] Implement campaign/resign lifecycle
- [ ] Add leader health monitoring
- [ ] Create graceful leadership transfer
- [ ] Implement leader-only operations guard
- [ ] Add leader change notifications
- [ ] Test split-brain scenarios

**2.3 Worker Autoscaling**
- [ ] Create Kubernetes Deployment manifests
- [ ] Implement KEDA ScaledObject configuration
- [ ] Add worker registration/deregistration
- [ ] Create worker health check endpoint
- [ ] Implement graceful shutdown with work completion
- [ ] Add worker-specific configuration (AI vs TM vs Format)
- [ ] Create Helm chart for deployment
- [ ] Document scaling parameters

**2.4 Checkpointing**
- [ ] Design checkpoint data structure
- [ ] Implement document-level checkpoint storage
- [ ] Add checkpoint creation after document completion
- [ ] Implement checkpoint restoration on startup
- [ ] Create checkpoint cleanup (TTL-based)
- [ ] Add checkpoint validation (detect corruption)
- [ ] Implement flow resumption from checkpoint
- [ ] Add checkpoint metrics and monitoring

#### Testing Strategy for Phase 2

**Unit Tests:**
```go
func TestWorkQueueOperations(t *testing.T) {
    // Submit adds to stream
    // Consume reads and acknowledges
    // Priority ordering works
    // Dead-letter after max retries
}

func TestLeaderElection(t *testing.T) {
    // Single candidate becomes leader
    // Leadership transfers on resign
    // Session expiry triggers re-election
    // Multiple candidates elect exactly one
}

func TestCheckpointing(t *testing.T) {
    // Checkpoint created after document
    // Restoration rebuilds correct state
    // Corrupted checkpoint detected
    // TTL cleanup works
}
```

**Integration Tests (require Redis/etcd):**
```go
func TestDistributedWorkProcessing(t *testing.T) {
    // Start 3 workers
    // Submit 100 work units
    // Verify all processed exactly once
    // Kill 1 worker mid-processing
    // Verify abandoned work reclaimed
}

func TestLeaderFailover(t *testing.T) {
    // Elect leader
    // Kill leader process
    // Verify new leader elected within TTL
    // Verify no duplicate leadership
}
```

**Chaos Tests:**
```go
func TestNetworkPartition(t *testing.T) {
    // Simulate network split between workers and Redis
    // Verify work not duplicated on reconnect
    // Verify checkpoint consistency
}

func TestWorkerCrashRecovery(t *testing.T) {
    // Worker crashes mid-document
    // Verify work reclaimed by other worker
    // Verify no partial state corruption
}
```

**Load Tests:**
```bash
# k6 or custom load test
# Target: 1000 documents/minute sustained
# Verify: Auto-scaling triggers correctly
# Verify: No memory leaks over 1 hour
```

---

### Phase 3: API & Protocol [Size: LARGE]

**Scope:** Expose gokapi functionality via gRPC API and enable Bowrain remote connection.

**Prerequisites:** Phase 2 complete

**Dependencies:** gRPC, protobuf compiler

#### Checklist

**3.1 gRPC API Design**
- [ ] Define proto files for all services
- [ ] Generate Go code from protos
- [ ] Implement `GokapiService` server
- [ ] Add authentication interceptor
- [ ] Implement rate limiting interceptor
- [ ] Add request logging middleware
- [ ] Create API versioning strategy
- [ ] Document all RPC methods

**3.2 Project Management API**
- [ ] Implement `CreateProject` RPC
- [ ] Implement `GetProject` RPC
- [ ] Implement `ListProjects` RPC with pagination
- [ ] Implement `DeleteProject` RPC
- [ ] Add project-level access control
- [ ] Create project settings management
- [ ] Implement project export/import

**3.3 Document & Block Operations**
- [ ] Implement `AddDocument` with streaming upload
- [ ] Implement `GetDocument` RPC
- [ ] Implement `StreamBlocks` for pagination
- [ ] Implement `UpdateTranslation` RPC
- [ ] Implement `BatchUpdateTranslations` streaming
- [ ] Add document locking for concurrent access
- [ ] Create document version tracking

**3.4 Connection Code System**
- [ ] Design connection code format
- [ ] Implement code generation with HMAC
- [ ] Create code parsing and validation
- [ ] Add code expiration handling
- [ ] Implement code revocation
- [ ] Create QR code generation option
- [ ] Add Bowrain UI for code entry
- [ ] Test code security (brute force resistance)

**3.5 Real-Time Sync**
- [ ] Implement `Subscribe` RPC with event streaming
- [ ] Create event bus for internal notifications
- [ ] Add event filtering per subscription
- [ ] Implement reconnection handling
- [ ] Create conflict detection logic
- [ ] Add sync state management
- [ ] Implement offline queue for pending changes
- [ ] Test high-frequency update scenarios

#### Testing Strategy for Phase 3

**Unit Tests:**
```go
func TestGRPCServiceMethods(t *testing.T) {
    // Each RPC method with valid input
    // Each RPC method with invalid input
    // Authentication required when configured
    // Rate limiting enforced
}

func TestConnectionCode(t *testing.T) {
    // Code generation produces valid format
    // Code parsing extracts correct fields
    // Expired code rejected
    // Tampered code (invalid HMAC) rejected
    // Brute force infeasible (timing analysis)
}

func TestRealTimeSync(t *testing.T) {
    // Event delivered to subscriber
    // Filtering works correctly
    // Reconnection resumes from last event
    // Conflicts detected correctly
}
```

**Integration Tests:**
```go
func TestEndToEndProjectWorkflow(t *testing.T) {
    // Create project via API
    // Add document via streaming
    // Execute translation flow
    // Verify translations via API
    // Export and verify KAZ
}

func TestBowrainConnection(t *testing.T) {
    // Generate connection code
    // Connect Bowrain (simulated)
    // Sync project data
    // Update translation in Bowrain
    // Verify sync to server
}
```

**Contract Tests:**
```go
func TestAPIBackwardCompatibility(t *testing.T) {
    // Record API responses as golden files
    // Verify no breaking changes on update
    // Test with previous client versions
}
```

**Performance Tests:**
```go
func BenchmarkStreamingUpload(b *testing.B) {
    // Upload 100MB document
    // Measure throughput and latency
}

func BenchmarkEventBroadcast(b *testing.B) {
    // 100 subscribers
    // 1000 events/second
    // Measure delivery latency
}
```

---

### Phase 4: Assets & Scale [Size: X-LARGE]

**Scope:** Production-ready asset handling and full Kubernetes deployment.

**Prerequisites:** Phase 3 complete

**Dependencies:** S3/MinIO, OCR service (optional), Kubernetes cluster

#### Checklist

**4.1 Content-Addressable Storage**
- [ ] Implement `ContentAddressableStorage` struct
- [ ] Add S3 backend implementation
- [ ] Add MinIO backend implementation
- [ ] Add local filesystem backend (for dev)
- [ ] Implement hash-based deduplication
- [ ] Create asset index with metadata
- [ ] Add garbage collection for unreferenced assets
- [ ] Implement asset integrity verification

**4.2 Chunked Upload/Download**
- [ ] Implement `ChunkedUploader` struct
- [ ] Create upload session management
- [ ] Implement chunk storage and tracking
- [ ] Add chunk completion and assembly
- [ ] Implement resumable uploads
- [ ] Add download with range requests
- [ ] Create progress reporting
- [ ] Test upload interruption recovery

**4.3 Asset Extraction Pipeline**
- [ ] Implement image OCR integration (optional)
- [ ] Add audio transcription integration (optional)
- [ ] Create alt-text extraction from images
- [ ] Implement video subtitle extraction
- [ ] Add extracted content to Block model
- [ ] Create extraction job queue
- [ ] Add extraction result caching

**4.4 Kubernetes Production Deployment**
- [ ] Create production Helm chart
- [ ] Add ConfigMaps for configuration
- [ ] Create Secrets management
- [ ] Implement health and readiness probes
- [ ] Add resource limits and requests
- [ ] Create PodDisruptionBudgets
- [ ] Implement NetworkPolicies
- [ ] Add Prometheus ServiceMonitor
- [ ] Create Grafana dashboards
- [ ] Document deployment procedures
- [ ] Create disaster recovery runbook

**4.5 Observability**
- [ ] Add OpenTelemetry tracing
- [ ] Implement structured logging
- [ ] Create custom metrics for business KPIs
- [ ] Add alerting rules
- [ ] Create operational runbooks
- [ ] Implement log aggregation
- [ ] Add distributed tracing correlation

#### Testing Strategy for Phase 4

**Unit Tests:**
```go
func TestContentAddressableStorage(t *testing.T) {
    // Store returns correct hash
    // Duplicate content returns existing asset
    // Get retrieves correct content
    // Missing asset returns error
}

func TestChunkedUpload(t *testing.T) {
    // Multi-chunk upload assembles correctly
    // Missing chunk detected
    // Out-of-order chunks handled
    // Session expiration works
}

func TestAssetExtraction(t *testing.T) {
    // OCR extracts text from image
    // Transcription extracts audio text
    // Extraction errors handled gracefully
}
```

**Integration Tests:**
```go
func TestS3Backend(t *testing.T) {
    // Upload to real S3 (localstack)
    // Download from S3
    // Verify content integrity
}

func TestEndToEndAssetWorkflow(t *testing.T) {
    // Upload document with images
    // Extract to KAZ
    // Verify assets in package
    // Translate with asset references
    // Merge and verify output
}
```

**End-to-End Tests (Kubernetes):**
```bash
# Deploy to test cluster
# Run full workflow
# Verify scaling behavior
# Test failover scenarios
# Measure resource utilization
```

**Security Tests:**
- [ ] Penetration testing of API endpoints
- [ ] Signed URL expiration verification
- [ ] Access control boundary testing
- [ ] Secrets exposure audit

**Disaster Recovery Tests:**
- [ ] Simulate node failure
- [ ] Simulate zone failure
- [ ] Test backup restoration
- [ ] Verify data integrity after recovery

---

### Phase 5: Collaboration [Size: LARGE]

**Scope:** Real-time collaborative editing in Bowrain with conflict resolution.

**Prerequisites:** Phase 3 complete (Phase 4 optional but recommended)

**Dependencies:** Yjs library, WebSocket infrastructure

#### Checklist

**5.1 Segment Locking**
- [ ] Implement `SegmentLocker` with Redis
- [ ] Add lock acquisition with TTL
- [ ] Implement lock extension (heartbeat)
- [ ] Add lock release on completion
- [ ] Create automatic lock expiration
- [ ] Implement lock ownership verification
- [ ] Add lock status in UI
- [ ] Test concurrent lock requests

**5.2 CRDT Integration (Yjs)**
- [ ] Evaluate Yjs Go port vs JavaScript runtime
- [ ] Implement Yjs document model for blocks
- [ ] Create Y.Text for translation content
- [ ] Add Y.Map for block metadata
- [ ] Implement sync protocol
- [ ] Create awareness protocol for presence
- [ ] Add offline change buffering
- [ ] Test merge scenarios

**5.3 Conflict Resolution**
- [ ] Design conflict detection logic
- [ ] Create conflict notification system
- [ ] Implement manual resolution UI
- [ ] Add automatic resolution rules
- [ ] Create conflict history tracking
- [ ] Implement resolution audit log
- [ ] Test complex merge scenarios

**5.4 Presence Indicators**
- [ ] Implement cursor position tracking
- [ ] Add user avatars/colors
- [ ] Create "who's editing" display
- [ ] Implement typing indicators
- [ ] Add online/offline status
- [ ] Create activity feed
- [ ] Test with many concurrent users

**5.5 Collaboration UI (Bowrain)**
- [ ] Add collaborative editing mode toggle
- [ ] Create presence display component
- [ ] Implement conflict resolution dialog
- [ ] Add sync status indicator
- [ ] Create offline mode indicator
- [ ] Implement collaborative comments
- [ ] Add @mentions in comments
- [ ] Test UI responsiveness under sync load

#### Testing Strategy for Phase 5

**Unit Tests:**
```go
func TestSegmentLocking(t *testing.T) {
    // Lock acquisition succeeds
    // Second lock attempt fails
    // Lock extension works
    // Expired lock can be acquired
    // Unlock only by owner
}

func TestCRDTMerge(t *testing.T) {
    // Concurrent inserts merge correctly
    // Concurrent deletes merge correctly
    // Insert + delete preserves intention
    // Complex multi-user scenario
}

func TestConflictDetection(t *testing.T) {
    // Same block, same time = conflict
    // Same block, different fields = no conflict
    // Resolve keeps correct version
}
```

**Integration Tests:**
```go
func TestCollaborativeEditing(t *testing.T) {
    // Two users open same project
    // User A edits block 1
    // User B sees update in real-time
    // User B edits block 1
    // User A sees update
    // Verify final state consistent
}

func TestOfflineSync(t *testing.T) {
    // User goes offline
    // User makes edits
    // User comes online
    // Changes sync correctly
    // No data loss
}
```

**User Acceptance Tests:**
- [ ] 5 concurrent translators on same project
- [ ] Network interruption recovery
- [ ] Large project (1000+ blocks) performance
- [ ] Mobile device (Bowrain future) compatibility

**Stress Tests:**
```go
func TestHighConcurrency(t *testing.T) {
    // 50 concurrent users
    // 10 updates per second per user
    // Verify no lost updates
    // Verify UI remains responsive
    // Measure sync latency p99
}
```

---

## Decision Summary

| Concern | Decision | Rationale |
|---------|----------|-----------|
| **Work Queue** | Redis Streams | Simple, reliable, KEDA integration |
| **Coordination** | etcd + leader election | Strong consistency for single-writer scenarios |
| **Block Identity** | Content-hash + context-hash | Stable across source changes |
| **AI Rate Limiting** | Token bucket + circuit breaker | Handles burst and failures |
| **TM Concurrency** | Optimistic locking + singleflight | Read-heavy workload optimization |
| **Real-time Sync** | gRPC streaming + CRDTs | Offline-first with conflict resolution |
| **Asset Storage** | Content-addressable (S3/MinIO) | Deduplication, immutability |
| **Scaling** | KEDA on Kubernetes | Event-driven, cost-efficient |

---

## Size Definitions

| Size | Characteristics | Team Scope |
|------|-----------------|------------|
| **Small** | Single component, <10 files changed, self-contained | 1 developer |
| **Medium** | Multiple components, 10-30 files, some integration | 1-2 developers |
| **Large** | Cross-cutting, 30-100 files, external dependencies | 2-3 developers |
| **X-Large** | System-wide, 100+ files, infrastructure changes | Team effort |

---

## Testing Philosophy

### Test Pyramid

```
                    /\
                   /  \
                  / E2E \         <- Few, slow, expensive
                 /──────\
                /  Integ  \       <- Some, moderate
               /──────────\
              /    Unit     \     <- Many, fast, cheap
             /________________\
```

### Test Categories by Phase

| Phase | Unit | Integration | E2E | Performance | Chaos |
|-------|------|-------------|-----|-------------|-------|
| 1 Foundation | ✓✓✓ | ✓✓ | ✓ | ✓ | - |
| 2 Distribution | ✓✓ | ✓✓✓ | ✓ | ✓✓ | ✓✓ |
| 3 API | ✓✓ | ✓✓ | ✓✓ | ✓ | ✓ |
| 4 Assets | ✓✓ | ✓✓ | ✓✓✓ | ✓✓ | ✓✓ |
| 5 Collaboration | ✓✓ | ✓✓✓ | ✓✓ | ✓✓✓ | ✓ |

### Continuous Integration Requirements

Each phase must pass:
1. All existing tests (regression)
2. New unit tests (>80% coverage for new code)
3. Integration tests for new features
4. Linting and static analysis
5. Security scanning (dependencies)

### Test Data Management

- Use factories for test data generation
- Golden files for expected outputs
- Seed data for integration tests
- Anonymized production data for load tests

---

## References

- Apache Beam Programming Model
- Kubernetes Job Patterns
- KEDA Event-Driven Autoscaling
- Redis Streams Documentation
- etcd Concurrency Package
- Temporal.io Workflow Patterns
- Yjs CRDT Documentation
- Phrase/Lokalise/Crowdin API Documentation
- Okapi Framework Architecture
- XLIFF 2.0 Specification
