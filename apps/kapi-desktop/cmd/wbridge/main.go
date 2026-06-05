// Command wbridge exposes the REAL Kapi Desktop backend (backend.App) over HTTP
// so the genuine frontend can run in a browser for walkthrough recording.
//
// On macOS the Wails runtime is served through the native WKWebView's custom
// scheme handler (not TCP), so a plain browser can't reach it. wbridge hosts the
// same backend.App and dispatches calls by method name via reflection, reading
// the same SQLite termbases/TMs the app reads. Point it at an isolated config
// root for testing:
//
//	KAPI_CONFIG_DIR=/tmp/iso/kapi WBRIDGE_PORT=5175 \
//	  go run -tags fts5 ./cmd/wbridge
//
// The frontend installs a custom Wails transport (see src/demo/real-main.tsx)
// that forwards binding calls to /wbridge. Nothing here is mocked: same backend
// code, same packages, same on-disk databases — only the transport differs (as
// it already does between Wails' macOS and Windows/Linux runtimes).
//
// SECURITY — this is a RECORDING/DEV-ONLY tool, never part of a shipped build
// (it is not imported by the app's main.go). It exposes the whole backend over
// HTTP, so it deliberately refuses to run unless:
//   - KAPI_CONFIG_DIR is set to an ISOLATED dir (never the user's real config),
//     so it can only ever operate on throwaway data; and
//   - it binds to 127.0.0.1 only, with CORS limited to the local dev origin.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	// Parity with the desktop app: register bowrain recipe schema extensions.
	_ "github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/neokapi/neokapi/kapi-desktop/backend"
)

// eventHub fans out backend events to all connected SSE clients. The desktop
// app delivers events to the frontend over the Wails runtime; in the browser
// there is no such channel, so the recorder subscribes here and re-dispatches
// each event into the Wails runtime client-side (see real-main.tsx).
type eventHub struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: make(map[chan []byte]struct{})}
}

func (h *eventHub) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// publish marshals one event and fans it out to every subscriber. Slow
// subscribers are skipped (non-blocking send) so a stalled client can't wedge
// the backend goroutine that emitted the event.
func (h *eventHub) publish(name string, data any) {
	payload, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	for ch := range h.subscribers {
		select {
		case ch <- payload:
		default:
		}
	}
	h.mu.Unlock()
}

var errType = reflect.TypeOf((*error)(nil)).Elem()

type callRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

// requireIsolatedConfig refuses to run unless KAPI_CONFIG_DIR is set to a dir
// that is NOT the user's default config — so wbridge can never expose the real
// backend/credentials, even if launched by mistake.
func requireIsolatedConfig() {
	dir := os.Getenv("KAPI_CONFIG_DIR")
	if dir == "" {
		log.Fatal("wbridge refuses to run: set KAPI_CONFIG_DIR to an isolated directory. " +
			"This tool exposes the backend over HTTP for recording only and must never serve your real config.")
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		def := filepath.Join(cfg, "kapi")
		abs, _ := filepath.Abs(dir)
		same := abs == def
		// Resolve symlinks too, so a link that ultimately points at the real
		// config dir can't slip past the literal-path comparison.
		if dr, e1 := filepath.EvalSymlinks(dir); e1 == nil {
			if df, e2 := filepath.EvalSymlinks(def); e2 == nil && dr == df {
				same = true
			}
		}
		if same {
			log.Fatalf("wbridge refuses to run: KAPI_CONFIG_DIR (%s) resolves to your default kapi config. Use an isolated directory.", dir)
		}
	}
}

func main() {
	requireIsolatedConfig()
	// CORS origin limited to the local dev server (overridable for other ports).
	allowOrigin := os.Getenv("WBRIDGE_ORIGIN")
	if allowOrigin == "" {
		allowOrigin = "http://localhost:5174"
	}

	app := backend.NewApp()
	appVal := reflect.ValueOf(app)

	// Stream backend events (plugin install, flow:event, …) to connected
	// browsers over SSE; real-main.tsx re-dispatches them into the Wails runtime.
	hub := newEventHub()
	app.SetEventSink(hub.publish)

	mux := http.NewServeMux()
	mux.HandleFunc("/wbridge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, fmt.Sprintf("panic in %v", rec), http.StatusInternalServerError)
			}
		}()

		var req callRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		m := appVal.MethodByName(req.Method)
		if !m.IsValid() {
			http.Error(w, "unknown method: "+req.Method, http.StatusNotFound)
			return
		}
		mt := m.Type()
		if mt.IsVariadic() {
			http.Error(w, "variadic methods unsupported: "+req.Method, http.StatusBadRequest)
			return
		}

		in := make([]reflect.Value, mt.NumIn())
		for i := range mt.NumIn() {
			pv := reflect.New(mt.In(i))
			if i < len(req.Args) && len(req.Args[i]) > 0 {
				if err := json.Unmarshal(req.Args[i], pv.Interface()); err != nil {
					http.Error(w, fmt.Sprintf("arg %d for %s: %v", i, req.Method, err), http.StatusBadRequest)
					return
				}
			}
			in[i] = pv.Elem()
		}

		var result any
		var callErr error
		for _, o := range m.Call(in) {
			if o.Type().Implements(errType) {
				if !o.IsNil() {
					callErr = o.Interface().(error)
				}
				continue
			}
			result = o.Interface()
		}
		if callErr != nil {
			http.Error(w, callErr.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	// SSE: stream backend events to the browser. The frontend re-dispatches each
	// into the Wails runtime so useWailsEvent() fires exactly as it does natively.
	mux.HandleFunc("/wevents", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch := hub.subscribe()
		defer hub.unsubscribe(ch)
		// Open the stream immediately so the client's EventSource fires `onopen`.
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case payload, open := <-ch:
				if !open {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	})

	port := os.Getenv("WBRIDGE_PORT")
	if port == "" {
		port = "5175"
	}
	addr := "127.0.0.1:" + port
	log.Printf("wbridge listening on http://%s/wbridge (config=%s)", addr, kapiConfigInfo())
	log.Fatal(http.ListenAndServe(addr, mux))
}

func kapiConfigInfo() string {
	if d := os.Getenv("KAPI_CONFIG_DIR"); d != "" {
		return d
	}
	return "<default>"
}
