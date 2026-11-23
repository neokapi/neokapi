GUARDS += check-reference-provenance
GUARD_LANES.check-reference-provenance := lint pre-push ci
check-reference-provenance: ## Guard: the committed reference dataset comes only from this repo (no okapi-bridge)
	@./scripts/check-reference-provenance.sh
