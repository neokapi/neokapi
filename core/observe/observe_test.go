package observe

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTracer records what it was asked to open.
type fakeTracer struct{ spans []*fakeSpan }

func (f *fakeTracer) Start(ctx context.Context, op, name string) (context.Context, Span) {
	s := &fakeSpan{op: op, name: name, attrs: map[string]string{}}
	f.spans = append(f.spans, s)
	return ctx, s
}

type fakeSpan struct {
	op, name string
	attrs    map[string]string
	ended    bool
	err      error
}

func (s *fakeSpan) SetAttr(k, v string) { s.attrs[k] = v }
func (s *fakeSpan) End(err error)       { s.ended, s.err = true, err }

// register installs a tracer for one test and takes it out again, so tests that
// touch process state do not leak into their neighbours.
func register(t *testing.T, tr Tracer) {
	t.Helper()
	Register(tr)
	t.Cleanup(func() { Register(nil) })
}

// An unconfigured framework traces nothing and works anyway.
//
// This is the case every kapi and kapi-desktop run is in, so it is the one that
// has to be free and impossible to get wrong.
func TestStart_WithoutATracerIsANoOp(t *testing.T) {
	ctx, span := Start(context.Background(), "gen_ai.call", "anthropic chat")

	assert.Equal(t, context.Background(), ctx, "the context is handed back untouched")
	assert.NotPanics(t, func() {
		span.SetAttr("k", "v")
		span.End(errors.New("boom"))
	})
}

// The no-op costs no allocation.
//
// Asserted rather than assumed: this sits on every call in a wrapped provider,
// and a Span interface holding a pointer instead of a zero-size value would put
// an allocation on a path that is supposed to be free when tracing is off.
func TestStart_NoOpDoesNotAllocate(t *testing.T) {
	ctx := context.Background()
	avg := testing.AllocsPerRun(100, func() {
		_, span := Start(ctx, "gen_ai.call", "anthropic chat")
		span.End(nil)
	})
	assert.Zero(t, avg)
}

func TestStart_RegisteredTracerSeesTheSpan(t *testing.T) {
	tr := &fakeTracer{}
	register(t, tr)

	_, span := Start(context.Background(), "gen_ai.call", "anthropic chat")
	span.SetAttr("gen_ai.system", "anthropic")
	span.End(nil)

	require.Len(t, tr.spans, 1)
	assert.Equal(t, "gen_ai.call", tr.spans[0].op)
	assert.Equal(t, "anthropic chat", tr.spans[0].name)
	assert.Equal(t, "anthropic", tr.spans[0].attrs["gen_ai.system"])
	assert.True(t, tr.spans[0].ended)
}

// A context tracer wins over the registered one, so a test or a tenant-scoped
// host can redirect spans without disturbing the process.
func TestStart_ContextTracerOverridesTheRegisteredOne(t *testing.T) {
	global, scoped := &fakeTracer{}, &fakeTracer{}
	register(t, global)

	_, span := Start(WithTracer(context.Background(), scoped), "flow.tool", "segment")
	span.End(nil)

	assert.Empty(t, global.spans)
	require.Len(t, scoped.spans, 1)
	assert.Equal(t, "segment", scoped.spans[0].name)
}

// The failure is carried to the backend rather than swallowed: a span that ends
// without its error looks like a success that happened to be slow.
func TestSpan_EndCarriesTheError(t *testing.T) {
	tr := &fakeTracer{}
	register(t, tr)

	boom := errors.New("upstream refused")
	_, span := Start(context.Background(), "gen_ai.call", "anthropic chat")
	span.End(boom)

	require.Len(t, tr.spans, 1)
	assert.Equal(t, boom, tr.spans[0].err)
}

func TestRegister_NilClearsTheTracer(t *testing.T) {
	tr := &fakeTracer{}
	register(t, tr)
	Register(nil)

	_, span := Start(context.Background(), "gen_ai.call", "anthropic chat")
	span.End(nil)

	assert.Empty(t, tr.spans)
}
