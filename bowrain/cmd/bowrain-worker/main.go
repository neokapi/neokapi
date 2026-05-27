package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/neokapi/neokapi/bowrain/agent"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	bowevent "github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/storage"
	blobazure "github.com/neokapi/neokapi/bowrain/storage/azureblob"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

func main() {
	// Structured logging — bridges existing log.Printf calls through slog.
	observe.SetupLogger(
		os.Getenv("BOWRAIN_LOG_FORMAT"),
		os.Getenv("BOWRAIN_LOG_LEVEL"),
	)

	if err := run(); err != nil {
		slog.Error("worker failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("BOWRAIN_DATABASE_URL")
	if dbURL == "" {
		return errors.New("BOWRAIN_DATABASE_URL is required (must be a postgres:// URL)")
	}
	if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
		return errors.New("BOWRAIN_DATABASE_URL must start with postgres:// or postgresql://")
	}

	return runWorker(dbURL)
}

func runWorker(dbURL string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbAuth := os.Getenv("BOWRAIN_DATABASE_AUTH")
	azureClientID := os.Getenv("AZURE_CLIENT_ID")

	serviceBusConn := os.Getenv("BOWRAIN_SERVICE_BUS_CONNECTION")
	natsURL := os.Getenv("BOWRAIN_NATS_URL")
	openaiEndpoint := os.Getenv("BOWRAIN_OPENAI_ENDPOINT")
	platformProvider := os.Getenv("BOWRAIN_PLATFORM_PROVIDER")
	credentialsPath := os.Getenv("BOWRAIN_CREDENTIALS_PATH")
	if credentialsPath == "" {
		credentialsPath = credentials.DefaultPath()
	}

	// Open PostgreSQL stores.
	var pgdb *storage.PgDB
	var err error
	if dbAuth == "azure" {
		pgdb, err = storage.OpenPostgresAzure(dbURL, azureClientID)
	} else {
		pgdb, err = storage.OpenPostgres(dbURL)
	}
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer pgdb.Close()

	pgCS, err := bstore.NewPostgresStoreFromDB(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL content store: %w", err)
	}
	var cs store.ContentStore = pgCS

	pgJS, err := jobs.NewJobStore(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL job store: %w", err)
	}

	pgQS, err := jobs.NewQuotaStore(pgdb)
	if err != nil {
		return fmt.Errorf("open PostgreSQL quota store: %w", err)
	}

	// Set up translation job queue.
	var translationQueue jobs.Queue
	switch {
	case serviceBusConn != "":
		translationQueue, err = jobs.NewServiceBusQueue(serviceBusConn, "translation-jobs")
		if err != nil {
			return fmt.Errorf("connect to Service Bus (translation): %w", err)
		}
	case natsURL != "":
		translationQueue, err = jobs.NewNATSQueue(natsURL)
		if err != nil {
			return fmt.Errorf("connect to NATS: %w", err)
		}
	default:
		translationQueue = jobs.NewChannelQueue(64)
	}
	defer translationQueue.Close()

	credStore := credentials.NewStore(credentialsPath)

	// Build translation worker dependencies.
	translationDeps := &jobs.WorkerDeps{
		JobStore:     pgJS,
		ContentStore: cs,
		CredStore:    credStore,
		Queue:        translationQueue,
		QuotaStore:   pgQS,
	}

	// Configure blob store for async sync push processing (Bowrain AD-009).
	var blobStore corestorage.BlobStore
	if azureStorageURL := os.Getenv("AZURE_STORAGE_ACCOUNT_URL"); azureStorageURL != "" {
		container := envOrDefault("AZURE_STORAGE_CONTAINER", "bowrain-assets")
		if connStr := os.Getenv("AZURE_STORAGE_CONNECTION_STRING"); connStr != "" {
			bs, err := blobazure.NewWithConnectionString(connStr, container)
			if err == nil {
				blobStore = bs
				slog.Info("using Azure Blob Storage for push processing")
			}
		} else {
			bs, err := blobazure.New(azureStorageURL, container)
			if err == nil {
				blobStore = bs
				slog.Info("using Azure Blob Storage (managed identity) for push processing")
			}
		}
	}
	if blobStore == nil {
		localDir := envOrDefault("LOCAL_BLOB_DIR", "/tmp/bowrain-blobs")
		if bs, err := bloblocal.New(localDir); err == nil {
			blobStore = bs
		}
		slog.Info("using local blob storage for push processing")
	}
	translationDeps.BlobStore = blobStore

	// Configure event bus for publishing EventPushCompleted after sync push (Bowrain AD-009).
	if serviceBusConn != "" {
		bus, err := bowevent.NewServiceBusEventBus(serviceBusConn)
		if err != nil {
			slog.Warn("failed to create Service Bus event bus for worker", "error", err)
		} else {
			translationDeps.EventBus = bus
			slog.Info("worker event bus configured", "backend", "azure_service_bus")
		}
	} else if natsURL != "" {
		bus, err := bowevent.NewNATSEventBus(natsURL)
		if err != nil {
			slog.Warn("failed to create NATS event bus for worker", "error", err)
		} else {
			translationDeps.EventBus = bus
			slog.Info("worker event bus configured", "backend", "nats_jetstream")
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
	case openaiEndpoint != "":
		translationDeps.Platform = &jobs.PlatformProviderConfig{
			Endpoint: openaiEndpoint,
			ClientID: azureClientID,
		}
		slog.Info("platform translation provider configured", "provider", "azure-openai")
	default:
		slog.Warn("no platform translation provider configured; " +
			"auto-translate jobs will fail (set BOWRAIN_PLATFORM_PROVIDER + key, or BOWRAIN_OPENAI_ENDPOINT)")
	}

	g, ctx := errgroup.WithContext(ctx)

	// Health endpoint for liveness/readiness probes.
	healthPort := envOrDefault("BOWRAIN_HEALTH_PORT", "8081")
	g.Go(func() error {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		srv := &http.Server{Addr: ":" + healthPort, Handler: mux}
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

	// Agent worker (optional — runs when BOWRAIN_AGENT_RUNTIME=aca is set).
	if agentRuntime := os.Getenv("BOWRAIN_AGENT_RUNTIME"); agentRuntime == "aca" {
		agentDeps, cleanup, err := buildAgentWorkerDeps(ctx, pgdb, serviceBusConn, azureClientID)
		if err != nil {
			return fmt.Errorf("init agent worker: %w", err)
		}
		defer cleanup()

		g.Go(func() error {
			return jobs.RunAgentWorker(ctx, agentDeps)
		})

		// Cleanup idle agent containers periodically.
		g.Go(func() error {
			agentDeps.Pool.RunCleanupLoop(ctx)
			return nil
		})
	}

	slog.Info("starting bowrain worker")
	if err := g.Wait(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// buildAgentWorkerDeps sets up the agent worker dependencies.
func buildAgentWorkerDeps(ctx context.Context, pgdb *storage.PgDB, serviceBusConn, azureClientID string) (*jobs.AgentWorkerDeps, func(), error) {
	// Agent store (conversations + messages).
	agentStore, err := agent.NewStore(pgdb)
	if err != nil {
		return nil, nil, err
	}

	// Agent job queue (separate Service Bus queue).
	var agentQueue jobs.Queue
	if serviceBusConn != "" {
		agentQueue, err = jobs.NewServiceBusQueue(serviceBusConn, "bravo-jobs")
		if err != nil {
			return nil, nil, err
		}
	} else {
		agentQueue = jobs.NewChannelQueue(64)
	}

	// Redis pub/sub for SSE relay.
	redisURL := os.Getenv("BOWRAIN_REDIS_URL")
	redisPassword := os.Getenv("BOWRAIN_REDIS_PASSWORD")
	if redisURL == "" {
		return nil, nil, errors.New("BOWRAIN_REDIS_URL is required for agent worker")
	}
	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse redis URL: %w", err)
	}
	if redisPassword != "" {
		redisOpts.Password = redisPassword
	}
	redisClient := redis.NewClient(redisOpts)
	pubsub := service.NewAgentPubSub(redisClient)

	// ACA container runtime.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("azure credential: %w", err)
	}

	runtime := service.NewACARuntime(service.ACAConfig{
		Credential:     cred,
		SubscriptionID: os.Getenv("BOWRAIN_AGENT_ACA_SUBSCRIPTION"),
		ResourceGroup:  os.Getenv("BOWRAIN_AGENT_ACA_RESOURCE_GROUP"),
		EnvironmentID:  os.Getenv("BOWRAIN_AGENT_ACA_ENVIRONMENT_ID"),
		Location:       os.Getenv("BOWRAIN_AGENT_ACA_LOCATION"),
	})

	pool := service.NewAgentPool(service.AgentPoolConfig{
		Runtime:          runtime,
		MCPEndpoint:      os.Getenv("BOWRAIN_AGENT_MCP_ENDPOINT"),
		BravoImage:       envOrDefault("BOWRAIN_AGENT_IMAGE", "ghcr.io/neokapi/bravo-agent:latest"),
		ModelProvider:    os.Getenv("BOWRAIN_AGENT_MODEL_PROVIDER"),
		ModelName:        os.Getenv("BOWRAIN_AGENT_MODEL_NAME"),
		ModelAPIBase:     os.Getenv("BOWRAIN_AGENT_MODEL_API_BASE"),
		ModelAPIKey:      os.Getenv("BOWRAIN_AGENT_MODEL_API_KEY"),
		RegistryServer:   os.Getenv("BOWRAIN_AGENT_REGISTRY_SERVER"),
		RegistryUsername: os.Getenv("BOWRAIN_AGENT_REGISTRY_USERNAME"),
		RegistryPassword: os.Getenv("BOWRAIN_AGENT_REGISTRY_PASSWORD"),
	})

	slog.Info("agent pool initialized", "runtime", "aca")

	cleanup := func() {
		agentQueue.Close()
		redisClient.Close()
		pool.StopAll(context.Background())
	}

	jwtSecret := os.Getenv("BOWRAIN_JWT_SECRET")
	if jwtSecret == "" {
		return nil, nil, errors.New("BOWRAIN_JWT_SECRET is required for agent worker MCP auth")
	}

	return &jobs.AgentWorkerDeps{
		Queue:      agentQueue,
		AgentStore: agentStore,
		Pool:       pool,
		PubSub:     pubsub,
		JWTSecret:  jwtSecret,
	}, cleanup, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
