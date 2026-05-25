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
# Rendering + publishing from existing captures need only node + ffmpeg. Capturing
# also needs a built kapi (`make -C .. build`), the kapi-claude plugin bundle, and
# Gemini/Anthropic credentials.

NPX := npx
RUN := $(NPX) tsx src/cli/run.ts

.PHONY: deps list studio videos render publish capture

deps: ## Install node deps + Playwright chromium
	npm install
	$(NPX) playwright install chromium

list: ## List available demos
	$(RUN) --list

studio: ## Open Remotion Studio (live preview)
	npm run studio

videos: ## Render + publish the docs videos (light + dark)
	$(RUN) published --only=render,publish --theme=both --force

render: ## Re-render the published demos (both themes, no publish)
	$(RUN) published --only=render --theme=both --force

publish: ## Convert existing renders → docs webm (no re-render)
	$(RUN) published --only=publish

capture: ## Full pipeline (capture → render → publish) for DEMO=<id>
	@test -n "$(DEMO)" || { echo "usage: make capture DEMO=<id>"; exit 1; }
	$(RUN) $(DEMO) --force
