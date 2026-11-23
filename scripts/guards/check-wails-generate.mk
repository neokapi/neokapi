GUARDS += check-wails-generate
GUARD_LANES.check-wails-generate := pre-push ci
check-wails-generate: ## Guard: a bindings generator warning about our own code fails
	@./scripts/wails-generate.sh --self-test
