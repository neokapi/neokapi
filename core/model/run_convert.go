package model

// This file holds the single, generic shape of the Run discriminated-union
// mapping. Every wire format (the v2 plugin gRPC proto, the bowrain sync
// proto, and the bowrain JSON sync structs) used to hand-write the same 7-arm
// Text/Ph/PcOpen/PcClose/Sub/Plural/Select switch with its recursive
// Plural.Forms / Select.Cases traversal. That recursion is the most
// error-prone part — a new Run kind or a missed recursive call silently
// corrupts a wire format. BuildRun / ParseRun centralize the dispatch and the
// recursion so each wire format only supplies the per-kind leaf mapping.

// RunBuilder maps a model.Run to a wire run of type R. The wire format also
// has a "run list" type L that wraps a slice of wire runs (e.g. a proto
// RunList message, or just []SyncRun). BuildRun handles the discriminator
// dispatch and the Plural/Select recursion; the builder only constructs each
// kind's wire payload from already-converted children.
type RunBuilder[R any, L any] interface {
	Text(*TextRun) R
	Ph(*PlaceholderRun) R
	PcOpen(*PcOpenRun) R
	PcClose(*PcCloseRun) R
	Sub(*SubRun) R
	// Plural builds the wire run for a plural construct. forms maps each
	// plural-form name to the already-converted wire run list for that form.
	Plural(pivot string, forms map[string]L) R
	// Select builds the wire run for a select construct. cases maps each case
	// key to the already-converted wire run list.
	Select(pivot string, cases map[string]L) R
	// List wraps a slice of wire runs into the wire list type L.
	List([]R) L
	// Zero is the wire run returned for an empty/invalid model.Run.
	Zero() R
}

// BuildRun converts one model.Run into its wire form using b, recursing into
// Plural.Forms / Select.Cases.
func BuildRun[R any, L any](r Run, b RunBuilder[R, L]) R {
	switch {
	case r.Text != nil:
		return b.Text(r.Text)
	case r.Ph != nil:
		return b.Ph(r.Ph)
	case r.PcOpen != nil:
		return b.PcOpen(r.PcOpen)
	case r.PcClose != nil:
		return b.PcClose(r.PcClose)
	case r.Sub != nil:
		return b.Sub(r.Sub)
	case r.Plural != nil:
		forms := make(map[string]L, len(r.Plural.Forms))
		for form, runs := range r.Plural.Forms {
			forms[string(form)] = b.List(BuildRuns(runs, b))
		}
		return b.Plural(r.Plural.Pivot, forms)
	case r.Select != nil:
		cases := make(map[string]L, len(r.Select.Cases))
		for key, runs := range r.Select.Cases {
			cases[key] = b.List(BuildRuns(runs, b))
		}
		return b.Select(r.Select.Pivot, cases)
	}
	return b.Zero()
}

// BuildRuns converts a slice of model.Run into wire runs. Returns nil for an
// empty input so wire formats keep their "nil, not empty slice" convention.
func BuildRuns[R any, L any](runs []Run, b RunBuilder[R, L]) []R {
	if len(runs) == 0 {
		return nil
	}
	out := make([]R, len(runs))
	for i, r := range runs {
		out[i] = BuildRun(r, b)
	}
	return out
}

// RunParser maps a wire run of type R back to a model.Run. Each method
// receives the wire payload for one discriminator and returns the matching
// model.Run. Plural/Select recursion is handled by ParseRun: the parser only
// needs to expose, via Forms/Cases, the wire list type L for a given wire run,
// and via ListRuns, the slice of wire runs inside an L.
type RunParser[R any, L any] interface {
	// Kind reports which discriminator r carries; the returned model.Run is
	// the converted result for leaf kinds. For Plural/Select, ParseRun uses
	// Forms/Cases to recurse and fills Forms/Cases on the returned run.
	Text(R) (*TextRun, bool)
	Ph(R) (*PlaceholderRun, bool)
	PcOpen(R) (*PcOpenRun, bool)
	PcClose(R) (*PcCloseRun, bool)
	Sub(R) (*SubRun, bool)
	// Plural reports whether r is a plural run and, if so, its pivot and the
	// wire forms map.
	Plural(R) (pivot string, forms map[string]L, ok bool)
	// Select reports whether r is a select run and, if so, its pivot and the
	// wire cases map.
	Select(R) (pivot string, cases map[string]L, ok bool)
	// ListRuns returns the slice of wire runs wrapped by a wire list L.
	ListRuns(L) []R
}

// ParseRun converts one wire run R back into a model.Run using p, recursing
// into plural forms / select cases.
func ParseRun[R any, L any](r R, p RunParser[R, L]) Run {
	if t, ok := p.Text(r); ok {
		return Run{Text: t}
	}
	if ph, ok := p.Ph(r); ok {
		return Run{Ph: ph}
	}
	if pc, ok := p.PcOpen(r); ok {
		return Run{PcOpen: pc}
	}
	if pc, ok := p.PcClose(r); ok {
		return Run{PcClose: pc}
	}
	if s, ok := p.Sub(r); ok {
		return Run{Sub: s}
	}
	if pivot, forms, ok := p.Plural(r); ok {
		out := make(map[PluralForm][]Run, len(forms))
		for form, list := range forms {
			out[PluralForm(form)] = ParseRuns(p.ListRuns(list), p)
		}
		return Run{Plural: &PluralRun{Pivot: pivot, Forms: out}}
	}
	if pivot, cases, ok := p.Select(r); ok {
		out := make(map[string][]Run, len(cases))
		for key, list := range cases {
			out[key] = ParseRuns(p.ListRuns(list), p)
		}
		return Run{Select: &SelectRun{Pivot: pivot, Cases: out}}
	}
	return Run{}
}

// ParseRuns converts a slice of wire runs back into model.Run. Returns nil for
// an empty input.
func ParseRuns[R any, L any](runs []R, p RunParser[R, L]) []Run {
	if len(runs) == 0 {
		return nil
	}
	out := make([]Run, len(runs))
	for i, r := range runs {
		out[i] = ParseRun(r, p)
	}
	return out
}
