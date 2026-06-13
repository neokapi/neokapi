package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
)

// relationArgForms lists the relation vocabulary in the lower-kebab form the
// CLI accepts, for help text and error messages.
const relationArgForms = "broader, narrower, part-of, has-part, related, replaced-by, use-instead, exact-match, close-match, competitor"

func (a *App) newTermbaseRelateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relate <source-concept-id> <relation> <target-concept-id>",
		Short: "Add a typed relation between two concepts",
		Long: `Add a typed relation between two concepts in the termbase.

The relation is one of the concept-relation vocabulary labels, written in
lower-kebab or upper form: ` + relationArgForms + `.

A relation may carry a validity: a half-open time interval (--valid-from,
--valid-to) plus free tags (--tag key=value) evaluated against a query-time
scope, so the same edge can hold in one market and be absent in another.
Both concepts must already exist in the termbase.`,
		Example: `  kapi termbase relate old-name replaced-by new-name --note "renamed at launch"
  kapi termbase relate c1 use-instead c2 --tag market=dach
  kapi termbase relate c1 related c2 --valid-from 2026-01-01 --valid-to 2026-06-01`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			relType, err := relationTypeArg(args[1])
			if err != nil {
				return err
			}
			note, _ := cmd.Flags().GetString("note")
			validity, err := validityFromFlags(cmd)
			if err != nil {
				return err
			}

			tb, dbPath, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			rel := termbase.ConceptRelation{
				ID:           uuid.NewString(),
				SourceID:     args[0],
				TargetID:     args[2],
				RelationType: relType,
				Note:         note,
				Validity:     validity,
				CreatedAt:    time.Now().UTC().Truncate(time.Second),
			}
			if err := tb.AddRelation(cmd.Context(), rel); err != nil {
				return fmt.Errorf("add relation: %w", err)
			}

			if a.Quiet {
				return nil
			}
			return output.Print(cmd, output.TermbaseRelateOutput{
				Relation: relationEntry(rel),
				DBPath:   dbPath,
			})
		},
	}

	cmd.Flags().String("note", "", "free-form note on the relation")
	cmd.Flags().String("valid-from", "", "start of the validity interval (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().String("valid-to", "", "end of the validity interval, exclusive (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringArray("tag", nil, "validity tag as key=value (repeatable, e.g. --tag market=dach)")

	return cmd
}

func (a *App) newTermbaseRelationsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relations [concept-id]",
		Short: "List relations between concepts",
		Long: `List the relations persisted in the termbase.

With a concept ID, only relations touching that concept are listed, in either
direction. Without --as-of and --tag, no validity filtering is applied and
every relation is shown. --as-of and --tag build a scope: only relations
whose validity matches the scope are listed (--tag alone evaluates at the
current time).`,
		Example: `  kapi termbase relations
  kapi termbase relations old-name
  kapi termbase relations old-name --as-of 2025-12-01 --tag market=dach`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := scopeFromFlags(cmd)
			if err != nil {
				return err
			}

			tb, _, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			var rels []termbase.ConceptRelation
			if len(args) == 1 {
				rels, err = tb.RelationsOf(cmd.Context(), args[0], scope)
			} else {
				rels, err = tb.ListRelations(cmd.Context(), scope)
			}
			if err != nil {
				return fmt.Errorf("list relations: %w", err)
			}

			entries := make([]output.TermbaseRelationEntry, len(rels))
			for i, r := range rels {
				entries[i] = relationEntry(r)
			}
			return output.Print(cmd, output.TermbaseRelationsOutput{
				Relations: entries,
				Total:     len(entries),
			})
		},
	}

	cmd.Flags().String("as-of", "", "evaluate validity as of this time (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringArray("tag", nil, "scope tag as key=value (repeatable, e.g. --tag market=dach)")

	return cmd
}

func (a *App) newTermbaseUnrelateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "unrelate <relation-id>",
		Short:   "Remove a relation between concepts",
		Example: "  kapi termbase unrelate 4f7c2d1e-9a3b-4c8d-b1e2-0f6a5d4c3b2a",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tb, dbPath, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			if err := tb.DeleteRelation(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("remove relation: %w", err)
			}

			if a.Quiet {
				return nil
			}
			return output.Print(cmd, output.TermbaseUnrelateOutput{
				RelationID: args[0],
				DBPath:     dbPath,
			})
		},
	}
}

func (a *App) newTermbaseShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <concept-id>",
		Short: "Show a concept in full: definition, terms, and relations",
		Long: `Show the full detail of one concept: its definition, domain, and source,
its terms per locale with status and validity, and its relations in both
directions. Related concepts are labeled with their preferred term in the
locale given by --source-locale.`,
		Example: `  kapi termbase show old-name
  kapi termbase show old-name -s de --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")

			tb, _, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			c, ok, err := tb.GetConcept(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get concept: %w", err)
			}
			if !ok {
				return fmt.Errorf("concept not found: %s", args[0])
			}

			rels, err := tb.RelationsOf(cmd.Context(), c.ID, nil)
			if err != nil {
				return fmt.Errorf("list relations: %w", err)
			}

			terms := make([]output.TermbaseShowTerm, len(c.Terms))
			for i, t := range c.Terms {
				terms[i] = output.TermbaseShowTerm{
					Text:         t.Text,
					Locale:       string(t.Locale),
					Status:       string(t.Status),
					PartOfSpeech: t.PartOfSpeech,
					Note:         t.Note,
					Validity:     t.Validity,
				}
			}

			relations := make([]output.TermbaseShowRelation, len(rels))
			for i, r := range rels {
				direction, otherID := "outgoing", r.TargetID
				if r.TargetID == c.ID && r.SourceID != c.ID {
					direction, otherID = "incoming", r.SourceID
				}
				entry := output.TermbaseShowRelation{
					ID:           r.ID,
					Direction:    direction,
					RelationType: r.RelationType,
					ConceptID:    otherID,
					Note:         r.Note,
					Validity:     r.Validity,
				}
				other, ok, err := tb.GetConcept(cmd.Context(), otherID)
				if err != nil {
					return fmt.Errorf("get concept: %w", err)
				}
				if ok {
					entry.ConceptTerm = conceptDisplayTerm(other, model.LocaleID(srcLocale))
				}
				relations[i] = entry
			}

			return output.Print(cmd, output.TermbaseShowOutput{
				ID:         c.ID,
				Domain:     c.Domain,
				Definition: c.Definition,
				Source:     string(c.Source),
				Properties: c.Properties,
				CreatedAt:  c.CreatedAt,
				UpdatedAt:  c.UpdatedAt,
				Terms:      terms,
				Relations:  relations,
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "locale used to label related concepts")

	return cmd
}

// relationTypeArg maps a CLI relation argument — lower-kebab (use-instead) or
// upper label (USE_INSTEAD) — onto the graph.Label* vocabulary.
func relationTypeArg(s string) (string, error) {
	label := strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
	if !termbase.KnownRelationType(label) {
		return "", fmt.Errorf("unknown relation %q (use one of: %s)", s, relationArgForms)
	}
	return label, nil
}

// parseValidityTime parses a --valid-from/--valid-to/--as-of value: a full
// RFC3339 timestamp, or a plain date interpreted as midnight UTC.
func parseValidityTime(flag, value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--%s: invalid time %q (use RFC3339 or YYYY-MM-DD)", flag, value)
}

// parseTagFlags parses repeated --tag key=value flags into a tag map.
func parseTagFlags(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	tags := make(map[string]string, len(values))
	for _, v := range values {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--tag: expected key=value, got %q", v)
		}
		tags[k] = val
	}
	return tags, nil
}

// validityFromFlags builds a relation validity from --valid-from, --valid-to,
// and --tag. Returns nil when none are set (the relation is always valid).
func validityFromFlags(cmd *cobra.Command) (*graph.Validity, error) {
	from, _ := cmd.Flags().GetString("valid-from")
	to, _ := cmd.Flags().GetString("valid-to")
	tagFlags, _ := cmd.Flags().GetStringArray("tag")

	tags, err := parseTagFlags(tagFlags)
	if err != nil {
		return nil, err
	}
	if from == "" && to == "" && len(tags) == 0 {
		return nil, nil
	}

	v := &graph.Validity{Tags: tags}
	if from != "" {
		t, err := parseValidityTime("valid-from", from)
		if err != nil {
			return nil, err
		}
		v.ValidFrom = &t
	}
	if to != "" {
		t, err := parseValidityTime("valid-to", to)
		if err != nil {
			return nil, err
		}
		v.ValidTo = &t
	}
	return v, nil
}

// scopeFromFlags builds a validity scope from --as-of and --tag. Returns nil
// when neither is set, meaning no validity filtering at all — distinct from a
// scope at the current time, which hides expired and not-yet-active edges.
func scopeFromFlags(cmd *cobra.Command) (*graph.Scope, error) {
	asOf, _ := cmd.Flags().GetString("as-of")
	tagFlags, _ := cmd.Flags().GetStringArray("tag")

	tags, err := parseTagFlags(tagFlags)
	if err != nil {
		return nil, err
	}
	if asOf == "" && len(tags) == 0 {
		return nil, nil
	}

	scope := graph.Scope{At: time.Now(), Tags: tags}
	if asOf != "" {
		t, err := parseValidityTime("as-of", asOf)
		if err != nil {
			return nil, err
		}
		scope.At = t
	}
	return &scope, nil
}

// relationEntry converts a termbase relation into its output form.
func relationEntry(rel termbase.ConceptRelation) output.TermbaseRelationEntry {
	return output.TermbaseRelationEntry{
		ID:           rel.ID,
		SourceID:     rel.SourceID,
		TargetID:     rel.TargetID,
		RelationType: rel.RelationType,
		Note:         rel.Note,
		Validity:     rel.Validity,
		CreatedAt:    rel.CreatedAt,
	}
}

// conceptDisplayTerm picks a human label for a concept: the preferred (or
// first usable) term in the given locale, falling back to the concept's first
// term in any locale.
func conceptDisplayTerm(c termbase.Concept, locale model.LocaleID) string {
	if t := c.PreferredTerm(locale); t != nil {
		return t.Text
	}
	if len(c.Terms) > 0 {
		return c.Terms[0].Text
	}
	return ""
}
