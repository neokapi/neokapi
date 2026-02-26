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
	"github.com/gokapi/gokapi/bowrain/server"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/bowrain/storage"
	"github.com/gokapi/gokapi/platform/store"
)

// runWorker starts the async job processing loop. It connects to the
// database and message queue, then processes translation jobs until
// interrupted by SIGINT/SIGTERM.
func runWorker(cfg server.ServerConfig) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbURL := cfg.DatabaseURL
	if dbURL == "" && cfg.StorePath != "" {
		dbURL = "sqlite:///" + cfg.StorePath
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
		storePath := cfg.StorePath
		if storePath == "" {
			storePath = strings.TrimPrefix(dbURL, "sqlite:///")
		}
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
	if cfg.ServiceBusConnection != "" {
		var err error
		queue, err = jobs.NewServiceBusQueue(cfg.ServiceBusConnection, "translation-jobs")
		if err != nil {
			log.Fatalf("Worker: connect to Service Bus: %v", err)
		}
	} else {
		queue = jobs.NewChannelQueue(64)
	}
	defer queue.Close()

	credStore := credentials.NewStore(credentials.DefaultPath())

	log.Println("Starting bowrain worker...")
	if err := jobs.RunWorker(ctx, jobStore, cs, credStore, queue); err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}
