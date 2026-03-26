# Agentic Testing — GitOps Makefile
#
# The agentic/ directory is the source of truth for workspace configuration.
# All operations read from workspaces/<name>/workspace.yaml.
#
# Usage:
#   make onboard  W=excalidraw          # Full onboard: users + workspace + content
#   make users    W=excalidraw          # Create Keycloak users only
#   make workspace W=excalidraw         # Create Bowrain workspace + project + members
#   make content  W=excalidraw          # Info: release walking is now server-side
#   make status   W=excalidraw          # Show workspace status
#   make deploy                         # Deploy persona-agent image via CI
#
# Environment (set in .env or export):
#   BOWRAIN_URL       — Server URL (default: http://localhost:8080)
#   JWT_SECRET        — JWT signing secret for local dev
#   KC_URL            — Keycloak URL (default: http://localhost:8180)
#   KC_ADMIN_PASS     — Keycloak admin password (default: admin)
#   KEY_VAULT         — Azure Key Vault name (optional, for prod token storage)

.PHONY: onboard users workspace content status deploy \
	validate list help

-include .env

BOWRAIN_URL  ?= http://localhost:8080
KC_URL       ?= http://localhost:8180
KC_ADMIN_PASS ?= admin
JWT_SECRET   ?= dev-secret-change-in-production
KEY_VAULT    ?=
W            ?=

SCRIPTS      := scripts
WS_DIR       := workspaces/$(W)
WS_YAML      := $(WS_DIR)/workspace.yaml

# ── Guards ───────────────────────────────────────────────────────────

define require_workspace
	@if [ -z "$(W)" ]; then echo "ERROR: Set W=<workspace-name> (e.g., make $@ W=excalidraw)"; exit 1; fi
	@if [ ! -f "$(WS_YAML)" ]; then echo "ERROR: $(WS_YAML) not found"; exit 1; fi
endef

# ── Primary targets ──────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

onboard: validate users workspace ## Full onboard: users + workspace (content via fleet repo)
	@echo ""
	@echo "=== Onboard complete for $(W) ==="

validate: ## Validate workspace config
	$(require_workspace)
	@echo "Validating $(WS_YAML)..."
	@grep -q '^name:' "$(WS_YAML)" || (echo "ERROR: missing 'name' field"; exit 1)
	@grep -q '^slug:' "$(WS_YAML)" || (echo "ERROR: missing 'slug' field"; exit 1)
	@grep -q '^source_language:' "$(WS_YAML)" || (echo "ERROR: missing 'source_language'"; exit 1)
	@grep -q '^target_languages:' "$(WS_YAML)" || (echo "ERROR: missing 'target_languages'"; exit 1)
	@grep -q '^agents:' "$(WS_YAML)" || (echo "ERROR: missing 'agents'"; exit 1)
	@ls "$(WS_DIR)/agents/"*/SOUL.md >/dev/null 2>&1 || (echo "ERROR: no agent SOUL.md files"; exit 1)
	@echo "  OK"

users: validate ## Create Keycloak users for all agents in the workspace
	$(require_workspace)
	KC_URL="$(KC_URL)" \
	KC_ADMIN_PASS="$(KC_ADMIN_PASS)" \
		$(SCRIPTS)/setup-keycloak-users.sh "$(W)"

workspace: validate ## Create Bowrain workspace + project + add agent members
	$(require_workspace)
	BOWRAIN_URL="$(BOWRAIN_URL)" \
	JWT_SECRET="$(JWT_SECRET)" \
		$(SCRIPTS)/setup-workspace-api.sh "$(WS_DIR)"

content: ## Walk upstream releases (server-side via agentic-server)
	@echo "Release walking is now server-side. The agentic-server reads"
	@echo "plan.yaml from neokapi/agentic-fleet and walks releases via"
	@echo "the walk_release MCP tool. Push plan.yaml + status.yaml to"
	@echo "the fleet repo to control release progression."
	@echo ""
	@echo "See: https://github.com/neokapi/agentic-fleet"

status: ## Show workspace status (blocks, coverage, agents)
	$(require_workspace)
	@SLUG=$$(grep '^slug:' $(WS_YAML) | sed 's/^slug: *//'); \
	echo "=== Workspace: $$SLUG ==="; \
	echo ""; \
	echo "Agents:"; \
	for d in $(WS_DIR)/agents/*/; do \
		name=$$(basename "$$d"); \
		role=$$(grep -A2 "name: $$name" $(WS_YAML) | grep 'role:' | sed 's/.*role: *"*//' | sed 's/"*$$//'); \
		echo "  $$name ($$role)"; \
	done; \
	echo ""; \
	echo "Target languages:"; \
	sed -n '/^target_languages:/,/^[^ ]/p' $(WS_YAML) | grep '^\s*-' | sed 's/.*- */  /'; \
	echo ""

list: ## List all configured workspaces
	@for ws in workspaces/*/workspace.yaml; do \
		dir=$$(dirname "$$ws"); \
		name=$$(basename "$$dir"); \
		slug=$$(grep '^slug:' "$$ws" | sed 's/^slug: *//'); \
		agents=$$(ls "$$dir/agents/" 2>/dev/null | grep -v _template | wc -l | tr -d ' '); \
		echo "  $$name ($$slug) — $$agents agents"; \
	done

deploy: ## Trigger persona-agent image build + deploy via GitHub Actions
	gh workflow run deploy-persona-agent.yml -f environment=dev

# ── Docker Compose (local dev) ───────────────────────────────────────

up: ## Start all services locally
	docker compose up -d

down: ## Stop all services
	docker compose down

logs: ## Follow all agent logs
	docker compose logs -f

up-%: ## Start a workspace (e.g., make up-excalidraw)
	@WS="$*"; \
	AGENTS=$$(ls workspaces/$$WS/agents/ 2>/dev/null | grep -v _template | sed "s/^/$$WS-/"); \
	docker compose up -d $$AGENTS

logs-%: ## Follow workspace logs (e.g., make logs-excalidraw)
	@WS="$*"; \
	AGENTS=$$(ls workspaces/$$WS/agents/ 2>/dev/null | grep -v _template | sed "s/^/$$WS-/"); \
	docker compose logs -f $$AGENTS
