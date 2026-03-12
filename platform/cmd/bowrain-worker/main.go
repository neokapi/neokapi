package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gokapi/gokapi/bowrain/credentials"
	"github.com/gokapi/gokapi/bowrain/jobs"
	"github.com/gokapi/gokapi/bowrain/storage"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/platform/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbURL := os.Getenv("BOWRAIN_DATABASE_URL")
	if dbURL == "" {
		if sp := os.Getenv("BOWRAIN_STORE"); sp != "" {
			dbURL = "sqlite:///" + sp
		}
	}
	if dbURL == "" {
		log.Fatal("BOWRAIN_DATABASE_URL or BOWRAIN_STORE is required")
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

	// Open stores.
	var cs store.ContentStore
	var jobStore jobs.JobStore
	var quotaStore jobs.QuotaStore

	if strings.HasPrefix(dbURL, "postgres://") || strings.HasPrefix(dbURL, "postgresql://") {
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
		cs = pgCS

		pgJS, err := jobs.NewPgJobStore(pgdb)
		if err != nil {
			log.Fatalf("Worker: open PostgreSQL job store: %v", err)
		}
		jobStore = pgJS

		pgQS, err := jobs.NewPgQuotaStore(pgdb)
		if err != nil {
			log.Fatalf("Worker: open PostgreSQL quota store: %v", err)
		}
		quotaStore = pgQS
	} else {
		storePath := strings.TrimPrefix(dbURL, "sqlite:///")

		sqCS, err := bstore.NewSQLiteStore(storePath)
		if err != nil {
			log.Fatalf("Worker: open SQLite content store: %v", err)
		}
		defer sqCS.Close()
		cs = sqCS

		db, err := storage.Open(storePath + ".jobs")
		if err != nil {
			log.Fatalf("Worker: open SQLite job store: %v", err)
		}
		defer db.Close()

		sqJS, err := jobs.NewSQLiteJobStore(db)
		if err != nil {
			log.Fatalf("Worker: init SQLite job store: %v", err)
		}
		jobStore = sqJS
	}

	// Set up message queue.
	var queue jobs.Queue
	switch {
	case serviceBusConn != "":
		var err error
		queue, err = jobs.NewServiceBusQueue(serviceBusConn, "translation-jobs")
		if err != nil {
			log.Fatalf("Worker: connect to Service Bus: %v", err)
		}
	case natsURL != "":
		var err error
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
		JobStore:     jobStore,
		ContentStore: cs,
		CredStore:    credStore,
		Queue:        queue,
		QuotaStore:   quotaStore,
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
