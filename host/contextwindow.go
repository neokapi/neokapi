package host

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
)

// What a submitted rule carries beyond its subject (C-11): the point it holds
// at, narrowed from the point its evidence resolves to, and the window of time
// it holds for. Every surface that records or keeps a rule (the command line,
// the agent tools, the desktop) passes the same fields here, so the checks
// and the errors are one set.

// ruleWindow reads a rule's lifetime window: each bound a bare date or an
// RFC3339 instant, read the way a profile's window is
// (project.ParseValidityBound), with valid_from inclusive and valid_to
// exclusive. Both empty is no window.
func ruleWindow(from, to string) (*time.Time, *time.Time, error) {
	start, err := project.ParseValidityBound(from)
	if err != nil {
		return nil, nil, fmt.Errorf("valid_from: %w", err)
	}
	end, err := project.ParseValidityBound(to)
	if err != nil {
		return nil, nil, fmt.Errorf("valid_to: %w", err)
	}
	if start != nil && end != nil && !end.After(*start) {
		return nil, nil, fmt.Errorf("valid_to %s is not after valid_from %s: the window holds from valid_from up to, and not including, valid_to",
			strings.TrimSpace(to), strings.TrimSpace(from))
	}
	return start, end, nil
}

// withWindow sets a scope's lifetime window from the two bounds a request
// carries, leaving the scope as it was when both are empty.
func withWindow(scope contextop.Scope, from, to string) (contextop.Scope, error) {
	start, end, err := ruleWindow(from, to)
	if err != nil {
		return scope, err
	}
	if start != nil {
		scope.ValidFrom = start
	}
	if end != nil {
		scope.ValidTo = end
	}
	// One bound may arrive now and the other have been set when the rule was
	// recorded; together they must still leave a window that holds somewhere.
	if scope.ValidFrom != nil && scope.ValidTo != nil && !scope.ValidTo.After(*scope.ValidFrom) {
		return scope, fmt.Errorf("the rule would hold from %s up to %s, which is no time at all: give a valid_to after %s, or a valid_from before %s",
			scope.ValidFrom.Format(time.DateOnly), scope.ValidTo.Format(time.DateOnly),
			scope.ValidFrom.Format(time.DateOnly), scope.ValidTo.Format(time.DateOnly))
	}
	return scope, nil
}

// narrowScope applies a submitted point to the scope its evidence resolved to.
//
// A point may only narrow. For each axis it names, the value must be legal for
// the axis as the recipe types it; where the evidence's point already sets the
// axis, the new value must sit within that one (axis.Space.Contains), and
// where it does not, the axis is added. Widening a rule is a person's act in
// review (--widen-to), so a value outside the evidence's point is refused with
// that route named. The structural axes come from the file the evidence was
// seen in and are never written by hand.
func (s *contextOpsSession) narrowScope(scope contextop.Scope, point map[string]string) (contextop.Scope, error) {
	if len(point) == 0 {
		return scope, nil
	}
	space := s.proj.AxisSpace()
	// An axis no content sits on would leave the rule answering nowhere, so a
	// point names one the recipe declares, or one the evidence already sits on.
	declared := map[string]bool{}
	for _, name := range s.proj.DeclaredAxisNames() {
		declared[name] = true
	}
	out := make(map[string]string, len(scope.Coordinates)+len(point))
	maps.Copy(out, scope.Coordinates)
	names := make([]string, 0, len(point))
	for name := range point {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, raw := range names {
		name, value := strings.TrimSpace(raw), strings.TrimSpace(point[raw])
		if name == "" || value == "" {
			return scope, errors.New("a point names an axis and a value, such as market=no")
		}
		if name == project.ProductAxis || name == project.ChannelAxis {
			return scope, fmt.Errorf("%s=%s: the %s comes from the file the evidence was seen in, so name a file that sits there as the evidence (--seen-in, or path)",
				name, value, name)
		}
		if _, held := out[name]; s.proj != nil && !declared[name] && !held {
			known := s.proj.DeclaredAxisNames()
			if len(known) == 0 {
				return scope, fmt.Errorf("%s=%s: the recipe declares no axis called %s, so no content sits on it and the rule would answer nowhere. "+
					"Declare it under defaults.coordinates or a collection's coordinates first", name, value, name)
			}
			return scope, fmt.Errorf("%s=%s: the recipe declares no axis called %s, so no content sits on it and the rule would answer nowhere. "+
				"The axes it declares are %s", name, value, name, strings.Join(known, ", "))
		}
		if err := space.Validate(name, value); err != nil {
			return scope, fmt.Errorf("%s=%s: %w", name, value, err)
		}
		if held, ok := out[name]; ok && !space.Contains(name, held, value) {
			return scope, fmt.Errorf("%s=%s: the evidence sits at %s=%s, and a note may narrow where it holds, never widen it. "+
				"Name a value within %s, or record it there and ask a person to widen it with `kapi context review --keep <id> --widen-to %s`",
				name, value, name, held, held, name)
		}
		out[name] = value
	}
	scope.Coordinates = out
	return scope, nil
}

// submittedScope is the scope a recorded rule takes: the one its evidence
// resolved to, narrowed by the submitted point and bounded by the submitted
// window.
func (s *contextOpsSession) submittedScope(scope contextop.Scope, point map[string]string, from, to string) (contextop.Scope, error) {
	scope, err := s.narrowScope(scope, point)
	if err != nil {
		return scope, err
	}
	return withWindow(scope, from, to)
}
