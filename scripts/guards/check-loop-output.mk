GUARDS += check-loop-output
GUARD_LANES.check-loop-output := pre-push ci
check-loop-output: ## Guard: the sync commits only sound loop output
	@./scripts/check-loop-output.sh --self-test
