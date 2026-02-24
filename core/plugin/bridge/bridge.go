package bridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

// JavaBridge manages a JVM subprocess that runs the Okapi bridge server.
// Communication is synchronous NDJSON over stdin/stdout.
//
// The JVM is stateful: Open sets the active filter, and Read/Close operate on
// it. Concurrent access is handled by BridgePool, which leases each bridge
// exclusively to one goroutine for the full Open→Read→Close lifecycle.
// The per-command mu serializes individual NDJSON round-trips.
type JavaBridge struct {
	cfg     BridgeConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	mu      sync.Mutex // per-command serialization
	logger  *log.Logger
	running bool
}

// NewJavaBridge creates a new bridge with the given config.
func NewJavaBridge(cfg BridgeConfig, logger *log.Logger) *JavaBridge {
	cfg = cfg.withDefaults()
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &JavaBridge{
		cfg:    cfg,
		logger: logger,
	}
}

// Start launches the JVM subprocess and waits for the ready signal.
func (b *JavaBridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("bridge already running")
	}

	b.cmd = exec.Command(b.cfg.Command, b.cfg.Args...)
	if len(b.cfg.Env) > 0 {
		b.cmd.Env = os.Environ()
		for k, v := range b.cfg.Env {
			b.cmd.Env = append(b.cmd.Env, k+"="+v)
		}
	}

	var err error
	b.stdin, err = b.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	// Stderr goes to logger.
	b.cmd.Stderr = &logWriter{logger: b.logger}

	b.scanner = bufio.NewScanner(stdout)
	b.scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("starting JVM: %w", err)
	}

	// Wait for the ready signal.
	readyCh := make(chan error, 1)
	go func() {
		if !b.scanner.Scan() {
			if err := b.scanner.Err(); err != nil {
				readyCh <- fmt.Errorf("reading ready signal: %w", err)
			} else {
				readyCh <- fmt.Errorf("JVM closed stdout before sending ready signal")
			}
			return
		}
		var resp Response
		if err := json.Unmarshal(b.scanner.Bytes(), &resp); err != nil {
			readyCh <- fmt.Errorf("parsing ready signal: %w", err)
			return
		}
		if !resp.IsOK() {
			readyCh <- fmt.Errorf("JVM startup failed: %s", resp.Error)
			return
		}
		var ready ReadyData
		if err := json.Unmarshal(resp.Data, &ready); err != nil || !ready.Ready {
			readyCh <- fmt.Errorf("unexpected ready data: %s", string(resp.Data))
			return
		}
		readyCh <- nil
	}()

	select {
	case err := <-readyCh:
		if err != nil {
			_ = b.cmd.Process.Kill()
			return err
		}
	case <-time.After(b.cfg.StartupTimeout):
		_ = b.cmd.Process.Kill()
		return fmt.Errorf("JVM startup timed out after %s", b.cfg.StartupTimeout)
	}

	b.running = true
	return nil
}

// Stop gracefully shuts down the JVM subprocess.
func (b *JavaBridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}
	b.running = false

	// Send shutdown command (no response expected).
	cmd := Command{Command: "shutdown"}
	data, err := json.Marshal(cmd)
	if err == nil {
		data = append(data, '\n')
		_, _ = b.stdin.Write(data)
	}
	_ = b.stdin.Close()

	if b.cmd == nil {
		return nil
	}

	// Wait for process to exit.
	done := make(chan error, 1)
	go func() { done <- b.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = b.cmd.Process.Kill()
	}

	return nil
}

// sendCommand sends a command and returns the response. Must be called with mu held externally
// only via the public methods which acquire the lock.
func (b *JavaBridge) sendCommand(cmd Command) (*Response, error) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshaling command: %w", err)
	}
	data = append(data, '\n')

	if _, err := b.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("writing command: %w", err)
	}

	respCh := make(chan *scanResult, 1)
	go func() {
		if b.scanner.Scan() {
			respCh <- &scanResult{data: b.scanner.Bytes()}
		} else {
			respCh <- &scanResult{err: b.scanner.Err()}
		}
	}()

	var sr *scanResult
	select {
	case sr = <-respCh:
	case <-time.After(b.cfg.CommandTimeout):
		return nil, fmt.Errorf("command %q timed out after %s", cmd.Command, b.cfg.CommandTimeout)
	}

	if sr.err != nil {
		return nil, fmt.Errorf("reading response: %w", sr.err)
	}
	if sr.data == nil {
		return nil, fmt.Errorf("JVM closed stdout unexpectedly")
	}

	var resp Response
	if err := json.Unmarshal(sr.data, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &resp, nil
}

type scanResult struct {
	data []byte
	err  error
}

// Info queries filter metadata.
func (b *JavaBridge) Info(filterClass string) (*InfoData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	resp, err := b.sendCommand(Command{
		Command: "info",
		Params:  InfoParams{FilterClass: filterClass},
	})
	if err != nil {
		return nil, err
	}
	if !resp.IsOK() {
		return nil, fmt.Errorf("info: %s", resp.Error)
	}
	var info InfoData
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		return nil, fmt.Errorf("parsing info data: %w", err)
	}
	return &info, nil
}

// Open opens a document for reading via the Java bridge.
func (b *JavaBridge) Open(params OpenParams) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	resp, err := b.sendCommand(Command{
		Command: "open",
		Params:  params,
	})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("open: %s", resp.Error)
	}
	return nil
}

// Read reads all parts from an opened document.
func (b *JavaBridge) Read() (*ReadData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	resp, err := b.sendCommand(Command{Command: "read"})
	if err != nil {
		return nil, err
	}
	if !resp.IsOK() {
		return nil, fmt.Errorf("read: %s", resp.Error)
	}
	var rd ReadData
	if err := json.Unmarshal(resp.Data, &rd); err != nil {
		return nil, fmt.Errorf("parsing read data: %w", err)
	}
	return &rd, nil
}

// Write sends translated parts and receives the reconstructed document.
func (b *JavaBridge) Write(params WriteParams) (*WriteData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	resp, err := b.sendCommand(Command{
		Command: "write",
		Params:  params,
	})
	if err != nil {
		return nil, err
	}
	if !resp.IsOK() {
		return nil, fmt.Errorf("write: %s", resp.Error)
	}
	var wd WriteData
	if err := json.Unmarshal(resp.Data, &wd); err != nil {
		return nil, fmt.Errorf("parsing write data: %w", err)
	}
	return &wd, nil
}

// CloseFilter releases the current filter resources in the JVM.
func (b *JavaBridge) CloseFilter() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	resp, err := b.sendCommand(Command{Command: "close"})
	if err != nil {
		return err
	}
	if !resp.IsOK() {
		return fmt.Errorf("close: %s", resp.Error)
	}
	return nil
}

// ListFilters returns all available Okapi filters.
func (b *JavaBridge) ListFilters() (*ListFiltersData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	resp, err := b.sendCommand(Command{Command: "list_filters"})
	if err != nil {
		return nil, err
	}
	if !resp.IsOK() {
		return nil, fmt.Errorf("list_filters: %s", resp.Error)
	}
	var lf ListFiltersData
	if err := json.Unmarshal(resp.Data, &lf); err != nil {
		return nil, fmt.Errorf("parsing list_filters data: %w", err)
	}
	return &lf, nil
}

// logWriter adapts a *log.Logger to io.Writer for stderr capture.
type logWriter struct {
	logger *log.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Printf("[bridge-jvm] %s", string(p))
	return len(p), nil
}
