package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// `kapi rules`: the person's spelling of what `kapi context review`, `note`,
// `search`, `terms` and `voice` hold. It stores nothing of its own: listing
// reads host.ListRules, deciding records through host.DecideContextReview,
// and narrowing records a keep at a narrower scope (host.NarrowRule).

// NewRulesCmd creates the rules command family.
func NewRulesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rules",
		Short:   "Decide, list, narrow or drop the rules your project keeps",
		GroupID: "work",
		Long: `The rules are what your project keeps to: names and their spelling, words to
avoid, and the voice's measures and patterns. An assistant suggests a rule
when it notices one in your files; checks report a suggestion and none fails
on it until you keep it.

With suggestions waiting, kapi rules asks about each one in turn: which form
your files use, or whether to keep it. Before you answer it shows what keeping
it does: how many blocks a check then reports, and where the rule applies.

  k  keep             n  not a rule         l  later
  a number picks another form, or types one of your own (Other…)

A kept name or word fails a check from then on. A kept note reports and fails
nothing. Elsewhere than at a terminal, kapi rules lists the questions with the
id each decision takes:

  --keep <id>        keep a suggestion (repeatable); --use picks the form
  --drop <id>        not a rule: set a suggestion or a kept rule aside
  --later <id>       leave it waiting; it keeps advising
  --choose <id>      keep one side where two suggestions disagree

kapi context review records the same decisions for an assistant.`,
		Example: "  kapi rules\n" +
			"  kapi rules list\n" +
			"  kapi rules --keep 0n794e2gk7 --use \"Sign in\"\n" +
			"  kapi rules keep 0n794e2gk7\n" +
			"  kapi rules drop 0n794e2gk7\n" +
			"  kapi rules narrow 0n794e2gk7 --to docs\n" +
			"  kapi rules find \"sign in\"",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			keep, _ := cmd.Flags().GetStringArray("keep")
			drop, _ := cmd.Flags().GetStringArray("drop")
			later, _ := cmd.Flags().GetStringArray("later")
			choose, _ := cmd.Flags().GetString("choose")
			use, _ := cmd.Flags().GetString("use")
			note, _ := cmd.Flags().GetString("note")
			decide := host.ContextReviewRequest{
				Project:     projectPath,
				Keep:        keep,
				Drop:        drop,
				Choose:      choose,
				Replacement: use,
				Advisory:    confirmAdvisory(cmd),
				Note:        note,
			}
			if !decide.Empty() || len(later) > 0 {
				return decideRulesByFlag(cmd, a, decide, later)
			}
			if use != "" || cmd.Flags().Changed("advisory") {
				return errors.New("--use and --advisory change what --keep or --choose names: name it with --keep or --choose")
			}
			view, err := a.DecideRules(cmd.Context(), host.RulesRequest{Project: projectPath})
			if err != nil {
				return err
			}
			if len(view.Decisions) > 0 && reviewInteractive(cmd) {
				return walkRules(cmd, a, projectPath, view, note)
			}
			return output.Print(cmd, view)
		},
	}
	addRulesDecideFlags(cmd)
	cmd.Flags().StringArray("drop", nil, "not a rule: set a suggestion or a kept rule aside by id (repeatable)")
	cmd.Flags().StringArray("later", nil, "leave a suggestion waiting by id (repeatable); it keeps advising")
	cmd.Flags().String("choose", "", "keep this side where two suggestions disagree and set the other aside")
	cmd.Flags().StringArray("keep", nil, "keep a suggestion by id (repeatable)")
	AddProjectFlag(cmd)

	for _, sub := range []*cobra.Command{
		newRulesListCmd(a),
		newRulesKeepCmd(a),
		newRulesDropCmd(a),
		newRulesNarrowCmd(a),
		newRulesFindCmd(a),
	} {
		cmd.AddCommand(sub)
	}
	return cmd
}

// addRulesDecideFlags adds the flags a keep takes.
func addRulesDecideFlags(cmd *cobra.Command) {
	cmd.Flags().String("use", "", "with one --keep or --choose: the form the rule says to write")
	cmd.Flags().Bool("advisory", false, "with a keep: the rule reports without failing a check (--advisory=false makes it fail)")
	cmd.Flags().String("note", "", "why")
}

// rulesDecided is what one round of flag decisions recorded.
type rulesDecided struct {
	Decided *host.ContextReviewResult `json:"decided,omitempty"`
	Later   []string                  `json:"later"`
}

// FormatText renders the decisions, then what was left for later.
func (r rulesDecided) FormatText(w io.Writer) error {
	if r.Decided != nil {
		if err := r.Decided.FormatText(w); err != nil {
			return err
		}
	}
	return host.RulesLaterResult{Later: r.Later}.FormatText(w)
}

// decideRulesByFlag records the decisions the flags name.
func decideRulesByFlag(cmd *cobra.Command, a *App, decide host.ContextReviewRequest, later []string) error {
	out := rulesDecided{Later: []string{}}
	if len(later) > 0 {
		res, err := a.LeaveRulesForLater(cmd.Context(), decide.Project, later)
		if err != nil {
			return err
		}
		out.Later = res.Later
	}
	if !decide.Empty() {
		res, err := a.DecideContextReview(cmd.Context(), decide)
		if err != nil {
			return err
		}
		out.Decided = &res
	}
	return output.Print(cmd, out)
}

func newRulesListCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the rules in force and the suggestions waiting",
		Long: `List every rule the project keeps, one line each, grouped as names, words,
tone and suggested. Each line says where the rule comes from (the terms store,
the voice profile, or who kept it and when) and, for a rule narrowed to part
of the project, where it applies. The id at the end of a line is what
kapi rules keep, drop and narrow take.`,
		Example: "  kapi rules list\n  kapi rules list --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.ListRules(cmd.Context(), host.RulesRequest{Project: projectPath})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	AddProjectFlag(cmd)
	return cmd
}

func newRulesKeepCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keep <id>",
		Short: "Keep a suggestion as a rule",
		Long: `Keep the suggestion an id names as a rule, the same as answering k in
kapi rules. --use picks the form a name is kept as.`,
		Example: "  kapi rules keep 0n794e2gk7\n  kapi rules keep 0n794e2gk7 --use \"Sign in\"",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			use, _ := cmd.Flags().GetString("use")
			note, _ := cmd.Flags().GetString("note")
			return decideRulesByFlag(cmd, a, host.ContextReviewRequest{
				Project: projectPath, Keep: args, Replacement: use, Advisory: confirmAdvisory(cmd), Note: note,
			}, nil)
		},
	}
	addRulesDecideFlags(cmd)
	AddProjectFlag(cmd)
	return cmd
}

func newRulesDropCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drop <id>",
		Short: "Set a suggestion or a kept rule aside",
		Long: `Set aside the suggestion or rule an id names, the same as answering n in
kapi rules. A kept rule stops applying at once and is taken back out of the
terms store. The decision is recorded beside the one it replaces, so the log
keeps both.`,
		Example: "  kapi rules drop 0n794e2gk7\n  kapi rules drop 0n794e2gk7 --note \"we say it both ways\"",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			note, _ := cmd.Flags().GetString("note")
			return decideRulesByFlag(cmd, a, host.ContextReviewRequest{Project: projectPath, Drop: args, Note: note}, nil)
		},
	}
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

func newRulesNarrowCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "narrow <id> --to <collection|folder>",
		Short: "Limit a kept rule to a collection or a folder",
		Long: `Limit a kept rule to one collection, or to one folder of the project. The
rule holds at the profile, channel and coordinates the collection or folder
sits at, and nowhere else. A folder that sits at the project's own point has
no coordinate to tell it apart, and kapi says so.`,
		Example: "  kapi rules narrow 0n794e2gk7 --to docs\n  kapi rules narrow 0n794e2gk7 --to web/src",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			to, _ := cmd.Flags().GetString("to")
			note, _ := cmd.Flags().GetString("note")
			res, err := a.NarrowRule(cmd.Context(), host.RulesNarrowRequest{Project: projectPath, ID: args[0], To: to, Note: note})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("to", "", "the collection or folder the rule applies to")
	cmd.Flags().String("note", "", "why")
	_ = cmd.MarkFlagRequired("to")
	AddProjectFlag(cmd)
	return cmd
}

func newRulesFindCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find <word>",
		Short: "Find the rules, suggestions and decisions that mention a word",
		Long: `Find what the project keeps about one word: the rules that mention it, the
suggestions waiting, and the suggestions a person decided were not a rule,
each with where it applies.`,
		Example: "  kapi rules find \"sign in\"\n  kapi rules find widget --json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.FindRules(cmd.Context(), host.RulesFindRequest{Project: projectPath, Word: args[0]})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	AddProjectFlag(cmd)
	return cmd
}

// errRulesQuit ends a walk early; what was decided stays recorded.
var errRulesQuit = errors.New("quit")

// rulesTally counts what a walk decided.
type rulesTally struct{ kept, dropped, later int }

func (t rulesTally) line() string {
	return fmt.Sprintf("Kept %d, not a rule %d, later %d.", t.kept, t.dropped, t.later)
}
