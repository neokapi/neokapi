package backend

import (
	"errors"
	"log/slog"

	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// enqueue adds a typed mutation to the offline outbox for later replay. The op
// carries both its persisted kind (op.opKind()) and its replay behavior, so the
// outbox is typed end-to-end. Silently logs on failure (a dropped queue entry
// is non-fatal — the reconnect refresh pulls authoritative server state anyway).
func (a *App) enqueue(op offlineOp) {
	if a.offlineQueue == nil {
		return
	}
	if err := a.offlineQueue.Enqueue(string(op.opKind()), op); err != nil {
		slog.Info("bowrain: failed to enqueue", "id", op.opKind(), "error", err)
	}
}

// permanentRejection reports whether a remote mutation failed because the
// server rejected the request itself (a permanent 4xx StatusError, e.g. a 422
// review of an untranslated block). Such a failure is not a connectivity
// problem: the write-through spines surface it to the caller instead of
// dropping to offline mode and enqueueing a replay that can never succeed.
func permanentRejection(err error) bool {
	var statusErr *apiclient.StatusError
	return errors.As(err, &statusErr) && statusErr.Permanent()
}

// writeThroughVoid is the server-first write-through spine for mutations that
// return only an error. When connected it calls remoteFn; on success it returns
// onSuccess() (pass nil to reconcile the local cache via localFn — the
// cache-authoritative case; pass an explicit func for server-authoritative ops
// that skip the local write). A permanent server rejection (4xx) is returned to
// the caller as-is — no offline transition, no enqueue, no local apply. On a
// network failure it drops to offline mode, enqueues op for replay, and falls
// through to localFn. When already offline it enqueues op and runs localFn. In
// pure local mode it just runs localFn.
func (a *App) writeThroughVoid(op offlineOp, remoteFn func() error, onSuccess func() error, localFn func() error) error {
	if a.isConnected() {
		if err := remoteFn(); err != nil {
			if permanentRejection(err) {
				return err
			}
			a.goOffline()
			a.enqueue(op)
		} else if onSuccess != nil {
			return onSuccess()
		} else {
			return localFn()
		}
	} else if a.isOffline() {
		a.enqueue(op)
	}
	return localFn()
}

// writeThroughResult is the server-first write-through spine for mutations that
// return a value. When connected it calls remoteFn; on success it runs the
// optional onSuccess side-effect (e.g. refreshing the local cache) and returns
// the server's result. A permanent server rejection (4xx) is returned to the
// caller as-is — no offline transition, no enqueue, no local apply. On a
// network failure it drops to offline mode, enqueues op for replay, and falls
// through to localFn; when already offline it enqueues op and runs localFn; in
// pure local mode it just runs localFn.
func writeThroughResult[V any](a *App, op offlineOp, remoteFn func() (V, error), onSuccess func(V), localFn func() (V, error)) (V, error) {
	if a.isConnected() {
		v, err := remoteFn()
		if err != nil {
			if permanentRejection(err) {
				var zero V
				return zero, err
			}
			a.goOffline()
			a.enqueue(op)
		} else {
			if onSuccess != nil {
				onSuccess(v)
			}
			return v, nil
		}
	} else if a.isOffline() {
		a.enqueue(op)
	}
	return localFn()
}
