package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/neokapi/neokapi/bowrain/agent"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbURL := os.Getenv("BOWRAIN_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("BOWRAIN_DATABASE_URL is required (must be a postgres:// URL)")
	}
	if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
		log.Fatal("BOWRAIN_DATABASE_URL must start with postgres:// or postgresql://")
	}

	dbAuth := os.Getenv("BOWRAIN_DATABASE_AUTH")
	azureClientID := os.Getenv("AZURE_CLIENT_ID")

	serviceBusConn := os.Getenv("BOWRAIN_SERVICE_BUS_CONNECTION")
	natsURL := os.Getenv("BOWRAIN_NATS_URL")
	openaiEndpoint := os.Getenv("BOWRAIN_OPENAI_ENDPOINT")
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
		log.Fatalf("Worker: open PostgreSQL: %v", err)
	}
	defer pgdb.Close()

	pgCS, err := bstore.NewPostgresStoreFromDB(pgdb)
	if err != nil {
		log.Fatalf("Worker: open PostgreSQL content store: %v", err)
	}
	var cs store.ContentStore = pgCS

	pgJS, err := jobs.NewPgJobStore(pgdb)
	if err != nil {
		log.Fatalf("Worker: open PostgreSQL job store: %v", err)
	}

	pgQS, err := jobs.NewPgQuotaStore(pgdb)
	if err != nil {
		log.Fatalf("Worker: open PostgreSQL quota store: %v", err)
	}

	// Set up translation job queue.
	var translationQueue jobs.Queue
	switch {
	case serviceBusConn != "":
		translationQueue, err = jobs.NewServiceBusQueue(serviceBusConn, "translation-jobs")
		if err != nil {
			log.Fatalf("Worker: connect to Service Bus (translation): %v", err)
		}
	case natsURL != "":
		translationQueue, err = jobs.NewNATSQueue(natsURL)
		if err != nil {
			log.Fatalf("Worker: connect to NATS: %v", err)
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

	// Configure platform Azure OpenAI if endpoint is set.
	if openaiEndpoint != "" {
		translationDeps.Platform = &jobs.PlatformProviderConfig{
			Endpoint: openaiEndpoint,
			ClientID: azureClientID,
		}
	}

	g, ctx := errgroup.WithContext(ctx)

	// Translation worker.
	g.Go(func() error {
		log.Println("Starting translation worker...")
		return jobs.RunWorkerWithDeps(ctx, translationDeps)
	})

	// Agent worker (optional — runs when BOWRAIN_AGENT_RUNTIME=aca is set).
	if agentRuntime := os.Getenv("BOWRAIN_AGENT_RUNTIME"); agentRuntime == "aca" {
		agentDeps, cleanup, err := buildAgentWorkerDeps(ctx, pgdb, serviceBusConn, azureClientID)
		if err != nil {
			log.Fatalf("Worker: init agent worker: %v", err)
		}
		defer cleanup()

		g.Go(func() error {
			return jobs.RunAgentWorker(ctx, agentDeps)
		})
	}

	log.Println("Starting bowrain worker...")
	if err := g.Wait(); err != nil && ctx.Err() == nil {
		log.Fatalf("Worker failed: %v", err)
	}
}

// buildAgentWorkerDeps sets up the agent worker dependencies.
func buildAgentWorkerDeps(ctx context.Context, pgdb *storage.PgDB, serviceBusConn, azureClientID string) (*jobs.AgentWorkerDeps, func(), error) {
	// Agent store (conversations + messages).
	agentStore, err := agent.NewPostgresStore(pgdb)
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
		return nil, nil, fmt.Errorf("BOWRAIN_REDIS_URL is required for agent worker")
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
		Runtime:       runtime,
		MCPEndpoint:   os.Getenv("BOWRAIN_AGENT_MCP_ENDPOINT"),
		BravoImage:    envOrDefault("BOWRAIN_AGENT_IMAGE", "ghcr.io/neokapi/bravo-agent:latest"),
		ModelProvider: os.Getenv("BOWRAIN_AGENT_MODEL_PROVIDER"),
		ModelName:     os.Getenv("BOWRAIN_AGENT_MODEL_NAME"),
		ModelAPIBase:  os.Getenv("BOWRAIN_AGENT_MODEL_API_BASE"),
		ModelAPIKey:   os.Getenv("BOWRAIN_AGENT_MODEL_API_KEY"),
	})

	tokenStore := service.NewAgentTokenStore()

	log.Printf("Agent pool initialized (runtime=aca)")

	cleanup := func() {
		agentQueue.Close()
		redisClient.Close()
		pool.StopAll(context.Background())
	}

	return &jobs.AgentWorkerDeps{
		Queue:      agentQueue,
		AgentStore: agentStore,
		Pool:       pool,
		TokenStore: tokenStore,
		PubSub:     pubsub,
	}, cleanup, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
