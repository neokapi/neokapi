package observe

import (
	"context"
	"strings"

	sentry "github.com/getsentry/sentry-go"
)

// Scope is the set of dimensions a trace should be filterable by.
//
// Duration alone answers "is it slow" and nothing a person can act on. The
// questions that get asked are "is it slow for THIS customer", "is it only the
// enterprise plan", "is it this project or all of them", "is it the dashboard or
// everything" — and none of them are answerable unless the trace carries the
// dimension. A transaction with a duration and no scope is a number without a
// subject.
//
// These are TAGS, never part of the transaction name: tags are indexed for
// filtering and may be high-cardinality, while a name is what Sentry groups by
// and must stay one row per shape of work. Putting a workspace id in a name
// makes every customer their own transaction and the aggregate disappears.
type Scope struct {
	// WorkspaceID is the customer. Opaque id rather than slug: a slug is a
	// display name that can be changed and read over someone's shoulder, and
	// the id is what every other system already joins on.
	WorkspaceID string
	// ProjectID scopes within a customer.
	ProjectID string
	// Plan is the entitlement tier — free, pro, enterprise. The lowest
	// cardinality dimension here and the highest value: "the slow ones are all
	// on the trial plan" is a finding, and it is one query.
	Plan string
	// Feature is the product area, derived from the route rather than declared,
	// so it cannot drift from what the code actually serves.
	Feature string
	// UserID is the acting user, opaque. Deliberately not name or email: those
	// are what SendDefaultPII=false and the BeforeSend scrubber exist to keep
	// out, and an id answers "is it one user or all of them" just as well.
	UserID string
}

// TagScope puts the dimensions on whatever transaction ctx is carrying.
//
// Empty fields are skipped rather than written as "", so a filter for a missing
// dimension finds nothing instead of finding everything that lacked it.
func TagScope(ctx context.Context, s Scope) {
	if !sentryEnabled {
		return
	}
	t := sentry.TransactionFromContext(ctx)
	if t == nil {
		return
	}
	for k, v := range map[string]string{
		"workspace_id": s.WorkspaceID,
		"project_id":   s.ProjectID,
		"plan":         s.Plan,
		"feature":      s.Feature,
		"user_id":      s.UserID,
	} {
		if v != "" {
			t.SetTag(k, v)
		}
	}
}

// FeatureFromRoute names the product area a route belongs to.
//
// Derived from the registered pattern rather than declared in a table, because
// a table drifts: a route added to a new area would keep whatever the table
// last said, and nobody would notice a filter quietly under-reporting.
//
// The rule is the first path segment that is not a version, a parameter, or the
// api prefix — `/api/v1/:ws/:id/dashboard/:ref` is "dashboard",
// `/api/v1/:ws/projects` is "projects". A route with nothing but parameters
// after the prefix is "workspace", which is what those routes serve.
func FeatureFromRoute(route string) string {
	if route == "" {
		return ""
	}
	for _, seg := range strings.Split(route, "/") {
		switch {
		case seg == "", seg == "api", seg == "v1":
			continue
		case strings.HasPrefix(seg, ":"), strings.HasPrefix(seg, "*"):
			continue
		default:
			return seg
		}
	}
	return "workspace"
}
