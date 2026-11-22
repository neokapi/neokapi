package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// scanItemBlocks is how many blocks of one item the sample reads at most. An
// item contributes termcandidate.DefaultItemRunes of text, which a few dozen
// blocks fill.
const scanItemBlocks = 40

// scanProject is a project a context scan reads: its collections at their
// coordinates, and the collection each item sits in.
type scanProject struct {
	project *platstore.Project
	stream  string
	cols    []termcandidate.Collection
	// collectionOf names the collection an item sits in, "" for one no
	// declared collection holds.
	collectionOf map[string]string
}

// CandidateReach is the reach of one candidate term over the whole project,
// and the coordinates it was scoped to.
type CandidateReach struct {
	Term        string            `json:"term"`
	Blocks      int               `json:"blocks"`
	Collections []CollectionReach `json:"collections,omitempty"`
	At          map[string]string `json:"at,omitempty"`
}

// CollectionReach is the blocks of one collection that use a term.
type CollectionReach struct {
	Name   string `json:"name"`
	Blocks int    `json:"blocks"`
}

// loadScanProject reads the project a scan names, refusing one outside the
// scan's workspace.
func loadScanProject(ctx context.Context, cs platstore.ContentStore, workspaceID, projectID string) (*scanProject, error) {
	if cs == nil {
		return nil, errors.New("the worker has no content store")
	}
	project, err := cs.GetProject(ctx, projectID)
	if err != nil || project == nil {
		return nil, errors.New("the project could not be read")
	}
	if workspaceID == "" || project.WorkspaceID != workspaceID {
		return nil, errors.New("the project could not be read")
	}
	sp := &scanProject{project: project, stream: project.DefaultStream, collectionOf: map[string]string{}}
	collections, err := cs.ListCollections(ctx, project.ID, sp.stream)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	items, err := cs.ListItems(ctx, project.ID, sp.stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	index := map[string]int{}
	for _, col := range collections {
		if col == nil || col.Name == "" || !venue.IsRecipeOwned(col.Owner) {
			continue
		}
		index[col.ID] = len(sp.cols)
		sp.cols = append(sp.cols, termcandidate.Collection{Name: col.Name, Coordinates: col.Context})
	}
	var loose []string
	for _, it := range items {
		if it == nil {
			continue
		}
		i, ok := index[it.CollectionID]
		if !ok {
			loose = append(loose, it.Name)
			continue
		}
		sp.cols[i].Items = append(sp.cols[i].Items, it.Name)
		sp.collectionOf[it.Name] = sp.cols[i].Name
	}
	if len(loose) > 0 {
		sp.cols = append(sp.cols, termcandidate.Collection{Items: loose})
	}
	return sp, nil
}

// sample draws the term-extract sample from every collection of the project.
func (sp *scanProject) sample(ctx context.Context, cs platstore.ContentStore) ([]termcandidate.Segment, error) {
	translatable := true
	fetch := func(ctx context.Context, item string, maxRunes int) (string, error) {
		blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: sp.project.ID, Stream: sp.stream, ItemName: item,
			Translatable: &translatable, Order: platstore.BlockOrderDocument, Limit: scanItemBlocks,
		})
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		runes := 0
		for _, b := range blocks {
			if b == nil || b.Block == nil {
				continue
			}
			text := strings.TrimSpace(b.SourceText())
			if text == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n")
				runes++
			}
			sb.WriteString(text)
			runes += utf8.RuneCountInString(text)
			if runes >= maxRunes {
				break
			}
		}
		return sb.String(), nil
	}
	return termcandidate.Sample(ctx, sp.cols, fetch, termcandidate.SampleOptions{
		TotalRunes: contextScanTermChunkRunes * contextScanMaxTermChunks,
	})
}

// count counts the blocks of the whole stream that use each candidate,
// walking in batches and reading only the blocks that contain one.
func (sp *scanProject) count(ctx context.Context, cs platstore.ContentStore, candidates []string) (*termcandidate.Counter, error) {
	counter := termcandidate.NewCounter(sourceLocale(sp.project), candidates)
	if len(counter.Terms()) == 0 {
		return counter, nil
	}
	translatable := true
	err := platstore.EachBlockBatch(ctx, cs, platstore.BlockQuery{
		ProjectID: sp.project.ID, Stream: sp.stream, Translatable: &translatable, TextAny: counter.Terms(),
	}, 0, func(batch []*venue.StoredBlock) error {
		for _, b := range batch {
			if b == nil || b.Block == nil {
				continue
			}
			counter.Add(sp.collectionOf[b.ItemName], b.SourceText())
		}
		return nil
	})
	return counter, err
}

// fileProjectCandidates counts each candidate over the project, scopes it to
// the coordinates its uses share, and files it as a proposed concept of the
// project in the workspace terms store. It returns the reach of every
// candidate, filed or not.
func fileProjectCandidates(ctx context.Context, deps *ContextScanWorkerDeps, job *ContextScanJob, sp *scanProject, entries []tools.TermEntry) ([]CandidateReach, error) {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Term
	}
	counter, err := sp.count(ctx, deps.ContentStore, names)
	if err != nil {
		return nil, fmt.Errorf("count term use: %w", err)
	}
	cands := make([]termcandidate.Candidate, 0, len(entries))
	out := make([]CandidateReach, 0, len(entries))
	for _, e := range entries {
		reach := counter.Reach(e.Term)
		at := termcandidate.Scope(reach, sp.cols)
		cands = append(cands, termcandidate.Candidate{
			Term: e.Term, Definition: e.Definition, Domain: e.Domain, Reach: reach, Coordinates: at,
		})
		cr := CandidateReach{Term: e.Term, Blocks: reach.Blocks, At: at}
		for _, cc := range reach.Collections {
			cr.Collections = append(cr.Collections, CollectionReach{Name: cc.Name, Blocks: cc.Blocks})
		}
		out = append(out, cr)
	}
	if deps.TermsResolver == nil {
		return out, errors.New("the worker has no terms store; the candidates were not filed")
	}
	tb, err := deps.TermsResolver.GetTB(job.WorkspaceSlug)
	if err != nil || tb == nil {
		return out, fmt.Errorf("open the terms store: %w", err)
	}
	// Re-read the project for the rejections: a person may have rejected a
	// candidate while the scan ran.
	rejected := termcandidate.Rejected(sp.project.Properties)
	if fresh, err := deps.ContentStore.GetProject(ctx, sp.project.ID); err == nil && fresh != nil {
		rejected = termcandidate.Rejected(fresh.Properties)
	}
	res, err := termcandidate.File(ctx, tb, sp.project.ID, sourceLocale(sp.project), cands, rejected,
		termcandidate.ScanIdentity, time.Now().UTC())
	if err != nil {
		return out, err
	}
	emitContextScanLog(deps, job.ID, "info",
		fmt.Sprintf("Filed %d candidate terms (%d updated, %d already terms, %d rejected before)",
			len(res.Filed), len(res.Refreshed), len(res.Existing), len(res.Rejected)), nil)
	return out, nil
}

// sourceLocale is the project's source language, English when it names none.
func sourceLocale(p *platstore.Project) model.LocaleID {
	if p.DefaultSourceLanguage != "" {
		return p.DefaultSourceLanguage
	}
	return model.LocaleEnglish
}

// readScanProject loads the project a scan names and draws its sample.
func readScanProject(ctx context.Context, deps *ContextScanWorkerDeps, job *ContextScanJob, projectID string) (*scanProject, []termcandidate.Segment, error) {
	sp, err := loadScanProject(ctx, deps.ContentStore, job.WorkspaceID, projectID)
	if err != nil {
		return nil, nil, err
	}
	segments, err := sp.sample(ctx, deps.ContentStore)
	if err != nil {
		return sp, nil, fmt.Errorf("sample the project: %w", err)
	}
	return sp, segments, nil
}
