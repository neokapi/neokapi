//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// The browser MT provider bridges the in-WASM MT tools to the platform's built-in
// Translator API (Chrome 138+, desktop), an on-device translator the browser
// manages itself. It is the browser counterpart of the keyed MT providers
// (deepl/google/…): the page cannot reach a credentialed API, so instead it calls
// out — via syscall/js — to a host function, exactly as the "gemma" AI provider
// and the "intl" segmentation engine do.
//
// # Host contract
//
//	globalThis.kapiBrowserTranslate(payloadJSON: string)
//	  => Promise<{ ok: boolean, translation?: string, error?: string }>
//
// payloadJSON is {text, sourceLang, targetLang}. The JS glue
// (web/src/lib/browserTranslate.ts) drives Translator.create(...).translate(...).
// When the function (or the API) is absent the provider returns a clear error so a
// page without it degrades visibly (the demo provider is the fallback).
const browserTranslateJSFunc = "kapiBrowserTranslate"

// BrowserMTProviderID is the MT-provider id the in-browser host registers. Kept
// out of the framework (providers/mt stays platform-agnostic); the wasm wiring
// lets `--provider browser` through (see forceDemoProviders).
const browserMTProviderID mtprovider.ProviderID = "browser"

func init() {
	mtprovider.RegisterProvider(browserMTProviderID, func() mtprovider.MTProvider { return &browserMTProvider{} })
	// Also config-constructible (ignores all credentials) so the standard
	// <provider>-translate config path can build it from a recipe/CLI config.
	mtprovider.RegisterConfigFactory(browserMTProviderID, func(_ mtprovider.MTConfig) mtprovider.MTProvider {
		return &browserMTProvider{}
	})
}

// browserTranslatorAvailable reports whether the page installed the bridge AND the
// platform exposes the Translator API, so callers can pick browser-if-available
// and fall back to the demo provider otherwise.
func browserTranslatorAvailable() bool {
	return js.Global().Get(browserTranslateJSFunc).Truthy() &&
		js.Global().Get("Translator").Truthy()
}

// resolveWasmMTProvider picks the MT engine for an mt-translate run in the
// browser: the on-device Translator API for the default/unset provider and for
// an explicit `--provider browser` WHEN the page supports it; otherwise (no
// Translator API, or an explicit `--provider demo`) the keyless demo provider.
// So the demos translate for real on a capable desktop Chrome and still produce
// illustrative output everywhere else. config may be nil (the no-config factory).
func resolveWasmMTProvider(config map[string]any) mtprovider.MTProvider {
	prov, _ := config["provider"].(string)
	if prov == string(mtprovider.Demo) {
		return mtprovider.NewDemoProvider()
	}
	if (prov == "" || prov == string(browserMTProviderID)) && browserTranslatorAvailable() {
		return &browserMTProvider{}
	}
	return mtprovider.NewDemoProvider()
}

type browserMTProvider struct{}

func (p *browserMTProvider) Name() mtprovider.ProviderID { return browserMTProviderID }
func (p *browserMTProvider) Close() error                { return nil }

func (p *browserMTProvider) Translate(ctx context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
	fn := js.Global().Get(browserTranslateJSFunc)
	if !fn.Truthy() {
		return nil, errors.New("browser translator not available: host did not define globalThis." +
			browserTranslateJSFunc + " — requires desktop Chrome 138+ with the Translator API; call installBrowserTranslateBridge() on the page")
	}
	payload := map[string]any{
		"text":       req.Source,
		"sourceLang": mtBaseLang(req.SourceLocale),
		"targetLang": mtBaseLang(req.TargetLocale),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// awaitPromise is shared with the gemma bridge (same js+wasm package): it parks
	// the goroutine on the JS promise while the event loop keeps running.
	res, err := awaitPromise(ctx, fn.Invoke(string(b)))
	if err != nil {
		return nil, err
	}
	trans, err := parseBrowserTranslateResult(res)
	if err != nil {
		return nil, err
	}
	return &mtprovider.TranslateResponse{Translation: trans}, nil
}

// mtBaseLang reduces a locale id to its base language tag ("fr-FR" → "fr"), which
// the Translator API expects.
func mtBaseLang(loc model.LocaleID) string {
	s := strings.ToLower(string(loc))
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		return s[:i]
	}
	return s
}

// parseBrowserTranslateResult reads the host function's resolved value, which is
// either a bare string or { ok, translation?, error? }.
func parseBrowserTranslateResult(res js.Value) (string, error) {
	switch res.Type() {
	case js.TypeString:
		return res.String(), nil
	case js.TypeObject:
		if e := res.Get("error"); e.Truthy() {
			return "", errors.New("browser translator: " + e.String())
		}
		if t := res.Get("translation"); t.Truthy() {
			return t.String(), nil
		}
		return "", errors.New("browser translator: empty result")
	default:
		return "", errors.New("browser translator: unexpected result")
	}
}
