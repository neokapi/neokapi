GUARDS += check-local-actions
GUARD_LANES.check-local-actions := lint ci
check-local-actions: ## Guard: a workflow's local `uses: ./…` resolves where that job checked the repo out
	@./scripts/check-local-actions.sh
