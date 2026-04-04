package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	agentictesting "github.com/neokapi/neokapi/bowrain/agentic-testing"
	"github.com/neokapi/neokapi/bowrain/agentic-testing/agenticmcp"
	"github.com/neokapi/neokapi/bowrain/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config{
		Port: 8080,
	}

	if v := os.Getenv("AGENTIC_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	cfg.DatabaseURL = os.Getenv("AGENTIC_DATABASE_URL")
	cfg.DatabaseAuth = os.Getenv("AGENTIC_DATABASE_AUTH")
	cfg.RedisURL = os.Getenv("AGENTIC_REDIS_URL")
	cfg.RedisPassword = os.Getenv("AGENTIC_REDIS_PASSWORD")
	cfg.JWTSecret = os.Getenv("AGENTIC_JWT_SECRET")
	cfg.FleetRepoURL = os.Getenv("AGENTIC_FLEET_REPO_URL")
	cfg.FleetRepoToken = os.Getenv("AGENTIC_FLEET_REPO_TOKEN")
	cfg.GitHubIssuesRepo = os.Getenv("AGENTIC_GITHUB_ISSUES_REPO")
	cfg.GitHubIssuesToken = os.Getenv("AGENTIC_GITHUB_ISSUES_TOKEN")
	cfg.BowrainAPIURL = os.Getenv("AGENTIC_BOWRAIN_API_URL")
	cfg.BowrainAPIToken = os.Getenv("AGENTIC_BOWRAIN_API_TOKEN")

	// Build MCP server options.
	mcpCfg := agenticmcp.Config{
		JWTSecret: cfg.JWTSecret,
	}
	var mcpOpts []agenticmcp.Option

	var fleetRepo *agenticmcp.GitFleetRepo
	if cfg.FleetRepoURL != "" {
		fleetRepo = &agenticmcp.GitFleetRepo{
			RepoURL:      cfg.FleetRepoURL,
			Token:        cfg.FleetRepoToken,
			CommitAuthor: "coordinator",
		}
		mcpOpts = append(mcpOpts, agenticmcp.WithFleetRepo(fleetRepo))
		log.Printf("Fleet repo configured: %s", cfg.FleetRepoURL)
	}

	// Wire release walker when fleet repo and bowrain API are both configured.
	// The walker clones forks on demand from plan.yaml — no local ForkDir needed.
	if fleetRepo != nil && cfg.BowrainAPIURL != "" {
		githubToken := cfg.FleetRepoToken // Reuse fleet repo token for fork cloning.
		bowrainClient := &agentictesting.BowrainClient{
			BaseURL: cfg.BowrainAPIURL,
			Token:   cfg.BowrainAPIToken,
		}
		walker := &agenticmcp.GitReleaseWalker{
			Fleet:       fleetRepo,
			Bowrain:     bowrainClient,
			GitHubToken: githubToken,
			CacheDir:    os.Getenv("AGENTIC_FORK_CACHE_DIR"), // Optional: persistent cache across restarts
		}
		mcpOpts = append(mcpOpts, agenticmcp.WithReleaseWalker(walker))
		log.Printf("Release walker configured (forks cloned on demand)")
	}

	if cfg.GitHubIssuesRepo != "" {
		token := cfg.GitHubIssuesToken
		if token == "" {
			token = cfg.FleetRepoToken
		}
		if owner, repo, ok := strings.Cut(cfg.GitHubIssuesRepo, "/"); ok {
			mcpOpts = append(mcpOpts, agenticmcp.WithIssueTracker(&agenticmcp.GitHubIssueTracker{
				Owner: owner,
				Repo:  repo,
				Token: token,
			}))
			log.Printf("Issue tracker configured: %s", cfg.GitHubIssuesRepo)
		}
	}

	// Wire execution store + Redis subscriber.
	var pgDB *storage.PgDB
	if cfg.DatabaseURL != "" {
		var err error
		if cfg.DatabaseAuth == "azure" {
			pgDB, err = storage.OpenPostgresAzure(cfg.DatabaseURL, os.Getenv("AZURE_CLIENT_ID"))
		} else {
			pgDB, err = storage.OpenPostgres(cfg.DatabaseURL)
		}
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		defer pgDB.Close()
	}

	if cfg.RedisURL != "" && pgDB != nil {
		execStore, err := agenticmcp.NewPostgresExecutionStore(pgDB)
		if err != nil {
			log.Printf("WARNING: failed to init execution store: %v", err)
		} else {
			eventHub := agenticmcp.NewEventHub()
			mcpOpts = append(mcpOpts,
				agenticmcp.WithExecutionStore(execStore),
				agenticmcp.WithEventHub(eventHub),
			)
			execSub, err := agenticmcp.NewExecutionSubscriber(cfg.RedisURL, cfg.RedisPassword, execStore)
			if err != nil {
				log.Printf("WARNING: failed to init execution subscriber: %v", err)
			} else {
				execSub.SetEventHub(eventHub)
				execSub.Start(context.Background())
				log.Printf("Execution subscriber active (Redis -> PostgreSQL + WebSocket)")
			}
		}
	}

	mcpServer, err := agenticmcp.NewServer(mcpCfg, mcpOpts...)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	mux := http.NewServeMux()

	// Agentic REST endpoints.
	registerHandlers(mux, mcpServer)

	// Bowrain data wrapper endpoints.
	if cfg.BowrainAPIURL != "" && cfg.BowrainAPIToken != "" {
		registerBowrainHandlers(mux, cfg)
		log.Printf("Bowrain API wrapper configured: %s", cfg.BowrainAPIURL)
	}

	// MCP endpoint at /mcp/.
	mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpServer.Handler()))

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Agentic Testing server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

type config struct {
	Port              int
	DatabaseURL       string
	DatabaseAuth      string
	RedisURL          string
	RedisPassword     string
	JWTSecret         string
	FleetRepoURL      string
	FleetRepoToken    string
	GitHubIssuesRepo  string
	GitHubIssuesToken string
	BowrainAPIURL     string
	BowrainAPIToken   string
}
