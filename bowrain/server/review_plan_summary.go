package server

import "slices"

// The review plan in brief: one step per level, each with its state, what
// waits there and the team's progress through it, so a client can show the
// next step and the ones after it without asking for the whole plan. The
// plan route carries the steps beside its detail, and the review inbox
// carries them alone.

// Step states, as the plan reports each level.
const (
	// stepNotStarted is a level with nothing to work from yet: a key
	// language with no scan and no concepts, or exceptions before any
	// project-wide check.
	stepNotStarted = "not_started"
	// stepOpen is a level with something waiting.
	stepOpen = "open"
	// stepDone is a level with nothing waiting.
	stepDone = "done"
	// stepAttention is a level the team finished that changed afterwards,
	// such as coordinates changed since the last confirmation.
	stepAttention = "attention"
)

// planSummaryDTO is a project's plan in brief, for one caller.
type planSummaryDTO struct {
	Involvement string `json:"involvement"`
	// Next is the first level with something waiting for the caller, or ""
	// when nothing does.
	Next  string        `json:"next"`
	Steps []planStepDTO `json:"steps"`
	// Held lists, per followed language, the translations the language deck
	// holds back until the key language is decided.
	Held []planHeldDTO `json:"held"`
}

// planHeldDTO is the translations one language's deck holds back, and why.
type planHeldDTO struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

// planStepDTO is one level of the plan in brief.
type planStepDTO struct {
	// Level is coordinates, key_language, exceptions, interfaces (the pages
	// a reviewer reads whole) or policy.
	Level string `json:"level"`
	// State is not_started, open, done or attention.
	State string `json:"state"`
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
	// Scan is the key-language level's last term scan state: queued,
	// processing, completed or failed.
	Scan string `json:"scan,omitempty"`
}

// keyLanguageNotStarted reports a key language with nothing to decide from:
// no concept with a source term, no context card waiting, and no term scan
// that completed. A step in this state is never done, however little waits.
func keyLanguageNotStarted(k planKeyLanguageDTO) bool {
	if k.Concepts > 0 || k.Waiting > 0 {
		return false
	}
	return k.Scan == nil || k.Scan.Status != "completed"
}

// summarizePlan reduces a plan to its steps and the translations held back.
func summarizePlan(p reviewPlanResponse) planSummaryDTO {
	out := planSummaryDTO{
		Involvement: p.Involvement,
		Next:        p.Next,
		Steps:       p.Steps,
		Held:        []planHeldDTO{},
	}
	if out.Steps == nil {
		out.Steps = planSteps(p)
	}
	if p.Held != "" {
		for _, l := range p.Languages {
			if l.Held > 0 {
				out.Held = append(out.Held, planHeldDTO{Locale: l.Locale, Count: l.Held, Reason: p.Held})
			}
		}
	}
	return out
}

// planSteps reduces a plan to its steps. The plan's languages are the
// caller's followed ones, so each count stays inside them.
func planSteps(p reviewPlanResponse) []planStepDTO {
	locales := make([]string, 0, len(p.Languages))
	dealt, failing, pages, settleable := 0, 0, 0, 0
	for _, l := range p.Languages {
		locales = append(locales, l.Locale)
		dealt += l.Waiting + l.Held
		failing += l.Failing
		pages += l.Pages
		settleable += l.Settleable
	}

	// Coordinates: confirmed by anyone settles the step for the team, until
	// the map changes after that confirmation.
	coordinates := planStepDTO{Level: planCoordinates, Changed: p.Coordinates.Changed}
	switch {
	case !p.Coordinates.Confirmed:
		coordinates.Count, coordinates.State = 1, stepOpen
	case p.Coordinates.Changed > 0:
		coordinates.Count, coordinates.Progress, coordinates.State = p.Coordinates.Changed, 100, stepAttention
	default:
		coordinates.Done, coordinates.Progress, coordinates.State = true, 100, stepDone
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
	key := planStepDTO{Level: planKeyLanguage, Count: open, Progress: percentOf(decided, decided+open)}
	if p.KeyLanguage.Scan != nil {
		key.Scan = p.KeyLanguage.Scan.Status
	}
	switch {
	case keyLanguageNotStarted(p.KeyLanguage):
		key.State, key.Progress = stepNotStarted, 0
	case open > 0:
		key.State = stepOpen
	default:
		key.State, key.Done = stepDone, true
	}

	// Exceptions: the rules behind the findings, in the source and in each
	// followed language; progress is the share of the translations dealt or
	// held that fail no check.
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
		Progress: percentOf(dealt-failing, dealt),
	}
	if exceptions.Count == 0 && !exceptions.Done {
		exceptions.Count = failing
	}
	switch {
	case !exceptions.Done:
		exceptions.State = stepOpen
	case p.Exceptions.SourceFindings == nil && dealt == 0:
		exceptions.State, exceptions.Done, exceptions.Progress = stepNotStarted, false, 0
	default:
		exceptions.State = stepDone
	}

	pagesStep := planStepDTO{Level: planInterfaces, Count: pages, Done: pages == 0, State: stepOpen}
	if pagesStep.Done {
		pagesStep.Progress, pagesStep.State = 100, stepDone
	}

	policy := planStepDTO{Level: planPolicy, Count: settleable, Settle: p.Policy.Settle, State: stepOpen}
	policy.Done = p.Policy.Settle == settleNone || settleable == 0
	if policy.Done {
		policy.Count, policy.Progress, policy.State = 0, 100, stepDone
	}

	return []planStepDTO{coordinates, key, exceptions, pagesStep, policy}
}

// percentOf is part of whole as a percentage, 100 when whole is empty.
func percentOf(part, whole int) int {
	if whole <= 0 {
		return 100
	}
	part = max(0, min(part, whole))
	return part * 100 / whole
}
