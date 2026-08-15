package backend

import "github.com/neokapi/neokapi/bowrain/editorclient"

// This file converts between the bowrain/core/client editor types (which mirror
// the server REST shapes, carrying canonical core/model.Run for block content)
// and the frontend-facing *Info types the Wails bindings expose. It is the one
// place the desktop translates the REST editor surface into its presentation
// model — the former bespoke gRPC editor wire encoding is gone.

// --- Workspaces ---

func editorWorkspaceToInfo(w editorclient.EditorWorkspace) WorkspaceInfo {
	return WorkspaceInfo{
		ID:          w.ID,
		Name:        w.Name,
		Slug:        w.Slug,
		Description: w.Description,
		LogoURL:     w.LogoURL,
		Role:        w.Role,
	}
}

func editorWorkspacesToInfos(ws []editorclient.EditorWorkspace) []WorkspaceInfo {
	out := make([]WorkspaceInfo, len(ws))
	for i, w := range ws {
		out[i] = editorWorkspaceToInfo(w)
	}
	return out
}

// --- Projects ---

func editorProjectItemToInfo(it editorclient.EditorProjectItem) ProjectItem {
	return ProjectItem{
		ID:         it.ID,
		Name:       it.Name,
		Format:     it.Format,
		Type:       it.Type,
		Size:       it.Size,
		BlockCount: it.BlockCount,
		WordCount:  it.WordCount,
	}
}

func editorProjectToInfo(p editorclient.EditorProject) ProjectInfo {
	items := make([]ProjectItem, len(p.Items))
	for i, it := range p.Items {
		items[i] = editorProjectItemToInfo(it)
	}
	return ProjectInfo{
		ID:                    p.ID,
		Name:                  p.Name,
		DefaultSourceLanguage: p.DefaultSourceLanguage,
		TargetLanguages:       p.TargetLanguages,
		Items:                 items,
		CreatedAt:             p.CreatedAt,
		ModifiedAt:            p.ModifiedAt,
	}
}

func editorProjectsToInfos(ps []editorclient.EditorProject) []ProjectInfo {
	out := make([]ProjectInfo, len(ps))
	for i, p := range ps {
		out[i] = editorProjectToInfo(p)
	}
	return out
}

// --- Blocks ---

func editorBlockToInfo(b editorclient.EditorBlock) BlockInfo {
	info := BlockInfo{
		ID:           b.ID,
		SourceRuns:   runsToRunInfos(b.SourceRuns),
		Translatable: b.Translatable,
		Properties:   b.Properties,
	}
	if len(b.Targets) > 0 {
		info.Targets = make(map[string]BlockTargetInfo, len(b.Targets))
		for locale, t := range b.Targets {
			info.Targets[locale] = BlockTargetInfo{Text: t.Text, Status: t.Status}
		}
	}
	if len(b.TargetRuns) > 0 {
		info.TargetRuns = make(map[string][]RunInfo, len(b.TargetRuns))
		for locale, runs := range b.TargetRuns {
			info.TargetRuns[locale] = runsToRunInfos(runs)
		}
	}
	return info
}

func editorBlocksToInfos(bs []editorclient.EditorBlock) []BlockInfo {
	out := make([]BlockInfo, len(bs))
	for i, b := range bs {
		out[i] = editorBlockToInfo(b)
	}
	return out
}

// --- Item actions ---

func editorStatsToStats(s *editorclient.EditorTranslationStats) *TranslationStats {
	if s == nil {
		return &TranslationStats{}
	}
	return &TranslationStats{
		TotalBlocks:      s.TotalBlocks,
		TranslatedBlocks: s.TranslatedBlocks,
		WordCount:        s.WordCount,
	}
}

func editorTermEnforceToResults(rs []editorclient.EditorTermEnforceResult) []TermEnforceResult {
	if len(rs) == 0 {
		return nil
	}
	out := make([]TermEnforceResult, len(rs))
	for i, r := range rs {
		out[i] = TermEnforceResult{
			BlockID:      r.BlockID,
			SourceTerm:   r.SourceTerm,
			ConceptID:    r.ConceptID,
			Expected:     r.Expected,
			SourceText:   r.SourceText,
			TargetText:   r.TargetText,
			SourceLocale: r.SourceLocale,
			TargetLocale: r.TargetLocale,
		}
	}
	return out
}

// --- content memory ---

func editorMemoryEntryToInfo(e editorclient.EditorMemoryEntry) MemoryEntryInfo {
	return MemoryEntryInfo{
		ID:           e.ID,
		Source:       e.Source,
		Target:       e.Target,
		SourceLocale: e.SourceLanguage,
		TargetLocale: e.TargetLanguage,
		UpdatedAt:    e.UpdatedAt,
	}
}

func editorMemoryResultToSearch(r *editorclient.EditorMemorySearchResult) *MemorySearchResult {
	entries := make([]MemoryEntryInfo, len(r.Entries))
	for i, e := range r.Entries {
		entries[i] = editorMemoryEntryToInfo(e)
	}
	return &MemorySearchResult{Entries: entries, TotalCount: r.TotalCount}
}

func editorMemoryMatchesToInfos(ms []editorclient.EditorMemoryMatch) []MemoryMatchInfo {
	out := make([]MemoryMatchInfo, len(ms))
	for i, m := range ms {
		out[i] = MemoryMatchInfo{Source: m.Source, Target: m.Target, Score: m.Score, MatchType: m.MatchType}
	}
	return out
}

func editorTermMatchesToBlockMatches(ms []editorclient.EditorTermMatch) []BlockTermMatch {
	out := make([]BlockTermMatch, len(ms))
	for i, m := range ms {
		out[i] = BlockTermMatch{
			SourceTerm:  m.SourceTerm,
			TargetTerms: m.TargetTerms,
			Domain:      m.Domain,
			Status:      m.Status,
			Start:       m.Start,
			End:         m.End,
		}
	}
	return out
}

// --- Terminology (concepts) ---

func termInfoToEditor(t TermInfo) editorclient.EditorTerm {
	return editorclient.EditorTerm{
		Text:         t.Text,
		Locale:       t.Locale,
		Status:       t.Status,
		PartOfSpeech: t.PartOfSpeech,
		Gender:       t.Gender,
		Note:         t.Note,
	}
}

func termInfosToEditor(ts []TermInfo) []editorclient.EditorTerm {
	out := make([]editorclient.EditorTerm, len(ts))
	for i, t := range ts {
		out[i] = termInfoToEditor(t)
	}
	return out
}

func editorTermToInfo(t editorclient.EditorTerm) TermInfo {
	return TermInfo{
		Text:         t.Text,
		Locale:       t.Locale,
		Status:       t.Status,
		PartOfSpeech: t.PartOfSpeech,
		Gender:       t.Gender,
		Note:         t.Note,
	}
}

func editorConceptToInfo(c editorclient.EditorConcept) ConceptInfo {
	terms := make([]TermInfo, len(c.Terms))
	for i, t := range c.Terms {
		terms[i] = editorTermToInfo(t)
	}
	return ConceptInfo{
		ID:         c.ID,
		Domain:     c.Domain,
		Definition: c.Definition,
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

func editorTermResultToSearch(r *editorclient.EditorTermSearchResult) *TermSearchResult {
	concepts := make([]ConceptInfo, len(r.Concepts))
	for i, c := range r.Concepts {
		concepts[i] = editorConceptToInfo(c)
	}
	return &TermSearchResult{Concepts: concepts, TotalCount: r.TotalCount}
}

// --- Providers ---

func editorProviderToInfo(c editorclient.EditorProviderConfig) ProviderConfigInfo {
	return ProviderConfigInfo{
		ID:           c.ID,
		Name:         c.Name,
		ProviderType: c.ProviderType,
		Model:        c.Model,
		BaseURL:      c.BaseURL,
	}
}

func editorProvidersToInfos(cs []editorclient.EditorProviderConfig) []ProviderConfigInfo {
	out := make([]ProviderConfigInfo, len(cs))
	for i, c := range cs {
		out[i] = editorProviderToInfo(c)
	}
	return out
}

func saveProviderReqToEditor(r SaveProviderRequest) editorclient.EditorSaveProviderRequest {
	return editorclient.EditorSaveProviderRequest{
		ID:           r.ID,
		Name:         r.Name,
		ProviderType: r.ProviderType,
		Model:        r.Model,
		BaseURL:      r.BaseURL,
		APIKey:       r.APIKey,
	}
}
