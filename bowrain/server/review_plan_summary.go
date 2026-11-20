package server

import "slices"

// The review plan in brief: one step per level, each with its state, what
// waits there and the team's progress through it, so a client can show the
// next step and the ones after it without asking for the whole plan. The
// plan route carries the steps beside its detail, and the review inbox
// carries them alone.

// Step states, as the plan reports each level.
const (
	// stepNotStarted is a level with nothing to work from yet: a context
	// level with no scan and no concepts, or exceptions before any
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
	// Next is the first level the caller can act on, or "" when none can be.
	Next string `json:"next"`
	// Waiting lists the levels skipped for Next because they only wait.
	Waiting []planWaitDTO `json:"waiting"`
	Steps   []planStepDTO `json:"steps"`
	// Held lists, per followed language, the translations the language deck
	// holds back until the context is decided.
	Held []planHeldDTO `json:"held"`
	// AsOf is when the plan's counts were taken.
	AsOf string `json:"as_of,omitempty"`
}

// planWaitDTO is a level with work ahead and nothing to decide yet, and why.
type planWaitDTO struct {
	Level string `json:"level"`
	// Reason is the context level's scan state: queued or processing while
	// the term scan reads the content, or waiting_content, waiting_quota,
	// waiting_credits or no_ai while it cannot start. It is other_languages
	// when the context's open decisions are target terms missing only in
	// languages the caller does not follow, which other reviewers decide.
	Reason string `json:"reason"`
	// QuotaResetsAt is when the workspace's monthly AI limit resets, set
	// with waiting_quota.
	QuotaResetsAt string `json:"quota_resets_at,omitempty"`
	// Locales is the languages with missing terms, set with other_languages.
	Locales []string `json:"locales,omitempty"`
}

// planWaitOtherLanguages is the wait reason for a context whose open
// decisions are target terms in languages the caller does not follow.
const planWaitOtherLanguages = "other_languages"

// planHeldDTO is the translations one language's deck holds back, and why.
type planHeldDTO struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

// planStepDTO is one level of the plan in brief.
type planStepDTO struct {
	// Level is coordinates, context, exceptions, interfaces (the pages
	// a reviewer reads whole) or policy.
	Level string `json:"level"`
	// State is not_started, open, done or attention.
	State string `json:"state"`
	// Count is what waits at the level: 1 for unconfirmed coordinates or
	// the changes since the last confirmation, the context decisions open
	// for the team, the rules behind the exceptions, the pages to read, the
	// translations a settle pass would establish. The context's open
	// decisions are the caller's context cards and the missing target terms
	// in every target language.
	Count int `json:"count"`
	// Mine is the part of Count the caller can decide. It differs from
	// Count only at the context level, where a missing target term is the
	// caller's to decide only in a language they follow.
	Mine int `json:"mine"`
	// Progress is the team's progress through the level, 0 to 100.
	Progress int  `json:"progress"`
	Done     bool `json:"done"`
	// Changed counts what changed at the level after the team finished it.
	Changed int `json:"changed,omitempty"`
	// Settle is the policy level's settle state: first_pass, policy, held
	// or none.
	Settle string `json:"settle,omitempty"`
	// Scan is the context level's term scan state: queued, processing,
	// completed or failed for the last scan, or, while the level has not
	// started and no scan runs, why the scan waits: waiting_content,
	// waiting_push, waiting_quota, waiting_credits or no_ai.
	Scan string `json:"scan,omitempty"`
	// Proposals is the context level's job proposing target terms: queued
	// or processing while it runs, or, while concepts wait for a proposed
	// term and no job runs, why none starts: waiting_quota, waiting_credits
	// or no_ai.
	Proposals string `json:"proposals,omitempty"`
	// QuotaResetsAt is when the workspace's monthly AI limit resets, set
	// with the scan or proposals state waiting_quota.
	QuotaResetsAt string `json:"quota_resets_at,omitempty"`
}

// contextNotStarted reports a context level with nothing to decide from:
// no concept with a source term, no context card waiting, and no term scan
// that completed. A step in this state is never done, however little waits.
func contextNotStarted(k planContextDTO) bool {
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
		Waiting:     p.Waiting,
		Steps:       p.Steps,
		Held:        []planHeldDTO{},
		AsOf:        p.AsOf,
	}
	if out.Steps == nil {
		out.Steps = planSteps(p)
	}
	if out.Waiting == nil {
		out.Waiting = []planWaitDTO{}
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

	// Context: the project's concepts with a source term, and their term in
	// every target language, against what is still open. The state and the
	// progress are the team's; Mine keeps to the followed languages.
	untranslated, mine := 0, 0
	for loc, n := range p.Context.Untranslated {
		untranslated += n
		if slices.Contains(locales, loc) {
			mine += n
		}
	}
	open := p.Context.Waiting + untranslated
	decided := p.Context.Concepts*(1+len(p.Context.Untranslated)) - untranslated
	contextStep := planStepDTO{
		Level: planContext, Count: open, Mine: p.Context.Waiting + mine,
		Progress: percentOf(decided, decided+open),
	}
	if p.Context.Scan != nil {
		contextStep.Scan = p.Context.Scan.Status
	}
	if p.Context.ScanWaiting != "" {
		contextStep.Scan, contextStep.QuotaResetsAt = p.Context.ScanWaiting, p.Context.QuotaResetsAt
	}
	switch {
	case p.Context.Proposals != nil && scanRunning(p.Context.Proposals.Status):
		contextStep.Proposals = p.Context.Proposals.Status
	case p.Context.ProposalsWaiting != "":
		contextStep.Proposals, contextStep.QuotaResetsAt = p.Context.ProposalsWaiting, p.Context.QuotaResetsAt
	}
	switch {
	case contextNotStarted(p.Context):
		contextStep.State, contextStep.Progress = stepNotStarted, 0
	case open > 0:
		contextStep.State = stepOpen
	default:
		contextStep.State, contextStep.Done = stepDone, true
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

	for _, st := range []*planStepDTO{&coordinates, &exceptions, &pagesStep, &policy} {
		st.Mine = st.Count
	}
	return []planStepDTO{coordinates, contextStep, exceptions, pagesStep, policy}
}

// percentOf is part of whole as a percentage, 100 when whole is empty.
func percentOf(part, whole int) int {
	if whole <= 0 {
		return 100
	}
	part = max(0, min(part, whole))
	return part * 100 / whole
}
