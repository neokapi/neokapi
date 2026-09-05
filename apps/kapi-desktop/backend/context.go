package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextgraph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/occurrence"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// The context explorer's backend: the three questions the explorer asks at a
// point, answered for the project a tab has open.
//
// Every answer comes from the same resolution the CLI and a run go through —
// project.ResolveGovernanceFor for the point, host.ContextSearchSourcesFor for
// the stores, host.SearchContext for by-content retrieval — so the desktop
// cannot show a different context from the one a `kapi check` would enforce.
//
// The dimensions are pinned here: a desktop tab holds one project on one
// stream, so workspace/project/stream never move and the answers carry no
// cross-project reach. That is a property of this mount, not of the questions.

// contextExplorerTimeout caps one explorer read. The panes are interactive, and
// a retrieval that has not answered in this long is better reported than waited
// on.
const contextExplorerTimeout = 30 * time.Second

// ContextPointDTO is the point a question resolved to.
type ContextPointDTO struct {
	Path       string `json:"path,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Collection string `json:"collection,omitempty"`
	// Ref is the recipe key the governing binding was declared on.
	Ref string `json:"ref,omitempty"`
	// Default is true when nothing finer than the project's default point
	// governs here.
	Default bool `json:"default"`
}

// ContextVoiceDTO is the voice profile in force at a point.
type ContextVoiceDTO struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	Field  string `json:"field,omitempty"`
	Guide  string `json:"guide,omitempty"`
}

// ContextGovernsResult answers "what governs here".
//
// Concepts are the project's vocabulary as the terms store holds it, not a
// re-modelled projection: the frontend already renders this shape, so the term
// status vocabulary is defined once.
type ContextGovernsResult struct {
	Point         ContextPointDTO          `json:"point"`
	Voice         *ContextVoiceDTO         `json:"voice,omitempty"`
	Concepts      []ConceptDTO             `json:"concepts"`
	ConceptsTotal int                      `json:"concepts_total"`
	Profiles      []host.ContextProfileHit `json:"profiles"`
	Notes         []string                 `json:"notes"`
}

// ContextCollectionDTO is one collection at or under a point.
type ContextCollectionDTO struct {
	Name    string `json:"name"`
	Channel string `json:"channel,omitempty"`
	Profile string `json:"profile,omitempty"`
	Blocks  int    `json:"blocks"`
}

// ContextCoverageDTO is one locale's coverage at a point.
type ContextCoverageDTO struct {
	Locale  string `json:"locale"`
	Units   int    `json:"units"`
	Covered int    `json:"covered"`
}

// ContextItemDTO is one content item at a point.
type ContextItemDTO struct {
	Hash       string `json:"hash"`
	BlockID    string `json:"block_id,omitempty"`
	Document   string `json:"document,omitempty"`
	Collection string `json:"collection,omitempty"`
	Locale     string `json:"locale,omitempty"`
	Text       string `json:"text,omitempty"`
}

// ContextLivesResult answers "what lives here".
type ContextLivesResult struct {
	Point       ContextPointDTO        `json:"point"`
	Collections []ContextCollectionDTO `json:"collections"`
	Coverage    []ContextCoverageDTO   `json:"coverage"`
	Items       []ContextItemDTO       `json:"items"`
	ItemsTotal  int                    `json:"items_total"`
	Notes       []string               `json:"notes"`
}

// ContextBlessingDTO is a decision that blessed a unit, and the source it cited.
type ContextBlessingDTO struct {
	Unit       string `json:"unit"`
	Variant    string `json:"variant,omitempty"`
	Status     string `json:"status,omitempty"`
	TargetHash string `json:"target_hash,omitempty"`
}

// ContextRelatesResult answers "how it relates", at project reach.
//
// Reach is stated rather than implied: a local project holds its own subgraph
// only, so "which projects use this concept" has exactly one honest answer here
// and the caller must be able to tell that from "no project uses it".
//
// Occurrences are the context graph's recorded uses, one row per (block, term,
// language) with its count, the shape the platform's relations pane reads for
// the same concept, so the desktop and the platform answer "how often" from
// the same edges.
type ContextRelatesResult struct {
	Subject          string               `json:"subject"`
	Reach            string               `json:"reach"`
	Occurrences      []host.ContextUse    `json:"occurrences"`
	OccurrencesTotal int                  `json:"occurrences_total"`
	Blessings        []ContextBlessingDTO `json:"blessings"`
	Notes            []string             `json:"notes"`
}

// ContextSearchResult is the grouped by-content answer plus the content group
// the shared retrieval surface does not carry.
//
// host.ContextSearchResult answers "what do we know about this word?" from the
// terms store and the content memory. Where the word actually appears is a
// different question over a third store, so it rides beside the shared answer
// rather than being folded into one of its groups.
type ContextSearchResult struct {
	Answer  *host.ContextSearchResult `json:"answer"`
	Content []ContextItemDTO          `json:"content"`
}

// ContextOptionDTO is one value a dimension can take under the current scope.
type ContextOptionDTO struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count,omitempty"`
}

// contextExplorerSources binds the stores and the recipe for one explorer read.
//
// It threads the tab's own recipe path through the project flag, so the
// flag-free openers resolve to the project this tab has open rather than to
// whatever an upward walk from the process's cwd would find.
func (a *App) contextExplorerSources(
	ctx context.Context,
	op *openProject,
) (host.ContextSearchSources, func(), error) {
	cmd, err := a.contextCommand(ctx, op)
	if err != nil {
		return host.ContextSearchSources{}, func() {}, err
	}
	src, cleanup := a.hostEngine().ContextSearchSourcesFor(cmd, "", "")
	return src, cleanup, nil
}

// contextCommand carries the tab's own recipe path on the project flag, so the
// flag-free openers resolve to the project this tab has open rather than to
// whatever an upward walk from the process's working directory would find.
func (a *App) contextCommand(ctx context.Context, op *openProject) (host.Command, error) {
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	cmd := host.NewEnvCommand(ctx, "context-explorer")
	host.AddProjectFlag(cmd)
	host.AddResourceFlags(cmd)
	if err := cmd.Flags().Set("project", op.Path); err != nil {
		return nil, fmt.Errorf("bind project: %w", err)
	}
	return cmd, nil
}

// contextPoint resolves the governance in force at (collection, path) and
// renders it as the point the panes name.
func contextPoint(
	proj *project.KapiProject,
	collection, relPath string,
	at time.Time,
) (ContextPointDTO, *project.ResolvedGovernance, error) {
	pt := project.GovernancePoint{Collection: collection, Path: relPath, At: at}
	rc, err := proj.ResolveGovernanceFor(pt)
	if err != nil {
		return ContextPointDTO{}, nil, err
	}
	ref := rc.Ref()
	return ContextPointDTO{
		Path:       relPath,
		Profile:    ref.Profile,
		Channel:    ref.Channel,
		Collection: collection,
		Ref:        rc.VoiceField,
		Default:    ref.Profile == "",
	}, rc, nil
}

// ContextAt answers "what governs here" for one FILE, through the same
// by-location resolution `kapi context <path>` and the `context://` resource
// serve.
//
// A file is the finest declared point, and it is the form the retrieval surface
// owns: one function assembles the answer, so the desktop cannot report a
// context the CLI would describe differently. The coarser forms — a collection,
// a bare project — have no location to resolve and go through ContextGoverns.
func (a *App) ContextAt(tabID, relPath string, limit int) (*host.ContextAnswer, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	if relPath == "" {
		return nil, errors.New("no path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	cmd, err := a.contextCommand(ctx, op)
	if err != nil {
		return nil, err
	}
	// The request resolves a path against the process's working directory, which
	// is not the project root for a desktop tab.
	req := host.ContextPointRequest{
		Path:  filepath.Join(filepath.Dir(op.Path), relPath),
		Limit: limit,
	}
	src, cleanup := a.hostEngine().ContextSourcesAt(cmd, req)
	defer cleanup()
	return host.ResolveContextAt(ctx, src, req)
}

// ContextGoverns answers "what governs here" for one point in the open project.
//
// The point resolves finest-first exactly as a run resolves it, and a binding
// skipped because its window had closed is reported in Notes rather than
// quietly replaced — governance changing on a date has to be visible.
func (a *App) ContextGoverns(tabID, collection, relPath string, limit int) (*ContextGovernsResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	src, cleanup, err := a.contextExplorerSources(ctx, op)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	point, rc, err := contextPoint(op.Project, collection, relPath, src.At)
	if err != nil {
		return nil, fmt.Errorf("resolve governance: %w", err)
	}

	out := &ContextGovernsResult{
		Point:    point,
		Profiles: src.Profiles,
		Notes:    append([]string{}, src.Freshness...),
		Concepts: []ConceptDTO{},
	}
	if note := a.hostEngine().GovernanceNote(rc); note != "" {
		out.Notes = append(out.Notes, note)
	}

	root := filepath.Dir(op.Path)
	if profile, _, source, ok, verr := a.hostEngine().LoadCollectionVoice(
		ctx, op.Project, root,
		host.VoiceResolveOptions{Point: project.GovernancePoint{Collection: collection, Path: relPath, At: src.At}},
	); verr == nil && ok && profile != nil {
		out.Voice = &ContextVoiceDTO{
			Name:   profile.Name,
			Source: relSource(root, source),
			Field:  rc.VoiceField,
			// The rendered guide, the same text the retrieval surface serves
			// (host.ContextVoice) and an AI proposal is steered by. The
			// description alone names the profile without saying how it sounds.
			Guide: coreprofile.RenderVoiceGuide(profile),
		}
	} else if verr != nil {
		out.Notes = append(out.Notes, fmt.Sprintf("voice profile: %v", verr))
	}

	switch {
	case src.TermsErr != nil:
		out.Notes = append(out.Notes, fmt.Sprintf("terms: %v", src.TermsErr))
	case src.Terms == nil:
		out.Notes = append(out.Notes, "this project binds no terms store")
	default:
		if limit <= 0 {
			limit = host.DefaultContextSearchLimit
		}
		concepts, total, terr := src.Terms.Search(ctx, "", "", "", 0, limit)
		if terr != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("terms: %v", terr))
			break
		}
		out.ConceptsTotal = total
		for _, c := range concepts {
			out.Concepts = append(out.Concepts, conceptToDTO(c))
		}
	}

	return out, nil
}

// ContextLives answers "what lives here": the collections at or under the
// point, per-locale coverage, and the content items themselves.
//
// Ship states and staleness are absent from a local answer on purpose. Both are
// derived from the decision ledger against a stream's current source hashes,
// which is a workspace question; a local pane showing a blank badge is honest,
// one showing a guessed badge is not.
func (a *App) ContextLives(tabID, collection, relPath string, limit int) (*ContextLivesResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	at := a.hostEngine().GovernanceInstant()
	point, _, err := contextPoint(op.Project, collection, relPath, at)
	if err != nil {
		return nil, fmt.Errorf("resolve governance: %w", err)
	}
	out := &ContextLivesResult{
		Point:       point,
		Collections: []ContextCollectionDTO{},
		Coverage:    []ContextCoverageDTO{},
		Items:       []ContextItemDTO{},
		Notes:       []string{},
	}

	status, serr := a.GetProjectStatus(tabID)
	if serr != nil {
		return nil, serr
	}
	if !status.HasData {
		out.Notes = append(out.Notes, "this project has not been extracted yet, so it holds no content to place")
	}
	coverage := map[string]*ContextCoverageDTO{}
	for _, cs := range status.Collections {
		if collection != "" && cs.Name != collection {
			continue
		}
		coll := ContextCollectionDTO{Name: cs.Name, Blocks: cs.BlockCount}
		if rc, rerr := op.Project.ResolveGovernanceFor(
			project.GovernancePoint{Collection: cs.Name, At: at},
		); rerr == nil {
			ref := rc.Ref()
			coll.Channel, coll.Profile = ref.Channel, ref.Profile
		}
		out.Collections = append(out.Collections, coll)

		for _, locale := range cs.TargetLanguages {
			row, ok := coverage[locale]
			if !ok {
				row = &ContextCoverageDTO{Locale: locale}
				coverage[locale] = row
			}
			row.Units += cs.BlockCount
			row.Covered += cs.Coverage[locale]
		}
	}
	for _, locale := range sortedKeys(coverage) {
		out.Coverage = append(out.Coverage, *coverage[locale])
	}

	if limit <= 0 {
		limit = host.DefaultContextSearchLimit
	}
	items, total, ierr := a.contextItems(ctx, op, collection, relPath, limit)
	if ierr != nil {
		out.Notes = append(out.Notes, ierr.Error())
	} else {
		out.Items, out.ItemsTotal = items, total
	}
	return out, nil
}

// contextItems reads the blocks stored at a point. A path narrows to one
// document; a bare collection lists what the collection holds.
func (a *App) contextItems(
	ctx context.Context,
	op *openProject,
	collection, relPath string,
	limit int,
) ([]ContextItemDTO, int, error) {
	root := filepath.Dir(op.Path)
	db, err := a.hostEngine().ProjectDB(ctx, root)
	if err != nil {
		return nil, 0, fmt.Errorf("open project store: %w", err)
	}
	sess, err := db.Blocks().Begin(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("read blocks: %w", err)
	}
	defer func() { _ = sess.Close() }()

	items := make([]ContextItemDTO, 0, limit)
	total := 0
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: collection}) {
		if berr != nil {
			return nil, 0, fmt.Errorf("read blocks: %w", berr)
		}
		if relPath != "" && !documentMatches(b.Properties.File, relPath) {
			continue
		}
		total++
		if len(items) >= limit {
			continue
		}
		for _, text := range blockstore.BlockTexts(b) {
			items = append(items, ContextItemDTO{
				Hash:       b.Hash,
				BlockID:    b.ID,
				Document:   b.Properties.File,
				Collection: collection,
				Locale:     text.Locale,
				Text:       text.Text,
			})
			if len(items) >= limit {
				break
			}
		}
	}
	return items, total, nil
}

// documentMatches answers whether a stored document sits at or under a path.
// Both a file and a directory are legitimate points, so a prefix match on a
// segment boundary is the rule — never a bare string prefix, which would place
// `help/billing-old.md` under `help/billing`.
func documentMatches(document, point string) bool {
	document = filepath.ToSlash(document)
	point = strings.Trim(filepath.ToSlash(point), "/")
	if point == "" {
		return true
	}
	return document == point || strings.HasPrefix(document, point+"/")
}

// ContextRelates answers "how it relates" for a term, a concept or a block.
//
// A subject that names a term or concept answers with where the project's
// content uses it, read from the uses_term edges the up path materializes, so
// the count here is the count the search pane and the platform report and all
// three mean "as of the last extraction". A subject that names a block's
// content key answers with the decisions that blessed it, from the same graph.
func (a *App) ContextRelates(tabID, kind, subject string, limit int) (*ContextRelatesResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if subject == "" {
		return nil, errors.New("no subject")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	out := &ContextRelatesResult{
		Subject:     subject,
		Reach:       "project",
		Occurrences: []host.ContextUse{},
		Blessings:   []ContextBlessingDTO{},
		Notes:       []string{},
	}
	if limit <= 0 {
		limit = host.DefaultContextSearchLimit
	}

	if kind == "block" {
		blessings, berr := a.contextBlessings(ctx, op, subject)
		if berr != nil {
			out.Notes = append(out.Notes, berr.Error())
			return out, nil
		}
		out.Blessings = blessings
		return out, nil
	}

	src, cleanup, err := a.contextExplorerSources(ctx, op)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if src.Terms == nil {
		if src.TermsErr != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("terms: %v", src.TermsErr))
		} else {
			out.Notes = append(out.Notes, "this project binds no terms store")
		}
		return out, nil
	}
	res, err := host.FindContextUses(ctx, src, subject, limit)
	if err != nil {
		if errors.Is(err, occurrence.ErrUnknownSubject) {
			out.Notes = append(out.Notes, fmt.Sprintf("no term or concept named %q in this project", subject))
			return out, nil
		}
		return nil, err
	}
	out.Occurrences = res.Uses
	out.OccurrencesTotal = res.Total
	out.Notes = append(out.Notes, res.Notes...)
	return out, nil
}

// contextBlessings reads the decisions that blessed one block, from the
// project's own context subgraph.
func (a *App) contextBlessings(ctx context.Context, op *openProject, contentKey string) ([]ContextBlessingDTO, error) {
	root := filepath.Dir(op.Path)
	g, err := a.hostEngine().ProjectGraph(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("open project graph: %w", err)
	}
	scope := host.ProjectScope(op.Project)
	found, err := contextgraph.BlessingsOfBlock(ctx, g, scope, contentKey)
	if err != nil {
		return nil, fmt.Errorf("read decisions: %w", err)
	}
	out := make([]ContextBlessingDTO, 0, len(found))
	for _, b := range found {
		out = append(out, ContextBlessingDTO{
			Unit:       b.Unit,
			Variant:    b.Variant,
			Status:     b.Status,
			TargetHash: b.TargetHash,
		})
	}
	return out, nil
}

// ContextSearch answers a by-content question over every store the project
// binds: its terms, its content memory, and its extracted content.
//
// The groups stay separate. A term match scores on lexical closeness to a
// concept and a memory match on segment similarity; the two numbers are not
// comparable, so one ranking over both would impose an order that means nothing.
func (a *App) ContextSearch(tabID, query, locale string, limit int) (*ContextSearchResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("empty query")
	}
	loc, lerr := canonicalLocale(locale)
	if lerr != nil {
		return nil, lerr
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	src, cleanup, err := a.contextExplorerSources(ctx, op)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	answer, err := host.SearchContext(ctx, src, host.ContextSearchRequest{
		Query:  query,
		Locale: loc,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	out := &ContextSearchResult{Answer: answer, Content: []ContextItemDTO{}}
	if src.Blocks == nil {
		return out, nil
	}
	if limit <= 0 {
		limit = host.DefaultContextSearchLimit
	}
	hits, herr := blockstore.SearchText(ctx, src.Blocks, query, blockstore.TextSearchOptions{Limit: limit})
	if herr != nil {
		answer.Notes = append(answer.Notes, fmt.Sprintf("content: %v", herr))
		return out, nil
	}
	for _, hit := range hits {
		item := ContextItemDTO{
			Hash:       hit.Hash,
			Collection: hit.Collection,
			Locale:     hit.Locale,
			Text:       hit.Text,
		}
		if hit.Block != nil {
			item.BlockID = hit.Block.ID
			item.Document = hit.Block.Properties.File
		}
		out.Content = append(out.Content, item)
	}
	return out, nil
}

// ContextOptions lists the values one dimension can take in the open project.
//
// Workspace, project and stream are pinned by this mount, so they have no
// options: a tab is one project on one stream, and offering a picker that
// cannot move would misdescribe the surface.
func (a *App) ContextOptions(tabID, dimension string) ([]ContextOptionDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return []ContextOptionDTO{}, nil
	}
	at := a.hostEngine().GovernanceInstant()
	out := []ContextOptionDTO{}

	switch dimension {
	case "collection":
		for _, coll := range op.Project.Collections {
			if coll.Name == "" {
				continue
			}
			out = append(out, ContextOptionDTO{Value: coll.Name})
		}
	case "locale":
		// A collection that declares no target languages inherits the project's,
		// which is the ordinary shape: a recipe usually states them once.
		seen := map[string]bool{}
		add := func(locales []model.LocaleID) {
			for _, locale := range locales {
				code := string(locale)
				if code == "" || seen[code] {
					continue
				}
				seen[code] = true
				out = append(out, ContextOptionDTO{Value: code})
			}
		}
		for _, coll := range op.Project.Collections {
			if len(coll.TargetLanguages) == 0 {
				add(op.Project.Defaults.TargetLanguages)
				continue
			}
			add(coll.TargetLanguages)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	case "path":
		// By-location is the primitive `kapi context <path>` serves, and a
		// reader browsing it should not have to type a path from memory. The
		// options are the files the recipe's own globs claim, project-relative
		// and labelled by the collection that claims them.
		matches, merr := a.MatchContent(tabID)
		if merr != nil {
			return nil, merr
		}
		seen := map[string]bool{}
		for _, m := range matches {
			rel := filepath.ToSlash(m.Relative)
			if rel == "" || seen[rel] {
				continue
			}
			seen[rel] = true
			out = append(out, ContextOptionDTO{Value: rel, Label: m.Collection})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	case "coordinate":
		seen := map[string]bool{}
		for _, coll := range op.Project.Collections {
			rc, err := op.Project.ResolveGovernanceFor(project.GovernancePoint{Collection: coll.Name, At: at})
			if err != nil {
				continue
			}
			point := rc.Ref().String()
			if point == "" || seen[point] {
				continue
			}
			seen[point] = true
			out = append(out, ContextOptionDTO{Value: point})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	}
	return out, nil
}

// sortedKeys returns a map's keys in ascending order, so a rendered list is
// stable between reads.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
