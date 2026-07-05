// Package host is the cobra-free application runtime and service layer
// shared by the kapi CLI, the kapi-bowrain plugin binary, and Kapi Desktop.
// It owns the App runtime (registries, config, credentials, plugin host)
// and the workflow services (flow runs, convergence, checks, coverage,
// review, project state). The cli package is the thin Cobra presentation
// shell over this package: command factories, flag registration, and
// terminal output live there.
package host

import (
	"context"
	"io"
	"os"

	"github.com/spf13/pflag"
)

// Command is the execution environment a service entry point runs in: a
// cancellation context, resolved flag values, and the run's IO streams.
// It is the cobra-free seam between the cobra shell (package cli) and the
// host services — *cobra.Command satisfies it natively, so cobra RunE
// adapters pass their command straight through, while embedders (the
// desktop, MCP tools, internal orchestrators) use EnvCommand.
type Command interface {
	Context() context.Context
	SetContext(ctx context.Context)
	Flags() *pflag.FlagSet
	OutOrStdout() io.Writer
	ErrOrStderr() io.Writer
	InOrStdin() io.Reader
	SetOut(w io.Writer)
	SetErr(w io.Writer)
	SetIn(r io.Reader)
	Name() string
}

// EnvCommand is a synthetic Command for embedded runs (the desktop, MCP
// tools, multi-locale orchestration): a plain flag set plus explicit IO.
// The zero value is usable — background context, empty flags, and, matching
// cobra's defaults, the process stdio.
type EnvCommand struct {
	ctx   context.Context
	flags *pflag.FlagSet
	out   io.Writer
	errW  io.Writer
	in    io.Reader
	name  string
}

// NewEnvCommand returns a synthetic Command named name carrying ctx.
// Callers register any flags the invoked service reads on Flags()
// (unregistered flags read as zero values, matching a cobra command that
// never declared them), and use SetOut/SetErr/SetIn to redirect IO away
// from the process stdio.
func NewEnvCommand(ctx context.Context, name string) *EnvCommand {
	return &EnvCommand{ctx: ctx, name: name}
}

// SetContext replaces the command's context.
func (c *EnvCommand) SetContext(ctx context.Context) { c.ctx = ctx }

// SetOut sets the writer OutOrStdout returns (nil → discard).
func (c *EnvCommand) SetOut(w io.Writer) { c.out = w }

// SetErr sets the writer ErrOrStderr returns (nil → discard).
func (c *EnvCommand) SetErr(w io.Writer) { c.errW = w }

// SetIn sets the reader InOrStdin returns (nil → empty).
func (c *EnvCommand) SetIn(r io.Reader) { c.in = r }

func (c *EnvCommand) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *EnvCommand) Flags() *pflag.FlagSet {
	if c.flags == nil {
		c.flags = pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
	}
	return c.flags
}

func (c *EnvCommand) OutOrStdout() io.Writer {
	if c.out == nil {
		return os.Stdout
	}
	return c.out
}

func (c *EnvCommand) ErrOrStderr() io.Writer {
	if c.errW == nil {
		return os.Stderr
	}
	return c.errW
}

func (c *EnvCommand) InOrStdin() io.Reader {
	if c.in == nil {
		return os.Stdin
	}
	return c.in
}

func (c *EnvCommand) Name() string {
	if c.name == "" {
		return "kapi"
	}
	return c.name
}
