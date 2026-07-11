package host

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// convergeWorker copies App field-by-field (App holds a sync.Once, so a struct
// copy is out). That copy is easy to forget to extend: a field added to App but
// not to the clone reads as its zero value inside every converge worker — a
// credential store or project context that is simply absent the moment a run
// fans out, with no compiler complaint.
//
// This test reconciles three things that must agree: App's actual fields, the
// convergeWorkerFields policy, and what the clone produces. Add a field to App
// and this fails until you have said what a worker does with it AND done it.
func TestConvergeWorker_CoversEveryAppField(t *testing.T) {
	fields := reflect.VisibleFields(reflect.TypeFor[App]())

	// 1. Census: every field is classified, and nothing stale lingers.
	seen := map[string]bool{}
	for _, f := range fields {
		seen[f.Name] = true
		_, ok := convergeWorkerFields[f.Name]
		assert.True(t, ok,
			"App.%s is not classified in convergeWorkerFields: decide whether a converge "+
				"worker shares the parent's value or owns its own, then wire it into convergeWorker",
			f.Name)
	}
	for name := range convergeWorkerFields {
		assert.True(t, seen[name], "convergeWorkerFields lists %q, which App no longer has", name)
	}

	// 2. Behaviour: a parent with every fabricable field set to a distinguishable
	// value must hand each shared field's value through to the worker.
	parent := &App{}
	// Pre-seed the runtime so ensurePluginRuntime hands it straight over instead
	// of building a real one against the fabricated PluginHost.
	parent.pluginRuntime = &pluginhost.Runtime{}
	pv := reflect.ValueOf(parent).Elem()
	fabricated := map[string]bool{}
	for _, f := range fields {
		if convergeWorkerFields[f.Name] == fieldOwned || f.Name == "pluginRuntime" {
			continue
		}
		if fabricate(pv.FieldByIndex(f.Index)) {
			fabricated[f.Name] = true
		}
	}
	require.NotEmpty(t, fabricated, "no App field could be fabricated — the check would be vacuous")

	tap := newConvergeTap("nb-NO")
	worker := parent.convergeWorker("nb-NO", tap)
	wv := reflect.ValueOf(worker).Elem()

	for _, f := range fields {
		if !fabricated[f.Name] {
			continue // interfaces/structs we cannot fabricate: census-only
		}
		assert.True(t, sameValue(pv.FieldByIndex(f.Index), wv.FieldByIndex(f.Index)),
			"App.%s is shared, but the converge worker did not inherit the parent's value "+
				"(missing from convergeWorker's field-by-field copy?)", f.Name)
	}

	// 3. The owned fields are owned as documented.
	assert.True(t, worker.Quiet, "workers are silent — progress speaks the event protocol")
	assert.Equal(t, "nb-NO", worker.TargetLang, "a worker is pinned to one locale")
	assert.Same(t, tap, worker.convergeProgressTap, "each worker counts its own locale's units")
	assert.Nil(t, worker.projectFlowTools, "a worker builds its own tool instances")
	assert.Same(t, parent.pluginRuntime, worker.pluginRuntime, "workers share one plugin runtime")
}

// fabricate sets v to a distinguishable non-zero value, reporting whether it
// could. Interfaces and plain structs are left alone: there is no generic way
// to conjure an inhabitant, so those fields are covered by the census only.
func fabricate(v reflect.Value) bool {
	v = addressable(v)
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.String:
		v.SetString("sentinel")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
	case reflect.Func:
		v.Set(reflect.MakeFunc(v.Type(), func(args []reflect.Value) []reflect.Value {
			out := make([]reflect.Value, v.Type().NumOut())
			for i := range out {
				out[i] = reflect.Zero(v.Type().Out(i))
			}
			return out
		}))
	default:
		return false
	}
	return true
}

// sameValue compares two field values, tolerating the kinds reflect.DeepEqual
// can't meaningfully compare (funcs are compared by code pointer).
func sameValue(a, b reflect.Value) bool {
	a, b = addressable(a), addressable(b)
	if a.Kind() == reflect.Func {
		return a.Pointer() == b.Pointer()
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}

// addressable re-derives a field value through its address so unexported fields
// can be read and written like exported ones. App's unexported state (docCache,
// translator, the plugin runtime) is exactly the state a forgotten copy would
// drop, so the check would be worth little if it skipped them.
func addressable(v reflect.Value) reflect.Value {
	if v.CanInterface() && v.CanSet() {
		return v
	}
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}
