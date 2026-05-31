package store

import "context"

// ChangeContext carries audit-relevant attribution for content writes so the
// store can record who changed what, why, and as part of which batch. It is set
// by the server (auth + project-access middleware and individual handlers) and
// read when appending block_history / change_log rows.
type ChangeContext struct {
	Actor         string // user ID performing the change
	ActorRole     string // the actor's workspace role at the time
	Reason        string // optional intent, e.g. "rollback", "revert_push", "import"
	CorrelationID string // groups all changes from one request/push/merge
}

type changeCtxKey struct{}

// WithChangeContext overlays the given non-empty fields onto any ChangeContext
// already on the context, returning a new context. This lets the auth
// middleware set the actor + correlation id early and later layers (project
// access, specific handlers) enrich the role or reason without clobbering.
func WithChangeContext(ctx context.Context, cc ChangeContext) context.Context {
	cur := ChangeContextFromContext(ctx)
	if cc.Actor != "" {
		cur.Actor = cc.Actor
	}
	if cc.ActorRole != "" {
		cur.ActorRole = cc.ActorRole
	}
	if cc.Reason != "" {
		cur.Reason = cc.Reason
	}
	if cc.CorrelationID != "" {
		cur.CorrelationID = cc.CorrelationID
	}
	return context.WithValue(ctx, changeCtxKey{}, cur)
}

// ChangeContextFromContext returns the ChangeContext on the context, or the zero
// value if none is set.
func ChangeContextFromContext(ctx context.Context) ChangeContext {
	if v, ok := ctx.Value(changeCtxKey{}).(ChangeContext); ok {
		return v
	}
	return ChangeContext{}
}
