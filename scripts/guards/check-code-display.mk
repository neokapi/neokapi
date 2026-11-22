GUARDS += check-code-display
GUARD_LANES.check-code-display := lint pre-push ci
check-code-display: ## Guard: no surface shows an inline code's native data or identifier
	@./scripts/check-code-display.sh
