package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DaemonProcess manages a long-running bridge JVM daemon.
type DaemonProcess struct {
	cmd     *exec.Cmd
	pid     int
	address string

	mu         sync.Mutex
	sampling   bool
	stopSample chan struct{}
	samples    []int64
}

// StartDaemon launches a bridge JAR in daemon mode with a long idle timeout.
// It reads the first line of stdout to get the gRPC listen address.
func StartDaemon(jarPath string) (*DaemonProcess, error) {
	cmd := exec.Command("java", "-Xmx16g", "-jar", jarPath, "--idle-timeout", "3600")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	// Read the first line which contains the listen address.
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		cmd.Process.Kill()
		return nil, fmt.Errorf("daemon produced no output")
	}
	address := strings.TrimSpace(scanner.Text())
	if address == "" {
		cmd.Process.Kill()
		return nil, fmt.Errorf("daemon produced empty address")
	}

	return &DaemonProcess{
		cmd:     cmd,
		pid:     cmd.Process.Pid,
		address: address,
	}, nil
}

// Address returns the gRPC listen address of the daemon.
func (d *DaemonProcess) Address() string {
	return d.address
}

// PID returns the process ID of the daemon.
func (d *DaemonProcess) PID() int {
	return d.pid
}

// StartRSSSampling begins polling the daemon's RSS at the given interval.
// Call StopRSSSampling to stop and retrieve samples.
func (d *DaemonProcess) StartRSSSampling(interval time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.sampling {
		return
	}
	d.sampling = true
	d.stopSample = make(chan struct{})
	d.samples = nil

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-d.stopSample:
				return
			case <-ticker.C:
				if rss := d.readRSS(); rss > 0 {
					d.mu.Lock()
					d.samples = append(d.samples, rss)
					d.mu.Unlock()
				}
			}
		}
	}()
}

// StopRSSSampling stops the RSS sampling goroutine and returns all collected samples (in KB).
func (d *DaemonProcess) StopRSSSampling() []int64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.sampling {
		return nil
	}
	close(d.stopSample)
	d.sampling = false
	samples := d.samples
	d.samples = nil
	return samples
}

// readRSS reads the RSS of the daemon process in KB using ps.
func (d *DaemonProcess) readRSS() int64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(d.pid)).Output()
	if err != nil {
		return 0
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// Shutdown kills the daemon process and waits for it to exit.
func (d *DaemonProcess) Shutdown() error {
	d.StopRSSSampling()
	if d.cmd.Process != nil {
		d.cmd.Process.Kill()
		d.cmd.Wait() // ignore error from kill
	}
	return nil
}
