package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Property keys for external command results.
const (
	PropExternalCommandExitCode = "external-command-exit-code"
	PropExternalCommandError    = "external-command-error"
)

// ExternalCommandConfig holds configuration for the external command tool.
type ExternalCommandConfig struct {
	Command      string         `schema:"title=Command Line,description=The command to execute"`                                         // The command to execute (required)
	Args         []string       `schema:"title=Arguments,description=Command arguments; use ${source} and ${target} as placeholders"`    // Command arguments. Use "${source}" and "${target}" as placeholders.
	ApplySource  bool           `schema:"title=Apply to Source,description=Process source text"`                                         // Process source text (default: false)
	ApplyTarget  bool           `schema:"title=Apply to Target,description=Process target text,default=true"`                            // Process target text (default: true)
	TargetLocale model.LocaleID `schema:"title=Target Locale,description=Target locale for processing,showIfSet=ApplyTarget"`            // Target locale (required when ApplyTarget)
	SendAsStdin  bool           `schema:"title=Send as Stdin,description=Send text via stdin instead of command arguments,default=true"` // Send text via stdin instead of args (default: true)
	Timeout      int            `schema:"title=Timeout,description=Timeout in seconds,default=30,min=1"`                                 // Timeout in seconds (default: 30)
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
		return errors.New("external-command: command must not be empty")
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return errors.New("external-command: target locale is required when ApplyTarget is true")
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
	// Transform producer: returns the command output as an edit plan; the
	// framework applier rewrites the block (AD-006). A command failure is
	// recorded in block properties and leaves the content untouched.
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		if !v.Translatable() {
			return tool.EditPlan{}, nil
		}

		conf := t.Cfg.(*ExternalCommandConfig)

		timeout := conf.Timeout
		if timeout <= 0 {
			timeout = 30
		}

		var plan tool.EditPlan

		// Process source text.
		if conf.ApplySource {
			result, exitCode, err := runCommand(v.Context(), conf, v.SourceText(), timeout)
			v.SetProperty(PropExternalCommandExitCode, strconv.Itoa(exitCode))
			if err != nil {
				v.SetProperty(PropExternalCommandError, err.Error())
				return tool.EditPlan{}, nil
			}
			if result != v.SourceText() {
				plan.ReplaceAll = &result
			}
		}

		// Process target text.
		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && v.HasTarget(conf.TargetLocale) {
			result, exitCode, err := runCommand(v.Context(), conf, v.TargetText(conf.TargetLocale), timeout)
			v.SetProperty(PropExternalCommandExitCode, strconv.Itoa(exitCode))
			if err != nil {
				v.SetProperty(PropExternalCommandError, err.Error())
				return tool.EditPlan{}, nil
			}
			if result != v.TargetText(conf.TargetLocale) {
				plan.SetTarget(conf.TargetLocale, []model.Run{{Text: &model.TextRun{Text: result}}})
			}
		}

		return plan, nil
	}
	return t
}

// runCommand executes the configured command with the given text input.
// It returns the stdout output, exit code, and any error.
func runCommand(ctx context.Context, cfg *ExternalCommandConfig, text string, timeoutSec int) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
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
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", -1, fmt.Errorf("external-command: %w", err)
		}
	}

	return strings.TrimRight(stdout.String(), "\n"), exitCode, nil
}
