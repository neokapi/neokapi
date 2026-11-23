GUARDS += check-locale-display
GUARD_LANES.check-locale-display := lint ci
check-locale-display: ## Guard: a language is shown by name, not by its code alone
	@./scripts/check-locale-display.sh
