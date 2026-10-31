package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
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

// NewExternalCommandConfig creates an ExternalCommandConfig at its documented
// defaults for the given target locale.
func NewExternalCommandConfig(targetLocale model.LocaleID) *ExternalCommandConfig {
	cfg := &ExternalCommandConfig{}
	cfg.Reset()
	cfg.TargetLocale = targetLocale
	return cfg
}

// NewExternalCommandFromConfig creates an external-command tool from a config
// map.
//
// Command and Args are the tool's entire behaviour and can only come from the
// step. Without this factory the tool was built with an empty Command, so every
// block's run failed with "exec: no command" and the failure was recorded as a
// block property nothing reads — the config that named the program to run was
// discarded before it got there (#1476). Validate() is enforced here so a step
// with no command fails at construction, naming the tool, instead of failing
// once per block.
func NewExternalCommandFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewExternalCommandConfig(model.LocaleID(targetLang))
	if err := applyStepConfig("external-command", config, cfg, targetLang, setExternalCommandLocale); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return NewExternalCommandTool(cfg), nil
}

func setExternalCommandLocale(c *ExternalCommandConfig, loc model.LocaleID) { c.TargetLocale = loc }

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
	// recorded in block properties and leaves the content untouched. The
	// program reads the block's text with each redaction placeholder drawn as
	// its visible token (redaction.VisibleText); a block with none reads as
	// SourceText and TargetText read it. Unredact puts each value back by its
	// token, so an output whose placeholders differ from the input's leaves
	// the whole block untouched and records a failing finding for each
	// placeholder that differs.
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
		known := blockTokens(v)

		// Process source text.
		if conf.ApplySource {
			text, tokens := redaction.VisibleText(v.SourceRuns())
			result, err := runOn(v, conf, text, timeout)
			if err != nil {
				v.SetProperty(PropExternalCommandError, err.Error())
				return tool.EditPlan{}, nil
			}
			if changes := redaction.CompareTokens(text, result, tokens, known); len(changes) > 0 {
				check.Annotate(v, string(ExternalCommand), tokenFindings(conf, "source", changes))
				return tool.EditPlan{}, nil
			}
			if result != text {
				plan.ReplaceAll = &result
			}
		}

		// Process target text.
		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && v.HasTarget(conf.TargetLocale) {
			text, tokens := redaction.VisibleText(v.TargetRuns(conf.TargetLocale))
			result, err := runOn(v, conf, text, timeout)
			if err != nil {
				v.SetProperty(PropExternalCommandError, err.Error())
				return tool.EditPlan{}, nil
			}
			if changes := redaction.CompareTokens(text, result, tokens, known); len(changes) > 0 {
				side := string(conf.TargetLocale) + " target"
				check.Annotate(v, string(ExternalCommand), tokenFindings(conf, side, changes))
				return tool.EditPlan{}, nil
			}
			if result != text {
				plan.SetTarget(conf.TargetLocale, []model.Run{{Text: &model.TextRun{Text: result}}})
			}
		}

		return plan, nil
	}
	return t
}

// RulePlaceholder is the category of the finding external-command records
// when the output of its program differs from the program's input in a
// redaction placeholder. A report names it external-command.redaction-placeholder.
const RulePlaceholder = "redaction-placeholder"

// blockTokens returns the visible token of every redaction placeholder the
// block holds: in its source, in each translation, and among the values the
// redact step ahead of the tool withheld.
func blockTokens(v tool.BlockView) []string {
	tokens := redaction.Tokens(v.SourceRuns())
	for _, locale := range v.TargetLocales() {
		tokens = append(tokens, redaction.Tokens(v.TargetRuns(locale))...)
	}
	if ann, ok := v.Annotations()[redaction.SecretAnnotationKey].(*redaction.SecretAnnotation); ok {
		for _, val := range ann.Values {
			if val.Disp != "" {
				tokens = append(tokens, val.Disp)
			}
		}
	}
	return tokens
}

// tokenFindings turns the placeholder changes in the program's output
// for one side of a block into failing findings. side names that side:
// "source", or "<locale> target".
func tokenFindings(conf *ExternalCommandConfig, side string, changes []redaction.TokenChange) []check.Finding {
	program := filepath.Base(conf.Command)
	findings := make([]check.Finding, 0, len(changes))
	for _, c := range changes {
		var msg string
		suggestion := fmt.Sprintf("Make %s give back %s exactly as it reads it", program, c.Token)
		switch c.Kind {
		case redaction.TokenAltered:
			msg = fmt.Sprintf("%s gave back %s where the %s holds the redaction placeholder %s; the block is unchanged", program, c.Variant, side, c.Token)
		case redaction.TokenDuplicated:
			msg = fmt.Sprintf("%s gave back the redaction placeholder %s %d times; the %s holds it %d time(s); the block is unchanged", program, c.Token, c.Found, side, c.Expected)
		case redaction.TokenAdded:
			msg = fmt.Sprintf("%s gave back the redaction placeholder %s, which the %s does not hold; the block is unchanged", program, c.Token, side)
			suggestion = fmt.Sprintf("Make %s give back only the redaction placeholders it reads", program)
		default:
			if c.Found > 0 {
				// A partial drop: the output keeps some occurrences.
				msg = fmt.Sprintf("%s gave back the redaction placeholder %s %d time(s); the %s holds it %d time(s); the block is unchanged", program, c.Token, c.Found, side, c.Expected)
				break
			}
			msg = fmt.Sprintf("%s dropped the redaction placeholder %s that the %s holds; the block is unchanged", program, c.Token, side)
		}
		findings = append(findings, check.Finding{
			Category:     RulePlaceholder,
			Fails:        true,
			Message:      msg,
			Suggestion:   suggestion,
			OriginalText: c.Token,
			Metadata: map[string]string{
				"kind":     string(c.Kind),
				"side":     side,
				"token":    c.Token,
				"expected": strconv.Itoa(c.Expected),
				"found":    strconv.Itoa(c.Found),
			},
		})
	}
	return findings
}

// runOn runs the command over text and records its exit code on the block.
// A non-zero exit counts as a failure, so the block is left as it was.
func runOn(v tool.BlockView, conf *ExternalCommandConfig, text string, timeout int) (string, error) {
	result, exitCode, err := runCommand(v.Context(), conf, text, timeout)
	v.SetProperty(PropExternalCommandExitCode, strconv.Itoa(exitCode))
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", fmt.Errorf("external-command: %s exited with code %d", conf.Command, exitCode)
	}
	return result, nil
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
