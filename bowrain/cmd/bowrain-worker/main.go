package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	bowsync "github.com/neokapi/neokapi/bowrain/sync"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/cmd/internal/boot"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/crypto"
	bowevent "github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	sqlmemory "github.com/neokapi/neokapi/bowrain/memory"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/resilience"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/storage/blobcfg"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/version"
	fwmemory "github.com/neokapi/neokapi/memory"
	fwterms "github.com/neokapi/neokapi/terms"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	// Register the AWS Bedrock AI provider ("bedrock") in the aiprovider registry,
	// so BOWRAIN_PLATFORM_PROVIDER=bedrock resolves for platform translation jobs.
	_ "github.com/neokapi/neokapi/bowrain/ai/bedrock"
)

func main() {
	// Structured logging — bridges existing log.Printf calls through slog.
	observe.SetupLogger(
		os.Getenv("BOWRAIN_LOG_FORMAT"),
		os.Getenv("BOWRAIN_LOG_LEVEL"),
	)

	// Error tracking (no-op without SENTRY_DSN). Flushed at the end of run().
	observe.InitSentryFromEnv("worker", version.Version+"+"+version.Commit)
	// Route the framework's own spans here too. Without this the framework
	// measures its work and hands it to a tracer that does not exist.
	observe.InstallFrameworkTracer()

	if err := run(); err != nil {
		slog.Error("worker failed", "error", err)
		observe.FlushSentry(2 * time.Second)
		os.Exit(1)
	}
}

func run() error {
	defer observe.FlushSentry(2 * time.Second)

	allowInsecureDev := flag.Bool("allow-insecure-dev", false,
		"Allow starting without BOWRAIN_DATABASE_URL (local development only; also BOWRAIN_ALLOW_INSECURE_DEV=1)")
	flag.Parse()

	dbURL := os.Getenv("BOWRAIN_DATABASE_URL")
	insecureDev := *allowInsecureDev || boot.AllowInsecureDevFromEnv()
	if err := validateWorkerDBURL(dbURL, insecureDev); err != nil {
		return err
	}

	return runWorker(dbURL)
}

// validateWorkerDBURL enforces the worker's fail-fast boot contract, mirroring
// bowrain-server (see cmd/internal/boot): a malformed BOWRAIN_DATABASE_URL is
// always fatal; a MISSING one is fatal unless the insecure-dev escape hatch is
// set. A worker cannot reach its stores without a database, so a typo'd or
// unset env var must abort startup with a clear message rather than boot a
// process that will nil-panic or silently do nothing.
func validateWorkerDBURL(dbURL string, insecureDev bool) error {
	if dbURL == "" {
		if insecureDev {
			slog.Warn("INSECURE DEV MODE: starting worker without BOWRAIN_DATABASE_URL")
			return nil
		}
		return fmt.Errorf("BOWRAIN_DATABASE_URL is required (set --allow-insecure-dev or %s=1 to override for local development)", boot.InsecureDevEnv)
	}
	// A non-empty but malformed URL is always fatal — never excusable as dev mode.
	return boot.ValidatePostgresURL("BOWRAIN_DATABASE_URL", dbURL)
}

func runWorker(dbURL string) error {
	if err := guardSecretsKey(os.Getenv("BOWRAIN_SECRETS_KEY"), dbURL); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	platformProvider := os.Getenv("BOWRAIN_PLATFORM_PROVIDER")
	credentialsPath := os.Getenv("BOWRAIN_CREDENTIALS_PATH")
	if credentialsPath == "" {
		credentialsPath = credentials.DefaultPath()
	}

	// Open PostgreSQL stores.
	pgdb, err := storage.OpenPostgres(dbURL)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer pgdb.Close()

	pgCS, err := bstore.NewPostgresStoreFromDB(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL content store: %w", err)
	}
	secretsCipher, err := crypto.NewCipher(os.Getenv("BOWRAIN_SECRETS_KEY"))
	if err != nil {
		return fmt.Errorf("invalid BOWRAIN_SECRETS_KEY: %w", err)
	}
	pgCS.SetSecretsCipher(secretsCipher)
	var cs store.ContentStore = pgCS

	pgJS, err := jobs.NewJobStore(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL job store: %w", err)
	}

	pgQS, err := jobs.NewQuotaStore(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL quota store: %w", err)
	}

	// The queue backend is selected the same way as the server (SQS when
	// configured, in-process otherwise) so both agree on the broker.
	sqsOpts := jobs.SQSOptionsFromEnv()

	// Set up translation job queue.
	var translationQueue jobs.Queue
	switch {
	case jobs.SQSConfigured():
		translationQueue, err = jobs.NewSQSQueue(ctx, sqsOpts, jobs.SQSTranslationQueue)
		if err != nil {
			return fmt.Errorf("connect to SQS (translation): %w", err)
		}
	default:
		translationQueue = jobs.NewChannelQueue(64)
	}
	defer translationQueue.Close()

	// Set up the extraction job queue (Bowrain AD-013/AD-015). The server's
	// auto-extract automation enqueues on the same broker; this worker
	// consumes. Mirrors the translation queue selection exactly.
	var extractionQueue jobs.Queue
	switch {
	case jobs.SQSConfigured():
		extractionQueue, err = jobs.NewSQSQueue(ctx, sqsOpts, jobs.SQSExtractionQueue)
		if err != nil {
			return fmt.Errorf("connect to SQS (extraction): %w", err)
		}
	default:
		extractionQueue = jobs.NewChannelQueue(64)
	}
	defer extractionQueue.Close()

	// Set up the brand-scan job queue (epic 016). The server's brand
	// onboarding scan endpoint enqueues; this worker consumes. Mirrors the
	// extraction queue selection exactly.
	var contextScanQueue jobs.Queue
	switch {
	case jobs.SQSConfigured():
		contextScanQueue, err = jobs.NewSQSQueue(ctx, sqsOpts, jobs.SQSContextScanQueue)
		if err != nil {
			return fmt.Errorf("connect to SQS (context scan): %w", err)
		}
	default:
		contextScanQueue = jobs.NewChannelQueue(64)
	}
	defer contextScanQueue.Close()

	credStore := credentials.NewStore(credentialsPath)

	// Per-workspace AI provider store (Epic 004). Resolves a job's saved BYO
	// provider config from Postgres, scoped to the job's workspace, with the API
	// key sealed at rest under the same BOWRAIN_SECRETS_KEY cipher as connector
	// secrets. Replaces the machine-global keychain CredStore for worker
	// translation providers.
	providerStore := bstore.NewProviderConfigStore(pgdb.DB, secretsCipher)

	// Connector configs for durable forge-ingest jobs: reads the persisted
	// (decrypted) connector config to instantiate the connector, and records
	// the ingest outcome (last-sync / last-error) on the row for the status
	// path. Same table + cipher as the server's store.
	connectorConfigs := bstore.NewConnectorConfigStore(pgdb.DB, secretsCipher)

	// Expose the DB pool's saturation as Prometheus gauges.
	observe.RegisterDBStats(pgdb.DB, "worker")

	// Build translation worker dependencies.
	translationDeps := &jobs.WorkerDeps{
		QueueName:     "translation",
		JobStore:      pgJS,
		ContentStore:  cs,
		CredStore:     credStore,
		ProviderStore: providerStore,
		Queue:         translationQueue,
		QuotaStore:    pgQS,
		// content memory-first convergence: give the worker per-workspace content memory access so a
		// convergence translation job recycles exact/near-exact matches before
		// paying for AI, and ingest seeds pushed targets into the content memory. Mirrors the
		// server's per-workspace, PostgreSQL-backed content memory (NewPostgresStoreFromDB).
		MemoryResolver: newWorkerMemoryResolver(pgdb),
		// Durable forge ingest (webhook/bind-triggered source ingests, enqueued
		// by the server): the worker performs the clone+fetch+store the server
		// used to fire-and-forget in-process, so a mid-deploy task death can no
		// longer lose an acked webhook.
		ConnectorFetcher: newIngestFetcher(cs, connectorConfigs),
		ConnectorConfigs: connectorConfigs,
	}

	// Auth store: consulted for the workspace-level default voice profile
	// (brand context, below) and per-workspace platform model choice (platform
	// resolver, below). Opening it is non-fatal: on failure those refinements
	// are skipped. The auth schema migration is idempotent + advisory-locked,
	// so the worker constructing the store alongside the server is safe.
	var authStore *auth.PostgresAuthStore
	if as, err := auth.NewAuthStoreFromDB(pgdb); err != nil {
		slog.Warn("auth store unavailable; worker skips per-workspace model choice and workspace-default voice", "error", err)
	} else {
		authStore = as
	}

	// Voice context (parity with the CLI flow's project bindings): the voice
	// store + workspace-default resolver bind the project's voice into
	// every AI translation the worker runs, and the terms store resolver supplies
	// the per-locale terminology. All optional — a failure here (or an
	// unbound project) degrades translations to bare, never blocks them.
	if bs, err := voicepg.NewPostgresVoiceStore(pgdb); err != nil {
		slog.Warn("voice store unavailable; translation jobs run without voice", "error", err)
	} else {
		translationDeps.VoiceStore = bs

		// Both stores came from this one PgDB, so one transaction covers both.
		// A push reconciles collections and writes voice profiles before its
		// content lands; binding them to the same transition is what stops a
		// failed push from leaving the workspace governed by a declaration
		// whose content never arrived.
		translationDeps.PushTransition = func(ctx context.Context, fn func(store.PushApplier, coreprofile.Store) error) error {
			return pgdb.Transition(ctx, func(tx storage.Runner) error {
				return fn(pgCS.Bind(tx), bs.Bind(tx))
			})
		}
	}
	if authStore != nil {
		translationDeps.WorkspaceDefault = &workerWorkspaceDefault{auth: authStore}
		// The platform is authoritative for review governance. A push can
		// carry approvals and sign-offs, and this is what lets the worker
		// ask the questions the review endpoint asks before writing them:
		// the pusher's review permission for the language, and the
		// workspace separation-of-duties policy. Without it a push that
		// carries a verdict fails rather than landing ungoverned.
		translationDeps.ReviewAuthority = auth.NewReviewAuthority(authStore)
	}
	translationDeps.TermsResolver = newWorkerTermsResolver(pgdb)

	// Product analytics (epic 018): the worker emits content_pushed after sync
	// push processing. Keyless deployments stay silent (nil tracker). Mirrors
	// the server's POSTHOG_API_KEY / POSTHOG_HOST configuration, including its
	// rejection of the provisioning placeholder — a token that reads as
	// configured builds a client whose every enqueue is dropped in silence.
	if phKey := os.Getenv("POSTHOG_API_KEY"); analytics.IsProjectAPIKey(phKey) {
		phClient, err := analytics.NewPostHogClient(phKey, os.Getenv("POSTHOG_HOST"))
		if err != nil {
			slog.Warn("failed to init PostHog client, analytics disabled", "error", err)
		} else {
			translationDeps.Tracker = phClient
			defer func() { _ = phClient.Close() }()
			slog.Info("PostHog analytics enabled")
		}
	}

	// Wire billing credit deduction + Stripe meter reporting into the worker
	// (Epic 004). Without this, async auto-translate records ai_usage and
	// enforces the token abuse cap but never deducts credits or fires Stripe
	// meters. Only platform-key jobs are metered (resolved.Source); BYO-key jobs
	// burn no credits. Degrades gracefully in self-hosted mode where the billing
	// store can't be built — the worker just skips credit deduction (mirrors the
	// server's nil-BillingHooks behavior).
	if hooks := buildWorkerBillingHooks(pgdb); hooks != nil {
		translationDeps.BillingHooks = hooks
	}

	// Configure blob store for async sync push processing (Bowrain AD-009). The
	// selection mirrors the server (via the shared blobcfg contract) so the two
	// can never read the same push from different backends.
	var blobStore corestorage.BlobStore
	if blobcfg.S3Configured() {
		if bs, err := blobcfg.NewS3FromEnv(ctx); err != nil {
			slog.Warn("failed to create S3 blob store for push processing", "error", err)
		} else {
			blobStore = bs
			slog.Info("using S3 blob storage for push processing", "bucket", os.Getenv("S3_BLOB_BUCKET"))
		}
	}
	if blobStore == nil {
		// Accept the server's BLOB_STORAGE_LOCAL_DIR too, so a shared-volume
		// deployment can't point the two at different directories.
		localDir := envOrDefault("BLOB_STORAGE_LOCAL_DIR", envOrDefault("LOCAL_BLOB_DIR", "/tmp/bowrain-blobs"))
		if bs, err := bloblocal.New(localDir); err == nil {
			blobStore = bs
		}
		slog.Info("using local blob storage for push processing")
	}
	translationDeps.BlobStore = blobStore

	// Configure event bus for publishing EventPushCompleted after sync push (Bowrain AD-009).
	// Selection mirrors the server (createEventBus): Redis Streams when opted in.
	redisURL := os.Getenv("BOWRAIN_REDIS_URL")
	if os.Getenv("BOWRAIN_EVENT_BACKEND") == "redis" && redisURL != "" {
		bus, err := bowevent.NewRedisEventBus(redisURL, os.Getenv("BOWRAIN_REDIS_PASSWORD"))
		if err != nil {
			slog.Warn("failed to create Redis event bus for worker", "error", err)
		} else {
			translationDeps.EventBus = bus
			slog.Info("worker event bus configured", "backend", "redis_streams")
		}
	}

	// The same Redis holds the server's sync diff cache (Bowrain AD-009). The
	// worker is the process that changes stored source content, so it carries
	// the invalidation half of that cache: without it, a push applied here
	// leaves the server negotiating diffs against pre-apply hashes for the
	// cache TTL, and the next push's changed blocks are silently skipped.
	if redisURL != "" {
		if redisOpts, err := redis.ParseURL(redisURL); err == nil {
			if pw := os.Getenv("BOWRAIN_REDIS_PASSWORD"); pw != "" {
				redisOpts.Password = pw
			}
			translationDeps.SyncCache = bowsync.NewRedisHashCache(redis.NewClient(redisOpts), 30*time.Minute)
			slog.Info("worker sync hash cache configured", "backend", "redis")
		}
	}

	// Configure the platform translation provider — used by jobs that carry no
	// per-workspace credential (the built-in auto-translate-on-push automation).
	//
	//   - BOWRAIN_PLATFORM_PROVIDER (e.g. "gemini", "openai", "anthropic",
	//     "ollama", "demo") selects a generic upstream for self-hosted / local
	//     dev with a plain API key. The key comes from BOWRAIN_PLATFORM_API_KEY
	//     or a provider-specific env var (e.g. GEMINI_API_KEY). This takes
	//     precedence over the Azure path.
	//   - BOWRAIN_OPENAI_ENDPOINT selects Azure OpenAI via managed identity
	//     (the hosted Bowrain cloud).
	switch {
	case platformProvider != "":
		apiKey := os.Getenv("BOWRAIN_PLATFORM_API_KEY")
		if apiKey == "" {
			apiKey = platformAPIKeyFromEnv(platformProvider)
		}
		translationDeps.Platform = &jobs.PlatformProviderConfig{
			Provider: platformProvider,
			APIKey:   apiKey,
			Model:    os.Getenv("BOWRAIN_PLATFORM_MODEL"),
			BaseURL:  os.Getenv("BOWRAIN_PLATFORM_BASE_URL"),
		}
		slog.Info("platform translation provider configured",
			"provider", platformProvider, "model", os.Getenv("BOWRAIN_PLATFORM_MODEL"))
	default:
		slog.Warn("no platform translation provider configured; " +
			"auto-translate jobs will fail (set BOWRAIN_PLATFORM_PROVIDER + key)")
	}

	// Instance-wide platform config (ctrl-managed). The worker reads the AI
	// provider/model from this service at job time via a resolver, so an admin
	// switching provider/model in ctrl takes effect without a worker restart. The
	// service's bootstrap defaults mirror the BOWRAIN_PLATFORM_* env above, so an
	// un-provisioned instance resolves exactly as before. A distributed event bus
	// (Redis) delivers platform_config.changed so the worker reloads on change.
	var platformResolver jobs.PlatformResolver
	if pcStore, err := platformconfig.NewStore(pgdb); err != nil {
		slog.Warn("platform_config store unavailable; worker uses static env provider config", "error", err)
	} else {
		pcSvc := platformconfig.NewService(pcStore, platformconfig.Defaults{
			AIProvider:     platformProvider,
			AIDefaultModel: os.Getenv("BOWRAIN_PLATFORM_MODEL"),
			AIBaseURL:      os.Getenv("BOWRAIN_PLATFORM_BASE_URL"),
			SignupsOpen:    true,
			DefaultPlan:    string(billing.PlanPro),
			TrialDays:      billing.DefaultTrialDays,
		})
		if err := pcSvc.Refresh(ctx); err != nil {
			slog.Warn("platform_config: initial worker load failed (serving env defaults)", "error", err)
		}
		// Circuit-breaker thresholds. The worker keeps its own registry, so it
		// trips on what it observes rather than inheriting the server's view —
		// the two run different call volumes against the same upstreams.
		resilience.Default().Configure(pcSvc.ResilienceOverrides())
		// The shared auth-store handle (opened above) lets the resolver apply a
		// workspace's chosen platform model (customer model choice) at job time,
		// so batch/auto translation resolves the same model as the interactive
		// editor. A nil store just serves the platform default.
		wsGetter := authStore
		platformResolver = func(ctx context.Context, wsID string) *jobs.PlatformProviderConfig {
			cfg := platformConfigFromService(pcSvc)
			// Skip the workspace lookup unless model choice is on and we have a
			// workspace to look up — the common path stays a pure in-memory read.
			if cfg == nil || wsID == "" || wsGetter == nil || !pcSvc.AICustomerChoice() {
				return cfg
			}
			if w, err := wsGetter.GetWorkspace(ctx, wsID); err == nil && w != nil {
				cfg.Model = pcSvc.ResolveWorkspaceModel(w.PreferredModel)
				// Mark an applied workspace preference as pinned: a workspace's
				// own model choice is never overridden by a measured model
				// recommendation (EV-4).
				cfg.ModelPinned = w.PreferredModel != "" && cfg.Model == w.PreferredModel
			}
			return cfg
		}
		translationDeps.PlatformResolver = platformResolver

		// Measured steerability (model recommendation sweeps): the sweep store
		// persists per-(project, locale, model) measurements, the settings
		// expose the ctrl-managed model_sweeps.enabled gate + candidate model
		// list, and the recommender lets platform model resolution prefer a
		// fresh measured winner when the workspace has not pinned a model. All
		// paths are gated on model_sweeps.enabled (default OFF).
		if sweepStore, err := jobs.NewModelSweepStore(pgdb); err != nil {
			slog.Warn("model-sweep store unavailable; sweeps disabled on this worker", "error", err)
		} else {
			translationDeps.SweepStore = sweepStore
			translationDeps.SweepSettings = pcSvc
			translationDeps.ModelRecommender = &jobs.SweepModelRecommender{
				Store:    sweepStore,
				Settings: pcSvc,
			}
		}

		// Fan-out reload: every worker reacts to a config change from ctrl.
		// Fine-to-miss: this only refreshes a cache loaded fresh at startup and
		// re-read on the next change event — no state advances here.
		if bus := translationDeps.EventBus; bus != nil {
			bus.Subscribe(platev.EventPlatformConfigChanged, func(platev.Event) {
				if err := pcSvc.Refresh(ctx); err != nil {
					slog.Warn("platform_config: worker reload after change event failed", "error", err)
				} else {
					resilience.Default().Configure(pcSvc.ResilienceOverrides())
					slog.Info("platform_config: worker reloaded settings", "provider", pcSvc.AIProvider(), "model", pcSvc.AIDefaultModel())
				}
			})
		}
	}

	g, ctx := errgroup.WithContext(ctx)

	// Health endpoint for liveness/readiness probes.
	healthPort := envOrDefault("BOWRAIN_HEALTH_PORT", "8081")
	g.Go(func() error {
		mux := http.NewServeMux()
		// Prometheus metrics (job outcomes, durations, in-flight, DB pool, Go
		// runtime). Gated the same way as the server's /metrics: a bearer token
		// when BOWRAIN_METRICS_TOKEN is set, else loopback/private source IPs
		// only.
		mux.Handle("/metrics", observe.MetricsAccessMiddlewareStd(
			os.Getenv("BOWRAIN_METRICS_TOKEN"), promhttp.Handler()))
		// Liveness: the process is up. Cheap, dependency-free — used by the
		// container/orchestrator to decide whether to restart the task.
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		// Readiness: the worker's dependencies are reachable. Mirrors the
		// server's /api/v1/ready so the ctrl Health page can show a real
		// per-component status for the worker, not just "process alive".
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			type comp struct {
				Status string `json:"status"`
				Error  string `json:"error,omitempty"`
			}
			components := map[string]comp{}
			overall := "ready"

			// Database.
			dbCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := pgdb.DB.PingContext(dbCtx); err != nil {
				components["database"] = comp{Status: "down", Error: err.Error()}
				overall = "unhealthy"
			} else {
				components["database"] = comp{Status: "up"}
			}

			// Job queue.
			if translationQueue.Healthy() {
				components["queue"] = comp{Status: "up"}
			} else {
				components["queue"] = comp{Status: "down"}
				if overall == "ready" {
					overall = "degraded"
				}
			}

			// Blob store (presence — the worker cannot run extraction without it).
			if blobStore != nil {
				components["blob"] = comp{Status: "up"}
			} else {
				components["blob"] = comp{Status: "unconfigured"}
			}

			w.Header().Set("Content-Type", "application/json")
			code := http.StatusOK
			if overall == "unhealthy" {
				code = http.StatusServiceUnavailable
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":     overall,
				"components": components,
			})
		})
		srv := &http.Server{Addr: ":" + healthPort, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			srv.Close()
		}()
		slog.Info("health endpoint listening", "port", healthPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("health server: %w", err)
		}
		return nil
	})

	// Translation worker.
	g.Go(func() error {
		slog.Info("starting translation worker")
		return jobs.RunWorkerWithDeps(ctx, translationDeps)
	})

	// Stale-job sweeper (epic 003 item 7): a worker that crashes between
	// ClaimJob (queued→processing) and completion leaves the row stuck in
	// 'processing' forever with no NAK to trigger redelivery. The sweeper
	// periodically resets such rows to 'queued' (with attempt tracking) and
	// re-enqueues them, or fails them once retries are exhausted. It shares the
	// translation queue and job store.
	staleSweeper := jobs.NewStaleJobSweeper(pgJS, translationQueue, 0, 0, 0)
	g.Go(func() error {
		slog.Info("starting stale-job sweeper")
		return staleSweeper.Run(ctx)
	})

	// Extraction worker (auto-extract-on-push automation, AD-013/AD-015).
	pgES, err := jobs.NewExtractionJobStore(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL extraction job store: %w", err)
	}
	extractionDeps := &jobs.ExtractionWorkerDeps{
		ExtractionJobStore: pgES,
		ContentStore:       cs,
		CredStore:          credStore,
		Queue:              extractionQueue,
		ReviewQueueCreator: &jobs.ReviewQueueStoreAdapter{Store: bstore.NewReviewQueueStore(pgdb.DB)},
		// KnownTermsLoader and NERProvider stay nil here: known-term
		// filtering reads workspace terms stores that live on the server box,
		// and NER needs per-deployment Azure config. Both are documented
		// optional — extraction degrades to the plain LLM pass.
		Platform:         translationDeps.Platform,
		PlatformResolver: platformResolver,
		// The same bus the translation worker publishes on, so a failed
		// extraction reaches the summons by the same route as a failed
		// translation. Nil without Redis, as it is over there.
		EventBus: translationDeps.EventBus,
		LogFunc: func(stepID, level, message string, data map[string]string) {
			slog.Info("extraction: "+message, "step_id", stepID, "level", level)
		},
	}
	g.Go(func() error {
		slog.Info("starting extraction worker")
		return jobs.RunExtractionWorker(ctx, extractionDeps)
	})

	// Extraction stale-job sweeper: the same crash backstop the translation and
	// brand-scan workers have. Without it an extraction job whose worker died
	// between claim and completion sits in 'processing' with nothing scanning
	// for it, and the push it belongs to reports in-progress until the
	// completion tracker's timeout.
	extractionSweeper := jobs.NewStaleJobSweeper(pgES, extractionQueue, 0, 0, 0)
	g.Go(func() error {
		slog.Info("starting extraction stale-job sweeper")
		return extractionSweeper.Run(ctx)
	})

	// Brand-scan worker (AI brand onboarding, epic 016). Scans run on the
	// platform provider only and deduct credits via the same billing hooks as
	// translation, so the wiring mirrors translationDeps: nil hooks (no
	// STRIPE_SECRET_KEY) degrade cleanly to unmetered scans.
	pgBSS, err := jobs.NewContextScanJobStore(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL brand-scan job store: %w", err)
	}
	contextScanDeps := &jobs.ContextScanWorkerDeps{
		Store:            pgBSS,
		Queue:            contextScanQueue,
		BlobStore:        blobStore,
		Platform:         translationDeps.Platform,
		PlatformResolver: platformResolver,
		BillingHooks:     translationDeps.BillingHooks,
		QuotaStore:       pgQS,
		LogFunc: func(stepID, level, message string, data map[string]string) {
			slog.Info("brand-scan: "+message, "job_id", stepID, "level", level)
		},
	}
	g.Go(func() error {
		slog.Info("starting brand-scan worker")
		return jobs.RunContextScanWorker(ctx, contextScanDeps)
	})

	// Brand-scan stale-job sweeper: same crash backstop as the translation
	// sweeper (a worker dying between claim and completion leaves the row in
	// 'processing' with no pending redelivery).
	contextScanSweeper := jobs.NewStaleJobSweeper(pgBSS, contextScanQueue, 0, 0, 0)
	g.Go(func() error {
		slog.Info("starting brand-scan stale-job sweeper")
		return contextScanSweeper.Run(ctx)
	})

	// Brand-scan upload retention: uploaded source envelopes (customer brand
	// material) are deleted once their job has been terminal for the retention
	// window. The window is what keeps Regenerate — which reuses the original
	// upload keys — working across a review session.
	if blobStore != nil {
		contextScanUploadSweeper := jobs.NewContextScanUploadSweeper(pgBSS, blobStore, 0, 0)
		g.Go(func() error {
			slog.Info("starting brand-scan upload sweeper")
			return contextScanUploadSweeper.Run(ctx)
		})
	}

	// Trial expiry: the 14-day Pro trial is local and card-free, so no Stripe
	// event ever ends it — this sweep is the only thing that does. Without it
	// every signup keeps Pro limits and Pro weekly credits indefinitely.
	if sweeper := buildTrialSweeper(pgdb); sweeper != nil {
		g.Go(func() error {
			slog.Info("starting trial-expiry sweeper")
			return sweeper.Run(ctx)
		})
	}

	slog.Info("starting bowrain worker")
	if err := g.Wait(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// buildWorkerBillingHooks constructs the worker's billing usage hooks: the
// Postgres-backed billing store for credit deduction plus the Stripe client for
// meter reporting. It is gated on STRIPE_SECRET_KEY — the signal that this is a
// billed, metered SaaS deployment. Without Stripe there are no subscriptions and
// therefore no weekly credit allocations, so every platform job's DeductCredits
// would return "no allocation for workspace" and log once per chunk on an
// otherwise-unbilled self-hosted deployment (Epic 004). Returning nil in that
// case makes the worker skip credit deduction cleanly (mirrors the server's
// nil-BillingHooks behavior). Migrations are namespaced + idempotent, so
// re-running them here (the server also runs them) is safe.
func buildWorkerBillingHooks(pgdb *storage.PgDB) *billing.UsageHooks {
	// A placeholder key (Terraform seeds the SSM parameter as "CHANGEME", epic
	// 002) is not a key: treating it as one would enable credit deduction and fire
	// every meter event at a Stripe account that rejects them.
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if !billing.IsStripeSecretKey(stripeKey) {
		slog.Info("worker billing disabled: no usable STRIPE_SECRET_KEY (self-hosted / unbilled / unprovisioned deployment)")
		return nil
	}
	billingStore, err := billing.NewPgBillingStore(pgdb)
	if err != nil {
		slog.Warn("worker billing disabled: failed to init billing store", "error", err)
		return nil
	}
	hooks := &billing.UsageHooks{
		Store:  billingStore,
		Stripe: billing.NewStripeClient(stripeKey),
	}
	slog.Info("worker billing credit deduction + Stripe meter reporting enabled")
	return hooks
}

// newWorkerMemoryResolver returns a per-workspace content memory resolver backed by the same
// PostgreSQL content memory the server uses (NewPostgresStoreFromDB). It caches one Store per
// workspace slug so a convergence pass does not rebuild the store per job. A
// resolution failure yields a nil store so the job degrades to AI-only rather
// than failing.
func newWorkerMemoryResolver(pgdb *storage.PgDB) jobs.MemoryResolver {
	var cache sync.Map // workspaceSlug -> fwmemory.Store
	return jobs.MemoryResolverFunc(func(workspaceSlug string) (fwmemory.Store, error) {
		if v, ok := cache.Load(workspaceSlug); ok {
			return v.(fwmemory.Store), nil
		}
		tm, err := sqlmemory.NewPostgresStoreFromDB(pgdb, workspaceSlug)
		if err != nil {
			return nil, err
		}
		actual, _ := cache.LoadOrStore(workspaceSlug, fwmemory.Store(tm))
		return actual.(fwmemory.Store), nil
	})
}

// workerWorkspaceDefault resolves the workspace-level default voice
// profile from the workspace record — the base rung of the voicescope ladder.
// Mirrors the server's mcpWorkspaceDefaultAdapter.
type workerWorkspaceDefault struct{ auth auth.AuthStore }

func (a *workerWorkspaceDefault) WorkspaceVoiceProfileID(ctx context.Context, workspaceID string) (string, error) {
	if a.auth == nil || workspaceID == "" {
		return "", nil
	}
	ws, err := a.auth.GetWorkspace(ctx, workspaceID)
	if err != nil || ws == nil {
		return "", err
	}
	return ws.VoiceProfileID, nil
}

// newWorkerTermsResolver returns a per-workspace terms resolver backed by the
// same PostgreSQL terms the server uses (NewPostgresStoreFromDB), so a
// translation job's prompt reads the very terminology the editor and
// term-check enforce. It caches one store per workspace slug (mirrors
// newWorkerMemoryResolver); a resolution failure yields nil so the job degrades to
// a translation without terminology rather than failing.
func newWorkerTermsResolver(pgdb *storage.PgDB) jobs.TermsResolver {
	var cache sync.Map // workspaceSlug -> fwterms.Terminology
	return jobs.TermsResolverFunc(func(workspaceSlug string) (fwterms.Terminology, error) {
		if v, ok := cache.Load(workspaceSlug); ok {
			return v.(fwterms.Terminology), nil
		}
		tb, err := sqlterms.NewPostgresStoreFromDB(pgdb, workspaceSlug)
		if err != nil {
			return nil, err
		}
		actual, _ := cache.LoadOrStore(workspaceSlug, fwterms.Terminology(tb))
		return actual.(fwterms.Terminology), nil
	})
}

// buildTrialSweeper constructs the trial-expiry sweeper. Unlike the billing
// hooks, it is NOT gated on Stripe: the trial is granted locally at workspace
// creation on any Postgres deployment (billing.SetupTrial from the workspace
// handler), so on a deployment that never provisions Stripe the trial would still
// be handed out and would still never end. Whoever grants it must expire it.
//
// It needs only the billing store: ExpireTrials keeps the workspace plan cache in
// step with the downgrade atomically, so no separate auth-side syncer is wired.
func buildTrialSweeper(pgdb *storage.PgDB) *billing.TrialSweeper {
	billingStore, err := billing.NewPgBillingStore(pgdb)
	if err != nil {
		slog.Warn("trial sweeper disabled: failed to init billing store", "error", err)
		return nil
	}
	return billing.NewTrialSweeper(billingStore, 0)
}

// guardSecretsKey enforces that a multi-tenant Postgres deployment configures a
// secrets key. crypto.NewCipher("") is a nil, pass-through cipher, so an ABSENT
// key silently stores every tenant's AI provider key and connector secret in
// PLAINTEXT in Postgres (only an INVALID key aborts at startup). On a billed SaaS
// deployment (STRIPE_SECRET_KEY set) this is a hard startup failure; on a
// self-hosted Postgres deployment it is a loud error-level warning for operators
// who knowingly accept plaintext-at-rest. Non-Postgres (dev/SQLite) is exempt.
func guardSecretsKey(secretsKey, databaseURL string) error {
	if secretsKey != "" {
		return nil
	}
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return nil
	}
	if billing.IsStripeSecretKey(os.Getenv("STRIPE_SECRET_KEY")) {
		return errors.New("BOWRAIN_SECRETS_KEY is required on a billed multi-tenant deployment: " +
			"without it every workspace's AI provider key and connector secret is stored in plaintext in Postgres")
	}
	slog.Error("BOWRAIN_SECRETS_KEY is not set: workspace AI provider keys and connector secrets " +
		"will be stored UNENCRYPTED in Postgres; set a 32-byte base64 key before storing tenant secrets")
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// platformConfigFromService builds the current platform provider config from the
// ctrl-managed settings service, mirroring the startup BOWRAIN_PLATFORM_* switch:
// a configured provider (e.g. "bedrock") takes the generic path with its env key.
// Provider, model and base URL come from the service (so a ctrl change is picked
// up live); the API key remains an env-sourced secret. Returns nil when no
// provider is configured.
func platformConfigFromService(svc *platformconfig.Service) *jobs.PlatformProviderConfig {
	if provider := svc.AIProvider(); provider != "" {
		apiKey := os.Getenv("BOWRAIN_PLATFORM_API_KEY")
		if apiKey == "" {
			apiKey = platformAPIKeyFromEnv(provider)
		}
		return &jobs.PlatformProviderConfig{
			Provider: provider,
			APIKey:   apiKey,
			Model:    svc.AIDefaultModel(),
			BaseURL:  svc.AIBaseURL(),
		}
	}
	return nil
}

// platformAPIKeyFromEnv resolves a platform-provider API key from the
// provider-specific environment variables, so operators can supply e.g.
// GEMINI_API_KEY directly without also setting BOWRAIN_PLATFORM_API_KEY.
// Keyless providers (ollama, demo) return "".
func platformAPIKeyFromEnv(provider string) string {
	var names []string
	switch strings.ToLower(provider) {
	case "gemini":
		names = []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}
	case "openai":
		names = []string{"OPENAI_API_KEY"}
	case "anthropic":
		names = []string{"ANTHROPIC_API_KEY"}
	case "azureopenai":
		names = []string{"AZURE_OPENAI_API_KEY"}
	}
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}
