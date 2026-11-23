# Ungated: retired framing lands wherever prose is edited, and it degrades
# quietly: a stale phrase reads as intentional to the next reader.
GUARDS += check-vocabulary
GUARD_LANES.check-vocabulary := lint pre-push ci
check-vocabulary: ## Guard: no retired framing or retired vocabulary in prose, product strings and build metadata
	@./scripts/check-vocabulary.sh
