# Demo-video harness — records kapi driven by Claude Code as narrated explainer
# videos and publishes them (theme-matched light + dark) into the docs site.
#
#   make deps                          # install node deps + Playwright chromium
#   make videos                        # render + convert the published demos → docs webm
#   make render                        # re-render the published demos (no publish)
#   make publish                       # convert existing renders → docs webm (no re-render)
#   make capture DEMO=03-translate-docx  # full pipeline for one demo (needs kapi + creds)
#   make studio                        # open Remotion Studio (live preview)
#
# Phased pipeline (record everything, then narrate, then package):
#   make record    # phase 1: capture all screencasts + artifacts (needs the stack)
#   make narrate   # phase 2: synthesize all narration (no Docker; TTS only)
#   make package   # phase 3: render + publish all assets (no Docker, no network)
# Each phase runs one stage-group across ALL published demos and is idempotent
# (skips existing outputs); pass FORCE=1 to redo. The bowrain stack is needed
# only for `record` — see ../Makefile `harness-videos-staged`, which brings it up
# once around phase 1. `package` re-renders freely from persisted captures.
#
# Rendering + publishing from existing captures need only node + ffmpeg. Capturing
# also needs a built kapi (`make -C .. build`), the kapi-claude plugin bundle, and
# Gemini/Anthropic credentials.
#
# TypeScript is pinned to 5.9.x here, not the repo-wide version. Every target
# that bundles (studio, render, videos, package) goes through @remotion/bundler,
# which consumes the TypeScript compiler API — `typescript.sys` and
# `typescript.readConfigFile`. TypeScript 7.0 ships no compiler API, so on 7.0
# both are undefined and the bundle fails; the stable API is targeted for 7.1.
# Lift the pin when a @remotion/* release runs on the 7.1 compiler API, and the
# harness rejoins the repo-wide TypeScript version. See README.md.

VPX := vpx
RUN := $(VPX) tsx src/cli/run.ts

.PHONY: deps check test typecheck list studio videos render publish capture seed record narrate package

deps: ## Install node deps + Playwright chromium
	corepack pnpm install
	$(VPX) playwright install chromium

check: typecheck test ## Typecheck + run the unit tests

typecheck: ## Typecheck the harness sources
	$(VPX) tsc --noEmit

test: ## Run the unit tests (no kapi binary, no network, no stack)
	$(VPX) tsx --test "src/**/*.test.ts"

list: ## List available demos
	$(RUN) --list

studio: ## Open Remotion Studio (live preview)
	$(VPX) remotion studio src/remotion/index.ts

videos: ## Render + publish the docs videos (light + dark)
	$(RUN) published --only=render,publish --theme=both --force

render: ## Re-render the published demos (both themes, no publish)
	$(RUN) published --only=render --theme=both --force

publish: ## Convert existing renders → docs webm (no re-render)
	$(RUN) published --only=publish

capture: ## Full pipeline (capture → render → publish) for DEMO=<id>
	@test -n "$(DEMO)" || { echo "usage: make capture DEMO=<id>"; exit 1; }
	$(RUN) $(DEMO) --force

seed: ## Phase 0: seed bowrain content + mint record tokens → harness/.env (idempotent; needs the stack)
	$(VPX) tsx scripts/seed-bowrain.ts

record: ## Phase 1: capture all screencasts + artifacts (needs the bowrain stack; FORCE=1 to re-record)
	$(RUN) published --phase=record $(if $(FORCE),--force)

narrate: ## Phase 2: synthesize all narration (no Docker; TTS only; FORCE=1 to re-synth)
	$(RUN) published --phase=narrate $(if $(FORCE),--force)

package: ## Phase 3: render + publish all assets from persisted captures (no Docker/network; FORCE=1 to re-render)
	$(RUN) published --phase=package --theme=both $(if $(FORCE),--force)
