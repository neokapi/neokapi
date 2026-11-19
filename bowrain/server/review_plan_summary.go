package server

import "slices"

// The review plan in brief, for the review inbox: one step per level, each
// with what waits there and the team's progress through it, so a client can
// draw the five steps as rings without asking for the whole plan.

// planSummaryDTO is a project's plan in brief, for one caller.
type planSummaryDTO struct {
	Involvement string `json:"involvement"`
	// Next is the first level with something waiting for the caller, or ""
	// when nothing does.
	Next  string        `json:"next"`
	Steps []planStepDTO `json:"steps"`
}

// planStepDTO is one level of the plan in brief.
type planStepDTO struct {
	// Level is coordinates, key_language, exceptions, interfaces (the pages
	// a reviewer reads whole) or policy.
	Level string `json:"level"`
	// Count is what waits at the level: 1 for unconfirmed coordinates or
	// the changes since the last confirmation, the key-language decisions
	// open in the caller's languages, the rules behind the exceptions, the
	// pages to read, the translations a settle pass would establish.
	Count int `json:"count"`
	// Progress is the team's progress through the level, 0 to 100.
	Progress int  `json:"progress"`
	Done     bool `json:"done"`
	// Changed counts what changed at the level after the team finished it.
	Changed int `json:"changed,omitempty"`
	// Settle is the policy level's settle state: first_pass, policy, held
	// or none.
	Settle string `json:"settle,omitempty"`
}

// summarizePlan reduces a plan to its steps. The plan's languages are the
// caller's followed ones, so each count stays inside them.
func summarizePlan(p reviewPlanResponse) planSummaryDTO {
	locales := make([]string, 0, len(p.Languages))
	waiting, failing, pages, settleable := 0, 0, 0, 0
	for _, l := range p.Languages {
		locales = append(locales, l.Locale)
		waiting += l.Waiting
		failing += l.Failing
		pages += l.Pages
		settleable += l.Settleable
	}

	// Coordinates: confirmed by anyone settles the step for the team, until
	// the map changes after that confirmation.
	coordinates := planStepDTO{Level: planCoordinates, Changed: p.Coordinates.Changed}
	switch {
	case !p.Coordinates.Confirmed:
		coordinates.Count = 1
	case p.Coordinates.Changed > 0:
		coordinates.Count, coordinates.Progress = p.Coordinates.Changed, 100
	default:
		coordinates.Done, coordinates.Progress = true, 100
	}

	// Key language: the project's concepts with a source term, and their
	// term in each followed language, against what is still open.
	untranslated := 0
	for loc, n := range p.KeyLanguage.Untranslated {
		if slices.Contains(locales, loc) {
			untranslated += n
		}
	}
	open := p.KeyLanguage.Waiting + untranslated
	decided := p.KeyLanguage.Concepts*(1+len(locales)) - untranslated
	key := planStepDTO{Level: planKeyLanguage, Count: open, Done: open == 0, Progress: percentOf(decided, decided+open)}

	// Exceptions: the rules behind the findings, in the source and in each
	// followed language; progress is the share of waiting translations that
	// fail no check.
	rules := map[string]bool{}
	for _, r := range p.Exceptions.ByRule.Source {
		rules[r.Key] = true
	}
	for _, loc := range locales {
		for _, r := range p.Exceptions.ByRule.Targets[loc] {
			rules[r.Key] = true
		}
	}
	sourceFindings := 0
	if p.Exceptions.SourceFindings != nil {
		sourceFindings = *p.Exceptions.SourceFindings
	}
	exceptions := planStepDTO{
		Level:    planExceptions,
		Count:    len(rules),
		Done:     sourceFindings == 0 && failing == 0,
		Progress: percentOf(waiting-failing, waiting),
	}
	if exceptions.Count == 0 && !exceptions.Done {
		exceptions.Count = failing
	}

	pagesStep := planStepDTO{Level: planInterfaces, Count: pages, Done: pages == 0}
	if pagesStep.Done {
		pagesStep.Progress = 100
	}

	policy := planStepDTO{Level: planPolicy, Count: settleable, Settle: p.Policy.Settle}
	policy.Done = p.Policy.Settle == settleNone || settleable == 0
	if policy.Done {
		policy.Count, policy.Progress = 0, 100
	}

	return planSummaryDTO{
		Involvement: p.Involvement,
		Next:        p.Next,
		Steps:       []planStepDTO{coordinates, key, exceptions, pagesStep, policy},
	}
}

// percentOf is part of whole as a percentage, 100 when whole is empty.
func percentOf(part, whole int) int {
	if whole <= 0 {
		return 100
	}
	part = max(0, min(part, whole))
	return part * 100 / whole
}
