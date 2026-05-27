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
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"

	// Parity with the desktop app: register bowrain recipe schema extensions.
	_ "github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/neokapi/neokapi/kapi-desktop/backend"
)

var errType = reflect.TypeOf((*error)(nil)).Elem()

type callRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

func main() {
	app := backend.NewApp()
	appVal := reflect.ValueOf(app)

	mux := http.NewServeMux()
	mux.HandleFunc("/wbridge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
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
		for i := 0; i < mt.NumIn(); i++ {
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
