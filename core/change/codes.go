package change

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Inline codes in new content. Content arrives as text, whose codes are the
// <x id="…"/> placeholders a read shows, or as runs, whose codes name their id,
// type and attributes and never their native bytes. Either way a code is the
// code of that id in a reference set: the content being replaced, or, for a
// new edition, the authoritative edition's. A derived edition may also take
// back a code the authoritative edition holds, which is how a translation that
// lost a link gets it back.

// codeKey identifies an inline code by its kind and id.
type codeKey struct {
	kind model.RunKind
	id   string
}

func (k codeKey) String() string {
	switch k.kind {
	case model.RunKindPcOpen:
		return fmt.Sprintf(`<x id="%s"/>`, k.id)
	case model.RunKindPcClose:
		return fmt.Sprintf(`<x id="/%s"/>`, k.id)
	case model.RunKindPh:
		return fmt.Sprintf(`<x id="%s/"/>`, k.id)
	case model.RunKindSub:
		return fmt.Sprintf(`<x id="sub:%s"/>`, k.id)
	}
	return string(k.kind) + ":" + k.id
}

func keyOf(r model.Run) (codeKey, bool) {
	switch {
	case r.Ph != nil, r.PcOpen != nil, r.PcClose != nil, r.Sub != nil:
		return codeKey{kind: r.Kind(), id: r.RunID()}, true
	}
	return codeKey{}, false
}

// indexCodes maps each code of a run tree, branches included, to its first
// run.
func indexCodes(runs []model.Run, into map[codeKey]model.Run) {
	for _, r := range runs {
		if k, ok := keyOf(r); ok {
			if _, seen := into[k]; !seen {
				into[k] = r
			}
			continue
		}
		forEachBranch(r, func(branch []model.Run) { indexCodes(branch, into) })
	}
}

func forEachBranch(r model.Run, fn func([]model.Run)) {
	switch {
	case r.Plural != nil:
		for _, k := range sortedKeys(pluralNames(r.Plural.Forms)) {
			fn(r.Plural.Forms[model.PluralForm(k)])
		}
	case r.Select != nil:
		for _, k := range sortedKeys(r.Select.Cases) {
			fn(r.Select.Cases[k])
		}
	}
}

// lookupCode finds the code k in the reference index, then in the restorable
// one.
func lookupCode(k codeKey, ref, restorable map[codeKey]model.Run) (model.Run, bool) {
	if r, ok := ref[k]; ok {
		return r, true
	}
	r, ok := restorable[k]
	return r, ok
}

// resolveTextCodes checks the codes of runs parsed from text. A code the
// reference holds came back whole from the parse; one only the authoritative
// edition holds is taken from it. One neither holds is refused, or under
// report kept as the parse made it and named in a finding.
func resolveTextCodes(parsed, ref, restorable []model.Run, report bool) ([]model.Run, []Finding, *Error) {
	refIdx := map[codeKey]model.Run{}
	indexCodes(ref, refIdx)
	restIdx := map[codeKey]model.Run{}
	indexCodes(restorable, restIdx)
	out := make([]model.Run, len(parsed))
	var findings []Finding
	for i, r := range parsed {
		k, ok := keyOf(r)
		if !ok {
			out[i] = r
			continue
		}
		if _, held := refIdx[k]; held {
			out[i] = r
			continue
		}
		if back, ok := restIdx[k]; ok {
			out[i] = back
			continue
		}
		msg := fmt.Sprintf("the text names %s, a code this edition does not hold", k)
		if report {
			out[i] = r
			findings = append(findings, Finding{Rule: "guard." + string(SubcodeCodesChanged), Message: msg + "; it is kept with no native form", Fails: true})
			continue
		}
		return nil, nil, &Error{Code: CodeGuard, Subcode: SubcodeCodesChanged, Field: "text", Found: k.String(), Message: msg}
	}
	return out, findings, nil
}

// reconcileRuns gives each code of a runs payload its native form. A code
// that names no data takes it from the code of the same id in the reference,
// and must agree with that code's type and attributes: an attribute changes
// through set_attribute, so a payload that changes one is refused rather than
// written as the old native form. A subblock reference is looked up by its id
// the same way, and a ref it names must be the one the reference holds. A code
// that names its data, which only an in-process caller can send, is kept as it
// is.
//
// A new code with no data needs the format to spell it, which no format here
// does, so it is refused as unsupported. Under report it is kept as sent, with
// no native form, and named in a finding: a translation that names a
// placeholder the source lacks lands, and the placeholder checks flag it.
func reconcileRuns(runs, ref, restorable []model.Run, report bool) ([]model.Run, []Finding, *Error) {
	if spelled(runs) {
		return runs, nil, nil
	}
	rc := reconciler{refIdx: map[codeKey]model.Run{}, restIdx: map[codeKey]model.Run{}, report: report}
	indexCodes(ref, rc.refIdx)
	indexCodes(restorable, rc.restIdx)
	out, err := rc.seq(runs)
	if err != nil {
		return nil, nil, err
	}
	return out, rc.findings, nil
}

// reconciler carries the indexes and the disposition reconcileRuns works
// with, and the findings it makes.
type reconciler struct {
	refIdx, restIdx map[codeKey]model.Run
	report          bool
	findings        []Finding
}

func (rc *reconciler) seq(runs []model.Run) ([]model.Run, *Error) {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		if !r.Valid() {
			return nil, &Error{Code: CodeInvalid, Field: "runs", Message: fmt.Sprintf("run %d has %s", i, discriminators(r))}
		}
		switch {
		case r.Plural != nil:
			p := *r.Plural
			p.Forms = make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, form := range r.Plural.Forms {
				next, err := rc.seq(form)
				if err != nil {
					return nil, err
				}
				p.Forms[k] = next
			}
			out[i] = model.Run{Plural: &p}
			continue
		case r.Select != nil:
			s := *r.Select
			s.Cases = make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				next, err := rc.seq(c)
				if err != nil {
					return nil, err
				}
				s.Cases[k] = next
			}
			out[i] = model.Run{Select: &s}
			continue
		}
		k, isCode := keyOf(r)
		if !isCode || codeData(r) != "" {
			out[i] = r
			continue
		}
		held, ok := lookupCode(k, rc.refIdx, rc.restIdx)
		if !ok {
			kind := "new code"
			if typ := codeType(r); typ != "" {
				kind = "new " + typ + " code"
			}
			if rc.report {
				out[i] = r
				rc.findings = append(rc.findings, Finding{Rule: "guard." + string(SubcodeCodesChanged),
					Message: fmt.Sprintf("the content names %s, a %s the reference does not hold; it is kept with no native form", k, kind), Fails: true})
				continue
			}
			return nil, &Error{Code: CodeUnsupported, Capability: "synthesize:" + codeKindName(r), Field: "runs",
				Message: fmt.Sprintf("%s is a %s; no format here can write a new code yet", k, kind)}
		}
		filled, err := fillCode(r, held)
		if err != nil {
			return nil, err
		}
		out[i] = filled
	}
	return out, nil
}

// sameCodeKeys reports whether two flat sequences hold the codes of the same
// kinds and ids in the same order. A plural or select is never the same.
func sameCodeKeys(a, b []model.Run) bool {
	i, j := 0, 0
	for {
		for i < len(a) && a[i].Text != nil {
			i++
		}
		for j < len(b) && b[j].Text != nil {
			j++
		}
		if i == len(a) || j == len(b) {
			return i == len(a) && j == len(b)
		}
		ka, okA := keyOf(a[i])
		kb, okB := keyOf(b[j])
		if !okA || !okB || ka != kb {
			return false
		}
		i++
		j++
	}
}

// spelled reports whether every run of a flat sequence is a valid union and
// every code in it names its own data, so it has nothing to take from a
// reference.
func spelled(runs []model.Run) bool {
	for _, r := range runs {
		if !r.Valid() || r.Plural != nil || r.Select != nil {
			return false
		}
		if _, isCode := keyOf(r); isCode && codeData(r) == "" {
			return false
		}
	}
	return true
}

// fillCode returns held for a payload code that names held's id and no data,
// after checking the payload names nothing held does not say.
func fillCode(r, held model.Run) (model.Run, *Error) {
	conflict := func(field string, want, got any) *Error {
		k, _ := keyOf(r)
		return &Error{Code: CodeUnsupported, Capability: "set_attribute", Field: "runs",
			Expected: fmt.Sprint(want), Found: fmt.Sprint(got),
			Message: fmt.Sprintf("%s has %s %v; the payload says %v. Change an attribute with set_attribute", k, field, want, got)}
	}
	check := func(field, got, want string) *Error {
		if got != "" && got != want {
			return conflict(field, want, got)
		}
		return nil
	}
	checkAttrs := func(got, want map[string]string) *Error {
		for name, v := range got {
			if want[name] != v {
				return conflict("attribute "+name, want[name], v)
			}
		}
		return nil
	}
	switch {
	case r.Ph != nil:
		h := held.Ph
		for _, e := range []*Error{check("type", r.Ph.Type, h.Type), check("subType", r.Ph.SubType, h.SubType), checkAttrs(r.Ph.Attrs, h.Attrs)} {
			if e != nil {
				return model.Run{}, e
			}
		}
	case r.PcOpen != nil:
		h := held.PcOpen
		for _, e := range []*Error{check("type", r.PcOpen.Type, h.Type), check("subType", r.PcOpen.SubType, h.SubType), checkAttrs(r.PcOpen.Attrs, h.Attrs)} {
			if e != nil {
				return model.Run{}, e
			}
		}
	case r.PcClose != nil:
		if e := check("type", r.PcClose.Type, held.PcClose.Type); e != nil {
			return model.Run{}, e
		}
	case r.Sub != nil:
		if e := check("ref", r.Sub.Ref, held.Sub.Ref); e != nil {
			return model.Run{}, e
		}
	}
	return held, nil
}

// codeData is a code's native data. A subblock reference has none of its own:
// its ref names another block, and a reference that holds the code gives it.
func codeData(r model.Run) string {
	switch {
	case r.Ph != nil:
		return r.Ph.Data
	case r.PcOpen != nil:
		return r.PcOpen.Data
	case r.PcClose != nil:
		return r.PcClose.Data
	}
	return ""
}

func codeType(r model.Run) string {
	switch {
	case r.Ph != nil:
		return r.Ph.Type
	case r.PcOpen != nil:
		return r.PcOpen.Type
	case r.PcClose != nil:
		return r.PcClose.Type
	}
	return string(r.Kind())
}

// codeKindName is a code's type, or its run kind when it names no type.
func codeKindName(r model.Run) string {
	if typ := codeType(r); typ != "" {
		return typ
	}
	return string(r.Kind())
}

func discriminators(r model.Run) string {
	n := 0
	for _, set := range []bool{r.Text != nil, r.Ph != nil, r.PcOpen != nil, r.PcClose != nil, r.Sub != nil, r.Plural != nil, r.Select != nil} {
		if set {
			n++
		}
	}
	if n == 0 {
		return "no kind"
	}
	return fmt.Sprintf("%d kinds", n)
}

// countCodes counts a run tree's codes: each code of the sequence, and per
// plural or select the most any one branch uses, since languages disagree on
// plural categories.
func countCodes(runs []model.Run) map[codeKey]int {
	counts := map[codeKey]int{}
	for _, r := range runs {
		if k, ok := keyOf(r); ok {
			counts[k]++
			continue
		}
		best := map[codeKey]int{}
		forEachBranch(r, func(branch []model.Run) {
			for k, n := range countCodes(branch) {
				best[k] = max(best[k], n)
			}
		})
		for k, n := range best {
			counts[k] += n
		}
	}
	return counts
}

// codeOrder lists the codes of a sequence's top level in order, each once.
func codeOrder(runs []model.Run) []codeKey {
	var out []codeKey
	seen := map[codeKey]bool{}
	for _, r := range runs {
		if k, ok := keyOf(r); ok && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// constraints returns the editing constraints of the code at k, read from the
// run or, when it carries none, from the vocabulary by type. A closing code
// takes its opening code's. A subblock reference may move and nothing else.
func (w *workset) constraints(k codeKey, idx map[codeKey]model.Run) model.RunConstraints {
	r := idx[k]
	if k.kind == model.RunKindPcClose {
		if open, ok := idx[codeKey{kind: model.RunKindPcOpen, id: k.id}]; ok {
			r = open
		}
	}
	switch {
	case r.Ph != nil && r.Ph.Constraints != nil:
		return *r.Ph.Constraints
	case r.PcOpen != nil && r.PcOpen.Constraints != nil:
		return *r.PcOpen.Constraints
	case r.Sub != nil:
		return model.RunConstraints{Reorderable: true}
	}
	vocab := w.env.Vocabulary
	if vocab == nil {
		vocab = model.DefaultVocabulary()
	}
	if info := vocab.LookupOrFallback(codeType(r)); info != nil {
		return model.RunConstraints(info.Constraints)
	}
	return model.RunConstraints{}
}

// checkCodes compares the codes of new content with the reference it replaces:
// a code may be dropped only when deletable, repeated only when cloneable
// (or restored up to the count the authoritative edition holds), moved past
// another code only when reorderable, and paired codes stay as well formed as
// the reference's. A violation refuses the operation, or under a report
// disposition lands it with the violation among the result's findings.
func (w *workset) checkCodes(ref, out, restorable []model.Run, res *OpResult) *Error {
	if sameCodeKeys(ref, out) {
		return nil // the same codes in the same order: nothing to check
	}
	refIdx := map[codeKey]model.Run{}
	indexCodes(ref, refIdx)
	indexCodes(restorable, refIdx)
	want, got := countCodes(ref), countCodes(out)
	rest := countCodes(restorable)

	var problems []string
	var missing, extra []string
	for _, k := range sortedCodeKeys(want) {
		if got[k] < want[k] && !w.constraints(k, refIdx).Deletable {
			missing = append(missing, k.String())
			problems = append(problems, fmt.Sprintf("drops %s, which may not be deleted", k))
		}
	}
	for _, k := range sortedCodeKeys(got) {
		allowed := max(want[k], rest[k])
		if allowed == 0 || got[k] <= allowed {
			continue
		}
		if !w.constraints(k, refIdx).Cloneable {
			extra = append(extra, k.String())
			problems = append(problems, fmt.Sprintf("repeats %s, which may not be copied", k))
		}
	}
	if moved := w.movedCodes(ref, out, refIdx); len(moved) > 0 {
		for _, k := range moved {
			problems = append(problems, fmt.Sprintf("moves %s, which may not be reordered", k))
		}
	}
	if !model.PairedCodesKeepShape(ref, out) {
		problems = append(problems, "leaves paired codes crossed or unbalanced")
	}
	if len(problems) == 0 {
		return nil
	}
	if w.env.Guards == Report {
		for _, p := range problems {
			res.Findings = append(res.Findings, Finding{Rule: "guard." + string(SubcodeCodesChanged), Message: "the content " + p, Fails: true})
		}
		return nil
	}
	return &Error{Code: CodeGuard, Subcode: SubcodeCodesChanged,
		Expected: strings.Join(missing, " "), Found: strings.Join(extra, " "),
		Message: "the content " + strings.Join(problems, "; ")}
}

// movedCodes lists the codes that may not be reordered and whose place among
// the codes both sequences share changed.
func (w *workset) movedCodes(ref, out []model.Run, idx map[codeKey]model.Run) []codeKey {
	a, b := codeOrder(ref), codeOrder(out)
	shared := map[codeKey]bool{}
	for _, k := range a {
		if slices.Contains(b, k) {
			shared[k] = true
		}
	}
	before := func(order []codeKey, k codeKey) map[codeKey]bool {
		set := map[codeKey]bool{}
		for _, o := range order {
			if o == k {
				break
			}
			if shared[o] {
				set[o] = true
			}
		}
		return set
	}
	var moved []codeKey
	for _, k := range a {
		if !shared[k] || w.constraints(k, idx).Reorderable {
			continue
		}
		if !maps.Equal(before(a, k), before(b, k)) {
			moved = append(moved, k)
		}
	}
	return moved
}

func sortedCodeKeys(m map[codeKey]int) []codeKey {
	keys := slices.Collect(maps.Keys(m))
	slices.SortFunc(keys, func(a, b codeKey) int {
		if c := strings.Compare(string(a.kind), string(b.kind)); c != 0 {
			return c
		}
		return strings.Compare(a.id, b.id)
	})
	return keys
}

// equalRun reports whether two runs are the same code or text, field for
// field.
func equalRun(a, b model.Run) bool { return reflect.DeepEqual(a, b) }
