//go:build js

package cli

// The wasm build of this module (`kapi/cmd/kapi-wasm-cli`, the CLI behind the
// docs playground) registers no form prompter.
//
// Not a limitation being worked around: there is no terminal in a browser, so
// there is nothing for a terminal form to draw on. `RunAISetupWizard` gates on
// IsTTY before it consults a prompter, and the wasm CLI is never a TTY, so the
// wizard cannot be reached there at all — it returns its actionable
// "not a terminal" error, which is the honest answer in a browser.
//
// Keeping the huh/bubbletea import out of this target is therefore both correct
// and necessary: bubbletea does not build for js/wasm (it reaches for a TTY,
// SIGWINCH, and the system clipboard), and a payload the browser downloads
// should not carry a terminal UI it can never use.
//
// installAISetupPrompter exists so the registration call site is
// platform-independent; here it is a no-op.
func installAISetupPrompter(*App) {}

func init() {
	RegisterAppInitializer(installAISetupPrompter)
}
