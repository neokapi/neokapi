package cli

import (
	"io"
	"regexp"
	"strings"
	"text/template"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// SetupStyledHelp installs a colorized help/usage renderer on root (inherited by
// every subcommand). It reuses cobra's own layout — command groups, flag
// columns, aliases, examples — but pipes each section through host/output's
// Theme, so help picks up the same WCAG-safe palette and --color / NO_COLOR /
// isatty handling as the rest of the CLI. When color resolves off (piped,
// --color=never, NO_COLOR) every style renders as a no-op and the output is the
// plain text cobra produces today.
//
// This must run after LocalizeCommandHelp: the template reads Short/Long/Example
// at render time, so localized help still flows through unchanged.
func SetupStyledHelp(root *cobra.Command) {
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		return renderStyledUsage(cmd, cmd.OutOrStdout())
	})
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		w := cmd.OutOrStdout()
		if long := strings.TrimSpace(cmd.Long); long != "" {
			_, _ = io.WriteString(w, long+"\n\n")
		} else if short := strings.TrimSpace(cmd.Short); short != "" {
			_, _ = io.WriteString(w, short+"\n\n")
		}
		if cmd.Runnable() || cmd.HasSubCommands() {
			_ = renderStyledUsage(cmd, w)
		}
	})
}

// themeFor resolves --color from the command's flags (help renders before any
// PreRun hook, so nothing else has set it) and returns a Theme bound to w. It
// passes the mode explicitly rather than through output's process-global, so
// help never leaks its color decision into later Print calls in the same
// process (embedded runs, Kapi Desktop, the wasm CLI, tests).
func themeFor(cmd *cobra.Command, w io.Writer) *output.Styles {
	return output.ThemeWithMode(w, output.ResolveColorMode(cmd))
}

func renderStyledUsage(cmd *cobra.Command, w io.Writer) error {
	st := themeFor(cmd, w)
	t := template.Must(template.New("usage").Funcs(usageFuncs(st)).Parse(styledUsageTemplate))
	return t.Execute(w, cmd)
}

// flagRe matches a flag token (-c or --config) only when preceded by start of
// line or whitespace, so hyphenated words in descriptions ("fr-FR", "BCP-47")
// are left alone.
var flagRe = regexp.MustCompile(`(^|\s)(--?[A-Za-z][\w-]*)`)

func usageFuncs(st *output.Styles) template.FuncMap {
	heading := st.Renderer().NewStyle().Bold(true)
	return template.FuncMap{
		// heading styles a section title (bold): "Work:", "Flags:", …
		"heading": heading.Render,
		// cmdname accents a command name — the token you'd retype next.
		"cmdname": st.Accent.Render,
		// muted renders secondary chrome (the footer hint).
		"muted": st.Muted.Render,
		// rpad right-pads to a fixed width, computed on the raw name, so
		// coloring the padded token keeps the columns aligned.
		"rpad": func(s string, padding int) string {
			return s + strings.Repeat(" ", max(0, padding-len(s)))
		},
		// styleFlags accents flag names inside cobra's pre-aligned FlagUsages
		// block. ANSI codes are inserted into the finished string; the visible
		// column widths are untouched.
		"styleFlags": func(block string) string {
			return flagRe.ReplaceAllStringFunc(block, func(m string) string {
				i := strings.IndexByte(m, '-')
				return m[:i] + st.Accent.Render(m[i:])
			})
		},
		"trim": func(s string) string { return strings.TrimRight(s, " \t\n") },
	}
}

// styledUsageTemplate mirrors cobra's default usage template, wrapping section
// headings, command names and flag names in style funcs.
const styledUsageTemplate = `{{heading "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{heading "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{heading "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{heading "Available Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cmdname (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{heading $group.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{cmdname (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{heading "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{cmdname (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{heading "Flags:"}}
{{styleFlags (trim .LocalFlags.FlagUsages)}}{{end}}{{if .HasAvailableInheritedFlags}}

{{heading "Global Flags:"}}
{{styleFlags (trim .InheritedFlags.FlagUsages)}}{{end}}{{if .HasHelpSubCommands}}

{{heading "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{cmdname (rpad .CommandPath .CommandPathPadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

{{muted (printf "Use \"%s [command] --help\" for more information about a command." .CommandPath)}}{{end}}
`
