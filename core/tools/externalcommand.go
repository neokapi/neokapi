package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// Property keys for external command results.
const (
	PropExternalCommandExitCode = "external-command-exit-code"
	PropExternalCommandError    = "external-command-error"
)

// ExternalCommandConfig holds configuration for the external command tool.
type ExternalCommandConfig struct {
	Command      string         // The command to execute (required)
	Args         []string       // Command arguments. Use "${source}" and "${target}" as placeholders.
	ApplySource  bool           // Process source text (default: false)
	ApplyTarget  bool           // Process target text (default: true)
	TargetLocale model.LocaleID // Target locale (required when ApplyTarget)
	SendAsStdin  bool           // Send text via stdin instead of args (default: true)
	Timeout      int            // Timeout in seconds (default: 30)
}

// ToolName returns the tool name this config applies to.
func (c *ExternalCommandConfig) ToolName() string { return "external-command" }

// Reset restores default values.
func (c *ExternalCommandConfig) Reset() {
	c.Command = ""
	c.Args = nil
	c.ApplySource = false
	c.ApplyTarget = true
	c.TargetLocale = ""
	c.SendAsStdin = true
	c.Timeout = 30
}

// Validate checks configuration validity.
func (c *ExternalCommandConfig) Validate() error {
	if c.Command == "" {
		return fmt.Errorf("external-command: command must not be empty")
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return fmt.Errorf("external-command: target locale is required when ApplyTarget is true")
	}
	return nil
}

// NewExternalCommandTool creates a new external command tool.
// It executes an external command-line program on block text, capturing stdout as the result.
func NewExternalCommandTool(cfg *ExternalCommandConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "external-command",
		ToolDescription: "Executes an external command on block text",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*ExternalCommandConfig)

		timeout := conf.Timeout
		if timeout <= 0 {
			timeout = 30
		}

		// Process source text.
		if conf.ApplySource {
			sourceText := block.SourceText()
			result, exitCode, err := runCommand(conf, sourceText, timeout)
			block.Properties[PropExternalCommandExitCode] = strconv.Itoa(exitCode)
			if err != nil {
				block.Properties[PropExternalCommandError] = err.Error()
				return part, nil
			}
			block.SetSourceText(result)
		}

		// Process target text.
		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && block.HasTarget(conf.TargetLocale) {
			targetText := block.TargetText(conf.TargetLocale)
			result, exitCode, err := runCommand(conf, targetText, timeout)
			block.Properties[PropExternalCommandExitCode] = strconv.Itoa(exitCode)
			if err != nil {
				block.Properties[PropExternalCommandError] = err.Error()
				return part, nil
			}
			block.SetTargetText(conf.TargetLocale, result)
		}

		return part, nil
	}
	return t
}

// runCommand executes the configured command with the given text input.
// It returns the stdout output, exit code, and any error.
func runCommand(cfg *ExternalCommandConfig, text string, timeoutSec int) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Build args with placeholder replacement.
	args := make([]string, len(cfg.Args))
	for i, arg := range cfg.Args {
		arg = strings.ReplaceAll(arg, "${source}", text)
		arg = strings.ReplaceAll(arg, "${target}", text)
		args[i] = arg
	}

	cmd := exec.CommandContext(ctx, cfg.Command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if cfg.SendAsStdin {
		cmd.Stdin = strings.NewReader(text)
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return "", -1, fmt.Errorf("external-command: %w", err)
		}
	}

	return strings.TrimRight(stdout.String(), "\n"), exitCode, nil
}
