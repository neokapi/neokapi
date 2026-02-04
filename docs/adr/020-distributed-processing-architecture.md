# ADR-020: Distributed Processing Architecture

## Status
**Proposed** — Deep research and design phase

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

### Phase 1: Foundation (Weeks 1-4)
- [ ] Implement stable block identity (content-hash based)
- [ ] Add optimistic locking to TM
- [ ] Create AI worker pool with rate limiting
- [ ] Add block batching for AI translation

### Phase 2: Distribution (Weeks 5-8)
- [ ] Implement Redis Streams work queue
- [ ] Add leader election for coordinator
- [ ] Create KEDA-based worker autoscaling
- [ ] Implement checkpointing for long flows

### Phase 3: API & Protocol (Weeks 9-12)
- [ ] Design and implement gRPC API
- [ ] Create KAZ streaming protocol
- [ ] Implement connection code system for Bowrain
- [ ] Add real-time sync manager

### Phase 4: Assets & Scale (Weeks 13-16)
- [ ] Implement content-addressable storage
- [ ] Add chunked upload/download
- [ ] Create asset extraction pipeline (OCR, transcription)
- [ ] Deploy to Kubernetes with full scaling

### Phase 5: Collaboration (Weeks 17-20)
- [ ] Implement segment locking
- [ ] Add CRDT-based offline editing (Yjs integration)
- [ ] Create conflict resolution UI
- [ ] Add presence indicators

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
