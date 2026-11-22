GUARDS += check-comment-history
GUARD_LANES.check-comment-history := lint ci
check-comment-history: ## Guard: comments state what the code IS, not what it was changed from
	@./scripts/check-comment-history.sh
