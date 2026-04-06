package host

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/registry"
)

// PluginManager discovers, launches, and manages plugin processes.
// It integrates with the core FormatRegistry to register plugin-provided
// format readers and writers.
type PluginManager struct {
	mu      sync.Mutex
	plugins map[string]*managedPlugin
	logger  *log.Logger
}

// managedPlugin tracks a running plugin process.
type managedPlugin struct {
	path   string
	client *plugin.Client
	name   string
	kind   string // "format-reader", "format-writer", "tool"
}

// NewPluginManager creates a new PluginManager.
func NewPluginManager(logger *log.Logger) *PluginManager {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &PluginManager{
		plugins: make(map[string]*managedPlugin),
		logger:  logger,
	}
}

// DiscoverAndRegister scans a directory for plugin executables, launches each one,
// queries its type and metadata, and registers it with the given FormatRegistry.
// Plugin executables are expected to be named neokapi-plugin-* (or neokapi-plugin-*.exe on Windows).
func (m *PluginManager) DiscoverAndRegister(dir string, reg *registry.FormatRegistry) error {
	pattern := filepath.Join(dir, "neokapi-plugin-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("scanning plugin directory %s: %w", dir, err)
	}

	for _, path := range matches {
		// Skip non-executable files (best-effort check).
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		// On Windows, any file with .exe extension is executable.
		// On Unix, check the execute permission bit.
		if runtime.GOOS != "windows" {
			if info.Mode()&0111 == 0 {
				continue
			}
		}

		if err := m.loadPlugin(path, reg); err != nil {
			m.logger.Printf("failed to load plugin %s: %v", path, err)
			continue
		}
	}
	return nil
}

// loadPlugin launches a single plugin process and registers it.
func (m *PluginManager) loadPlugin(path string, reg *registry.FormatRegistry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[path]; exists {
		return nil // already loaded
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins:         PluginMap(),
		Cmd:             exec.Command(path), //nolint:noctx // long-lived plugin subprocess
		Logger:          nil,                // use default hclog
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("connecting to plugin %s: %w", path, err)
	}

	// Try format-reader first.
	if raw, err := rpcClient.Dispense(FormatReaderPluginName); err == nil {
		if reader, ok := raw.(*FormatReaderRPCClient); ok {
			name := reader.Name()
			if name != "" {
				mp := &managedPlugin{
					path:   path,
					client: client,
					name:   name,
					kind:   FormatReaderPluginName,
				}
				m.plugins[path] = mp

				if reg != nil {
					sig := reader.Signature()
					displayName := reader.DisplayName()
					reg.RegisterReader(name, func() format.DataFormatReader {
						return reader
					}, sig, displayName)
					m.logger.Printf("registered format reader plugin: %s (mime=%v, ext=%v)",
						name, sig.MIMETypes, sig.Extensions)
				}
				return nil
			}
		}
	}

	// Try format-writer.
	if raw, err := rpcClient.Dispense(FormatWriterPluginName); err == nil {
		if writer, ok := raw.(*FormatWriterRPCClient); ok {
			writerName := writer.Name()
			if writerName != "" {
				mp := &managedPlugin{
					path:   path,
					client: client,
					name:   writerName,
					kind:   FormatWriterPluginName,
				}
				m.plugins[path] = mp

				if reg != nil {
					reg.RegisterWriter(writerName, func() format.DataFormatWriter {
						return writer
					})
					m.logger.Printf("registered format writer plugin: %s", writerName)
				}
				return nil
			}
		}
	}

	// Try tool.
	if raw, err := rpcClient.Dispense(ToolPluginName); err == nil {
		if t, ok := raw.(*ToolRPCClient); ok {
			toolName := t.Name()
			if toolName != "" {
				mp := &managedPlugin{
					path:   path,
					client: client,
					name:   toolName,
					kind:   ToolPluginName,
				}
				m.plugins[path] = mp
				m.logger.Printf("registered tool plugin: %s", toolName)
				return nil
			}
		}
	}

	// No recognized plugin type.
	client.Kill()
	return fmt.Errorf("plugin %s did not provide any recognized plugin type", path)
}

// LoadFormatReader launches a specific plugin executable as a format reader
// and returns the reader client.
func (m *PluginManager) LoadFormatReader(path string) (*FormatReaderRPCClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins:         PluginMap(),
		Cmd:             exec.Command(path), //nolint:noctx // long-lived plugin subprocess
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("connecting to plugin %s: %w", path, err)
	}

	raw, err := rpcClient.Dispense(FormatReaderPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispensing format-reader from %s: %w", path, err)
	}

	reader, ok := raw.(*FormatReaderRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin %s did not provide a format reader", path)
	}

	m.plugins[path] = &managedPlugin{
		path:   path,
		client: client,
		name:   reader.Name(),
		kind:   FormatReaderPluginName,
	}
	return reader, nil
}

// PluginDetail describes a loaded plugin.
type PluginDetail struct {
	Name   string
	Kind   string // "format-reader", "format-writer", "tool"
	Source string // executable path
}

// PluginNames returns the names of all loaded plugins.
func (m *PluginManager) PluginNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.plugins))
	for _, mp := range m.plugins {
		names = append(names, mp.name)
	}
	return names
}

// PluginDetails returns structured info for all loaded plugins.
func (m *PluginManager) PluginDetails() []PluginDetail {
	m.mu.Lock()
	defer m.mu.Unlock()
	details := make([]PluginDetail, 0, len(m.plugins))
	for _, mp := range m.plugins {
		details = append(details, PluginDetail{
			Name:   mp.name,
			Kind:   mp.kind,
			Source: mp.path,
		})
	}
	return details
}

// PluginCount returns the number of loaded plugins.
func (m *PluginManager) PluginCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.plugins)
}

// IsLoaded returns true if a plugin at the given path is loaded.
func (m *PluginManager) IsLoaded(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.plugins[path]
	return ok
}

// Shutdown gracefully stops all plugin processes.
func (m *PluginManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for path, mp := range m.plugins {
		m.logger.Printf("shutting down plugin: %s (%s)", mp.name, path)
		mp.client.Kill()
	}
	m.plugins = make(map[string]*managedPlugin)
}

// ShutdownPlugin stops a specific plugin process by path.
func (m *PluginManager) ShutdownPlugin(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mp, ok := m.plugins[path]
	if !ok {
		return fmt.Errorf("plugin not loaded: %s", path)
	}
	mp.client.Kill()
	delete(m.plugins, path)
	return nil
}

// pluginBaseName returns the base name of a plugin executable without
// the "neokapi-plugin-" prefix and OS-specific suffix.
func pluginBaseName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimPrefix(base, "neokapi-plugin-")
	if runtime.GOOS == "windows" {
		base = strings.TrimSuffix(base, ".exe")
	}
	return base
}
