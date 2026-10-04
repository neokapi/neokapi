package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// PairedLateContext is context a person adds while the agent works, for a
// task that measures recovery from a refusal: once a tool call has read the
// project's context, or the agent first writes a task file, each file Append
// names gains its text and the project's context is read in again. The rule
// then governs the agent's write without having been in the context it read,
// so the write meets the check rather than a rule it already followed.
type PairedLateContext struct {
	Append      []PairedAppend `json:"append"`
	Description string         `json:"description,omitempty"`
}

// PairedAppend is text added to the end of one fixture file.
type PairedAppend struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

var (
	// pairedContextCommand is a shell command that reads the project's
	// context: a kapi context, voice or terms verb, a check, or the style
	// files themselves.
	pairedContextCommand = regexp.MustCompile(`\bkapi(?:-files)?\s+(?:voice|context|terms|check)\b|STYLE\.md|\.kapi/voice\.yaml`)
	// pairedContextTool is a kapi MCP tool that reads the project's context.
	pairedContextTool = regexp.MustCompile(`^mcp__kapi__\w*(?:context|voice|terms|check)\w*$`)
)

// pairedReadsContext reports whether a tool call reads the project's
// context.
func pairedReadsContext(tool string, input map[string]any) bool {
	switch {
	case tool == "Bash" || tool == "shell":
		return pairedContextCommand.MatchString(pairedString(input, "command"))
	case tool == "Read":
		file := filepath.ToSlash(pairedString(input, "file_path"))
		return strings.HasSuffix(file, "STYLE.md") || strings.HasSuffix(file, ".kapi/voice.yaml")
	case strings.HasPrefix(tool, "mcp__kapi__"):
		return pairedContextTool.MatchString(tool) && !strings.HasSuffix(tool, "_observe") && !strings.HasSuffix(tool, "_correct")
	}
	return false
}

// pairedLateContextRun lands one task's late context in a cell.
type pairedLateContextRun struct {
	spec      PairedLateContext
	workspace string
	kapiBin   string
}

// apply appends each file's text and reads the project's context in again.
func (l *pairedLateContextRun) apply(ctx context.Context) PairedInterferenceRecord {
	record := PairedInterferenceRecord{Triggered: true}
	if err := pairedAppendLateContext(l.workspace, l.spec); err != nil {
		record.Error = err.Error()
		return record
	}
	if err := readPairedContext(ctx, l.workspace, l.kapiBin); err != nil {
		record.Error = err.Error()
		return record
	}
	record.Applied = true
	return record
}

// pairedAppendLateContext appends each text of spec to its file under
// workspace.
func pairedAppendLateContext(workspace string, spec PairedLateContext) error {
	for _, a := range spec.Append {
		file, err := os.OpenFile(filepath.Join(workspace, filepath.FromSlash(a.Path)), os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return fmt.Errorf("late context: %w", err)
		}
		_, writeErr := file.WriteString(a.Text)
		closeErr := file.Close()
		if writeErr != nil {
			return fmt.Errorf("late context: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("late context: %w", closeErr)
		}
	}
	return nil
}

// landLateContext lands the task's late context once, naming what set it
// off.
func (o *pairedObserver) landLateContext(ctx context.Context, trigger string) {
	if o.lateContext == nil || (o.result.LateContext != nil && o.result.LateContext.Triggered) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	record := o.lateContext.apply(ctx)
	record.Trigger = trigger
	record.AfterMS = time.Since(o.started).Milliseconds()
	o.result.LateContext = &record
}
