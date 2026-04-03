package server

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/redis/go-redis/v9"
)

// buildAgentPool creates an AgentPool with the configured container runtime.
// Used for direct mode (docker, aca). Returns nil if not applicable.
func (s *Server) buildAgentPool() *service.AgentPool {
	cfg := s.Config

	var runtime service.ContainerRuntime

	switch cfg.AgentRuntime {
	case "docker":
		runtime = service.NewDockerRuntime(service.DockerRuntimeConfig{
			Host:    cfg.AgentDockerHost,
			Network: cfg.AgentDockerNetwork,
		})

	case "aca":
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			log.Printf("WARNING: failed to create Azure credential for agent runtime: %v", err)
			return nil
		}
		runtime = service.NewACARuntime(service.ACAConfig{
			Credential:     cred,
			SubscriptionID: cfg.AgentACASubscription,
			ResourceGroup:  cfg.AgentACAResourceGroup,
			EnvironmentID:  cfg.AgentACAEnvironmentID,
			Location:       cfg.AgentACALocation,
		})

	default:
		return nil
	}

	mcpEndpoint := cfg.mcpEndpointForAgent()

	return service.NewAgentPool(service.AgentPoolConfig{
		Runtime:         runtime,
		MCPEndpoint:     mcpEndpoint,
		BravoImage:      cfg.AgentImage,
		MaxPerWorkspace: cfg.AgentMaxConcurrent,
		ModelProvider:   cfg.AgentModelProvider,
		ModelName:       cfg.AgentModelName,
		ModelAPIBase:    cfg.AgentModelAPIBase,
		ModelAPIKey:     cfg.AgentModelAPIKey,
	})
}

// setupAgentQueue configures queue-based agent orchestration.
// The API server enqueues jobs to Service Bus and subscribes to Redis pub/sub
// for SSE relay. The worker handles container lifecycle.
func (s *Server) setupAgentQueue(cfg ServerConfig) error {
	if cfg.ServiceBusConnection == "" {
		return fmt.Errorf("BOWRAIN_SERVICE_BUS_CONNECTION is required for queue mode")
	}
	if cfg.RedisURL == "" {
		return fmt.Errorf("BOWRAIN_REDIS_URL is required for queue mode")
	}

	// Service Bus sender for bravo-jobs queue.
	queue, err := jobs.NewServiceBusQueue(cfg.ServiceBusConnection, "bravo-jobs")
	if err != nil {
		return fmt.Errorf("connect to Service Bus (bravo-jobs): %w", err)
	}

	// Redis pub/sub client.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse Redis URL: %w", err)
	}
	if cfg.RedisPassword != "" {
		redisOpts.Password = cfg.RedisPassword
	}
	redisClient := redis.NewClient(redisOpts)
	pubsub := service.NewAgentPubSub(redisClient)

	s.AgentService.SetQueue(queue, pubsub)
	return nil
}

// mcpEndpointForAgent returns the MCP endpoint URL that agent containers
// should use to call back to this server.
func (cfg ServerConfig) mcpEndpointForAgent() string {
	host := cfg.Host
	if host == "" || host == "0.0.0.0" {
		host = "host.docker.internal" // Docker for Mac/Windows
	}
	port := cfg.Port
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("http://%s:%d/mcp/", host, port)
}
