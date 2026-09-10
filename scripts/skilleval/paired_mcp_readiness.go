package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const pairedMCPProbeTimeout = 10 * time.Second

// PairedMCPReadiness records direct server discovery without invoking a tool.
// It does not establish what an agent host exposes or what an agent uses.
type PairedMCPReadiness struct {
	Status            string   `json:"status"`
	Evidence          string   `json:"evidence"`
	AgentHostExposure string   `json:"agent_host_exposure"`
	ProtocolVersion   string   `json:"protocol_version,omitempty"`
	ServerName        string   `json:"server_name,omitempty"`
	ServerVersion     string   `json:"server_version,omitempty"`
	Capabilities      []string `json:"capabilities"`
	Tools             []string `json:"tools"`
	Resources         []string `json:"resources"`
	ResourceTemplates []string `json:"resource_templates"`
	DurationMS        int64    `json:"duration_ms"`
	Error             string   `json:"error,omitempty"`
}

func preparePairedMCPReadiness(ctx context.Context, prepared *PairedPrepared) {
	if prepared.Launch.Condition != "mcp" {
		return
	}
	readiness := probePairedMCP(ctx, prepared.Launch)
	prepared.MCPReadiness = &readiness
	if readiness.Status != "ready" {
		prepared.Blockers = append(prepared.Blockers, "kapi MCP readiness: "+readiness.Error)
	}
	prepared.IsolationNotes = append(prepared.IsolationNotes,
		"MCP readiness is a direct, no-inference server handshake and capability inventory. Agent-host exposure and use require separate session evidence.")
}

func probePairedMCP(ctx context.Context, launch PairedLaunch) PairedMCPReadiness {
	readiness := PairedMCPReadiness{
		Status: "failed", Evidence: "direct-server-discovery", AgentHostExposure: "unverified",
		Capabilities: []string{}, Tools: []string{}, Resources: []string{}, ResourceTemplates: []string{},
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, pairedMCPProbeTimeout)
	defer cancel()
	err := discoverPairedMCP(ctx, launch, &readiness)
	readiness.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		readiness.Error = err.Error()
		if ctx.Err() != nil {
			readiness.Status = "canceled"
			readiness.Error = ctx.Err().Error()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				readiness.Status = "timeout"
			}
		}
		return readiness
	}
	readiness.Status = "ready"
	return readiness
}

// The probe uses only initialization and discovery requests over MCP's newline
// JSON-RPC transport. There is no tools/call, resources/read or sampling route.
func discoverPairedMCP(ctx context.Context, launch PairedLaunch, readiness *PairedMCPReadiness) error {
	if launch.KapiBin == "" {
		return errors.New("kapi binary is unavailable")
	}
	command := exec.CommandContext(ctx, launch.KapiBin,
		"-p", filepath.Join(launch.Workspace, "kapi.yaml"), "mcp")
	command.Dir = launch.Workspace
	command.Env = pairedEnvironment(launch)
	// Server diagnostics are not protocol messages. They cannot block discovery
	// or copy the developer's environment into a readiness report.
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	pairedConfigureProcess(command)
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	defer output.Close()
	stopClose := context.AfterFunc(ctx, func() {
		_ = input.Close()
		_ = output.Close()
	})
	defer stopClose()
	if err := command.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer func() {
		_ = input.Close()
		_ = pairedStopProcess(command)
		_ = output.Close()
		_ = command.Wait()
	}()
	client := newPairedMCPDiscoveryClient(input, output)
	if err := client.initialize(readiness); err != nil {
		return err
	}
	if !slices.Contains(readiness.Capabilities, "tools") || !slices.Contains(readiness.Capabilities, "resources") {
		return errors.New("server must advertise tools and resources capabilities")
	}
	for _, listing := range []struct {
		method, key, field string
		values             *[]string
	}{
		{method: "tools/list", key: "tools", field: "name", values: &readiness.Tools},
		{method: "resources/list", key: "resources", field: "uri", values: &readiness.Resources},
		{method: "resources/templates/list", key: "resourceTemplates", field: "uriTemplate", values: &readiness.ResourceTemplates},
	} {
		if err := client.list(listing.method, listing.key, listing.field, listing.values); err != nil {
			return err
		}
	}
	missing := []string{}
	for _, name := range []string{"check_file", "check_text"} {
		if !slices.Contains(readiness.Tools, name) {
			missing = append(missing, name)
		}
	}
	if !slices.ContainsFunc(readiness.Resources, pairedLocationContextURI) &&
		!slices.ContainsFunc(readiness.ResourceTemplates, pairedLocationContextURI) {
		missing = append(missing, "context:// location resource or template")
	}
	if len(missing) != 0 {
		return fmt.Errorf("required surfaces unavailable: %s", strings.Join(missing, ", "))
	}
	return nil
}

func pairedLocationContextURI(uri string) bool {
	location, ok := strings.CutPrefix(uri, "context://")
	return ok && location != "" && !strings.HasPrefix(location, "profile/")
}

type pairedMCPDiscoveryClient struct {
	encoder *json.Encoder
	scanner *bufio.Scanner
	nextID  int
}

func newPairedMCPDiscoveryClient(input io.Writer, output io.Reader) *pairedMCPDiscoveryClient {
	scanner := bufio.NewScanner(io.LimitReader(output, 16*1024*1024))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	return &pairedMCPDiscoveryClient{encoder: json.NewEncoder(input), scanner: scanner}
}

func (client *pairedMCPDiscoveryClient) initialize(readiness *PairedMCPReadiness) error {
	result, err := client.call("initialize", map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
		"clientInfo": map[string]string{"name": "kapi-paired-readiness", "version": "1"},
	})
	if err != nil {
		return err
	}
	readiness.ProtocolVersion = pairedString(result, "protocolVersion")
	if !slices.Contains([]string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}, readiness.ProtocolVersion) {
		return fmt.Errorf("unsupported MCP protocol version %q", readiness.ProtocolVersion)
	}
	server := pairedObject(result, "serverInfo")
	readiness.ServerName = pairedString(server, "name")
	readiness.ServerVersion = pairedString(server, "version")
	if readiness.ServerName == "" || readiness.ServerVersion == "" {
		return errors.New("initialize omitted server identity")
	}
	for capability := range pairedObject(result, "capabilities") {
		readiness.Capabilities = append(readiness.Capabilities, capability)
	}
	slices.Sort(readiness.Capabilities)
	return client.encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
}

func (client *pairedMCPDiscoveryClient) list(method, key, field string, values *[]string) error {
	cursor := ""
	seen := map[string]bool{}
	for range 64 {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := client.call(method, params)
		if err != nil {
			return err
		}
		entries, ok := result[key].([]any)
		if !ok {
			return fmt.Errorf("%s omitted its %s array", method, key)
		}
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok || pairedString(entry, field) == "" {
				return fmt.Errorf("%s returned an invalid %s entry", method, key)
			}
			*values = append(*values, pairedString(entry, field))
		}
		slices.Sort(*values)
		*values = slices.Compact(*values)
		cursor = pairedString(result, "nextCursor")
		if cursor == "" {
			return nil
		}
		if seen[cursor] {
			return fmt.Errorf("%s repeated a pagination cursor", method)
		}
		seen[cursor] = true
	}
	return fmt.Errorf("%s exceeded the discovery page limit", method)
}

func (client *pairedMCPDiscoveryClient) call(method string, params map[string]any) (map[string]any, error) {
	client.nextID++
	if err := client.encoder.Encode(map[string]any{
		"jsonrpc": "2.0", "id": client.nextID, "method": method, "params": params,
	}); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}
	for range 256 {
		if !client.scanner.Scan() {
			if err := client.scanner.Err(); err != nil {
				return nil, fmt.Errorf("receive %s: %w", method, err)
			}
			return nil, fmt.Errorf("server disconnected during %s", method)
		}
		message := map[string]any{}
		if err := json.Unmarshal(client.scanner.Bytes(), &message); err != nil {
			return nil, fmt.Errorf("malformed MCP message during %s: %w", method, err)
		}
		if pairedString(message, "jsonrpc") != "2.0" {
			return nil, errors.New("MCP message omitted JSON-RPC version 2.0")
		}
		if incoming := pairedString(message, "method"); incoming != "" {
			if id, exists := message["id"]; exists {
				// This discovery client offers no sampling or elicitation handlers.
				// A protocol ping can be answered without invoking any model.
				reply := map[string]any{"jsonrpc": "2.0", "id": id}
				reply["error"] = map[string]any{"code": -32601, "message": "discovery client does not support this method"}
				if incoming == "ping" {
					delete(reply, "error")
					reply["result"] = map[string]any{}
				}
				if err := client.encoder.Encode(reply); err != nil {
					return nil, err
				}
			}
			continue
		}
		if id, ok := message["id"].(float64); !ok || id != float64(client.nextID) {
			return nil, fmt.Errorf("unexpected MCP response identity during %s", method)
		}
		if rpcError := pairedObject(message, "error"); rpcError != nil {
			return nil, fmt.Errorf("%s failed: %s", method, pairedString(rpcError, "message"))
		}
		result := pairedObject(message, "result")
		if result == nil {
			return nil, fmt.Errorf("%s returned no result object", method)
		}
		return result, nil
	}
	return nil, fmt.Errorf("too many unsolicited MCP messages during %s", method)
}
