package server

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/neokapi/neokapi/bowrain/service"
)

// buildAgentPool creates an AgentPool with the configured container runtime.
// Returns nil if no agent runtime is configured (mock mode).
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

	case "":
		return nil // No runtime configured — mock mode.

	default:
		log.Printf("WARNING: unknown agent runtime %q, falling back to mock mode", cfg.AgentRuntime)
		return nil
	}

	// Build MCP endpoint URL from server's own address.
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
