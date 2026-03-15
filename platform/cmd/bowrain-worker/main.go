package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/platform/store"
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

	// Set up message queue.
	var queue jobs.Queue
	switch {
	case serviceBusConn != "":
		queue, err = jobs.NewServiceBusQueue(serviceBusConn, "translation-jobs")
		if err != nil {
			log.Fatalf("Worker: connect to Service Bus: %v", err)
		}
	case natsURL != "":
		queue, err = jobs.NewNATSQueue(natsURL)
		if err != nil {
			log.Fatalf("Worker: connect to NATS: %v", err)
		}
	default:
		queue = jobs.NewChannelQueue(64)
	}
	defer queue.Close()

	credStore := credentials.NewStore(credentialsPath)

	// Build worker dependencies.
	deps := &jobs.WorkerDeps{
		JobStore:     pgJS,
		ContentStore: cs,
		CredStore:    credStore,
		Queue:        queue,
		QuotaStore:   pgQS,
	}

	// Configure platform Azure OpenAI if endpoint is set.
	if openaiEndpoint != "" {
		deps.Platform = &jobs.PlatformProviderConfig{
			Endpoint: openaiEndpoint,
			ClientID: azureClientID,
		}
	}

	log.Println("Starting bowrain worker...")
	if err := jobs.RunWorkerWithDeps(ctx, deps); err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}
