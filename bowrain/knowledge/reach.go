package knowledge

import (
	"sort"

	"github.com/neokapi/neokapi/core/model"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// The reach split: what a change-set's blast radius actually costs.
//
// A correction is one of two acts, and the product keeps them distinct. An
// ANNOTATION says "this is a term" or "this violates the voice": it changes what
// the checks flag and nothing else, so the next check fans it out and the
// approved translations that now violate it come back into review. A TRANSFORM
// says the source text itself is wrong: acting on it edits source, which
// invalidates every translation of that block and routes the decision to the
// people entitled to edit source.
//
// A change-set never edits source text — its ops are graph ops — so the class is
// read off the guidance the draft would leave on each affected block rather than
// off the op. A block the after-state tells you to REWRITE (a forbidden term
// that resolves to a replacement, a vocabulary rule that carries one) is
// transform-bound; a block it merely flags is annotate-bound.
//
// The split is computed during the same walk that produces the blast radius, so
// it costs no second scan, and it is what a reviewer needs before approving:
// "1,400 blocks affected" buys wildly different amounts of work depending on how
// many of them the draft is telling someone to rewrite.

// Reach splits a change-set's affected content into the two cost classes.
type Reach struct {
	Annotate  ReachClass `json:"annotate"`
	Transform ReachClass `json:"transform"`
	// TransformProjects names the projects whose source a transform reaches, in
	// walk order, so a panel can say whose desk the work lands on rather than
	// only how much of it there is. Empty when nothing is transform-bound.
	TransformProjects []ProjectRef `json:"transform_projects"`
}

// ReachClass is one cost class: the content it covers, and the translations that
// content already carries.
type ReachClass struct {
	Blocks      int `json:"blocks"`
	Words       int `json:"words"`
	Collections int `json:"collections"`
	Projects    int `json:"projects"`
	// Targets counts the translations the class's blocks already carry, and
	// Approved the subset at reviewed or above. For the annotate class those are
	// the translations a re-check pulls back into review; for the transform
	// class they are the translations a source rewrite invalidates outright.
	Targets  int `json:"targets"`
	Approved int `json:"approved"`
	// Locales names the locales those translations are in, sorted.
	Locales []string `json:"locales"`
}

// ProjectRef identifies a project in a report without carrying the whole record.
type ProjectRef struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
}

// ReachSummary is the reach split reduced to the numbers a stored summary keeps:
// the same two classes without the per-locale and per-project detail, which the
// summary never carried for the rest of the report either.
type ReachSummary struct {
	AnnotateBlocks   int `json:"annotate_blocks"`
	AnnotateTargets  int `json:"annotate_targets"`
	AnnotateApproved int `json:"annotate_approved"`
	TransformBlocks  int `json:"transform_blocks"`
	// TransformTargets counts the translations a source rewrite would
	// invalidate — the number that makes a transform expensive.
	TransformTargets  int `json:"transform_targets"`
	TransformApproved int `json:"transform_approved"`
	TransformProjects int `json:"transform_projects"`
}

// Summarize reduces a reach split to the form stored on a change-set.
func (r Reach) Summarize() ReachSummary {
	return ReachSummary{
		AnnotateBlocks:    r.Annotate.Blocks,
		AnnotateTargets:   r.Annotate.Targets,
		AnnotateApproved:  r.Annotate.Approved,
		TransformBlocks:   r.Transform.Blocks,
		TransformTargets:  r.Transform.Targets,
		TransformApproved: r.Transform.Approved,
		TransformProjects: len(r.TransformProjects),
	}
}

// Report renders a stored reach summary back as a Reach. The per-locale lists
// and the project names stay empty — the summary never carried them, and a
// reader that needs them asks for a fresh scan, exactly as it does for the
// per-project breakdown.
func (s ReachSummary) Report() Reach {
	return Reach{
		Annotate: ReachClass{
			Blocks:   s.AnnotateBlocks,
			Targets:  s.AnnotateTargets,
			Approved: s.AnnotateApproved,
			Locales:  []string{},
		},
		Transform: ReachClass{
			Blocks:   s.TransformBlocks,
			Targets:  s.TransformTargets,
			Approved: s.TransformApproved,
			Projects: s.TransformProjects,
			Locales:  []string{},
		},
		TransformProjects: []ProjectRef{},
	}
}

// ---------------------------------------------------------------------------
// Accumulation
// ---------------------------------------------------------------------------

// reachAcc classifies affected blocks as the walk meets them. It accumulates per
// BLOCK, not per evaluation unit: a block evaluated in three locales is one
// block to rewrite, not three, and its translations must not be counted three
// times. A block whose class differs between its rows is transform-bound — one
// prescribed rewrite is enough to make the source the thing that has to change.
type reachAcc struct {
	order []string
	seen  map[string]*reachBlock
}

type reachBlock struct {
	transform bool
	proj      ProjectRef
	colKey    string
	words     int
	targets   []string
	approved  []string
}

func newReachAcc() *reachAcc {
	return &reachAcc{seen: map[string]*reachBlock{}}
}

// observe folds one affected evaluation unit into the split. key must identify
// the block within the walk (project, stream and block id).
func (a *reachAcc) observe(key string, transform bool, p *store.Project, colKey string, words int, targets, approved []string) {
	if rb, ok := a.seen[key]; ok {
		rb.transform = rb.transform || transform
		return
	}
	var ref ProjectRef
	if p != nil {
		ref = ProjectRef{ProjectID: p.ID, ProjectName: p.Name}
	}
	a.seen[key] = &reachBlock{
		transform: transform,
		proj:      ref,
		colKey:    colKey,
		words:     words,
		targets:   targets,
		approved:  approved,
	}
	a.order = append(a.order, key)
}

// reach folds the observed blocks into the two classes. Iteration follows
// first-seen order so the project list and every count are deterministic.
func (a *reachAcc) reach() Reach {
	annotate := newClassAcc()
	transform := newClassAcc()
	var transformProjects []ProjectRef
	seenTransformProject := map[string]bool{}

	for _, key := range a.order {
		rb := a.seen[key]
		if rb.transform {
			transform.add(rb)
			if rb.proj.ProjectID != "" && !seenTransformProject[rb.proj.ProjectID] {
				seenTransformProject[rb.proj.ProjectID] = true
				transformProjects = append(transformProjects, rb.proj)
			}
			continue
		}
		annotate.add(rb)
	}

	if transformProjects == nil {
		transformProjects = []ProjectRef{}
	}
	return Reach{
		Annotate:          annotate.class(),
		Transform:         transform.class(),
		TransformProjects: transformProjects,
	}
}

type classAcc struct {
	blocks, words, targets, approved int
	cols, projs, locales             map[string]bool
}

func newClassAcc() *classAcc {
	return &classAcc{
		cols:    map[string]bool{},
		projs:   map[string]bool{},
		locales: map[string]bool{},
	}
}

func (c *classAcc) add(rb *reachBlock) {
	c.blocks++
	c.words += rb.words
	c.targets += len(rb.targets)
	c.approved += len(rb.approved)
	if rb.proj.ProjectID != "" {
		c.projs[rb.proj.ProjectID] = true
	}
	c.cols[rb.proj.ProjectID+"\x00"+rb.colKey] = true
	for _, l := range rb.targets {
		c.locales[l] = true
	}
}

func (c *classAcc) class() ReachClass {
	locales := make([]string, 0, len(c.locales))
	for l := range c.locales {
		locales = append(locales, l)
	}
	sort.Strings(locales)
	return ReachClass{
		Blocks:      c.blocks,
		Words:       c.words,
		Collections: len(c.cols),
		Projects:    len(c.projs),
		Targets:     c.targets,
		Approved:    c.approved,
		Locales:     locales,
	}
}

// ---------------------------------------------------------------------------
// Block translations
// ---------------------------------------------------------------------------

// blockTargetLocales reports the locales a block already carries a translation
// in, and the subset at reviewed or above. Variants collapse to their locale: a
// tone or channel variant is still that locale's translation, and a reader
// counting "how many languages does this land on" wants the language once.
//
// A translation with no committed runs is not a translation — the store keeps a
// row for a target the moment one is queued — so an empty one is skipped rather
// than counted as work a change would invalidate.
func blockTargetLocales(b *store.StoredBlock) (locales, approved []string) {
	if b == nil || b.Block == nil || len(b.Targets) == 0 {
		return nil, nil
	}
	best := map[model.LocaleID]model.TargetStatus{}
	var order []model.LocaleID
	for key, t := range b.Targets {
		if t == nil || len(t.Runs) == 0 {
			continue
		}
		if _, ok := best[key.Locale]; !ok {
			order = append(order, key.Locale)
			best[key.Locale] = t.Status
			continue
		}
		// Several variants of one locale: the highest rung wins, so a locale
		// counts as approved when any variant of it has been reviewed.
		if t.Status.Rank() > best[key.Locale].Rank() {
			best[key.Locale] = t.Status
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	for _, l := range order {
		locales = append(locales, string(l))
		if best[l].Rank() >= model.TargetStatusReviewed.Rank() {
			approved = append(approved, string(l))
		}
	}
	return locales, approved
}
