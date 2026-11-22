package host

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// unsafeTargetWarning turns the refusal of a target, for a source whose name
// is not one safe path segment or whose target is a link out of the project,
// into the warning that names the file. It reports false for any other error,
// which the caller handles as it did.
func unsafeTargetWarning(file string, locale model.LocaleID, err error) (check.Warning, bool) {
	if use := (*project.UnsafeSegmentError)(nil); errors.As(err, &use) {
		return check.Warning{
			Code:    check.WarningUnsafeTargetName,
			Message: fmt.Sprintf("%s has no %s target: %v. Rename the file to give it one", file, locale, use),
			Source:  file,
		}, true
	}
	if out := (*project.TargetOutsideError)(nil); errors.As(err, &out) {
		return check.Warning{
			Code:    check.WarningTargetOutside,
			Message: fmt.Sprintf("%s has no %s target: %v. Remove the link to give it one", file, locale, out),
			Source:  file,
		}, true
	}
	return check.Warning{}, false
}

// unsafeTargetWarnings names every source file the project's coverage leaves
// out because its name cannot fill its collection's target template, or
// because its target is a link out of the project, so a report that counts
// what it covers also says what it could not.
func (a *App) unsafeTargetWarnings(proj *project.KapiProject, root, localeFilter string) []check.Warning {
	pctx := project.NewProjectContext(proj, filepath.Join(root, project.RecipeFileName))
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil
	}
	var out []check.Warning
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" || rf.CommentsOnly() {
			continue
		}
		for _, loc := range rf.Item.ResolvedTargetLanguages(nil, proj.Defaults) {
			if localeFilter != "" && string(loc) != localeFilter {
				continue
			}
			target, terr := project.ResolveTargetPathIn(rf.Item.Path, rf.Item.Base, rf.Item.Target, rf.Relative, string(loc), proj.Defaults.LocaleFormat)
			if terr == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(root, target)
				}
				terr = project.CheckTargetInside(root, target)
			}
			if w, ok := unsafeTargetWarning(filepath.ToSlash(rf.Relative), loc, terr); ok {
				out = append(out, w)
			}
		}
	}
	return out
}

// unsafeTargetsError is the error a command returns, after writing every
// other file, when the warnings name sources it wrote no target for. It is
// nil when there are none.
func unsafeTargetsError(warnings []check.Warning) error {
	if len(warnings) == 0 {
		return nil
	}
	files := make([]string, 0, len(warnings))
	seen := map[string]bool{}
	for _, w := range warnings {
		if !seen[w.Source] {
			seen[w.Source] = true
			files = append(files, w.Source)
		}
	}
	return fmt.Errorf("merge: no target was written for %s, named above; every other file was written", strings.Join(files, ", "))
}
