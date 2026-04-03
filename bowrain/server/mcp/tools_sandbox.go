package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerSandboxTools registers the execute_script tool for running code
// in an isolated sandbox environment.
func (s *MCPServer) registerSandboxTools() {
	sdkmcp.AddTool(s.server, &sdkmcp.Tool{
		Name:        "execute_script",
		Description: "Run a script in an isolated sandbox. Supports Python, Bash, and Node.js. No network access. Useful for data transformations, file parsing, and batch operations.",
	}, s.handleExecuteScript)
}

type executeScriptInput struct {
	Language string            `json:"language" jsonschema:"script language: python, bash, or node"`
	Code     string            `json:"code" jsonschema:"the script source code"`
	Files    map[string]string `json:"files,omitempty" jsonschema:"input files (name -> content) mounted in /workspace"`
	Env      map[string]string `json:"env,omitempty" jsonschema:"environment variables"`
}
type executeScriptOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func (s *MCPServer) handleExecuteScript(ctx context.Context, req *sdkmcp.CallToolRequest, input executeScriptInput) (*sdkmcp.CallToolResult, executeScriptOutput, error) {
	if s.sandbox == nil {
		return nil, executeScriptOutput{}, fmt.Errorf("sandbox executor not configured")
	}
	if input.Code == "" {
		return nil, executeScriptOutput{}, fmt.Errorf("code is required")
	}

	validLangs := map[string]bool{"python": true, "bash": true, "node": true}
	if !validLangs[input.Language] {
		return nil, executeScriptOutput{}, fmt.Errorf("unsupported language %q — use python, bash, or node", input.Language)
	}

	// Convert string files to byte slices.
	var files map[string][]byte
	if len(input.Files) > 0 {
		files = make(map[string][]byte, len(input.Files))
		for name, content := range input.Files {
			files[name] = []byte(content)
		}
	}

	result, err := s.sandbox.Execute(ctx, SandboxRequest{
		Language: input.Language,
		Code:     input.Code,
		Files:    files,
		Env:      input.Env,
	})
	if err != nil {
		return nil, executeScriptOutput{}, fmt.Errorf("execute script: %w", err)
	}

	return nil, executeScriptOutput{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, nil
}
