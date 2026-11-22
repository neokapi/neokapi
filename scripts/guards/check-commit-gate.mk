GUARDS += check-commit-gate
GUARD_LANES.check-commit-gate := pre-push ci
check-commit-gate: ## Guard: the commit gate hook passes its self-test
	@./.claude/hooks/commit-gate.sh --self-test
