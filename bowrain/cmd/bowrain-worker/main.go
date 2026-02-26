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
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/bowrain/storage"
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

	serviceBusConn := os.Getenv("BOWRAIN_SERVICE_BUS_CONNECTION")
	credentialsPath := os.Getenv("BOWRAIN_CREDENTIALS_PATH")
	if credentialsPath == "" {
		credentialsPath = credentials.DefaultPath()
	}

	// Open stores.
	var cs store.ContentStore
	var jobStore jobs.JobStore

	if strings.HasPrefix(dbURL, "postgres://") || strings.HasPrefix(dbURL, "postgresql://") {
		pgdb, err := storage.OpenPostgres(dbURL)
		if err != nil {
			log.Fatalf("Worker: open PostgreSQL: %v", err)
		}
		defer pgdb.Close()

		pgCS, err := bstore.NewPostgresStore(dbURL)
		if err != nil {
			log.Fatalf("Worker: open PostgreSQL content store: %v", err)
		}
		defer pgCS.Close()
		cs = pgCS

		pgJS, err := jobs.NewPgJobStore(pgdb)
		if err != nil {
			log.Fatalf("Worker: open PostgreSQL job store: %v", err)
		}
		jobStore = pgJS
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
	if serviceBusConn != "" {
		var err error
		queue, err = jobs.NewServiceBusQueue(serviceBusConn, "translation-jobs")
		if err != nil {
			log.Fatalf("Worker: connect to Service Bus: %v", err)
		}
	} else {
		queue = jobs.NewChannelQueue(64)
	}
	defer queue.Close()

	credStore := credentials.NewStore(credentialsPath)

	log.Println("Starting bowrain worker...")
	if err := jobs.RunWorker(ctx, jobStore, cs, credStore, queue); err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}
