package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

const (
	// termsPullPageSize is the page size used when paginating the workspace
	// concept search during a terms pull.
	termsPullPageSize = 200

	// termsPullRelationConcurrency bounds the number of concurrent relation
	// fetches issued while snapshotting the workspace graph, so a large
	// workspace is pulled in O(pages + concepts/concurrency) round-trips rather
	// than one serial request per concept.
	termsPullRelationConcurrency = 8
)

// newTermsCmd builds the `kapi terms` command tree, whose `pull` subcommand
// snapshots the workspace knowledge graph into the project's bound termbase.
func newTermsCmd() *cobra.Command {
	termsCmd := &cobra.Command{
		Use:   "terms",
		Short: "Work with the project's governed terminology",
		Long: `Work with the project's governed terminology.

The workspace brand knowledge graph is the source of truth for terms; 'terms
pull' snapshots it into the project's local termbase so terminology gates run
offline.`,
	}

	pullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Snapshot the workspace terminology into the local termbase",
		Long: `Snapshot the workspace brand knowledge graph into the project's bound
termbase (.kapi/termbase.db by default).

Every governed concept (brand vocabulary and terminology) is fetched from the
bowrain server, with its typed relations, and written into the local termbase,
refreshing any concept already present. After a pull, 'kapi verify --terms'
gates offline against the same governed terminology — this is the CI gating
loop: pull the truth once, then verify without a network round-trip.

The project must be claimed into a workspace and you must be authenticated
(run 'kapi auth login', or set BOWRAIN_AUTH_TOKEN in CI).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			dbPath, err := projectTermbasePath(proj)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				return fmt.Errorf("create termbase directory: %w", err)
			}

			tb, err := termbase.NewSQLiteTermBase(dbPath)
			if err != nil {
				return fmt.Errorf("open termbase: %w", err)
			}
			defer tb.Close()

			result, err := pullWorkspaceTerms(cmd.Context(), client, tb)
			if err != nil {
				return err
			}

			return output.Print(cmd, output.TermsPullOutput{
				DBPath:    dbPath,
				Workspace: proj.Recipe.Server.Workspace(),
				Server:    proj.Recipe.Server.ServerURL(),
				Concepts:  result.concepts,
				Terms:     result.terms,
				Relations: result.relations,
			})
		},
	}

	termsCmd.AddCommand(pullCmd)
	return termsCmd
}

// termsPullResult holds the counts reported by a terms pull.
type termsPullResult struct {
	concepts  int
	terms     int
	relations int
}

// pullWorkspaceTerms paginates the workspace concept search, writes every
// concept into tb (refreshing by concept ID), then writes the typed relations
// touching those concepts. Relation fetching is a second pass over the pulled
// concept IDs so the per-concept round-trips run as a bounded parallel fan-out
// rather than serially inside the paging loop, and relations are added only
// once every concept is present so the termbase's referential check (both
// endpoints must exist) is satisfied.
func pullWorkspaceTerms(ctx context.Context, client *apiclient.BowrainClient, tb termbase.TermBase) (termsPullResult, error) {
	var res termsPullResult

	known := map[string]bool{}
	var conceptIDs []string

	offset := 0
	for {
		page, err := client.ListConcepts(ctx, apiclient.ListConceptsParams{
			Offset: offset,
			Limit:  termsPullPageSize,
		})
		if err != nil {
			return termsPullResult{}, fmt.Errorf("list workspace concepts: %w", err)
		}
		if len(page.Concepts) == 0 {
			break
		}

		for _, ci := range page.Concepts {
			concept := conceptInfoToConcept(ci)
			if err := tb.AddConcept(ctx, concept); err != nil {
				return termsPullResult{}, fmt.Errorf("write concept %s: %w", ci.ID, err)
			}
			known[concept.ID] = true
			conceptIDs = append(conceptIDs, concept.ID)
			res.concepts++
			res.terms += len(concept.Terms)
		}

		offset += len(page.Concepts)
		if offset >= page.TotalCount || len(page.Concepts) < termsPullPageSize {
			break
		}
	}

	relations, err := fetchConceptRelations(ctx, client, conceptIDs)
	if err != nil {
		return termsPullResult{}, err
	}

	// Add relations once every concept is present. Skip any edge whose
	// endpoints were not pulled (the termbase rejects dangling relations).
	for _, rel := range relations {
		if !known[rel.SourceID] || !known[rel.TargetID] {
			continue
		}
		if err := tb.AddRelation(ctx, rel); err != nil {
			return termsPullResult{}, fmt.Errorf("write relation %s: %w", rel.ID, err)
		}
		res.relations++
	}

	return res, nil
}

// fetchConceptRelations fetches the typed relations touching every concept in
// conceptIDs using a bounded parallel fan-out, and returns them de-duplicated
// by relation ID (each edge is reported on both of its concept endpoints).
// Results are sorted by relation ID so the pull stays deterministic despite the
// concurrent fetch.
func fetchConceptRelations(ctx context.Context, client *apiclient.BowrainClient, conceptIDs []string) ([]termbase.ConceptRelation, error) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(termsPullRelationConcurrency)

	var (
		mu        sync.Mutex
		seen      = map[string]bool{}
		relations []termbase.ConceptRelation
	)

	for _, id := range conceptIDs {
		g.Go(func() error {
			rels, err := client.ListConceptRelations(ctx, id, "", "")
			if err != nil {
				return fmt.Errorf("list relations for concept %s: %w", id, err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, rel := range rels {
				if rel.ID == "" || seen[rel.ID] {
					continue
				}
				seen[rel.ID] = true
				relations = append(relations, rel)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	sort.Slice(relations, func(i, j int) bool { return relations[i].ID < relations[j].ID })
	return relations, nil
}

// conceptInfoToConcept maps a server concept DTO into the framework termbase
// concept type, casting the term status/locale strings and parsing the RFC3339
// timestamps.
func conceptInfoToConcept(ci apiclient.ConceptInfo) termbase.Concept {
	concept := termbase.Concept{
		ID:         ci.ID,
		ProjectID:  ci.ProjectID,
		Domain:     ci.Domain,
		Definition: ci.Definition,
		Properties: ci.Properties,
	}
	for _, t := range ci.Terms {
		concept.Terms = append(concept.Terms, termbase.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		})
	}
	if ts, err := time.Parse(time.RFC3339, ci.CreatedAt); err == nil {
		concept.CreatedAt = ts
	}
	if ts, err := time.Parse(time.RFC3339, ci.UpdatedAt); err == nil {
		concept.UpdatedAt = ts
	}
	return concept
}

// projectTermbasePath returns the SQLite termbase file a terms pull writes to.
// It mirrors the CLI's project-bound resolution: defaults.termbase from the
// recipe (relative to the project root), else the conventional
// <root>/.kapi/termbase.db.
func projectTermbasePath(proj *project.Project) (string, error) {
	if bound := proj.Recipe.Defaults.Termbase; bound != "" {
		if filepath.IsAbs(bound) {
			return bound, nil
		}
		return filepath.Join(proj.Root, bound), nil
	}
	// Convention: the project's authoritative termbase under .kapi/.
	return filepath.Join(proj.StateDir(), "termbase.db"), nil
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(newTermsCmd()) })
}
