package server

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/service"
)

// buildAgentPool creates an AgentPool with the configured container runtime.
// Used for direct mode (docker). Returns nil if not applicable.
func (s *Server) buildAgentPool() *service.AgentPool {
	cfg := s.Config

	var runtime service.ContainerRuntime

	switch cfg.AgentRuntime {
	case "docker":
		runtime = service.NewDockerRuntime(service.DockerRuntimeConfig{
			Host:    cfg.AgentDockerHost,
			Network: cfg.AgentDockerNetwork,
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

// mcpEndpointForAgent returns the MCP endpoint URL that agent containers
// should use to call back to this server.
func (cfg Config) mcpEndpointForAgent() string {
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
